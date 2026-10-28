<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import Icon from '../components/Icon.vue'

const route = useRoute()
const router = useRouter()
const descriptions: Record<string, string> = {
  papers: 'Vue 版智能组卷正在迁移中，当前仍可在旧版首页使用完整组卷功能。',
  wrong: 'Vue 版错题本正在迁移中，当前仍可在旧版首页使用完整错题功能。',
  favorites: 'Vue 版收藏正在迁移中，当前仍可在旧版首页使用完整收藏功能。',
  custom: 'Vue 版自定义题库正在迁移中，当前仍可在旧版首页使用完整功能。',
  notes: 'Vue 版笔记正在迁移中，当前仍可在旧版首页使用完整功能。',
  ai: 'Vue 版 AI 设置正在迁移中，当前仍可在旧版首页使用完整功能。',
}
</script>

<template>
  <div class="card vue-placeholder">
    <div class="vue-placeholder-icon"><Icon name="sparkles" :size="28" /></div>
    <h2>{{ route.meta.title }}</h2>
    <p class="muted">{{ descriptions[String(route.name)] || '该页面正在迁移中。' }}</p>
    <button class="btn btn-primary btn-block" @click="router.push('/')">返回首页</button>
  </div>
</template>

<style scoped>
.vue-placeholder { text-align: center; padding: 30px 20px; margin-top: 18px; }
.vue-placeholder-icon {
  width: 54px; height: 54px; border-radius: 16px; margin: 0 auto 12px;
  display: grid; place-items: center; color: #fff; background: var(--primary);
}
.vue-placeholder h2 { margin: 0 0 8px; }
</style>
