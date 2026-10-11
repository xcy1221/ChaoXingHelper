const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const source = fs.readFileSync('chaoxing-helper.user.js', 'utf8');
function frame(path, settings = {}) {
  const listeners = new Map();
  const window = { top: {}, addEventListener(type, callback) { listeners.set(type, callback); } };
  window.self = window;
  const document = { hidden: true, visibilityState: 'hidden', hasFocus: () => false };
  const context = vm.createContext({ window, document, location: { pathname: path }, localStorage: { getItem: () => JSON.stringify(settings) }, console, WeakSet, Object, Array, JSON });
  vm.runInContext(source, context);
  return { window, document, listeners, context };
}
const video = frame('/ananas/modules/video/index.html');
assert.equal(video.document.hidden, false);
assert.equal(video.document.visibilityState, 'visible');
assert.equal(video.document.hasFocus(), true);
let stopped = 0;
video.listeners.get('blur')({ type: 'blur', target: video.window, stopImmediatePropagation() { stopped++; } });
assert.equal(stopped, 1);
video.listeners.get('blur')({ type: 'blur', target: {}, stopImmediatePropagation() { stopped++; } });
assert.equal(stopped, 1, '输入框失焦不应被拦截');
video.listeners.get('visibilitychange')({ type: 'visibilitychange', target: video.document, stopImmediatePropagation() { stopped++; } });
assert.equal(stopped, 2);
vm.runInContext(source, video.context);
assert.equal(video.listeners.size, 6);
const disabled = frame('/ananas/modules/video/index.html', { focus: false });
assert.equal(disabled.document.hidden, false, '旧配置不再控制新版');
assert.equal(disabled.listeners.size, 6);
const cards = frame('/mooc-ans/knowledge/cards', { seek: true, rate: 2 });
const data = { attachments: [
  { type: 'video', property: { switchwindow: 'true', fastforward: 'true', doublespeed: 0 }, isPassed: false, playTime: 0, otherInfo: 'unchanged' },
  { type: 'work', property: { switchwindow: 'true', doublespeed: 0 } }
] };
cards.window.mArg = data;
assert.equal(data.attachments[0].property.switchwindow, 'false');
assert.equal(data.attachments[0].property.fastforward, 'true');
assert.equal(data.attachments[0].property.doublespeed, 0);
assert.equal(data.attachments[0].isPassed, false);
assert.equal(data.attachments[0].playTime, 0);
assert.equal(data.attachments[0].otherInfo, 'unchanged');
assert.equal(data.attachments[1].property.switchwindow, 'true');
console.log('PASS: focus, input blur, legacy config ignored, duplicate load, original playback limits preserved');
