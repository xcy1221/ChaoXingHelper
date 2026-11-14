(() => {
  'use strict';
  const shown = e => Boolean(e && e.getClientRects().length);
  function binding() {
    if (location.protocol !== 'https:' || location.host !== 'passport2.chaoxing.com' || location.pathname !== '/login') {
      throw new Error('登录域名不符');
    }
  }
  function blocked() {
    binding();
    const captcha = document.querySelector('#captcha');
    const required = document.querySelector('#needVcode')?.value;
    return Boolean((required && required !== '0') ||
      document.querySelector('#doubleFactorLogin')?.value === '1' ||
      (shown(captcha) && captcha.children.length) ||
      ['#phoneMsg', '#pwdMsg', '#err-txt'].some(s => {
        const e = document.querySelector(s);
        return shown(e) && e.textContent.trim();
      }));
  }
  function submit(credentials) {
    binding();
    if (blocked()) throw new Error('登录需人工处理');
    const one = selector => {
      const items = document.querySelectorAll(selector);
      if (items.length !== 1 || !shown(items[0])) throw new Error('登录控件不符');
      return items[0];
    };
    const phone = one('#phone');
    const password = one('#pwd');
    const button = one('#loginBtn');
    if (phone.type !== 'text' || password.type !== 'password' || button.disabled ||
      button.getAttribute('onclick') !== 'loginByPhoneAndPwd();') throw new Error('登录方式不符');
    const remember = document.querySelector('.auto-lg-next .check-input');
    if (shown(remember) && remember.className.trim() === 'check-input') remember.click();
    phone.value = credentials.phone;
    password.value = credentials.password;
    for (const e of [phone, password]) {
      e.dispatchEvent(new Event('input', {bubbles:true}));
      e.dispatchEvent(new Event('change', {bubbles:true}));
    }
    button.click();
  }
  return {submit, blocked};
})()
