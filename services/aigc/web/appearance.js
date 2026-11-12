(() => {
  'use strict';
  const root = document.documentElement, key = 'aigc-layout-v1';
  const $ = id => document.getElementById(id);
  const form = $('aigc-appearance-form'), dialog = $('aigc-appearance'), trigger = $('aigc-appearance-trigger');
  let enabled = false, layout = { density: 'comfortable', columns: 'split' };
  function read(name) { try { return localStorage.getItem(name); } catch { return null; } }
  function save(name,value) { try { localStorage.setItem(name,value); } catch {} }
  function readLayout() {
    try { const value = JSON.parse(read(key)); return { density: value?.density === 'compact' ? 'compact' : 'comfortable', columns: value?.columns === 'stack' ? 'stack' : 'split' }; }
    catch { return { density: 'comfortable', columns: 'split' }; }
  }
  function apply() {
    if (!enabled) return;
    const theme = read('scheduletool-theme') === 'light' ? 'light' : 'dark';
    const savedTone = read('scheduletool-tone'), tone = ['green','blue','red','pink','purple'].includes(savedTone) ? savedTone : 'green';
    Object.assign(root.dataset, { aigcInternal: 'true', theme, tone, aigcDensity: layout.density, aigcColumns: layout.columns });
    form.elements.namedItem('theme').value = theme; form.elements.namedItem('tone').value = tone;
    for (const name of ['density','columns']) form.elements.namedItem(name).value = layout[name];
  }
  window.addEventListener('aigc-internal-context', event => {
    enabled = event.detail === true;
    $('aigc-server-header').hidden = !enabled; $('aigc-external-header').hidden = enabled;
    if (enabled) { layout = readLayout(); apply(); }
    else { delete root.dataset.aigcInternal; if (dialog.open) dialog.close(); }
  });
  trigger.onclick = () => { dialog.showModal(); trigger.setAttribute('aria-expanded','true'); form.querySelector('input[name=theme]:checked').focus(); };
  $('aigc-appearance-close').onclick = () => dialog.close();
  dialog.addEventListener('close', () => { trigger.setAttribute('aria-expanded','false'); trigger.focus(); });
  dialog.addEventListener('click', event => { if (event.target === dialog && event.clientX < dialog.getBoundingClientRect().left) dialog.close(); });
  form.onsubmit = event => event.preventDefault();
  form.onchange = event => {
    const input = event.target;
    if (input.name === 'theme') save('scheduletool-theme', input.value);
    else if (input.name === 'tone') save('scheduletool-tone', input.value);
    else if (['density','columns'].includes(input.name)) { layout[input.name] = input.value; save(key,JSON.stringify(layout)); }
    apply();
  };
  $('aigc-layout-reset').onclick = () => { layout = { density: 'comfortable', columns: 'split' }; save(key,JSON.stringify(layout)); apply(); $('aigc-layout-status').textContent = '已恢复 AIGC 默认排版。'; };
  window.addEventListener('storage', event => {
    if (event.key !== null && ![key,'scheduletool-theme','scheduletool-tone'].includes(event.key)) return;
    layout = readLayout(); apply();
  });
})();
