// ==UserScript==
// @name         学习通助手
// @namespace    local.chaoxing.helper
// @version      0.4.1
// @description  视频失焦保护与顺序连播；作业编辑器允许粘贴文本。
// @homepageURL  https://github.com/xcy1221/ChaoXingHelper
// @match        https://*.chaoxing.com/mycourse/studentstudy*
// @match        https://*.chaoxing.com/mooc-ans/mycourse/studentstudy*
// @match        https://*.chaoxing.com/mooc-ans/knowledge/cards*
// @match        https://*.chaoxing.com/ananas/modules/video/*
// @match        https://mooc1-api.chaoxing.com/mooc-ans/mooc2/work/dowork*
// @match        https://mooc1.chaoxing.com/mooc-ans/mooc2/work/dowork*
// @run-at       document-start
// @grant        none
// @sandbox      raw
// ==/UserScript==

(() => {
'use strict';

function installVideoFeature() {
  'use strict';
  if (window.__cxVideoHelper) return;
  window.__cxVideoHelper = true;
  let running = false;
  let status = '待开始';
  const hooked = new WeakSet();
  const override = (object, name, value) => {
    try {
      Object.defineProperty(object, name, { configurable: true, get: () => value });
    } catch { status = `无法覆盖 ${name}`; }
  };

  {
    for (const name of ['hidden', 'webkitHidden', 'msHidden']) override(document, name, false);
    for (const name of ['visibilityState', 'webkitVisibilityState']) override(document, name, 'visible');
    override(document, 'hasFocus', () => true);
    const block = event => {
      const target = event.target;
      const visibility = /visibilitychange$/.test(event.type);
      const windowBlur = event.type === 'blur' && (target === window || target === document);
      const boundary = /mouse(out|leave)$/.test(event.type) &&
        (target === window || target === document || target === document.body ||
          target === document.documentElement || target?.matches?.('video, .video-js'));
      if (visibility || windowBlur || (boundary && !event.relatedTarget)) event.stopImmediatePropagation();
    };
    for (const type of ['blur', 'visibilitychange', 'webkitvisibilitychange', 'msvisibilitychange', 'mouseout', 'mouseleave']) {
      window.addEventListener(type, block, true);
    }
  }

  // cards 页在初始化播放器前赋值 mArg；只调整前端播放控制配置。
  if (location.pathname.includes('/knowledge/cards')) {
    let argument;
    const adjust = value => {
      if (!value || !Array.isArray(value.attachments)) return value;
      for (const item of value.attachments) {
        if (item.type !== 'video' || !item.property) continue;
        item.property.switchwindow = 'false';
      }
      return value;
    };
    try {
      argument = adjust(window.mArg);
      Object.defineProperty(window, 'mArg', {
        configurable: true,
        get: () => argument,
        set: value => { argument = adjust(value); }
      });
    } catch { status = '播放器配置挂接失败'; }
  }

  // 播放器也读取 frameElement 的 data/属性，在它的初始化脚本之前同步。
  if (location.pathname.includes('/modules/video/')) {
    try {
      const frame = window.frameElement;
      if (frame) {
        const data = JSON.parse(frame.getAttribute('data') || '{}');
        data.switchwindow = 'false';
        frame.setAttribute('switchwindow', 'false');
        frame.setAttribute('data', JSON.stringify(data));
      }
    } catch { status = '播放器属性挂接失败'; }
  }

  if (window.top !== window.self) return;
  const runKey = 'cx-helper-autoplay-v1';
  try { running = sessionStorage.getItem(runKey) === 'true'; } catch { /* 存储不可用时只保留本页状态。 */ }
  function saveRunning() {
    try { sessionStorage.setItem(runKey, String(running)); } catch { /* 不影响当前页连播。 */ }
  }

  function documents(root = document, list = [], depth = 0) {
    list.push(root);
    if (depth >= 4) return list;
    for (const frame of root.querySelectorAll('iframe')) {
      try { if (frame.contentDocument) documents(frame.contentDocument, list, depth + 1); }
      catch { /* 跨域 frame 由其自身脚本处理。 */ }
    }
    return list;
  }

  function videoTasks() {
    const cardFrame = document.querySelector('#iframe');
    let cards;
    try { cards = cardFrame?.contentDocument; } catch { return []; }
    if (!cards) return [];
    const activeId = document.querySelector('.posCatalog_active')?.id.replace(/^cur/, '');
    try {
      if (activeId && new URL(cards.URL).searchParams.get('knowledgeid') !== activeId) return [];
    } catch { return []; }
    return [...cards.querySelectorAll('iframe.ans-insertvideo-online')].map(frame => {
      let video = null;
      try { video = frame.contentDocument?.querySelector('video'); } catch { /* 等待加载 */ }
      const task = frame.closest('.ans-attach-ct') || frame.parentElement;
      const icon = task?.querySelector('.ans-job-icon');
      let attachment;
      try {
        const data = JSON.parse(frame.getAttribute('data') || '{}');
        const jobid = frame.getAttribute('jobid') || data.jobid || data._jobid;
        attachment = cards.defaultView?.mArg?.attachments?.find(item =>
          item.type === 'video' && jobid && String(item.jobid || item.property?.jobid) === String(jobid));
      } catch { /* 配置尚未加载时使用任务点 DOM。 */ }
      const hasJob = !!icon || attachment?.job === true;
      const passed = !!task?.classList.contains('ans-job-finished') ||
        !!icon?.classList.contains('ans-job-finished') ||
        /任务点已完成/.test(icon?.getAttribute('aria-label') || '') || attachment?.isPassed === true ||
        (!hasJob && !!video?.ended);
      return { frame, video, passed };
    });
  }

  function nextCard() {
    const tabs = [...document.querySelectorAll('#prev_tab .prev_ul > li, .prev_list > ul > li, .prev_ul > li')];
    if (!tabs.length) return false;
    const active = tabs.find(tab => tab.classList.contains('active') || tab.getAttribute('aria-selected') === 'true');
    if (!active) { running = false; status = '无法识别当前页签，已停止'; return true; }
    const siblings = [...active.parentElement.children].filter(tab => tab.tagName === 'LI');
    const next = siblings[siblings.indexOf(active) + 1];
    if (!next) return false;
    status = '切换本节下一页签';
    next.click();
    return true;
  }

  function nextChapter() {
    const rows = [...document.querySelectorAll('.posCatalog_select[id^="cur"]')];
    const active = rows.findIndex(row => row.classList.contains('posCatalog_active'));
    if (active < 0) { status = '找不到当前章节，已停止'; running = false; return; }
    const next = rows.slice(active + 1).find(row => {
      const name = row.querySelector('.posCatalog_name');
      return /视频/.test(name?.getAttribute('title') || name?.textContent || '') &&
        !row.querySelector('.icon_Completed');
    });
    if (!next) { status = '后续视频已处理；测验和资料请自行完成'; running = false; return; }
    status = '切换下一视频章节';
    next.querySelector('.posCatalog_name')?.click();
  }

  let awaitingChapter = false;
  let lastChapter = '';
  let lastCards = null;
  let waitSince = 0;
  let waitTarget = null;
  let switchSince = 0;
  function pauseExcept(tasks, keep = null) {
    for (const { video } of tasks) if (video && video !== keep && !video.paused) video.pause();
  }
  function advance(tasks) {
    pauseExcept(tasks);
    awaitingChapter = true;
    switchSince = Date.now();
    if (!nextCard()) nextChapter();
  }
  function tick() {
    const chapter = document.querySelector('.posCatalog_active')?.id || '';
    let cards;
    try { cards = document.querySelector('#iframe')?.contentDocument; } catch { /* 等待加载 */ }
    if (chapter !== lastChapter || (cards && cards !== lastCards)) {
      lastChapter = chapter;
      lastCards = cards;
      awaitingChapter = false;
      waitSince = 0;
      waitTarget = null;
    }
    const tasks = videoTasks();
    for (const { video } of tasks) {
      if (!video || hooked.has(video)) continue;
      hooked.add(video);
      video.addEventListener('error', () => { running = false; status = '视频加载失败，已停止'; });
    }
    if (running && awaitingChapter && Date.now() - switchSince > 15000) {
      running = false;
      status = '页面切换超时，已停止';
    }
    if (running && !awaitingChapter) {
      if (!tasks.length) {
        const loaded = cards && /interactive|complete/.test(cards.readyState);
        const attachments = cards?.defaultView?.mArg?.attachments;
        const matchesChapter = cards && (!chapter || new URL(cards.URL).searchParams.get('knowledgeid') === chapter.replace(/^cur/, ''));
        if (loaded && matchesChapter && Array.isArray(attachments) && !attachments.some(item => item.type === 'video')) {
          advance(tasks);
        } else status = '等待本节视频加载';
      } else {
        const pending = tasks.find(task => !task.passed);
        pauseExcept(tasks, pending?.video);
        if (!pending) {
          advance(tasks);
        } else if (!pending.video) {
          status = `等待第 ${tasks.indexOf(pending) + 1}/${tasks.length} 个视频加载`;
        } else if (pending.video.ended) {
          if (waitTarget !== pending.frame) { waitTarget = pending.frame; waitSince = Date.now(); }
          status = `第 ${tasks.indexOf(pending) + 1}/${tasks.length} 个视频已结束，等待任务点确认`;
          if (Date.now() - waitSince > 90000) {
            running = false;
            status = '平台尚未确认完成，已停止；请检查任务点';
          }
        } else {
          waitSince = 0;
          waitTarget = null;
          const video = pending.video;
          if (video.paused && !video.seeking) {
            const panels = [...video.ownerDocument.querySelectorAll('.ans-videoquiz, .vjs-quiz, #videoquiz, #validate, .popDiv')];
            if (panels.some(el => el.getClientRects().length && el.textContent.trim())) {
              running = false;
              status = '播放器要求人工操作，已停止';
            } else {
              video.play().catch(() => { running = false; status = '请先手动点击一次播放，再开始连播'; });
            }
          }
          if (running) status = `本页第 ${tasks.indexOf(pending) + 1}/${tasks.length} 个视频 · 已播放 ${Math.floor(video.currentTime)} 秒`;
        }
      }
    }
    const text = document.querySelector('#cx-helper-status');
    if (text) text.textContent = status;
    saveRunning();
  }

  function mount() {
    if (!document.body || document.querySelector('#cx-helper')) return;
    const box = document.createElement('div');
    box.id = 'cx-helper';
    box.style.cssText = 'position:fixed;bottom:16px;left:16px;z-index:2147483647;background:#fff;color:#222;border:1px solid #aaa;border-radius:8px;padding:12px;box-shadow:0 2px 12px #0002;font:13px/1.7 sans-serif;width:270px';
    box.innerHTML = '<b>学习通视频助手 · 0.3</b><div>失焦保护已启用</div><div id="cx-helper-status"></div>' +
      '<button id="cx-helper-start">开始连播</button> <button id="cx-helper-stop">停止</button>' +
      '<div>顺序播放视频，进度以平台任务点为准。</div>';
    document.body.append(box);
    box.querySelector('#cx-helper-start').onclick = () => { running = true; saveRunning(); awaitingChapter = false; waitSince = 0; waitTarget = null; status = '启动连播'; tick(); };
    box.querySelector('#cx-helper-stop').onclick = () => {
      running = false;
      status = '已停止';
      for (const doc of documents()) for (const video of doc.querySelectorAll('video')) video.pause();
      tick();
    };
    tick();
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount, { once: true });
  else mount();
  setInterval(tick, 1500);
}

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

function isHomeworkPage(path) {
  return /^\/mooc-ans\/mooc2\/work\/dowork\/?$/i.test(path);
}

const currentPath = location.pathname;
const isVideoPage = /^\/(?:mooc-ans\/)?mycourse\/studentstudy(?:$|[/.])/.test(currentPath) ||
  /^\/(?:mooc-ans\/)?knowledge\/cards(?:$|[/.])/.test(currentPath) ||
  /^\/ananas\/modules\/video\//.test(currentPath);
if (isVideoPage) installVideoFeature();
else if (isHomeworkPage(currentPath)) installHomeworkPaste();
})();
