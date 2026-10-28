<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

interface Batch {
  id: number
  name: string
  subject: string
  count: number
  created_at: string
}

const router = useRouter()
const batches = ref<Batch[]>([])
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await api<{ batches: Batch[] }>('/api/custom/batches')
    batches.value = data.batches || []
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function start(batch: Batch) {
  router.push({
    name: 'practice',
    params: { subject: '自定义' },
    query: { batchId: String(batch.id) },
  })
}

async function rename(batch: Batch) {
  const name = window.prompt('批次名称', batch.name)
  if (!name || !name.trim() || name.trim() === batch.name) return
  try {
    await api(`/api/custom/batch?id=${batch.id}`, {
      method: 'PUT',
      body: JSON.stringify({ name: name.trim(), subject: batch.subject }),
    })
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '改名失败'
  }
}

async function remove(batch: Batch) {
  if (!window.confirm(`删除批次「${batch.name}」及其 ${batch.count} 道题？`)) return
  try {
    await api(`/api/custom/batch?id=${batch.id}`, { method: 'DELETE' })
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '删除失败'
  }
}

function importLegacy() {
  window.location.href = '/?view=custom'
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <template v-else>
    <div class="card vue-custom-head">
      <div>
        <h3>自定义题库</h3>
        <p class="muted">导入自己的题目，按批次刷题</p>
      </div>
      <button class="btn btn-ghost" @click="importLegacy">导入题目</button>
    </div>
    <div class="vue-custom-list">
      <div v-for="b in batches" :key="b.id" class="card vue-custom-item">
        <button class="vue-custom-main" @click="start(b)">
          <span class="vue-custom-icon">▣</span>
          <span class="li-main">
            <b>{{ b.name }}</b>
            <small>{{ b.subject }} · {{ b.count }} 题 · {{ b.created_at }}</small>
          </span>
          <span class="li-arrow">›</span>
        </button>
        <div class="vue-custom-actions">
          <button @click="rename(b)">改名</button>
          <button class="danger" @click="remove(b)">删除</button>
        </div>
      </div>
    </div>
    <div v-if="!batches.length" class="empty">还没有自定义题库，请先到旧版导入题目</div>
  </template>
</template>

<style scoped>
.vue-custom-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 16px; }
.vue-custom-head h3 { margin: 0 0 3px; }
.vue-custom-list { display: grid; gap: 10px; margin-top: 12px; }
.vue-custom-item { padding: 0; overflow: hidden; }
.vue-custom-main { display: flex; align-items: center; gap: 12px; width: 100%; padding: 14px; border: 0; background: none; color: var(--text); text-align: left; cursor: pointer; }
.vue-custom-icon { width: 38px; height: 38px; border-radius: 11px; display: grid; place-items: center; background: var(--primary-soft); color: var(--primary); font-size: 20px; flex: 0 0 auto; }
.vue-custom-main .li-main { display: flex; flex-direction: column; align-items: flex-start; min-width: 0; flex: 1; }
.vue-custom-main b { font-size: 14px; }
.vue-custom-main small { margin-top: 3px; color: var(--text-3); font-size: 11.5px; }
.vue-custom-actions { display: flex; justify-content: flex-end; gap: 14px; padding: 0 14px 12px; }
.vue-custom-actions button { border: 0; background: none; color: var(--text-2); font-size: 12px; cursor: pointer; }
.vue-custom-actions button.danger { color: var(--red); }
</style>
