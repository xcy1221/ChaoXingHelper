import { after, before, test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, mkdir } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import { chromium } from '@playwright/test';

const root = resolve(import.meta.dirname, '..'), legacy = resolve(root, '../web');
let browser, server, origin; const uploads = [];
const id = 'a'.repeat(32); let pollCount = 0;
before(async () => {
  server = createServer(async (request, response) => {
    const url = new URL(request.url, 'http://localhost');
    const json = value => { response.setHeader('Content-Type', 'application/json'); response.end(JSON.stringify(value)); };
    const external = url.pathname.startsWith('/external/') || url.pathname.startsWith('/legacy-external/');
    if (url.pathname.includes('/api/')) {
      const path = url.pathname.split('/api/')[1];
      if (path === 'health') return json({ ready: true, paused: false, canViewRecords: !external });
      if (path === 'jobs' && request.method === 'POST') { let bytes=0; for await (const chunk of request) bytes+=chunk.length; uploads.push({ key:request.headers['idempotency-key'], bytes }); return json({ id, status:'queued' }); }
      if (path === 'jobs/' + id) return json({ id, status:++pollCount > 1 ? 'succeeded':'detecting', summary:'隔离报告样例', reportAvailable:pollCount > 1 });
      if (path === 'records' || path === 'access-records') return json({ total:51, size:50, records:path === 'records' ? [{ id, createdAt:1, user:'内网', clientIP:'192.0.2.1', fileName:'fixture.docx', status:'succeeded', reportAvailable:true }] : [{ createdAt:1, user:'内网', clientIP:'192.0.2.1', method:'GET', path:'/', status:200 }] });
      response.statusCode=404; return json({error:'fixture missing'});
    }
    let file;
    if (url.pathname.startsWith('/legacy-external/')) file=resolve(legacy,url.pathname.slice('/legacy-external/'.length)||'index.html');
    else if (url.pathname.startsWith('/legacy/')) file=resolve(legacy,url.pathname.slice('/legacy/'.length)||'index.html');
    else if (/^\/_web\/aigc\/[^/]+\//.test(url.pathname)) file=resolve(root,'dist',url.pathname.replace(/^\/_web\/aigc\/[^/]+\//,''));
    else file=resolve(root,'dist',url.pathname.endsWith('records.html')?'records.html':'index.html');
    try { response.setHeader('Content-Type',({'.js':'text/javascript','.css':'text/css','.html':'text/html'})[extname(file)]||'application/octet-stream'); response.setHeader('Content-Security-Policy',"default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'"); response.end(await readFile(file)); }
    catch { response.statusCode=404; response.end(); }
  });
  await new Promise(done=>server.listen(0,'127.0.0.1',done)); origin='http://127.0.0.1:'+server.address().port;
  browser=await chromium.launch({channel:process.env.PLAYWRIGHT_CHANNEL||'chrome',headless:true});
});
after(async()=>{await browser?.close(); await new Promise(done=>server?.close(done));});

async function layoutSnapshot(page, selectors, relativeToHeader = false) {
  return page.evaluate(({ selectors, relativeToHeader }) => {
    const offset = relativeToHeader ? document.querySelector('#aigc-server-header').getBoundingClientRect().bottom : 0;
    return selectors.map(selector => {
      const element = document.querySelector(selector);
      const { x, y, width, height } = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      return { selector, x, y: y - offset, width, height, font: style.fontSize, bg: style.backgroundColor, color: style.color };
    });
  }, { selectors, relativeToHeader });
}

test('公共顶栏覆盖内网明暗五色与宽窄屏，业务区布局保留', async () => {
  const selectors = ['main', '.intro h1', '.workspace', '.card', '#dropzone', '#submit', 'footer'];
  for (const width of [1440, 360]) for (const theme of ['dark', 'light']) for (const tone of ['green', 'blue', 'red', 'pink', 'purple']) {
    const context = await browser.newContext({ viewport: { width, height: 960 } });
    await context.addInitScript(({ theme, tone }) => {
      localStorage.setItem('scheduletool-theme', theme);
      localStorage.setItem('scheduletool-tone', tone);
    }, { theme, tone });
    const snapshots = [];
    for (const url of ['/legacy/', '/chaoxing/']) {
      const page = await context.newPage();
      const errors = [];
      page.on('pageerror', error => errors.push(error.message));
      page.on('console', message => { if (message.type() === 'error' && /content security policy/i.test(message.text())) errors.push(message.text()); });
      await page.goto(origin + url);
      await page.waitForSelector('html[data-aigc-internal=true]');
      if (url === '/chaoxing/') {
        await page.waitForSelector('html[data-layout-page=aigc]');
        const header = page.locator('#aigc-server-header');
        assert.equal(await header.getByRole('link', { name: '←返回server panel' }).getAttribute('target'), '_top');
        assert.equal(await header.getByRole('button', { name: '外观与排版' }).isVisible(), true);
        assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).colorScheme), theme);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
        if (tone === 'green') {
          const folder = resolve(root, '../../../.inspection/web-migration');
          await mkdir(folder, { recursive: true });
          await page.screenshot({ path: resolve(folder, `${width}-${theme}.png`), fullPage: true });
        }
      }
      const layout = await layoutSnapshot(page, selectors, true);
      snapshots.push(layout.map(({ bg, color, ...geometry }) => geometry));
      assert.deepEqual(errors, []);
      await page.close();
    }
    assert.deepEqual(snapshots[1], snapshots[0], `${width}/${theme}/${tone}`);
    await context.close();
  }
});

test('外部独立样式及内部记录业务区布局保留', async () => {
  for (const width of [1440, 360]) for (const records of [false, true]) {
    const context = await browser.newContext({ viewport: { width, height: 960 } }), snapshots = [];
    const paths = records ? ['/legacy/records.html', '/chaoxing/records.html'] : ['/legacy-external/', '/external/'];
    const selectors = records ? ['main', '.intro h1', '.records-scroll', '.records-table', 'footer'] : ['#aigc-external-header', 'main', '.workspace', '.card', 'footer'];
    for (const url of paths) {
      const page = await context.newPage();
      await page.goto(origin + url);
      if (records) {
        await page.waitForSelector('#records-body tr');
        await page.waitForSelector('html[data-aigc-internal=true]');
        if (url === '/chaoxing/records.html') await page.waitForSelector('html[data-layout-page=aigc-records]');
      } else {
        await page.waitForSelector('#connection:text("检测服务已连接")');
        assert.equal(await page.getByRole('button', { name: '外观与排版' }).count(), 0);
      }
      const layout = await layoutSnapshot(page, selectors, records);
      snapshots.push(records ? layout.map(({ bg, color, ...geometry }) => geometry) : layout);
      await page.close();
    }
    assert.deepEqual(snapshots[1], snapshots[0], `${width}/records=${records}`);
    await context.close();
  }
});

test('上传幂等键、轮询、报告及记录分页保留',async()=>{
  const context=await browser.newContext(),page=await context.newPage();
  await page.goto(origin+'/chaoxing/');await page.waitForSelector('html[data-aigc-internal=true]');
  await page.locator('#file').setInputFiles({name:'fixture.txt',mimeType:'text/plain',buffer:Buffer.from('invalid')});
  assert.equal(await page.locator('#submit').isDisabled(),true);
  await page.locator('#file').setInputFiles({name:'fixture.docx',mimeType:'application/vnd.openxmlformats-officedocument.wordprocessingml.document',buffer:Buffer.from('isolated mock upload')});
  await page.locator('#submit').click();await page.waitForSelector('#report:not([hidden])',{timeout:12000});
  assert.match(await page.locator('#report').getAttribute('href'),new RegExp(id+'/report$'));
  assert.equal(uploads.length,1);assert.match(uploads[0].key,/^[a-f0-9-]{36}$/);assert.ok(uploads[0].bytes>0);
  await page.goto(origin+'/chaoxing/records.html');await page.waitForSelector('#records-body tr');
  assert.match(await page.locator('#records-body').innerText(),/fixture.docx/);await page.locator('#next').click();assert.equal(await page.locator('#page').textContent(),'2 / 2');
  await page.locator('#show-access').click();await page.waitForSelector('#records-body td:nth-child(3):text("GET")');
  const trigger = page.getByRole('button', { name: '外观与排版' });
  const dialog = page.getByRole('dialog', { name: '外观与排版' });
  await trigger.click();
  await dialog.locator('input[name=tone][value=purple]').check();
  await dialog.getByRole('button', { name: '紧凑' }).click();
  assert.equal(await page.evaluate(() => localStorage.getItem('scheduletool-tone')), 'purple');
  assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('serverpanel-page-layout-v1:aigc-records')).density), 75);
  assert.equal(await page.evaluate(() => localStorage.getItem('serverpanel-page-layout-v1:aigc')), null);
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(await trigger.evaluate(element => element === document.activeElement), true);
  await page.goto(origin+'/external/');await page.waitForSelector('#connection:text("检测服务已连接")');
  assert.equal(await page.locator('#aigc-server-header').isVisible(),false);assert.equal(await page.locator('#records-link').isVisible(),false);
  await context.close();
});
