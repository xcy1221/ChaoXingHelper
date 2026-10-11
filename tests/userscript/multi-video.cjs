const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const source = fs.readFileSync('chaoxing-helper.user.js', 'utf8');
function card(count, num = 0, noJob = false) {
  const state = { URL: `https://mooc1.chaoxing.com/mooc-ans/knowledge/cards?knowledgeid=42&num=${num}`, readyState: 'complete' };
  const entries = Array.from({length: count}, (_, i) => {
    const metadata = {type: 'video', jobid: String(i + 1), job: !noJob, isPassed: false};
    let domPassed = false;
    const ownerDocument = {querySelectorAll: selector => selector === 'video' ? [video] : []};
    const video = {ownerDocument, paused: true, ended: false, seeking: false, currentTime: 0, playCount: 0,
      addEventListener() {}, play() { this.playCount++; this.paused = false; return Promise.resolve(); }, pause() { this.paused = true; }};
    const icon = {classList: {contains: () => false}, getAttribute: () => domPassed ? '任务点已完成' : '任务点未完成'};
    const wrapper = {classList: {contains: () => false}, querySelector: () => noJob ? null : icon};
    const frame = {getAttribute: name => name === 'jobid' ? metadata.jobid : '{}', closest: () => wrapper,
      contentDocument: {querySelector: () => video, querySelectorAll: selector => selector === 'video' ? [video] : []}};
    return {metadata, video, frame, passDOM() {domPassed = true;}};
  });
  state.defaultView = {mArg: {attachments: entries.map(e => e.metadata)}};
  state.querySelectorAll = selector => selector === 'iframe' || selector.includes('iframe.ans-insertvideo-online') ? entries.map(e => e.frame) : [];
  return {state, entries};
}
function harness(initial, nextCard, thirdCard, storage = new Map()) {
  let current = initial, interval, box;
  const controls = Object.fromEntries(['#cx-helper-start', '#cx-helper-stop', '#cx-helper-status'].map(k => [k, {}]));
  const calls = {chapter: 0, card: 0};
  const rows = [
    {id: 'cur42', classList: {contains: () => true}},
    {id: 'cur43', classList: {contains: () => false}, querySelector: selector => selector === '.icon_Completed' ? null : {
      textContent: '下一节视频', getAttribute: () => '下一节视频', click() {calls.chapter++;}
    }}
  ];
  const pages = [initial, nextCard, thirdCard].filter(Boolean);
  const tabs = nextCard ? pages.map((page, index) => ({tagName: 'LI', index, getAttribute: () => null,
    classList: {contains: () => current === page},
    click() {calls.card++; current = page;}
  })) : [];
  for (const tab of tabs) tab.parentElement = {children: tabs};
  const document = {
    URL: 'https://mooc1.chaoxing.com/mycourse/studentstudy', readyState: 'complete', body: {append(value) {box = value;}},
    createElement: () => ({style: {}, querySelector: key => controls[key]}),
    querySelector: selector => selector === '#iframe' ? {contentDocument: current.state} :
      selector === '.posCatalog_active' ? rows[0] : selector === '#cx-helper' ? box : controls[selector] || null,
    querySelectorAll: selector => selector === 'iframe' ? [{contentDocument: current.state}] :
      selector.includes('prev_ul') ? tabs : selector.includes('posCatalog_select') ? rows : [],
  };
  const window = {addEventListener() {}};
  window.self = window.top = window;
  vm.runInNewContext(source, {window, document, location: {pathname: '/mycourse/studentstudy'}, URL,
    sessionStorage: {getItem: key => storage.get(key) || null, setItem: (key, value) => storage.set(key, value)},
    setInterval(callback) {interval = callback;}, console});
  return {start() {controls['#cx-helper-start'].onclick();}, stop() {controls['#cx-helper-stop'].onclick();},
    tick() {interval();}, calls, storage, status: () => controls['#cx-helper-status'].textContent};
}
// 同页两个任务点：未确认前等待，确认后只启动下一个，最后才换章节。
{
  const c = card(2), h = harness(c);
  c.entries[1].video.paused = false;
  h.start();
  assert.equal(c.entries[0].video.playCount, 1);
  assert.equal(c.entries[1].video.paused, true);
  c.entries[0].video.ended = true;
  h.tick();
  assert.equal(c.entries[1].video.playCount, 0);
  assert.match(h.status(), /等待任务点确认/);
  c.entries[0].metadata.isPassed = true;
  h.tick();
  assert.equal(c.entries[0].video.paused, true);
  assert.equal(c.entries[1].video.playCount, 1);
  assert.equal(h.calls.chapter, 0);
  assert.match(h.status(), /第 2\/2/);
  c.entries[1].passDOM();
  h.tick();
  assert.equal(c.entries[1].video.paused, true);
  assert.equal(h.calls.chapter, 1);
}
// 同节两个页签：处理下一页签，全部完成后才换章节。
{
  const a = card(1), b = card(1, 1), h = harness(a, b);
  h.start();
  a.entries[0].metadata.isPassed = true;
  h.tick();
  assert.equal(h.calls.card, 1);
  assert.equal(h.calls.chapter, 0);
  assert.equal(a.entries[0].video.paused, true);
  h.tick();
  assert.equal(b.entries[0].video.playCount, 1);
  b.entries[0].metadata.isPassed = true;
  h.tick();
  assert.equal(h.calls.chapter, 1);
  assert.equal(b.entries[0].video.paused, true);
}
// 后续播放器延迟加载时不跳过；停止后不自动恢复。
{
  const c = card(2), h = harness(c);
  c.entries[0].metadata.isPassed = true;
  const secondDoc = c.entries[1].frame.contentDocument;
  c.entries[1].frame.contentDocument = null;
  h.start();
  assert.match(h.status(), /等待第 2\/2/);
  assert.equal(h.calls.chapter, 0);
  c.entries[1].frame.contentDocument = secondDoc;
  h.tick();
  assert.equal(c.entries[1].video.playCount, 1);
  h.stop(); h.tick();
  assert.equal(c.entries[1].video.paused, true);
  assert.equal(c.entries[1].video.playCount, 1);
}
// 无任务点的补充视频按 ended 顺序播放，不等待不存在的完成标记。
{
  const c = card(2, 0, true), h = harness(c);
  h.start(); c.entries[0].video.ended = true; h.tick();
  assert.equal(c.entries[1].video.playCount, 1);
  assert.equal(h.calls.chapter, 0);
}
// 截图对应的三页签场景：视频1 → 视频2 → 视频3 → 下一节。
{
  const a = card(1), b = card(1, 1), c = card(1, 2), h = harness(a, b, c);
  h.start();
  a.entries[0].metadata.isPassed = true; h.tick(); h.tick();
  assert.equal(b.entries[0].video.playCount, 1);
  assert.equal(h.calls.chapter, 0);
  b.entries[0].metadata.isPassed = true; h.tick(); h.tick();
  assert.equal(c.entries[0].video.playCount, 1);
  assert.equal(h.calls.card, 2);
  assert.equal(h.calls.chapter, 0);
  c.entries[0].metadata.isPassed = true; h.tick();
  assert.equal(h.calls.chapter, 1);
  assert.ok([a, b, c].every(page => page.entries[0].video.paused));
}
// 整页刷新后的连播续接，以及停止后不续接。
{
  const storage = new Map();
  const a = card(1), h = harness(a, null, null, storage);
  h.start(); assert.equal(storage.get('cx-helper-autoplay-v1'), 'true');
  const b = card(1, 1), resumed = harness(b, null, null, storage);
  assert.equal(b.entries[0].video.playCount, 1);
  resumed.stop(); assert.equal(storage.get('cx-helper-autoplay-v1'), 'false');
  const c = card(1, 2); harness(c, null, null, storage);
  assert.equal(c.entries[0].video.playCount, 0);
}
console.log('PASS: same-page queue, completion wait, 2/3 chapter tabs, single playback, delayed loading, stop, non-job videos, reload resume');
