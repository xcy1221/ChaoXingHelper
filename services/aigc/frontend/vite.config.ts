import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { resolve } from 'node:path';
export default defineConfig({
  plugins: [vue()], base: process.env.WEB_ASSET_BASE || '/_web/aigc/dev/',
  build: { rollupOptions: { input: { index: resolve(import.meta.dirname, 'index.html'), records: resolve(import.meta.dirname, 'records.html') } } }
});
