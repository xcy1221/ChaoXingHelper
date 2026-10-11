'use strict';
const $ = id => document.getElementById(id);
const labels = {queued:'排队中',uploading:'上传中',submitting:'提交中',detecting:'检测中',waiting:'等待结果',succeeded:'完成',failed:'失败',uncertain:'待核对'};
let page = 1, mode = 'jobs';
function cell(row, text) {
 const td = document.createElement('td'); td.textContent = text; row.append(td); return td;
}
async function load() {
 $('previous').disabled = $('next').disabled = true;
 try {
  const response = await fetch('api/' + (mode === 'jobs' ? 'records' : 'access-records') + '?page=' + page, {cache:'no-store'});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || '记录暂时不可用');
  $('records-body').replaceChildren();
  $('show-jobs').disabled = mode === 'jobs'; $('show-access').disabled = mode === 'access';
  const head = document.createElement('tr');
  for (const text of (mode === 'jobs' ? ['提交时间','设备 / IP','文档','状态','结果','报告'] : ['访问时间','设备 / IP','方法','路径','HTTP 状态'])) { const th = document.createElement('th'); th.textContent = text; head.append(th); }
  $('records-head').replaceChildren(head);
  for (const item of data.records) {
   const row = document.createElement('tr');
   cell(row, new Date(item.createdAt * 1000).toLocaleString('zh-CN'));
   cell(row, item.user + '\n' + (item.clientIP || '来源未记录'));
   if (mode === 'access') {
    cell(row, item.method); cell(row, item.path); cell(row, String(item.status));
    $('records-body').append(row); continue;
   }
   cell(row, item.fileName);
   cell(row, labels[item.status] || item.status);
   cell(row, item.summary || item.message || '—');
   const report = cell(row, '—');
   if (item.reportAvailable) {
    const link = document.createElement('a'); link.textContent = '下载';
    link.href = 'api/jobs/' + encodeURIComponent(item.id) + '/report'; report.replaceChildren(link);
   }
   $('records-body').append(row);
  }
  const pages = Math.max(1, Math.ceil(data.total / data.size));
  $('records-status').textContent = '共 ' + data.total + ' 条记录';
  $('page').textContent = page + ' / ' + pages;
  $('previous').disabled = page <= 1; $('next').disabled = page >= pages;
 } catch (error) { $('records-status').textContent = error.message; }
}
$('previous').addEventListener('click', () => { page--; load(); });
$('next').addEventListener('click', () => { page++; load(); });
$('show-jobs').addEventListener('click', () => { mode = 'jobs'; page = 1; load(); });
$('show-access').addEventListener('click', () => { mode = 'access'; page = 1; load(); });
load();
