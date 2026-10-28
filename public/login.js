'use strict';

const form = document.getElementById('login-form');
const btn = document.getElementById('login-btn');
const errorBox = document.getElementById('login-error');

fetch('/api/auth/me', { credentials: 'same-origin' })
  .then((r) => { if (r.ok) location.replace('/'); })
  .catch(() => {});

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  errorBox.textContent = '';
  btn.disabled = true;
  btn.textContent = '登录中…';
  try {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        username: document.getElementById('username').value.trim(),
        password: document.getElementById('password').value,
      }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || '登录失败');
    location.replace('/');
  } catch (err) {
    errorBox.textContent = err.message || '登录失败，请重试';
    btn.disabled = false;
    btn.textContent = '登录';
    document.getElementById('password').select();
  }
});
