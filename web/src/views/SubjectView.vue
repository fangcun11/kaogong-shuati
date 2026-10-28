<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'

interface Category {
  category: string
  papers: number
  questions: number
}

const route = useRoute()
const router = useRouter()
const categories = ref<Category[]>([])
const loading = ref(true)
const error = ref('')

onMounted(async () => {
  try {
    const name = String(route.params.name || '')
    document.title = `${name} · 没钱考什么公`
    categories.value = await api<Category[]>(`/api/categories?subject=${encodeURIComponent(name)}`)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <template v-else>
    <div class="card">
      <h3>{{ route.params.name }}</h3>
      <p class="muted" style="margin: 4px 0 0">按来源分类查看题库</p>
    </div>
    <div class="vue-category-list">
      <button v-for="c in categories" :key="c.category" class="list-item vue-category-item">
        <span class="li-icon tint-blue">◫</span>
        <span class="li-main">
          <span class="li-title">{{ c.category }}</span>
          <span class="li-sub">{{ c.papers }} 套 · {{ c.questions }} 题</span>
        </span>
        <span class="li-arrow">›</span>
      </button>
    </div>
    <button class="btn btn-ghost btn-block" style="margin-top: 14px" @click="router.push('/')">返回首页</button>
  </template>
</template>

<style scoped>
.vue-category-list { margin-top: 12px; display: grid; gap: 8px; }
.vue-category-item { width: 100%; text-align: left; border: 1px solid var(--border); background: var(--card); }
.vue-category-item .li-main { display: flex; flex-direction: column; align-items: flex-start; }
</style>
