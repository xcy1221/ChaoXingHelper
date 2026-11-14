<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import Appearance from './Appearance.vue';
import { api, labels, stages, terminal, storedJob, remember } from './api';
import type { Job, Health } from './api';
const records = location.pathname.endsWith('/records.html');
const internal = ref(false), appearance = ref<InstanceType<typeof Appearance>>();
const selected = ref<File | null>(null), current = ref<Job | null>(null), ready = ref(false), pending = ref(false), drag = ref(false);
const connection = ref('正在连接检测服务…'), connectionError = ref(false), message = ref('提交文档后，可在这里查看处理进度。');
const disabled = computed(() => !ready.value || !selected.value || pending.value || !!(current.value && !terminal.has(current.value.status)));
let requestId = '', alive = false, pollTimer: ReturnType<typeof setTimeout>, healthTimer: ReturnType<typeof setInterval>;
const abort = new AbortController();
const mode = ref<'jobs'|'access'>('jobs'), page = ref(1), pages = ref(1), rows = ref<any[]>([]), loading = ref(false), recordsStatus = ref('正在读取记录…');
const heads = computed(() => mode.value === 'jobs' ? ['提交时间','设备 / IP','文档','状态','结果','报告'] : ['访问时间','设备 / IP','方法','路径','HTTP 状态']);
let recordSequence = 0;
watch(internal, value => { if (value) document.documentElement.dataset.aigcInternal = 'true'; else delete document.documentElement.dataset.aigcInternal; });
async function health() {
  try {
    const data = await api<Health>('health', { signal: abort.signal }); if (!alive) return;
    ready.value = data.ready && !data.paused; internal.value = data.canViewRecords;
    connection.value = data.paused ? '检测队列暂停，管理员正在核对上一份提交。' : data.ready ? ({ sleeping: '服务待命，提交后自动启动检测浏览器', starting: '正在启动检测浏览器' }[data.browserState || ''] || '检测服务已连接') : '检测连接尚未就绪，请等待管理员接入学习通。';
    connectionError.value = !ready.value;
  } catch { if (alive) { ready.value = false; connection.value = '暂时无法连接服务，请稍后重试。'; connectionError.value = true; } }
}
function select(file?: File) {
  if (!file) return;
  if (!file.name.toLowerCase().endsWith('.docx') || file.size > 10 * 1024 * 1024) { connection.value = '请选择不超过 10 MB 的 .docx 文档。'; connectionError.value = true; selected.value = null; return; }
  selected.value = file; requestId = crypto.randomUUID(); connectionError.value = false; health();
}
function render(job: Job) {
  current.value = job; remember(job.id);
  message.value = job.message || ({ queued: '文档已加入队列，请稍候。', uploading: '正在上传文档。', submitting: '正在提交本次作业。', detecting: '正在进行 AIGC 检测。', waiting: '等待学习通返回本次结果。' }[job.status] || '正在处理。');
  if (job.status === 'uncertain') health();
}
async function poll(id: string) {
  clearTimeout(pollTimer);
  try { const job = await api<Job>('jobs/' + encodeURIComponent(id), { signal: abort.signal }); if (!alive) return; render(job); if (terminal.has(job.status)) { health(); return; } }
  catch (error: any) { if (!alive) return; message.value = error.message + '，稍后继续查询。'; }
  pollTimer = setTimeout(() => poll(id), 5000);
}
async function submit() {
  if (disabled.value) return; pending.value = true; connection.value = '正在提交文档…';
  const body = new FormData(); body.append('file', selected.value!);
  try { const job = await api<Job>('jobs', { method: 'POST', headers: { 'Idempotency-Key': requestId }, body, signal: abort.signal }); if (!alive) return; render(job); poll(job.id); }
  catch (error: any) { if (alive) { connection.value = error.message; connectionError.value = true; } }
  finally { pending.value = false; }
}
async function loadRecords() {
  const sequence = ++recordSequence; loading.value = true;
  try {
    const data = await api<{ records: any[]; total: number; size: number }>((mode.value === 'jobs' ? 'records' : 'access-records') + '?page=' + page.value, { signal: abort.signal });
    if (!alive || sequence !== recordSequence) return;
    rows.value = data.records; pages.value = Math.max(1, Math.ceil(data.total / data.size)); recordsStatus.value = '共 ' + data.total + ' 条记录';
  } catch (error: any) { if (alive && sequence === recordSequence) recordsStatus.value = error.message; }
  finally { if (sequence === recordSequence) loading.value = false; }
}
function changeMode(value: 'jobs'|'access') { mode.value = value; page.value = 1; loadRecords(); }
function changePage(delta: number) { page.value += delta; loadRecords(); }
onMounted(() => { alive = true; health(); if (records) loadRecords(); else { healthTimer = setInterval(health, 15000); const id = storedJob(); if (id && /^[a-f0-9]{32}$/.test(id)) { current.value = { id, status: 'queued' }; poll(id); } } });
onUnmounted(() => { alive = false; abort.abort(); clearTimeout(pollTimer); clearInterval(healthTimer); });
</script>
<template>
  <header id="aigc-server-header" class="server-panel-header" aria-label="Server Panel 通用顶部栏" :hidden="!internal"><a class="server-panel-brand" href="/server-readme/" target="_top"><span>HOMESERVER ·</span><strong>AIGC 文档检测</strong></a><a class="server-panel-return" href="/server-readme/" target="_top">←返回server panel</a><button id="aigc-appearance-trigger" class="appearance-trigger" type="button" aria-haspopup="dialog" aria-controls="aigc-appearance" aria-expanded="false" @click="appearance?.open()"><span aria-hidden="true">◐</span>外观与排版</button></header>
  <Appearance v-if="internal" ref="appearance" :records="records" />
  <header id="aigc-external-header" :hidden="internal"><a class="brand" href="./">学习通工具<span>ChaoXingHelper</span></a><a v-if="records" href="./">返回检测 ↗</a></header>
  <main v-if="!records">
    <section class="intro"><span class="eyebrow">文档检测</span><h1>上传一份文档，<br>查看 AIGC 检测结果。</h1><p>通过配置好的学习通课程完成提交与检测，结果保存在本次任务中。</p></section>
    <nav class="internal-links"><a id="records-link" href="records.html" :hidden="!internal">检测记录 ↗</a></nav>
    <section class="workspace">
      <form id="upload-form" class="card" @submit.prevent="submit">
        <div class="heading"><h2>选择文档</h2><span class="badge">Word · .docx</span></div>
        <label class="dropzone" :class="{ drag }" id="dropzone" for="file" @dragenter.prevent="drag=true" @dragover.prevent="drag=true" @dragleave.prevent="drag=false" @drop.prevent="event => { drag=false; select(event.dataTransfer?.files[0]); }">
          <svg viewBox="0 0 48 48" aria-hidden="true"><path d="M14 5h14l9 9v29H14zM28 5v10h9M20 25h11M20 31h11M20 37h7" /></svg>
          <strong id="file-name">{{ selected?.name || '点击选择，或拖入 Word 文档' }}</strong><span>支持 .docx，最大 10 MB</span><input id="file" type="file" accept=".docx" aria-label="选择 Word 文档" @change="event => select((event.target as HTMLInputElement).files?.[0])">
        </label>
        <p class="connection" :class="{ error: connectionError }" id="connection" role="status">{{ connection }}</p><button type="submit" id="submit" :disabled="disabled">提交检测 <span aria-hidden="true">→</span></button>
        <p class="footnote">关闭页面后，可在同一浏览器会话中继续查看本次任务。</p>
      </form>
      <section class="card result" aria-labelledby="result-title"><div class="heading"><h2 id="result-title">检测进度</h2><span id="status" class="badge">{{ current ? labels[current.status] || '处理中' : '尚未提交' }}</span></div>
        <ol class="steps"><li v-for="(step,index) in ['排队','提交作业','检测','结果']" :key="step" :data-step="index" :class="{ active: index <= (stages[current?.status || ''] ?? -1) }">{{ step }}</li></ol>
        <div class="result-body" aria-live="polite"><p id="message">{{ message }}</p><pre id="summary" :hidden="!current?.summary">{{ current?.summary || '' }}</pre><a id="report" class="report" :hidden="!current?.reportAvailable" :href="current?.reportAvailable ? 'api/jobs/' + encodeURIComponent(current.id) + '/report' : undefined">下载检测报告 ↓</a></div>
      </section>
    </section><p class="note">文档与报告由服务器保存。检测结果以学习通提供的报告为准。</p>
  </main>
  <main v-else><section class="intro"><span class="eyebrow">内部管理</span><h1>使用与访问记录</h1><p>外部按设备证书名称识别，内网记录来源 IP。访问页保留最近 10000 条页面访问、提交、报告下载及错误记录。</p></section>
    <nav class="records-pages"><button id="show-jobs" :disabled="mode === 'jobs'" @click="changeMode('jobs')">检测记录</button><button id="show-access" :disabled="mode === 'access'" @click="changeMode('access')">访问记录</button></nav>
    <p id="records-status" role="status">{{ recordsStatus }}</p><div class="records-scroll"><table class="records-table"><thead id="records-head"><tr><th v-for="head in heads" :key="head">{{ head }}</th></tr></thead><tbody id="records-body"><tr v-for="(item,index) in rows" :key="item.id || index"><td>{{ new Date(item.createdAt * 1000).toLocaleString('zh-CN') }}</td><td>{{ item.user + '\n' + (item.clientIP || '来源未记录') }}</td><template v-if="mode === 'access'"><td>{{ item.method }}</td><td>{{ item.path }}</td><td>{{ item.status }}</td></template><template v-else><td>{{ item.fileName }}</td><td>{{ item.status === 'succeeded' ? '完成' : labels[item.status] || item.status }}</td><td>{{ item.summary || item.message || '—' }}</td><td><a v-if="item.reportAvailable" :href="'api/jobs/' + encodeURIComponent(item.id) + '/report'">下载</a><template v-else>—</template></td></template></tr></tbody></table></div>
    <nav class="records-pages"><button id="previous" :disabled="loading || page <= 1" @click="changePage(-1)">上一页</button><span id="page">{{ page }} / {{ pages }}</span><button id="next" :disabled="loading || page >= pages" @click="changePage(1)">下一页</button></nav>
  </main>
  <footer>学习通工具 <span>{{ records ? '检测记录仅内部可见' : '视频辅助 · 作业粘贴 · 文档检测' }}</span></footer>
</template>
