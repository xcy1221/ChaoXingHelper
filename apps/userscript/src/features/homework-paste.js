function installHomeworkPaste() {
  if (window.__cxHomeworkPasteHelper) return;
  window.__cxHomeworkPasteHelper = true;
  const documents = new WeakSet();
  const frames = new WeakSet();

  function editable(node) {
    if (node?.nodeType !== 1) node = node?.parentElement;
    if (!node) return null;
    const field = node.closest('textarea, input');
    if (field) {
      if (field.disabled || field.readOnly) return null;
      return field.tagName === 'TEXTAREA' || /^(text|search|url|tel|email)$/.test(field.type) ? field : null;
    }
    const host = node.closest('[contenteditable]');
    if (host) return host.isContentEditable ? host : null;
    return node.ownerDocument.designMode?.toLowerCase() === 'on' ? node.ownerDocument.body : null;
  }

  function notify(target, text) {
    const doc = target.ownerDocument;
    const view = doc.defaultView;
    let event;
    try { event = new view.InputEvent('input', { bubbles: true, inputType: 'insertFromPaste', data: text }); }
    catch { event = new view.Event('input', { bubbles: true }); }
    target.dispatchEvent(event);
    target.dispatchEvent(new view.Event('change', { bubbles: true }));
    // UEditor 的答案同步依赖 contentchange；不创建或重置实例。
    let owner = view;
    for (let depth = 0; depth < 3; depth++) {
      try {
        for (const editor of Object.values(owner.UE?.instants || {})) {
          if (editor.document === doc || editor.body === target || editor.body?.contains(target)) {
            editor.fireEvent?.('contentchange');
          }
        }
        if (owner.parent === owner) break;
        owner = owner.parent;
      } catch { break; }
    }
  }

  function insert(target, text) {
    const doc = target.ownerDocument;
    if (/^(INPUT|TEXTAREA)$/.test(target.tagName)) {
      const start = target.selectionStart ?? target.value.length;
      const end = target.selectionEnd ?? start;
      if (target.maxLength >= 0) text = text.slice(0, Math.max(0, target.maxLength - target.value.length + end - start));
      const value = target.value.slice(0, start) + text + target.value.slice(end);
      const prototype = target.tagName === 'TEXTAREA' ? doc.defaultView.HTMLTextAreaElement.prototype : doc.defaultView.HTMLInputElement.prototype;
      const setter = Object.getOwnPropertyDescriptor(prototype, 'value')?.set;
      if (setter) setter.call(target, value);
      else target.value = value;
      try { target.setSelectionRange(start + text.length, start + text.length); } catch { /* email 没有文本选区。 */ }
    } else {
      const selection = doc.getSelection();
      if (!selection) return false;
      let range = selection.rangeCount ? selection.getRangeAt(0) : null;
      if (!range || !target.contains(range.startContainer) || !target.contains(range.endContainer)) {
        range = doc.createRange();
        range.selectNodeContents(target);
        range.collapse(false);
      }
      const fragment = doc.createDocumentFragment();
      text.replace(/\r\n?/g, '\n').split('\n').forEach((line, index) => {
        if (index) fragment.appendChild(doc.createElement('br'));
        fragment.appendChild(doc.createTextNode(line));
      });
      const last = fragment.lastChild;
      range.deleteContents();
      range.insertNode(fragment);
      range.setStartAfter(last);
      range.collapse(true);
      selection.removeAllRanges();
      selection.addRange(range);
    }
    notify(target, text);
    return true;
  }

  function hook(doc) {
    if (!doc || documents.has(doc)) return;
    documents.add(doc);
    const view = doc.defaultView;
    if (view.__cxHomeworkPasteDocument === doc) return;
    view.__cxHomeworkPasteDocument = doc;
    view.addEventListener('paste', event => {
      if (!isHomeworkPage(location.pathname)) return;
      const target = editable(event.target);
      const clipboard = event.clipboardData;
      if (!target || !clipboard || !Array.from(clipboard.types || []).includes('text/plain')) return;
      const text = clipboard.getData('text/plain');
      if (!text) return;
      if (insert(target, text)) {
        event.preventDefault();
        event.stopImmediatePropagation();
      }
    }, true);
    view.addEventListener('keydown', event => {
      if (isHomeworkPage(location.pathname) && editable(event.target) &&
          (event.ctrlKey || event.metaKey) && !event.altKey && event.key?.toLowerCase() === 'v') {
        event.stopImmediatePropagation(); // 保留浏览器默认粘贴。
      }
    }, true);
    const scan = () => {
      for (const frame of doc.querySelectorAll('iframe')) {
        const connect = () => { try { hook(frame.contentDocument); } catch { /* 跨域 frame 无法访问。 */ } };
        if (!frames.has(frame)) {
          frames.add(frame);
          frame.addEventListener('load', connect);
        }
        connect();
      }
    };
    new view.MutationObserver(scan).observe(doc, { childList: true, subtree: true });
    scan();
  }
  hook(document);
}
