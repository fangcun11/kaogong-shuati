<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'

interface Question {
  id: number | string
  questionId: number | string
  type?: number | string
  chapter: string
  subject?: string
  content: string
  contentHtml?: string
  material?: string
  materialHtml?: string
  options: string[]
  answer: string
  answerIndex: number
  analysis?: string
  difficulty?: number
}

const route = useRoute()
const router = useRouter()
const question = ref<Question | null>(null)
const loading = ref(true)
const error = ref('')

function optionText(text: string) {
  return String(text || '').replace(/^\s*[A-Ha-h][.、)]\s*/, '')
}

const isCustom = computed(() => question.value?.type === 'custom')

onMounted(async () => {
  try {
    const id = String(route.params.id || '')
    question.value = await api<Question>(`/api/question?id=${encodeURIComponent(id)}`)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载题目失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">{{ error }}</div>
  <template v-else-if="question">
    <div class="q-meta">
      <span class="tag tag-chapter">{{ question.chapter || '题目' }}</span>
      <span v-if="isCustom" class="tag tag-type">自定义题</span>
      <span v-else class="tag tag-type">难度 {{ question.difficulty || '—' }}</span>
    </div>
    <article class="q-card">
      <div v-if="question.material" class="q-material">
        <details open>
          <summary>给定材料</summary>
          <div class="q-content" v-html="question.materialHtml || question.material" />
        </details>
      </div>
      <div class="q-content" v-html="question.contentHtml || question.content" />
      <div class="vue-detail-options">
        <div v-for="(opt, i) in question.options || []" :key="i" class="option">
          <span class="opt-key">{{ String.fromCharCode(65 + i) }}</span>
          <span>{{ optionText(opt) }}</span>
        </div>
      </div>
      <div class="answer-box" style="margin-top: 16px">
        <div class="ab-title">答案</div>
        <div class="ab-body">{{ question.answer || '暂无答案' }}</div>
      </div>
      <div v-if="question.analysis" class="answer-box" style="margin-top: 10px">
        <div class="ab-title">解析</div>
        <div class="ab-body">{{ question.analysis }}</div>
      </div>
    </article>
    <button class="btn btn-ghost btn-block" @click="router.back()">返回</button>
  </template>
</template>

<style scoped>
.vue-detail-options { display: grid; gap: 10px; margin-top: 18px; }
.q-material { margin: 0 0 16px; padding: 12px 14px; border-left: 3px solid var(--accent); background: var(--card-2); border-radius: 10px; }
.q-material summary { cursor: pointer; color: var(--primary); font-weight: 700; font-size: 13px; }
.q-material .q-content { margin-top: 8px; font-size: 14px; line-height: 1.7; }
</style>
