<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface FavoriteItem {
  questionId: number | string
  subject: string
  chapter: string
  time: string
  content: string
  type: number | string
}

const router = useRouter()
const items = ref<FavoriteItem[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  try {
    const data = await api<{ list: FavoriteItem[]; total: number }>('/api/favorites?limit=200')
    items.value = data.list || []
    total.value = data.total || 0
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载收藏失败'
  } finally {
    loading.value = false
  }
}

function open(item: FavoriteItem) {
  router.push({ name: 'question', params: { id: String(item.questionId) } })
}

async function remove(item: FavoriteItem) {
  try {
    await api('/api/favorites', {
      method: 'DELETE',
      body: JSON.stringify({ questionId: item.questionId }),
    })
    items.value = items.value.filter((x) => String(x.questionId) !== String(item.questionId))
    total.value = Math.max(0, total.value - 1)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '取消收藏失败'
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <template v-else>
    <div class="card vue-list-summary">
      <h3>收藏</h3>
      <p class="muted">共 {{ total }} 道收藏题</p>
    </div>
    <div class="vue-record-list">
      <div v-for="item in items" :key="String(item.questionId)" class="list-item vue-record-item">
        <button class="vue-record-main" @click="open(item)">
          <span class="li-icon tint-amber">★</span>
          <span class="li-main">
            <span class="li-title">{{ item.content || '（题目内容已移除）' }}</span>
            <span class="li-sub">{{ item.subject }} · {{ item.chapter || '未分类' }} · {{ item.time }}</span>
          </span>
        </button>
        <button class="vue-delete-btn" title="取消收藏" @click="remove(item)">×</button>
      </div>
    </div>
    <div v-if="!items.length" class="empty">还没有收藏题目</div>
  </template>
</template>

<style scoped>
.vue-list-summary { margin-bottom: 12px; }
.vue-record-list { display: grid; gap: 8px; }
.vue-record-item { width: 100%; border: 1px solid var(--border); background: var(--card); padding: 0; }
.vue-record-main { display: flex; align-items: center; gap: 12px; flex: 1; min-width: 0; text-align: left; border: 0; background: none; padding: 14px; color: var(--text); cursor: pointer; }
.vue-record-main .li-main { display: flex; flex-direction: column; align-items: flex-start; min-width: 0; }
.vue-record-main .li-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.vue-delete-btn { flex: 0 0 auto; border: 0; background: none; color: var(--text-3); font-size: 22px; padding: 0 14px; cursor: pointer; }
.vue-delete-btn:hover { color: var(--red); }
</style>
