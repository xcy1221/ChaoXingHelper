'use strict';
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, 'login.js'), 'utf8');
function fixture(url='https://passport2.chaoxing.com/login') {
  let clicks=0;
  const element = data => ({getClientRects:()=>[{}], dispatchEvent(){}, ...data});
  const nodes = {
    '#phone':element({type:'text',value:''}), '#pwd':element({type:'password',value:''}),
    '#loginBtn':element({getAttribute:()=> 'loginByPhoneAndPwd();',click(){clicks++;}}),
    '#needVcode':{value:''}, '#doubleFactorLogin':{value:'0'},
    '#captcha':element({children:[]}),
    '.auto-lg-next .check-input':element({className:'check-input ',click(){this.className='check-input checked';}})
  };
  const document={querySelector:s=>nodes[s],querySelectorAll:s=>nodes[s]?[nodes[s]]:[]};
  const adapter=vm.runInNewContext(source,{location:new URL(url),document,Event:class Event{}});
  return {adapter,nodes,clicks:()=>clicks};
}
const credentials={phone:'13800000000',password:'fixture-secret'};
{
  const f=fixture();
  f.adapter.submit(credentials);
  assert.equal(f.clicks(),1);
  assert.equal(f.nodes['#phone'].value,credentials.phone);
  assert.equal(f.nodes['#pwd'].value,credentials.password);
  assert.equal(f.nodes['.auto-lg-next .check-input'].className,'check-input checked');
  assert.equal(f.adapter.blocked(),false);
}
{
  const f=fixture();
  f.nodes['.auto-lg-next .check-input'].className='check-input checked';
  f.nodes['.auto-lg-next .check-input'].click=()=>assert.fail('must preserve checked remember option');
  f.adapter.submit(credentials);
  assert.equal(f.clicks(),1);
}
for (const url of ['http://passport2.chaoxing.com/login','https://passport2.chaoxing.com.attacker.example/login','https://passport2.chaoxing.com/other']) {
  const f=fixture(url);
  assert.throws(()=>f.adapter.submit(credentials),/域名/);
  assert.equal(f.nodes['#pwd'].value,'');
  assert.equal(f.clicks(),0);
}
for (const change of [
  f=>f.nodes['#needVcode'].value='1',
  f=>f.nodes['#doubleFactorLogin'].value='1',
  f=>f.nodes['#captcha'].children.push({}),
  f=>f.nodes['#err-txt']={getClientRects:()=>[{}],textContent:'密码错误'},
  f=>f.nodes['#pwd'].type='text',
  f=>f.nodes['#loginBtn'].disabled=true,
  f=>delete f.nodes['#phone']
]) {
  const f=fixture();change(f);
  assert.throws(()=>f.adapter.submit(credentials));
  assert.equal(f.nodes['#pwd'].value,'');
  assert.equal(f.clicks(),0);
}
console.log('login adapter: origin, password form and challenge checks passed');
