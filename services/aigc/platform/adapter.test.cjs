'use strict';
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, 'adapter.js'), 'utf8');
const config = {courseId:'1',classId:'2',workId:'3',answerId:'4',studentNumber:'5'};
// 来自实采样本的时间结构，移除账号、任务和报告授权值。
const sampledTimes = ['2026-10-10 21:59:02','2026-10-10 18:37:16','2026-09-15 17:57:41'];
function refresh(times, current) {
  const clicked = [];
  const input = {value:current,click(){},getClientRects:()=>[{}]};
  const options = times.map(textContent=>({textContent,click(){clicked.push(textContent);},getClientRects:()=>[{}]}));
  const document = {
    querySelector(selector) {
      if (selector === '#personalInfor dd') return {textContent:config.studentNumber};
      if (selector === '#selectTime1') return input;
      if (selector === '#zSelect') return {getClientRects:()=>[{}]};
      throw new Error('unexpected selector');
    },
    querySelectorAll(selector) {
      if (selector === '#zSelect li.select-item') return options;
      throw new Error('unexpected selector');
    }
  };
  const adapter = vm.runInNewContext(source, {document,URL,location:{href:'https://mooc2-ans.chaoxing.com/mooc2-ans/work/library/review-work?courseid=1&clazzid=2&workId=3&workAnswerId=4'}});
  adapter.refresh(config);
  return clicked;
}
assert.deepEqual(refresh(sampledTimes,sampledTimes[1]),[sampledTimes[0]],'旧输入值不能让轮询停留在旧报告');
assert.deepEqual(refresh([sampledTimes[2],sampledTimes[0],sampledTimes[1]],sampledTimes[2]),[sampledTimes[0]],'不能假定选项顺序');
assert.deepEqual(refresh(['请选择',...sampledTimes],''),[sampledTimes[0]],'应忽略非批次选项');
assert.throws(()=>refresh([sampledTimes[0],sampledTimes[0]],sampledTimes[0]),/不唯一/,'同一时间多批次必须停止');

{
  let open=false,visible=false,inputClicks=0,chosen='';
  const item={textContent:sampledTimes[0],getClientRects:()=>visible?[{}]:[],click(){chosen=this.textContent;}};
  const input={getClientRects:()=>[{}],click(){inputClicks++;open=!open;}};
  const document={
    querySelector(s){
      if(s==='#personalInfor dd')return{textContent:config.studentNumber};
      if(s==='#selectTime1')return input;
      if(s==='#zSelect')return{getClientRects:()=>open?[{}]:[]};
      throw new Error('unexpected selector');
    },
    querySelectorAll(s){assert.equal(s,'#zSelect li.select-item');return[item];}
  };
  const adapter=vm.runInNewContext(source,{document,URL,location:{href:'https://mooc2-ans.chaoxing.com/mooc2-ans/work/library/review-work?courseid=1&clazzid=2&workId=3&workAnswerId=4'}});
  assert.equal(adapter.refresh(config),false,'动画未结束应等待');
  visible=true;
  assert.equal(adapter.refresh(config),true,'已展开菜单不可重复点击关闭');
  assert.equal(inputClicks,1);
  assert.equal(chosen,sampledTimes[0]);
}
console.log('platform adapter: 5 checks passed');
