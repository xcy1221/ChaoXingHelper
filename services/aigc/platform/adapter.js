(() => {
  'use strict';
  const shown = e => Boolean(e && e.getClientRects().length);
  const requireOne = (root, selector) => {
    const nodes = root.querySelectorAll(selector);
    if (nodes.length !== 1) throw new Error('页面结构不符：' + selector);
    return nodes[0];
  };
  function binding(c, student) {
    const u = new URL(location.href);
    const hosts = student ? ['mooc1.chaoxing.com','mooc1-api.chaoxing.com'] : ['mooc2-ans.chaoxing.com'];
    if (u.protocol !== 'https:' || !hosts.includes(u.hostname)) throw new Error('平台域名不符');
    const expect = student
      ? {cpi:c.cpi,courseId:c.courseId,classId:c.classId,workId:c.workId,answerId:c.answerId}
      : {courseid:c.courseId,clazzid:c.classId,workId:c.workId,workAnswerId:c.answerId};
    const path = student ? '/mooc-ans/mooc2/work/dowork' : '/mooc2-ans/work/library/review-work';
    if (u.pathname !== path) throw new Error('未进入指定作业页面');
    for (const [k,v] of Object.entries(expect)) {
      if (u.searchParams.get(k) !== v) throw new Error('作业绑定不符：' + k);
    }
    if (!student && document.querySelector('#personalInfor dd')?.textContent.trim() !== c.studentNumber) {
      throw new Error('批阅对象不符');
    }
  }
  function editor(c) {
    binding(c, true);
    const frame = requireOne(document, '#question' + c.questionId + ' iframe[id^="ueditor_"]');
    const body = frame.contentDocument?.body;
    if (!body || body.getAttribute('contenteditable') !== 'true') throw new Error('编辑器不可用');
    return {frame,body};
  }
  const attachments = root => Array.from(root.querySelectorAll('iframe.attach-iframe')).map(e => ({
    objectId:e.getAttribute('objectid'),name:e.getAttribute('filename'),html:e.closest('.editor-iframe')?.outerHTML
  }));
  function student(c) {
    const {body} = editor(c);
    return {attachments:attachments(body),html:body.innerHTML};
  }
  function review(c) {
    binding(c, false);
    const answer = requireOne(document, '#stuanswer_' + c.stemId);
    const rows = Array.from(document.querySelectorAll('#checkDataList tr.aigcItem')).map(e => {
      const cells = e.querySelectorAll('td');
      return {
        id:e.id,question:e.id.split('-')[1],similarity:cells[3]?.textContent.trim() || '',
        aigc:cells[4]?.textContent.trim() || '',
        reportURL:e.querySelector('a[href*="/getAigcReport?"]')?.getAttribute('href') || ''
      };
    });
    return {
      attachments:attachments(answer),rows,
      time:document.querySelector('#selectTime1')?.value || '',
      resultVisible:shown(document.querySelector('#dyPersonCheckResult'))
    };
  }
  function openResults(c) {
    binding(c, false);
    const link = requireOne(document, 'a[onclick="showCheckDiv()"]');
    link.click();
  }
  function resultsReady(c) {
    binding(c, false);
    return shown(document.querySelector('li[onclick="tabChange(this)"][data="2"]'));
  }
  function openDaya(c) {
    binding(c, false);
    const tab = requireOne(document, 'li[onclick="tabChange(this)"][data="2"]');
    if (!shown(tab)) throw new Error('查重页签未显示');
    tab.click();
  }
  function trigger(c) {
    binding(c, false);
    const box = requireOne(document, '#aigcRecheck');
    if (!shown(box)) throw new Error('重新检测入口未就绪');
    box.dispatchEvent(new MouseEvent('mouseover',{bubbles:true}));
    const action = requireOne(box, 'li.aigcli[onclick="checkDuplicateByDY(1);"]');
    if (!shown(action) || action.textContent.trim() !== 'AIGC检测和大雅查重') throw new Error('AIGC 菜单不符');
    action.click(); // 平台立即开始检测，禁止重试此动作。
  }
  function refresh(c) {
    binding(c, false);
    const input = document.querySelector('#selectTime1');
    if (!shown(input)) return false;
    const list = document.querySelector('#zSelect');
    if (!shown(list)) input.click();
    // 新批次建立时输入框可能仍显示旧时间，必须从实际选项中选最新批次。
    const items = Array.from(document.querySelectorAll('#zSelect li.select-item'))
      .map(e=>({element:e,time:e.textContent.trim()}))
      .filter(item=>/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(item.time));
    if (!items.length) return false;
    const latest = items.reduce((time,item)=>item.time>time?item.time:time,'');
    const matches = items.filter(item=>item.time===latest);
    if (matches.length!==1) throw new Error('最新检测批次不唯一');
    if (!shown(matches[0].element)) return false;
    matches[0].element.click();
    return true;
  }
  function openUpload(c) {
    editor(c);
    const button = requireOne(document.querySelector('#question' + c.questionId), '[title="附件"]');
    button.click();
  }
  function uploadInput(c) {
    binding(c, true);
    const frame = requireOne(document, '.edui-dialog.edui-for-attachment_new iframe');
    return requireOne(frame.contentDocument, '#pickfiles input[type="file"]');
  }
  function keepOnly(c, objectId, expectedName) {
    const {body} = editor(c);
    const item = attachments(body).filter(a=>a.objectId===objectId && a.name===expectedName);
    if (item.length !== 1 || !item[0].html) throw new Error('新附件未确认，不替换答案');
    const ue = Object.values(window.UE?.instants || {}).find(e=>e.body===body);
    if (!ue || typeof ue.setContent !== 'function') throw new Error('无法同步编辑器');
    ue.setContent('<p><br></p>' + item[0].html + '<p><br></p>');
    ue.fireEvent('contentchange');
    const after = attachments(body);
    if (after.length !== 1 || after[0].objectId !== objectId) throw new Error('附件替换校验失败');
    return true;
  }
  function submit(c, objectId, expectedName) {
    const current = student(c).attachments;
    if (current.length!==1 || current[0].objectId!==objectId || current[0].name!==expectedName) {
      throw new Error('待提交附件不符');
    }
    requireOne(document, 'a.completeBtn[onclick="submitValidate(3);"]').click();
  }
  function confirm(c, objectId, expectedName) {
    const current = student(c).attachments;
    if (current.length!==1 || current[0].objectId!==objectId || current[0].name!==expectedName) throw new Error('确认前附件被更改');
    binding(c, true);
    const message = requireOne(document, '#popcontent');
    const button = requireOne(document, '#popok');
    if (!shown(button) || message.textContent.trim() !== '确认提交？' || button.textContent.trim() !== '提交') {
      throw new Error('提交确认窗口不符');
    }
    button.click(); // 只执行一次；结果必须从教师端核实。
  }
  return {student,review,openResults,resultsReady,openDaya,trigger,refresh,openUpload,uploadInput,keepOnly,submit,confirm};
})()
