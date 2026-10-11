'use strict';
document.getElementById('beszel').href = 'http://' + location.hostname + ':8090/';
document.getElementById('mihomo').href = 'http://' + location.hostname + ':8080/';
fetch('/chaoxing/api/health', { cache: 'no-store' })
  .then(response => { if (!response.ok) throw new Error('unavailable'); return response.json(); })
  .then(data => { document.getElementById('aigc-status').textContent = data.paused ? '队列暂停' : data.ready ? '可用' : '检测待接入'; })
  .catch(() => { document.getElementById('aigc-status').textContent = '服务尚未连接'; });
