<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, post } from '../api/client'
import Icon from '../components/Icon.vue'
import ResultPanel from '../components/ResultPanel.vue'
import ScratchPad from '../components/ScratchPad.vue'

interface Question {
  id?: number | string
  questionId: number | string
  paperId?: number
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

interface ResultItem {
  index: number
  number: number
  state: 'correct' | 'wrong' | 'todo' | 'answered'
}

const route = useRoute()
const router = useRouter()
const subject = computed(() => String(route.params.subject || ''))
const category = computed(() => (route.params.category ? String(route.params.category) : ''))
const paperId = computed(() => (typeof route.query.paperId === 'string' ? route.query.paperId : ''))
const batchId = computed(() => (typeof route.query.batchId === 'string' ? route.query.batchId : ''))
const generatedMode = computed(() => route.query.generated === '1')
const questions = ref<Question[]>([])
const loading = ref(true)
const error = ref('')
const index = ref(0)
const answers = reactive<Record<string, AnswerState>>({})
const favoriteIds = ref(new Set<string>())
const noteOpen = ref(false)
const noteText = ref('')
const noteSaved = ref(false)
const saving = ref(false)
const startedAt = ref(Date.now())
const finished = ref(false)
const reviewing = ref(false)
const redoMode = ref(false)
const showCard = ref(false)
const reciteMode = ref(false)
const showScratch = ref(false)
const scratchRef = ref<InstanceType<typeof ScratchPad> | null>(null)

const current = computed(() => questions.value[index.value])
const currentKey = computed(() => (current.value ? String(current.value.id ?? current.value.questionId) : ''))
const currentAnswer = computed(() => (current.value ? answers[currentKey.value] : undefined))
const submitted = computed(() => !!currentAnswer.value?.check)
const showResult = computed(() => submitted.value || reciteMode.value)
const isMulti = computed(() => current.value?.type === 2)
const progress = computed(() => (questions.value.length ? Math.round(((index.value + 1) / questions.value.length) * 100) : 0))
const answeredCount = computed(() => Object.keys(answers).filter((key) => !!answers[key].check).length)
const correctCount = computed(() => Object.values(answers).filter((a) => a.check?.ok === true).length)
const wrongCount = computed(() => Object.values(answers).filter((a) => a.check?.ok === false).length)
const accuracy = computed(() => {
  const checked = Object.values(answers).filter((a) => !!a.check).length
  return checked ? Math.round((correctCount.value / checked) * 100) : 0
})
const resultItems = computed<ResultItem[]>(() =>
  questions.value.map((q, i) => ({ index: i, number: i + 1, state: resultState(q) })),
)
const isFavorite = computed(() => !!current.value && favoriteIds.value.has(String(current.value.id ?? current.value.questionId)))

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

function correctIndices(q: Question): number[] {
  if (Number.isFinite(q.answerIndex) && q.answerIndex >= 0) return [q.answerIndex]
  const raw = String(q.answer || '').trim()
  if (raw.startsWith('[')) {
    try {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr)) return arr.map(Number).filter((x) => Number.isFinite(x))
    } catch {
      /* fall through */
    }
  }
  if (/^[A-H]{2,4}$/i.test(raw)) return [...raw.toUpperCase()].map((ch) => ch.charCodeAt(0) - 65)
  if (/^[A-H]$/i.test(raw)) return [raw.toUpperCase().charCodeAt(0) - 65]
  return []
}

function answerText(q: Question): string {
  const indices = correctIndices(q)
  if (indices.length) return indices.map((i) => optionKey(i)).join('、')
  return q.answer || '暂无答案'
}

function resultState(q: Question): ResultItem['state'] {
  const state = answers[String(q.id ?? q.questionId)]
  if (!state?.check) return 'todo'
  if (state.check.ok === true) return 'correct'
  if (state.check.ok === false) return 'wrong'
  return 'answered'
}

function questionState(i: number): string {
  const q = questions.value[i]
  const key = String(q.id ?? q.questionId)
  const state = answers[key]
  if (i === index.value) return 'current'
  if (!state?.check) return 'todo'
  if (state.check.ok === true) return 'correct'
  if (state.check.ok === false) return 'wrong'
  return 'answered'
}

function jumpTo(i: number) {
  index.value = i
  showCard.value = false
  startedAt.value = Date.now()
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
    } else if (batchId.value) {
      const data = await api<{ questions: Question[]; batch: { name: string } }>(
        `/api/custom/practice?batch_id=${encodeURIComponent(batchId.value)}&count=50`,
      )
      questions.value = data.questions || []
      document.title = `${data.batch?.name || '自定义题库'} · 刷题`
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
    reviewing.value = false
    redoMode.value = false
    noteOpen.value = false
    showScratch.value = false
    scratchRef.value?.reset()
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
    const data = await api<{ list: Array<{ questionId: number | string }> }>('/api/favorites?limit=200')
    favoriteIds.value = new Set((data.list || []).map((x) => String(x.questionId)))
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
  if (!current.value || submitted.value || reciteMode.value) return
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
  if (reciteMode.value && !state?.check) {
    return {
      selected: state?.selected.includes(i),
      correct: !!current.value && correctIndices(current.value).includes(i),
    }
  }
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
    showCard.value = false
    reviewing.value = false
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

function review(i: number) {
  index.value = i
  reviewing.value = true
  finished.value = false
  showCard.value = false
  noteOpen.value = false
  loadNote()
}

function backToResult() {
  reviewing.value = false
  showCard.value = false
  noteOpen.value = false
  finished.value = true
}

function redoWrong() {
  const wrong = questions.value.filter((q) => resultState(q) === 'wrong')
  if (!wrong.length) return
  questions.value = wrong
  for (const key of Object.keys(answers)) delete answers[key]
  redoMode.value = true
  reviewing.value = false
  finished.value = false
  index.value = 0
  showCard.value = false
  noteOpen.value = false
  showScratch.value = false
  scratchRef.value?.reset()
  startedAt.value = Date.now()
  loadNote()
}

async function toggleFavorite() {
  const q = current.value
  if (!q) return
  const id = String(q.id ?? q.questionId)
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
  reviewing.value = false
  index.value = 0
  showCard.value = false
  showScratch.value = false
  scratchRef.value?.reset()
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
    <div v-if="!finished" class="q-progress-bar"><div class="fill" :style="{ width: `${progress}%` }" /></div>
    <div v-if="!finished" class="q-meta">
      <span class="tag tag-chapter">{{ current?.module || current?.chapter || category || subject }}</span>
      <span class="tag tag-type">{{ isMulti ? '多选' : '单选' }}</span>
      <span class="tag tag-difficulty">难度 {{ current?.difficulty || '—' }}</span>
      <span class="q-actions">
        <button class="q-card-btn" title="答题卡" @click="showCard = true">▦</button>
        <button class="q-card-btn" :class="{ on: showScratch }" title="草稿纸" @click="showScratch = true">稿</button>
        <button class="q-card-btn" :class="{ on: reciteMode }" title="背题模式" @click="reciteMode = !reciteMode">背</button>
        <button class="q-fav" :class="{ on: isFavorite }" title="收藏" @click="toggleFavorite">
          <Icon name="star" :size="15" />
        </button>
      </span>
      <span class="q-progress-text">{{ index + 1 }} / {{ questions.length }} · 已答 {{ answeredCount }}</span>
    </div>

    <div v-if="reviewing" class="review-bar">
      <button class="review-back" @click="backToResult">← 返回结果</button>
      <span class="review-pos">回看第 {{ index + 1 }} 题</span>
      <span v-if="currentAnswer?.check?.ok === false" class="review-tag no">错</span>
      <span v-else-if="currentAnswer?.check?.ok === true" class="review-tag ok">对</span>
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
          :disabled="submitted || reciteMode"
          @click="selectOption(i)"
        >
          <span class="opt-key">{{ optionKey(i) }}</span>
          <span>{{ optionText(opt) }}</span>
        </button>
      </div>

      <div v-if="showResult" class="result-banner" :class="reciteMode ? 'ok' : currentAnswer?.check?.ok === false ? 'no' : 'ok'">
        <b>{{ reciteMode ? '背题模式' : currentAnswer?.check?.ok === false ? '回答错误' : currentAnswer?.check?.ok === true ? '回答正确' : '已提交' }}</b>
        <span>正确答案：{{ current ? answerText(current) : '—' }}</span>
      </div>

      <div v-if="showResult && current?.analysis" class="answer-box">
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
        <button v-if="!submitted && !reciteMode" class="btn btn-primary" :disabled="!currentAnswer?.selected.length || saving" @click="submitAnswer">
          {{ saving ? '判分中…' : '提交答案' }}
        </button>
        <button v-else class="btn btn-primary" @click="goNext">{{ index === questions.length - 1 ? (reviewing ? '返回结果' : '完成练习') : '下一题' }}</button>
      </div>
    </article>

    <ResultPanel
      v-else
      :total="questions.length"
      :answered="answeredCount"
      :correct="correctCount"
      :wrong="wrongCount"
      :accuracy="accuracy"
      :items="resultItems"
      :redo-mode="redoMode"
      @review="review"
      @redo="redoWrong"
      @restart="restart"
      @wrong-book="router.push({ name: 'wrong' })"
      @home="router.push({ name: 'home' })"
    />

    <div v-if="showCard" class="vue-sheet-overlay" @click.self="showCard = false">
      <div class="vue-sheet">
        <div class="vue-sheet-head">
          <b>答题卡</b>
          <button @click="showCard = false">关闭</button>
        </div>
        <div class="vue-answer-grid">
          <button
            v-for="(q, i) in questions"
            :key="i"
            :class="questionState(i)"
            @click="jumpTo(i)"
          >{{ i + 1 }}</button>
        </div>
        <div class="vue-answer-legend">
          <span><i class="todo" />未答</span>
          <span><i class="current" />当前</span>
          <span><i class="correct" />正确</span>
          <span><i class="wrong" />错误</span>
        </div>
      </div>
    </div>

    <ScratchPad ref="scratchRef" :open="showScratch" @close="showScratch = false" />

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
.review-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 10px;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card-2);
  font-size: 12.5px;
}
.review-back {
  border: 0;
  background: none;
  padding: 0;
  color: var(--primary);
  font: inherit;
  font-weight: 600;
  cursor: pointer;
}
.review-pos { color: var(--text-2); }
.review-tag {
  margin-left: auto;
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
}
.review-tag.ok { color: var(--green); background: var(--green-soft); }
.review-tag.no { color: var(--red); background: var(--red-soft); }
.q-card-btn.on { border-color: var(--primary); color: var(--primary); background: var(--primary-soft); }
.vue-sheet-overlay { position: fixed; inset: 0; z-index: 500; display: flex; align-items: flex-end; justify-content: center; background: rgba(0,0,0,.38); }
.vue-sheet { width: min(560px, 100%); max-height: 80vh; overflow: auto; border-radius: 20px 20px 0 0; background: var(--card); padding: 18px 18px calc(18px + env(safe-area-inset-bottom)); box-shadow: 0 -8px 30px rgba(0,0,0,.18); }
.vue-sheet-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px; }
.vue-sheet-head button { border: 0; background: none; color: var(--primary); font: inherit; font-size: 13px; cursor: pointer; }
.vue-answer-grid { display: grid; grid-template-columns: repeat(8, 1fr); gap: 8px; }
.vue-answer-grid button { aspect-ratio: 1; border: 1px solid var(--border); border-radius: 9px; background: var(--card); color: var(--text-2); font: inherit; font-size: 13px; cursor: pointer; }
.vue-answer-grid button.current { border-color: var(--primary); color: var(--primary); background: var(--primary-soft); font-weight: 800; }
.vue-answer-grid button.correct { border-color: var(--green); color: var(--green); background: var(--green-soft); }
.vue-answer-grid button.wrong { border-color: var(--red); color: var(--red); background: var(--red-soft); }
.vue-answer-grid button.answered { border-color: var(--primary); color: var(--primary); }
.vue-answer-legend { display: flex; flex-wrap: wrap; gap: 14px; margin-top: 16px; color: var(--text-2); font-size: 11px; }
.vue-answer-legend i { display: inline-block; width: 10px; height: 10px; margin-right: 4px; border-radius: 3px; border: 1px solid var(--border); vertical-align: -1px; }
.vue-answer-legend i.current { border-color: var(--primary); background: var(--primary-soft); }
.vue-answer-legend i.correct { border-color: var(--green); background: var(--green-soft); }
.vue-answer-legend i.wrong { border-color: var(--red); background: var(--red-soft); }
</style>
