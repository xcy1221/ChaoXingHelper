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

test('既有DOM、明暗五色及宽窄屏布局一致', async()=>{
  const selectors=['#aigc-server-header','main','.intro h1','.workspace','.card','#dropzone','#submit','footer'];
  for(const width of [1440,390]) for(const theme of ['dark','light']) for(const tone of ['green','blue','red','pink','purple']) {
    const context=await browser.newContext({viewport:{width,height:960}});
    await context.addInitScript(({theme,tone})=>{localStorage.setItem('scheduletool-theme',theme);localStorage.setItem('scheduletool-tone',tone);},{theme,tone});
    const snapshots=[];
    for(const path of ['/legacy/','/chaoxing/']) {
      const page=await context.newPage(); const errors=[];page.on('pageerror',e=>errors.push(e.message));
      await page.goto(origin+path);await page.waitForSelector('html[data-aigc-internal=true]');
      await page.waitForTimeout(30);
      snapshots.push(await page.evaluate(selectors=>selectors.map(selector=>{const e=document.querySelector(selector),r=e.getBoundingClientRect(),s=getComputedStyle(e);return {selector,x:r.x,y:r.y,width:r.width,height:r.height,font:s.fontSize,bg:s.backgroundColor,color:s.color};}),selectors));
      if (tone==='green' && path==='/chaoxing/') { const folder=resolve(root,'../../../.inspection/web-migration');await mkdir(folder,{recursive:true});await page.screenshot({path:resolve(folder,`${width}-${theme}.png`),fullPage:true}); }
      assert.deepEqual(errors,[]);
      await page.close();
    }
    assert.deepEqual(snapshots[1],snapshots[0],`${width}/${theme}/${tone}`);
    await context.close();
  }
});

test('外部独立样式及内部记录页保持原布局',async()=>{
  for(const width of [1440,390]) for(const records of [false,true]) {
    const context=await browser.newContext({viewport:{width,height:960}}), snapshots=[];
    for(const path of records?['/legacy/records.html','/chaoxing/records.html']:['/legacy-external/','/external/']) {
      const page=await context.newPage();await page.goto(origin+path);
      if(records) await page.waitForSelector('#records-body tr');else await page.waitForSelector('#connection:text("检测服务已连接")');
      await page.waitForTimeout(30);
      const selectors=records?['main','.intro h1','.records-scroll','.records-table','footer']:['#aigc-external-header','main','.workspace','.card','footer'];
      snapshots.push(await page.evaluate(selectors=>selectors.map(selector=>{const e=document.querySelector(selector),r=e.getBoundingClientRect(),s=getComputedStyle(e);return {selector,x:r.x,y:r.y,width:r.width,height:r.height,font:s.fontSize,bg:s.backgroundColor,color:s.color};}),selectors));
      await page.close();
    }
    assert.deepEqual(snapshots[1],snapshots[0],`${width}/records=${records}`);await context.close();
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
  await page.locator('#aigc-appearance-trigger').click();await page.locator('input[name=tone][value=purple]').check();
  assert.equal(await page.evaluate(()=>localStorage.getItem('scheduletool-tone')),'purple');
  await page.goto(origin+'/external/');await page.waitForSelector('#connection:text("检测服务已连接")');
  assert.equal(await page.locator('#aigc-server-header').isVisible(),false);assert.equal(await page.locator('#records-link').isVisible(),false);
  await context.close();
});
