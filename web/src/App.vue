<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import Icon from './components/Icon.vue'
import LoginView from './views/LoginView.vue'
import { logout } from './stores/session'

const route = useRoute()
const router = useRouter()
const isLogin = computed(() => route.name === 'login')
const title = computed(() => String(route.meta.title || '没钱考什么公'))
const theme = ref<'light' | 'dark'>(document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light')

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', theme.value)
  localStorage.setItem('theme', theme.value)
}

async function signOut() {
  await logout()
  await router.replace({ name: 'login' })
}

function onUnauthorized() {
  if (route.name !== 'login') router.replace({ name: 'login', query: { redirect: route.fullPath } })
}

onMounted(() => window.addEventListener('kg:unauthorized', onUnauthorized))
onUnmounted(() => window.removeEventListener('kg:unauthorized', onUnauthorized))
</script>

<template>
  <LoginView v-if="isLogin" />
  <div v-else class="vue-shell">
    <header id="topbar">
      <button
        v-if="route.name !== 'home'"
        class="icon-btn"
        aria-label="返回"
        @click="router.back()"
      >
        <Icon name="chevronLeft" :size="22" :stroke="2.4" />
      </button>
      <button v-else class="icon-btn spacer" aria-label="返回" />
      <h1 id="app-title">{{ title }}</h1>
      <nav id="desk-nav" aria-label="主导航">
        <RouterLink :class="{ active: route.name === 'home' }" to="/">首页</RouterLink>
        <RouterLink :class="{ active: route.name === 'papers' }" to="/papers">智能组卷</RouterLink>
        <RouterLink :class="{ active: route.name === 'wrong' }" to="/wrong">错题</RouterLink>
        <RouterLink :class="{ active: route.name === 'favorites' }" to="/favorites">收藏</RouterLink>
      </nav>
      <div id="topbar-right">
        <button class="icon-btn" aria-label="切换深浅色" @click="toggleTheme">
          <Icon name="sun" :size="20" />
        </button>
        <button class="icon-btn" aria-label="退出登录" title="退出登录" @click="signOut">
          <Icon name="logout" :size="19" />
        </button>
      </div>
    </header>

    <main id="view" class="vue-view" :class="{ 'view-home': route.name === 'home' }">
      <RouterView />
    </main>

    <nav id="bottom-nav">
      <RouterLink class="nav-item" :class="{ active: route.name === 'home' }" to="/">
        <span class="nav-ico"><Icon name="home" :size="21" /></span>
        <span>首页</span>
      </RouterLink>
      <RouterLink class="nav-item" :class="{ active: route.name === 'papers' }" to="/papers">
        <span class="nav-ico"><Icon name="target" :size="21" /></span>
        <span>智能组卷</span>
      </RouterLink>
      <RouterLink class="nav-item" :class="{ active: route.name === 'wrong' }" to="/wrong">
        <span class="nav-ico"><Icon name="book" :size="21" /></span>
        <span>错题</span>
      </RouterLink>
      <RouterLink class="nav-item" :class="{ active: route.name === 'favorites' }" to="/favorites">
        <span class="nav-ico"><Icon name="star" :size="21" /></span>
        <span>收藏</span>
      </RouterLink>
    </nav>
  </div>
</template>

<style scoped>
#desk-nav { display: none; }

@media (min-width: 900px) {
  #topbar { padding: 0 32px; }
  #topbar h1 {
    flex: 0 1 auto;
    margin-right: 18px;
    text-align: left;
  }
  #desk-nav {
    display: flex;
    align-items: center;
    gap: 26px;
    margin-left: auto;
    margin-right: 20px;
  }
  #desk-nav a {
    color: var(--text-2);
    font-size: 13px;
    letter-spacing: .02em;
    text-decoration: none;
    padding: 6px 0;
    transition: color .15s ease;
  }
  #desk-nav a:hover { color: var(--text); }
  #desk-nav a.active { color: var(--text); font-weight: 600; }
  #bottom-nav { display: none; }
  #view { padding-bottom: 40px; }
  #view.view-home {
    max-width: 1040px;
    padding-left: 32px;
    padding-right: 32px;
  }
}
</style>
