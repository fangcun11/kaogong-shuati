<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from '../stores/session'
import Icon from '../components/Icon.vue'

const router = useRouter()
const route = useRoute()
const form = reactive({ username: '', password: '' })
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await login(form.username.trim(), form.password)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '登录失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="vue-login-page">
    <section class="vue-login-card">
      <div class="vue-brand-mark">公</div>
      <h1>没钱考什么公</h1>
      <p class="vue-login-sub">登录后继续刷题。你的题库记录、错题、收藏和笔记只属于你的账号。</p>
      <form @submit.prevent="submit">
        <label for="username">用户名</label>
        <div class="vue-input-wrap">
          <Icon name="user" :size="17" />
          <input id="username" v-model="form.username" autocomplete="username" required autofocus />
        </div>
        <label for="password">密码</label>
        <div class="vue-input-wrap">
          <Icon name="lock" :size="17" />
          <input id="password" v-model="form.password" type="password" autocomplete="current-password" required />
        </div>
        <button class="vue-login-button" type="submit" :disabled="loading">
          {{ loading ? '登录中…' : '登录' }}
        </button>
        <div class="vue-login-error" role="alert">{{ error }}</div>
      </form>
      <div class="vue-login-note">账号由管理员创建；忘记密码请联系管理员重置。</div>
    </section>
  </main>
</template>

<style scoped>
.vue-login-page {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
  background: var(--bg);
  color: var(--text);
}
.vue-login-card {
  width: min(420px, 100%);
  padding: 32px 28px 26px;
  border: 1px solid var(--border);
  border-radius: 24px;
  background: var(--card);
  box-shadow: var(--shadow);
}
.vue-brand-mark {
  width: 56px;
  height: 56px;
  border-radius: 16px;
  display: grid;
  place-items: center;
  background: var(--primary);
  color: #fff;
  font-size: 26px;
  font-weight: 800;
  margin-bottom: 20px;
}
h1 { margin: 0 0 8px; font-size: 25px; letter-spacing: -0.5px; }
.vue-login-sub { margin: 0 0 26px; color: var(--text-2); font-size: 14px; line-height: 1.6; }
label { display: block; margin: 16px 0 7px; font-size: 13px; font-weight: 650; }
.vue-input-wrap {
  display: flex;
  align-items: center;
  gap: 9px;
  height: 48px;
  padding: 0 14px;
  border: 1px solid var(--border-strong);
  border-radius: 12px;
  color: var(--text-3);
}
.vue-input-wrap:focus-within { border-color: var(--primary); box-shadow: 0 0 0 3px var(--primary-soft); }
.vue-input-wrap input { flex: 1; min-width: 0; border: 0; outline: 0; background: transparent; color: var(--text); font-size: 16px; }
.vue-login-button {
  width: 100%;
  height: 50px;
  margin-top: 24px;
  border: 0;
  border-radius: 13px;
  background: var(--primary);
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  cursor: pointer;
}
.vue-login-button:disabled { opacity: 0.55; cursor: default; }
.vue-login-error { min-height: 20px; margin-top: 12px; color: var(--red); font-size: 13px; line-height: 1.5; }
.vue-login-note { margin-top: 22px; padding-top: 16px; border-top: 1px solid var(--border); color: var(--text-2); font-size: 12px; line-height: 1.6; }
</style>
