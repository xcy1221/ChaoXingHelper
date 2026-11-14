<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue';
import { useTheme, tones, toneNames, read } from '@homeserver/theme';
import { LayoutControls } from '@homeserver/ui';
const props = defineProps<{ records: boolean }>();
const { state, select } = useTheme();
const dialog = ref<HTMLDialogElement>(); const controls = ref<InstanceType<typeof LayoutControls>>();
const message = ref(''); const pageId = props.records ? 'aigc-records' : 'aigc';
function applyLegacy() {
  let saved: any = {}; try { saved = JSON.parse(read('aigc-layout-v1', '{}') || '{}'); } catch {}
  let current: any; try { current = JSON.parse(read('serverpanel-page-layout-v1:' + pageId, 'null') || 'null'); } catch {}
  const density = current?.density ?? saved.density, columns = current?.columns ?? saved.columns;
  document.documentElement.dataset.aigcDensity = density === 'compact' || Number(density) <= 75 ? 'compact' : 'comfortable';
  document.documentElement.dataset.aigcColumns = columns === 'stack' || Number(columns) === 1 ? 'stack' : 'split';
}
function open() { dialog.value?.showModal(); document.getElementById('aigc-appearance-trigger')?.setAttribute('aria-expanded', 'true'); dialog.value?.querySelector<HTMLInputElement>('input[name=theme]:checked')?.focus(); }
function closed() { document.getElementById('aigc-appearance-trigger')?.setAttribute('aria-expanded', 'false'); document.getElementById('aigc-appearance-trigger')?.focus(); }
function reset() { controls.value?.reset(); applyLegacy(); message.value = '已恢复 AIGC 默认排版。'; }
function storage(event: StorageEvent) { if (event.key === null || event.key === 'aigc-layout-v1' || event.key === 'serverpanel-page-layout-v1:' + pageId) applyLegacy(); }
onMounted(() => { applyLegacy(); window.addEventListener('storage', storage); window.addEventListener('serverpanel-layout-change', applyLegacy); });
onUnmounted(() => { window.removeEventListener('storage', storage); window.removeEventListener('serverpanel-layout-change', applyLegacy); });
defineExpose({ open });
</script>
<template>
  <dialog ref="dialog" id="aigc-appearance" aria-labelledby="aigc-appearance-title" @close="closed" @click="event => { if (event.target === dialog && event.clientX < dialog!.getBoundingClientRect().left) dialog?.close(); }">
    <div class="aigc-settings-heading"><h2 id="aigc-appearance-title">外观与排版</h2><button id="aigc-appearance-close" type="button" aria-label="关闭外观与排版" @click="dialog?.close()">×</button></div>
    <p>调整 AIGC 检测与记录页面的显示方式，设置保存在当前浏览器。</p><form id="aigc-appearance-form" @submit.prevent>
      <fieldset><legend>界面明暗</legend><div class="aigc-options"><label v-for="theme in ['light','dark']" :key="theme"><input type="radio" name="theme" :value="theme" :checked="state.theme === theme" @change="select('theme', theme)">{{ theme === 'light' ? '浅色' : '深色' }}</label></div></fieldset>
      <fieldset><legend>界面色调</legend><div class="aigc-options"><label v-for="(tone,index) in tones" :key="tone"><input type="radio" name="tone" :value="tone" :checked="state.tone === tone" @change="select('tone',tone)">{{ toneNames[index] }}</label></div></fieldset>
      <LayoutControls ref="controls" :page="{ id: pageId, legacyKey: 'aigc-layout-v1', columns: records ? 0 : 2, defaultColumns: 2, gap: 22 }" />
      <button id="aigc-layout-reset" type="button" @click="reset">恢复默认排版</button><p id="aigc-layout-status" role="status">{{ state.storageFailed ? '浏览器未允许保存设置；本次调整仍然生效。' : message }}</p>
    </form>
  </dialog>
</template>
