<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, post } from '../api/client'
import Icon from '../components/Icon.vue'

interface Question {
  id?: number
  questionId: number
  paperId: number
  chapter: string
  type: number
  content: string
  contentHtml?: string
  options: string[]
  answer: string
  answerIndex: number
  difficulty?: number
  analysis?: string
  material?: string
  materialHtml?: string
  module?: string
}

interface CheckResult {
  ok: boolean | null
  correct: number[]
  selected: number[]
  correctText: string[]
}

interface AnswerState {
  selected: number[]
  check?: CheckResult
  costMs: number
}

const route = useRoute()
const router = useRouter()
const subject = computed(() => String(route.params.subject || ''))
const category = computed(() => (route.params.category ? String(route.params.category) : ''))
const paperId = computed(() => (typeof route.query.paperId === 'string' ? route.query.paperId : ''))
const generatedMode = computed(() => route.query.generated === '1')
const questions = ref<Question[]>([])
const loading = ref(true)
const error = ref('')
const index = ref(0)
const answers = reactive<Record<string, AnswerState>>({})
const favoriteIds = ref(new Set<number>())
const noteOpen = ref(false)
const noteText = ref('')
const noteSaved = ref(false)
const saving = ref(false)
const startedAt = ref(Date.now())
const finished = ref(false)

const current = computed(() => questions.value[index.value])
const currentKey = computed(() => (current.value ? String(current.value.id ?? current.value.questionId) : ''))
const currentAnswer = computed(() => (current.value ? answers[currentKey.value] : undefined))
const submitted = computed(() => !!currentAnswer.value?.check)
const isMulti = computed(() => current.value?.type === 2)
const progress = computed(() => (questions.value.length ? Math.round(((index.value + 1) / questions.value.length) * 100) : 0))
const answeredCount = computed(() => Object.keys(answers).filter((key) => !!answers[key].check).length)
const isFavorite = computed(() => !!current.value && favoriteIds.value.has(Number(current.value.id ?? current.value.questionId)))

function optionText(text: string): string {
  return String(text || '').replace(/^\s*[A-Ha-h][.、)]\s*/, '')
}

function optionKey(i: number): string {
  return String.fromCharCode(65 + i)
}

function answerKey() {
  const q = current.value
  if (!q) return ''
  return String(q.id ?? q.questionId)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    if (generatedMode.value) {
      const raw = sessionStorage.getItem('kg_generated_paper')
      if (!raw) throw new Error('组卷结果已失效，请重新生成')
      const paper = JSON.parse(raw) as { title: string; questions: Question[] }
      questions.value = paper.questions || []
      document.title = `${paper.title} · 刷题`
    } else if (paperId.value) {
      const paper = await api<{ name: string; questions: Question[] }>(`/api/papers/${encodeURIComponent(paperId.value)}`)
      questions.value = paper.questions || []
      document.title = `${paper.name} · 刷题`
    } else {
      const params = new URLSearchParams({ subject: subject.value, n: '15' })
      if (category.value) params.set('group', category.value)
      questions.value = await api<Question[]>(`/api/practice?${params.toString()}`)
      document.title = `${category.value || subject.value} · 刷题`
    }
    index.value = 0
    finished.value = false
    noteOpen.value = false
    startedAt.value = Date.now()
    if (questions.value.length) {
      await loadFavorites()
      await loadNote()
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载题目失败'
  } finally {
    loading.value = false
  }
}

async function loadFavorites() {
  try {
    const data = await api<{ list: Array<{ questionId: number }> }>('/api/favorites?limit=200')
    favoriteIds.value = new Set((data.list || []).map((x) => Number(x.questionId)))
  } catch {
    favoriteIds.value = new Set()
  }
}

async function loadNote() {
  noteSaved.value = false
  noteText.value = ''
  if (!current.value) return
  try {
    const data = await api<{ list: Array<{ note: string }> }>(`/api/notes?qid=${encodeURIComponent(currentKey.value)}`)
    if (data.list?.[0]) {
      noteText.value = data.list[0].note
      noteSaved.value = true
    }
  } catch {
    /* optional */
  }
}

function selectOption(i: number) {
  if (!current.value || submitted.value) return
  const key = answerKey()
  const state = answers[key] || (answers[key] = { selected: [], costMs: 0 })
  if (isMulti.value) {
    state.selected = state.selected.includes(i) ? state.selected.filter((x) => x !== i) : [...state.selected, i].sort((a, b) => a - b)
  } else {
    state.selected = [i]
  }
}

async function submitAnswer() {
  const q = current.value
  const key = answerKey()
  if (!q || submitted.value || saving.value) return
  const state = answers[key]
  if (!state || state.selected.length === 0) return
  saving.value = true
  state.costMs = Date.now() - startedAt.value
  try {
    const check = await post<CheckResult>('/api/check', {
      questionId: q.id ?? q.questionId,
      selected: state.selected,
    })
    state.check = check
    await post('/api/records', {
      questionId: q.id ?? q.questionId,
      subject: subject.value,
      chapter: q.chapter,
      type: q.type,
      selected: state.selected,
      correct: check.ok,
      costMs: state.costMs,
    }).catch(() => {})
  } catch (e) {
    error.value = e instanceof Error ? e.message : '判分失败'
  } finally {
    saving.value = false
  }
}

function optionClass(i: number) {
  const state = currentAnswer.value
  if (!state?.check) return { selected: state?.selected.includes(i) }
  return {
    selected: state.selected.includes(i),
    correct: state.check.correct.includes(i),
    wrong: state.check.ok === false && state.selected.includes(i) && !state.check.correct.includes(i),
    locked: true,
  }
}

function goNext() {
  if (index.value < questions.value.length - 1) {
    index.value++
    startedAt.value = Date.now()
    noteOpen.value = false
    loadNote()
  } else {
    finished.value = true
  }
}

function goPrev() {
  if (index.value > 0) {
    index.value--
    startedAt.value = Date.now()
    noteOpen.value = false
    loadNote()
  }
}

async function toggleFavorite() {
  const q = current.value
  if (!q) return
  const id = Number(q.id ?? q.questionId)
  const wasFav = favoriteIds.value.has(id)
  try {
    await api('/api/favorites', {
      method: wasFav ? 'DELETE' : 'POST',
      body: JSON.stringify({ questionId: q.id ?? q.questionId, subject: subject.value, chapter: q.chapter }),
    })
    const next = new Set(favoriteIds.value)
    if (wasFav) next.delete(id)
    else next.add(id)
    favoriteIds.value = next
  } catch (e) {
    error.value = e instanceof Error ? e.message : '收藏失败'
  }
}

async function saveNote() {
  const q = current.value
  if (!q) return
  const text = noteText.value.trim()
  if (!text) return
  try {
    await post('/api/notes', {
      questionId: q.id ?? q.questionId,
      subject: subject.value,
      chapter: q.chapter,
      note: text,
    })
    noteSaved.value = true
    noteOpen.value = false
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存笔记失败'
  }
}

async function deleteNote() {
  const q = current.value
  if (!q) return
  try {
    await api('/api/notes', {
      method: 'DELETE',
      body: JSON.stringify({ questionId: q.id ?? q.questionId }),
    })
    noteText.value = ''
    noteSaved.value = false
  } catch (e) {
    error.value = e instanceof Error ? e.message : '删除笔记失败'
  }
}

function restart() {
  finished.value = false
  index.value = 0
  for (const key of Object.keys(answers)) delete answers[key]
  startedAt.value = Date.now()
}

watch(index, () => {
  noteOpen.value = false
  loadNote()
})

onMounted(load)
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error && !questions.length" class="empty">加载失败：{{ error }}</div>
  <div v-else-if="!questions.length" class="empty">该分类暂无题目</div>

  <template v-else>
    <div class="q-progress-bar"><div class="fill" :style="{ width: `${progress}%` }" /></div>
    <div class="q-meta">
      <span class="tag tag-chapter">{{ current?.module || current?.chapter || category || subject }}</span>
      <span class="tag tag-type">{{ isMulti ? '多选' : '单选' }}</span>
      <span class="tag tag-difficulty">难度 {{ current?.difficulty || '—' }}</span>
      <span class="q-actions">
        <button class="q-fav" :class="{ on: isFavorite }" title="收藏" @click="toggleFavorite">
          <Icon name="star" :size="15" />
        </button>
      </span>
      <span class="q-progress-text">{{ index + 1 }} / {{ questions.length }} · 已答 {{ answeredCount }}</span>
    </div>

    <article v-if="!finished" class="q-card">
      <div v-if="current?.material" class="q-material">
        <details open>
          <summary>给定材料</summary>
          <div class="q-content" v-html="current.materialHtml || current.material" />
        </details>
      </div>
      <div class="q-content" v-html="current?.contentHtml || current?.content" />

      <div class="vue-options">
        <button
          v-for="(opt, i) in current?.options || []"
          :key="i"
          class="option"
          :class="optionClass(i)"
          :disabled="submitted"
          @click="selectOption(i)"
        >
          <span class="opt-key">{{ optionKey(i) }}</span>
          <span>{{ optionText(opt) }}</span>
        </button>
      </div>

      <div v-if="submitted" class="result-banner" :class="currentAnswer?.check?.ok === false ? 'no' : 'ok'">
        <b>{{ currentAnswer?.check?.ok === false ? '回答错误' : currentAnswer?.check?.ok === true ? '回答正确' : '已提交' }}</b>
        <span v-if="currentAnswer?.check?.correctText?.length">正确答案：{{ currentAnswer.check.correctText.join('、') }}</span>
      </div>

      <div v-if="submitted && current?.analysis" class="answer-box">
        <div class="ab-title">解析</div>
        <div class="ab-body">{{ current.analysis }}</div>
      </div>

      <div v-if="noteOpen" class="vue-note-panel">
        <textarea v-model="noteText" rows="4" placeholder="记下考点、易错点或解题思路…" />
        <div class="action-row">
          <button class="btn btn-ghost" @click="noteOpen = false">取消</button>
          <button v-if="noteSaved" class="btn btn-ghost" @click="deleteNote">删除笔记</button>
          <button class="btn btn-primary" :disabled="!noteText.trim()" @click="saveNote">保存笔记</button>
        </div>
      </div>

      <div class="action-row vue-quiz-actions">
        <button class="btn btn-ghost" :disabled="index === 0" @click="goPrev">上一题</button>
        <button class="btn btn-ghost" @click="noteOpen = !noteOpen">{{ noteSaved ? '查看笔记' : '添加笔记' }}</button>
        <button v-if="!submitted" class="btn btn-primary" :disabled="!currentAnswer?.selected.length || saving" @click="submitAnswer">
          {{ saving ? '判分中…' : '提交答案' }}
        </button>
        <button v-else class="btn btn-primary" @click="goNext">{{ index === questions.length - 1 ? '完成练习' : '下一题' }}</button>
      </div>
    </article>

    <section v-else class="card vue-finished">
      <div class="vue-finished-icon"><Icon name="trophy" :size="34" /></div>
      <h2>练习完成</h2>
      <p class="muted">共 {{ questions.length }} 题，已答 {{ answeredCount }} 题</p>
      <button class="btn btn-primary btn-block" @click="restart">再来一组</button>
      <button class="btn btn-ghost btn-block" style="margin-top: 8px" @click="router.push({ name: 'subject', params: { name: subject } })">返回题库</button>
    </section>

    <div v-if="error" class="vue-inline-error">{{ error }}</div>
  </template>
</template>

<style scoped>
.vue-options { display: grid; gap: 10px; margin-top: 18px; }
.q-material { margin: 0 0 14px; padding: 12px 14px; border-left: 3px solid var(--accent); background: var(--card-2); border-radius: 10px; }
.q-material summary { cursor: pointer; color: var(--primary); font-weight: 700; font-size: 13px; }
.q-material .q-content { margin-top: 8px; font-size: 14px; line-height: 1.7; }
.result-banner { display: flex; gap: 10px; align-items: center; padding: 12px 14px; border-radius: 12px; margin-top: 16px; font-size: 14px; }
.result-banner.ok { color: var(--green); background: var(--green-soft); }
.result-banner.no { color: var(--red); background: var(--red-soft); }
.answer-box { margin-top: 14px; }
.vue-quiz-actions { margin-top: 18px; }
.vue-note-panel { margin-top: 16px; padding: 12px; border: 1px solid var(--border); border-radius: 12px; background: var(--card-2); }
.vue-note-panel textarea { width: 100%; box-sizing: border-box; border: 1px solid var(--border-strong); border-radius: 10px; padding: 10px; resize: vertical; background: var(--card); color: var(--text); font: inherit; }
.vue-inline-error { margin-top: 12px; color: var(--red); font-size: 13px; }
.vue-finished { text-align: center; padding: 30px 20px; margin-top: 18px; }
.vue-finished-icon { width: 58px; height: 58px; border-radius: 18px; display: grid; place-items: center; margin: 0 auto 12px; background: var(--amber-soft); color: var(--amber); }
</style>
