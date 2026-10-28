<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface NoteItem {
  questionId: number | string
  subject: string
  chapter: string
  note: string
  time: string
  content: string
}

const router = useRouter()
const items = ref<NoteItem[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  try {
    const data = await api<{ list: NoteItem[]; total: number }>('/api/notes?limit=200')
    items.value = data.list || []
    total.value = data.total || 0
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载笔记失败'
  } finally {
    loading.value = false
  }
}

function open(item: NoteItem) {
  router.push({ name: 'question', params: { id: String(item.questionId) } })
}

async function remove(item: NoteItem) {
  try {
    await api('/api/notes', {
      method: 'DELETE',
      body: JSON.stringify({ questionId: item.questionId }),
    })
    items.value = items.value.filter((x) => String(x.questionId) !== String(item.questionId))
    total.value = Math.max(0, total.value - 1)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '删除笔记失败'
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <template v-else>
    <div class="card vue-list-summary">
      <h3>笔记</h3>
      <p class="muted">共 {{ total }} 条笔记</p>
    </div>
    <div class="vue-note-list">
      <div v-for="item in items" :key="String(item.questionId)" class="card vue-note-card">
        <div class="vue-note-head">
          <span class="muted">{{ item.subject }} · {{ item.chapter || '未分类' }}</span>
          <button class="vue-note-delete" @click="remove(item)">删除</button>
        </div>
        <button class="vue-note-content" @click="open(item)">{{ item.note }}</button>
        <div class="muted" style="font-size: 12px; margin-top: 8px">{{ item.time }}</div>
      </div>
    </div>
    <div v-if="!items.length" class="empty">还没有笔记</div>
  </template>
</template>

<style scoped>
.vue-list-summary { margin-bottom: 12px; }
.vue-note-list { display: grid; gap: 10px; }
.vue-note-card { padding: 14px; }
.vue-note-head { display: flex; justify-content: space-between; align-items: center; gap: 10px; font-size: 12px; }
.vue-note-delete { border: 0; background: none; color: var(--red); cursor: pointer; font-size: 12px; }
.vue-note-content { display: block; width: 100%; margin-top: 8px; border: 0; background: none; padding: 0; text-align: left; color: var(--text); font-size: 14px; line-height: 1.7; white-space: pre-wrap; cursor: pointer; }
</style>
