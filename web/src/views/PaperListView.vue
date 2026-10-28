<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'

interface Paper {
  id: number
  category: string
  name: string
  questionCount: number
  difficulty: number
}

const route = useRoute()
const router = useRouter()
const papers = ref<Paper[]>([])
const loading = ref(true)
const error = ref('')

function openPaper(p: Paper) {
  router.push({
    name: 'practice',
    params: { subject: String(route.params.subject) },
    query: { paperId: String(p.id) },
  })
}

onMounted(async () => {
  try {
    const subject = String(route.params.subject || '')
    const category = String(route.params.category || '')
    document.title = `${category} · ${subject}`
    papers.value = await api<Paper[]>(`/api/papers?subject=${encodeURIComponent(subject)}&category=${encodeURIComponent(category)}`)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载试卷失败'
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
      <h3>{{ route.params.category }}</h3>
      <p class="muted" style="margin: 4px 0 0">{{ papers.length }} 套试卷</p>
    </div>
    <div class="vue-paper-list">
      <button v-for="p in papers" :key="p.id" class="list-item vue-paper-item" @click="openPaper(p)">
        <span class="li-icon tint-green">▤</span>
        <span class="li-main">
          <span class="li-title">{{ p.name }}</span>
          <span class="li-sub">{{ p.questionCount }} 题 · 难度 {{ p.difficulty || '—' }}</span>
        </span>
        <span class="li-arrow">›</span>
      </button>
    </div>
    <div v-if="!papers.length" class="empty">该分类暂无试卷</div>
    <button class="btn btn-ghost btn-block" style="margin-top: 14px" @click="router.back()">返回题库</button>
  </template>
</template>

<style scoped>
.vue-paper-list { display: grid; gap: 8px; margin-top: 12px; }
.vue-paper-item { width: 100%; text-align: left; border: 1px solid var(--border); background: var(--card); }
.vue-paper-item .li-main { display: flex; flex-direction: column; align-items: flex-start; }
</style>
