'use strict';
const $ = (id) => document.getElementById(id);
const terminal = new Set(['succeeded', 'failed', 'uncertain']);
const stages = { queued: 0, uploading: 1, submitting: 1, detecting: 2, waiting: 2, succeeded: 3 };
const labels = { queued: '排队中', uploading: '上传中', submitting: '提交中', detecting: '检测中', waiting: '等待结果', succeeded: '已完成', failed: '失败', uncertain: '待核对' };
let selected = null, requestId = '', current = null, ready = false, pending = false;
let pollTimer;
const storedId = (() => { try { return new URLSearchParams(location.search).get('job') || sessionStorage.getItem('aigc-job'); } catch { return null; } })();
function remember(id) {
  try { sessionStorage.setItem('aigc-job', id); } catch { /* 页面内仍可查看 */ }
  const url = new URL(location.href); url.searchParams.set('job', id); history.replaceState(null, '', url);
}
function enable() { $('submit').disabled = !ready || !selected || pending || (current && !terminal.has(current.status)); }
async function api(path, options) {
  const response = await fetch(path, { ...options, cache: 'no-store' });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || '服务暂时不可用');
  return data;
}
function select(file) {
  if (!file) return;
  if (!file.name.toLowerCase().endsWith('.docx') || file.size > 10 * 1024 * 1024) {
    $('connection').textContent = '请选择不超过 10 MB 的 .docx 文档。'; $('connection').classList.add('error');
    selected = null; enable(); return;
  }
  selected = file; requestId = crypto.randomUUID();
  $('file-name').textContent = file.name; $('connection').classList.remove('error'); health();
}
function render(job) {
  current = job; remember(job.id);
  $('status').textContent = labels[job.status] || '处理中';
  $('message').textContent = job.message || ({ queued: '文档已加入队列，请稍候。', uploading: '正在上传文档。', submitting: '正在提交本次作业。', detecting: '正在进行 AIGC 检测。', waiting: '等待学习通返回本次结果。' }[job.status] || '正在处理。');
  $('summary').hidden = !job.summary; $('summary').textContent = job.summary || '';
  $('report').hidden = !job.reportAvailable;
  if (job.reportAvailable) $('report').href = 'api/jobs/' + encodeURIComponent(job.id) + '/report';
  document.querySelectorAll('[data-step]').forEach(el => el.classList.toggle('active', Number(el.dataset.step) <= (stages[job.status] ?? -1)));
  enable();
  if (job.status === 'uncertain') health();
}
async function poll(id) {
  clearTimeout(pollTimer);
  try {
    const job = await api('api/jobs/' + encodeURIComponent(id)); render(job);
    if (terminal.has(job.status)) { health(); return; }
  } catch (error) { $('message').textContent = error.message + '，稍后继续查询。'; }
  pollTimer = setTimeout(() => poll(id), 5000);
}
async function health() {
  try {
    const data = await api('api/health'); ready = data.ready && !data.paused;
    $('records-link').hidden = $('panel-link').hidden = !data.canViewRecords;
    $('connection').textContent = data.paused ? '检测队列暂停，管理员正在核对上一份提交。' : data.ready ? ({ sleeping: '服务待命，提交后自动启动检测浏览器', starting: '正在启动检测浏览器' }[data.browserState] || '检测服务已连接') : '检测连接尚未就绪，请等待管理员接入学习通。';
    $('connection').classList.toggle('error', !ready);
  } catch { ready = false; $('connection').textContent = '暂时无法连接服务，请稍后重试。'; $('connection').classList.add('error'); }
  enable();
}
$('file').addEventListener('change', e => select(e.target.files[0]));
for (const event of ['dragenter', 'dragover']) $('dropzone').addEventListener(event, e => { e.preventDefault(); $('dropzone').classList.add('drag'); });
for (const event of ['dragleave', 'drop']) $('dropzone').addEventListener(event, e => { e.preventDefault(); $('dropzone').classList.remove('drag'); });
$('dropzone').addEventListener('drop', e => select(e.dataTransfer.files[0]));
$('upload-form').addEventListener('submit', async e => {
  e.preventDefault(); if ($('submit').disabled) return;
  pending = true; enable(); $('connection').textContent = '正在提交文档…';
  const body = new FormData(); body.append('file', selected);
  try {
    const job = await api('api/jobs', { method: 'POST', headers: { 'Idempotency-Key': requestId }, body });
    render(job); poll(job.id);
  } catch (error) { $('connection').textContent = error.message; $('connection').classList.add('error'); }
  finally { pending = false; enable(); }
});
health(); setInterval(health, 15000);
if (storedId && /^[a-f0-9]{32}$/.test(storedId)) { current = { status: 'queued' }; enable(); poll(storedId); }
