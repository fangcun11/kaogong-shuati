<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface WrongItem {
  id: number
  questionId: number | string
  subject: string
  chapter: string
  time: string
  content: string
  type: number
}

const router = useRouter()
const items = ref<WrongItem[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')

function open(item: WrongItem) {
  router.push({ name: 'question', params: { id: String(item.questionId) } })
}

onMounted(async () => {
  try {
    const data = await api<{ list: WrongItem[]; total: number }>('/api/records/wrong?limit=50')
    items.value = data.list || []
    total.value = data.total || 0
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载错题失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <template v-else>
    <div class="card vue-list-summary">
      <h3>错题本</h3>
      <p class="muted">共 {{ total }} 道待订正错题</p>
    </div>
    <div class="vue-record-list">
      <button v-for="item in items" :key="item.id" class="list-item vue-record-item" @click="open(item)">
        <span class="li-icon tint-red">错</span>
        <span class="li-main">
          <span class="li-title">{{ item.content || '（题目内容已移除）' }}</span>
          <span class="li-sub">{{ item.subject }} · {{ item.chapter || '未分类' }} · {{ item.time }}</span>
        </span>
        <span class="li-arrow">›</span>
      </button>
    </div>
    <div v-if="!items.length" class="empty">还没有错题，继续刷题吧</div>
  </template>
</template>

<style scoped>
.vue-list-summary { margin-bottom: 12px; }
.vue-record-list { display: grid; gap: 8px; }
.vue-record-item { width: 100%; text-align: left; border: 1px solid var(--border); background: var(--card); }
.vue-record-item .li-main { display: flex; flex-direction: column; align-items: flex-start; min-width: 0; }
.vue-record-item .li-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
</style>
