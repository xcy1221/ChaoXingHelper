const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const source = fs.readFileSync('chaoxing-helper.user.js', 'utf8');
const workPath = '/mooc-ans/mooc2/work/dowork';

function documentMock() {
  const listeners = new Map(), observations = [], iframeList = [];
  const view = {
    addEventListener(type, callback) {
      if (!listeners.has(type)) listeners.set(type, []);
      listeners.get(type).push(callback);
    },
    Event: class { constructor(type, options) { this.type = type; Object.assign(this, options); } },
    MutationObserver: class {
      constructor(callback) { observations.push(callback); }
      observe() {}
    }
  };
  view.InputEvent = view.Event;
  view.parent = view;
  const doc = {
    defaultView: view, hidden: true, visibilityState: 'hidden', designMode: 'off',
    querySelectorAll: () => iframeList,
    createTextNode: text => ({ nodeType: 3, textContent: text }),
    createElement: tag => ({ nodeType: 1, tagName: tag.toUpperCase() }),
    createDocumentFragment() {
      return { children: [], appendChild(node) { this.children.push(node); this.lastChild = node; } };
    },
    createRange() {
      const range = {
        startContainer: {}, endContainer: {},
        selectNodeContents(target) { this.selected = target; },
        deleteContents() { this.deleted = true; },
        insertNode(fragment) { this.inserted = fragment.children; },
        setStartAfter(node) { this.after = node; },
        collapse(value) { this.collapsed = value; }
      };
      return range;
    }
  };
  let selected = null;
  doc.getSelection = () => ({
    rangeCount: selected ? 1 : 0, getRangeAt: () => selected,
    removeAllRanges() { selected = null; }, addRange(range) { selected = range; }
  });
  class Field {
    constructor(tag = 'TEXTAREA') {
      this.nodeType = 1; this.tagName = tag; this.type = 'text'; this.ownerDocument = doc;
      this._value = ''; this.selectionStart = this.selectionEnd = 0; this.maxLength = -1; this.events = [];
    }
    get value() { return this._value; }
    set value(value) { this._value = value; }
    closest(selector) { return selector === 'textarea, input' ? this : null; }
    setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end; }
    dispatchEvent(event) { this.events.push(event); }
  }
  view.HTMLTextAreaElement = Field;
  view.HTMLInputElement = Field;
  return {
    doc, view, Field, listeners, iframeList,
    scan() { observations.forEach(callback => callback()); },
    rich() {
      const host = {
        nodeType: 1, tagName: 'DIV', ownerDocument: doc, isContentEditable: true, events: [],
        closest(selector) { return selector === '[contenteditable]' ? this : null; },
        contains(node) { return node === this || node?.parentElement === this; },
        dispatchEvent(event) { this.events.push(event); }
      };
      return host;
    },
    selection: () => selected
  };
}
function load(path = workPath) {
  const env = documentMock();
  const location = { pathname: path };
  const context = vm.createContext({ window: env.view, document: env.doc, location, console });
  vm.runInContext(source, context);
  return { ...env, location, reload() { vm.runInContext(source, context); } };
}
function paste(env, target, text, types = ['text/plain']) {
  const event = {
    target, clipboardData: { types, getData: () => text },
    preventDefault() { this.prevented = true; },
    stopImmediatePropagation() { this.stopped = true; }
  };
  (env.listeners.get('paste') || []).forEach(callback => callback(event));
  return event;
}

const env = load();
assert.equal(env.doc.hidden, true, '作业页不能运行视频失焦覆盖');
assert.equal(env.view.__cxVideoHelper, undefined);
const field = new env.Field();
field.value = '甲乙丙'; field.selectionStart = 1; field.selectionEnd = 2;
let interceptedSetter = 0;
Object.defineProperty(field, 'value', {
  get() { return this._value; }, set() { interceptedSetter++; }
});
const event = paste(env, field, '文\n字');
assert.equal(field.value, '甲文\n字丙');
assert.equal(field.selectionStart, 4);
assert.equal(interceptedSetter, 0, '通过原型 setter 通知受控输入框');
assert.equal(event.prevented, true); assert.equal(event.stopped, true);
assert.deepEqual(field.events.map(event => event.type), ['input', 'change']);
assert.equal(field.events[0].inputType, 'insertFromPaste');

const bounded = new env.Field();
bounded.value = 'abc'; bounded.selectionStart = 1; bounded.selectionEnd = 2; bounded.maxLength = 4;
paste(env, bounded, 'XYZ');
assert.equal(bounded.value, 'aXYc');
for (const setting of ['readOnly', 'disabled']) {
  const blocked = new env.Field(); blocked[setting] = true;
  assert.equal(paste(env, blocked, '不能写').prevented, undefined);
  assert.equal(blocked.value, '');
}
const password = new env.Field('INPUT'); password.type = 'password';
assert.equal(paste(env, password, '不能写').prevented, undefined);
assert.equal(paste(env, new env.Field(), 'image', ['image/png']).prevented, undefined);
assert.equal(paste(env, new env.Field(), '').prevented, undefined);

const rich = env.rich();
let syncCount = 0;
env.view.UE = { instants: { answer: { body: rich, fireEvent(type) { if (type === 'contentchange') syncCount++; } } } };
paste(env, rich, '<script>alert(1)</script>\r\n第二行');
const range = env.selection();
assert.equal(range.selected, rich);
assert.equal(range.deleted, true);
assert.equal(range.inserted[0].nodeType, 3, '剪贴板 HTML 必须作为文本');
assert.equal(range.inserted[0].textContent, '<script>alert(1)</script>');
assert.equal(range.inserted[1].tagName, 'BR');
assert.equal(range.inserted[2].textContent, '第二行');
assert.equal(syncCount, 1);
const readonlyRich = env.rich(); readonlyRich.isContentEditable = false;
assert.equal(paste(env, readonlyRich, '不能写').prevented, undefined);

const child = documentMock(), frameListeners = new Map();
const frame = { contentDocument: null, addEventListener(type, callback) { frameListeners.set(type, callback); } };
env.iframeList.push(frame); env.scan();
frame.contentDocument = child.doc; frameListeners.get('load')();
const childField = new child.Field();
paste(child, childField, 'iframe 答案');
assert.equal(childField.value, 'iframe 答案');
env.scan(); env.reload();
assert.equal(env.listeners.get('paste').length, 1);
assert.equal(child.listeners.get('paste').length, 1);
env.iframeList.push({ get contentDocument() { throw Error('cross origin'); }, addEventListener() {} });
assert.doesNotThrow(() => env.scan());

const shortcut = { target: field, ctrlKey: true, key: 'v', stopImmediatePropagation() { this.stopped = true; } };
env.listeners.get('keydown')[0](shortcut);
assert.equal(shortcut.stopped, true);
assert.equal(shortcut.prevented, undefined, '不取消浏览器默认快捷键');
env.location.pathname = '/mooc-ans/mooc2/exam/test';
assert.equal(paste(env, field, '考试不生效').prevented, undefined);
const outside = load('/mooc-ans/mooc2/work/preview');
assert.equal(outside.listeners.size, 0);
assert.equal(outside.doc.hidden, true);
console.log('PASS: homework route isolation, selection replacement, input events, plaintext/newlines, readonly, dynamic iframe, duplicate load');
