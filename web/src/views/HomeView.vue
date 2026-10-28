<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import Icon from '../components/Icon.vue'

interface Subject {
  subjectName: string
  papers: number
  questions: number
  done: number
}
interface Stats {
  total: number
  rate: number
  wrong: number
}
interface Batch {
  count: number
}

const router = useRouter()
const subjects = ref<Subject[]>([])
const batches = ref<Batch[]>([])
const stats = ref<Stats | null>(null)
const loading = ref(true)
const error = ref('')

const SUBJECT_ORDER = ['公务员·行测', '公务员·申论', '事业编·职测', '事业编·综应']
const DESC: Record<string, string> = {
  '公务员·行测': '言语 · 判断 · 资料 · 数量 · 常识',
  '公务员·申论': '归纳概括 · 综合分析 · 大作文',
  '事业编·职测': '言语 · 判断 · 资料 · 数量 · 常识',
  '事业编·综应': '归纳概括 · 综合分析 · 大作文',
}

const orderedSubjects = computed(() => {
  const rank = (name: string) => {
    const i = SUBJECT_ORDER.indexOf(name)
    return i < 0 ? SUBJECT_ORDER.length : i
  }
  return [...subjects.value].sort((a, b) => rank(a.subjectName) - rank(b.subjectName))
})

const hasData = computed(() => !!stats.value && stats.value.total > 0)
const continueSubject = computed(() => {
  const started = orderedSubjects.value.filter((s) => (s.done || 0) > 0)
  return started.sort((a, b) => b.done - a.done)[0] || orderedSubjects.value[0]
})
const customTotal = computed(() => batches.value.reduce((sum, b) => sum + b.count, 0))
const hasSubjects = computed(() => orderedSubjects.value.length > 0)
const customLine = computed(() =>
  batches.value.length
    ? `${batches.value.length} 个批次 · ${format(customTotal.value)} 题`
    : '点击导入',
)
const ctaLabel = computed(() => (hasData.value ? '继续练习' : hasSubjects.value ? '开始刷题' : '导入题库'))
const ctaHint = computed(() => (hasSubjects.value ? '按科目挑试卷' : '支持 PDF / Excel / Word'))
const dateLine = computed(() => {
  const d = new Date()
  const week = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'][d.getDay()]
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日 · ${week}`
})
const headTitle = computed(() =>
  hasData.value && continueSubject.value ? `继续，${continueSubject.value.subjectName}` : '从第一题开始。',
)
const headLead = computed(() => {
  const s = continueSubject.value
  if (!s) return '还没有可练习的题库，先导入一份。'
  const done = s.done || 0
  return `${s.papers} 套 · ${format(s.questions)} 题 · ${done ? `已做 ${format(done)}` : '尚未开始'}`
})

function format(n: number | undefined | null) {
  return Number(n || 0).toLocaleString()
}

function percent(s: Subject) {
  if (!s.questions) return 0
  return Math.round(((s.done || 0) / s.questions) * 100)
}

function openSubject(name: string) {
  router.push({ name: 'subject', params: { name } })
}

function startPractice() {
  if (continueSubject.value) openSubject(continueSubject.value.subjectName)
  else router.push({ name: 'custom' })
}

function openQuick(to: string) {
  const routes: Record<string, string> = {
    papers: 'papers',
    wrong: 'wrong',
    favorites: 'favorites',
    notes: 'notes',
    ai: 'ai',
    stats: 'stats',
    custom: 'custom',
  }
  if (routes[to]) router.push({ name: routes[to] })
}

onMounted(async () => {
  document.body.classList.add('home-surface')
  try {
    const [subjectData, statData, batchData] = await Promise.all([
      api<Subject[]>('/api/subjects'),
      api<Stats>('/api/records/stats').catch(() => null),
      api<{ batches: Batch[] }>('/api/custom/batches').catch(() => ({ batches: [] })),
    ])
    subjects.value = subjectData
    stats.value = statData
    batches.value = batchData.batches
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
})

onUnmounted(() => document.body.classList.remove('home-surface'))
</script>

<template>
  <div class="home">
    <div v-if="loading" class="spinner" />
    <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
    <template v-else>
      <section class="head">
        <div class="eyebrow">
          <span>{{ dateLine }}</span>
          <span class="eyebrow-state">{{ hasData ? '学习中' : '新档案' }}</span>
        </div>
        <h1 class="display">{{ headTitle }}</h1>
        <p class="lead">{{ headLead }}</p>
        <div class="actions">
          <button class="btn-ink" @click="startPractice">
            {{ ctaLabel }}
            <Icon name="play" :size="14" :stroke="1.8" />
          </button>
          <span class="hint">{{ ctaHint }}</span>
        </div>
      </section>

      <section class="stats" aria-label="学习概览">
        <h2 class="stats-title">概览</h2>
        <div class="stat">
          <b>{{ format(stats?.total) }}</b>
          <span>累计做题</span>
        </div>
        <div class="stat">
          <b>{{ hasData ? `${stats?.rate || 0}%` : '—' }}</b>
          <span>正确率</span>
        </div>
        <div class="stat">
          <b :class="{ red: (stats?.wrong || 0) > 0 }">{{ format(stats?.wrong) }}</b>
          <span>待订正</span>
        </div>
      </section>

      <section class="list-section">
        <header class="section-head">
          <h2>题库</h2>
          <span>{{ 1 + orderedSubjects.length }} 个来源</span>
        </header>
        <div class="list">
          <button class="row custom" @click="openQuick('custom')">
            <span class="row-ic"><Icon name="database" :size="17" :stroke="1.6" /></span>
            <span class="row-body">
              <span class="row-top"><span class="row-title">自定义题库</span></span>
              <span class="row-meta">{{ customLine }} · PDF / Excel / Word</span>
            </span>
            <Icon class="chev" name="chevronRight" :size="15" :stroke="1.7" />
          </button>
          <button v-for="s in orderedSubjects" :key="s.subjectName" class="row" @click="openSubject(s.subjectName)">
            <span class="row-body">
              <span class="row-top">
                <span class="row-title">{{ s.subjectName }}</span>
                <span class="row-pct">{{ percent(s) }}%</span>
              </span>
              <span class="row-meta">{{ s.papers }} 套 · {{ format(s.questions) }} 题 · {{ s.done ? `已做 ${format(s.done)}` : '未开始' }}</span>
              <span class="track"><i :style="{ width: percent(s) + '%' }" /></span>
            </span>
            <Icon class="chev" name="chevronRight" :size="15" :stroke="1.7" />
          </button>
        </div>
      </section>

      <section class="tools-section">
        <header class="section-head"><h2>工具</h2></header>
        <div class="tools">
          <button class="tool" @click="openQuick('wrong')">
            <Icon name="book" :size="16" :stroke="1.6" />
            <span>错题本</span>
            <Icon class="tool-chev" name="chevronRight" :size="14" :stroke="1.7" />
          </button>
          <button class="tool" @click="openQuick('favorites')">
            <Icon name="star" :size="16" :stroke="1.6" />
            <span>收藏</span>
            <Icon class="tool-chev" name="chevronRight" :size="14" :stroke="1.7" />
          </button>
          <button class="tool" @click="openQuick('notes')">
            <Icon name="note" :size="16" :stroke="1.6" />
            <span>笔记</span>
            <Icon class="tool-chev" name="chevronRight" :size="14" :stroke="1.7" />
          </button>
          <button class="tool" @click="openQuick('stats')">
            <Icon name="chart" :size="16" :stroke="1.6" />
            <span>学习统计</span>
            <Icon class="tool-chev" name="chevronRight" :size="14" :stroke="1.7" />
          </button>
          <button class="tool" @click="openQuick('papers')">
            <Icon name="target" :size="16" :stroke="1.6" />
            <span>智能组卷</span>
            <Icon class="tool-chev" name="chevronRight" :size="14" :stroke="1.7" />
          </button>
          <button class="tool" @click="openQuick('ai')">
            <Icon name="sparkles" :size="16" :stroke="1.6" />
            <span>AI 设置</span>
            <Icon class="tool-chev" name="chevronRight" :size="14" :stroke="1.7" />
          </button>
        </div>
      </section>
    </template>
  </div>
</template>

<style scoped>
.home {
  --paper: #f7f5ef;
  --ink: #1a1916;
  --muted: #6f6a60;
  --faint: #a39c90;
  --line: rgba(26, 25, 22, .11);
  --line-soft: rgba(26, 25, 22, .07);
  --accent: #2e5a49;
  --accent-soft: rgba(46, 90, 73, .09);
  --red: #a8482f;
  --btn: #1c1b18;
  --btn-ink: #f7f5ef;
  --hover: rgba(26, 25, 22, .04);
  color: var(--ink);
  padding-bottom: 4px;
}

html[data-theme='dark'] .home {
  --paper: #131210;
  --ink: #ece8df;
  --muted: #a29b8f;
  --faint: #706a61;
  --line: rgba(236, 232, 223, .12);
  --line-soft: rgba(236, 232, 223, .07);
  --accent: #93b8a7;
  --accent-soft: rgba(147, 184, 167, .12);
  --red: #d3836b;
  --btn: #ece8df;
  --btn-ink: #17150f;
  --hover: rgba(236, 232, 223, .05);
}

@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) .home {
    --paper: #131210;
    --ink: #ece8df;
    --muted: #a29b8f;
    --faint: #706a61;
    --line: rgba(236, 232, 223, .12);
    --line-soft: rgba(236, 232, 223, .07);
    --accent: #93b8a7;
    --accent-soft: rgba(147, 184, 167, .12);
    --red: #d3836b;
    --btn: #ece8df;
    --btn-ink: #17150f;
    --hover: rgba(236, 232, 223, .05);
  }
}

.eyebrow {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  font-size: 11px;
  letter-spacing: .08em;
  color: var(--muted);
}
.eyebrow-state { color: var(--faint); }
.display {
  margin: 10px 0 0;
  font-size: 29px;
  line-height: 1.3;
  font-weight: 600;
  letter-spacing: -.01em;
  color: var(--ink);
}
.lead {
  margin: 8px 0 0;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--muted);
  font-variant-numeric: tabular-nums;
}
.actions { display: flex; align-items: center; gap: 14px; margin-top: 18px; }
.btn-ink {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 18px;
  border: 0;
  border-radius: 10px;
  background: var(--btn);
  color: var(--btn-ink);
  font-size: 13.5px;
  font-weight: 600;
  letter-spacing: .01em;
  cursor: pointer;
  transition: opacity .15s ease;
}
.btn-ink:hover { opacity: .9; }
.btn-ink:active { transform: scale(.98); }
.hint { font-size: 11.5px; color: var(--faint); }

.stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  margin-top: 24px;
  padding: 15px 0;
  border-top: 1px solid var(--line);
  border-bottom: 1px solid var(--line);
}
.stats-title { display: none; }
.stat { padding: 0 16px; min-width: 0; }
.stat:first-of-type { padding-left: 0; }
.stat + .stat { border-left: 1px solid var(--line-soft); }
.stat b {
  display: block;
  font-size: 23px;
  font-weight: 450;
  line-height: 1.1;
  letter-spacing: -.01em;
  font-variant-numeric: tabular-nums;
}
.stat span {
  display: block;
  margin-top: 5px;
  font-size: 10.5px;
  letter-spacing: .04em;
  color: var(--muted);
}
.red { color: var(--red); }

.section-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin: 24px 0 4px;
}
.section-head h2 {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .14em;
  color: var(--ink);
}
.section-head span { font-size: 11px; color: var(--faint); }

.list { display: flex; flex-direction: column; }
.row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  padding: 14px 0;
  border: 0;
  border-bottom: 1px solid var(--line-soft);
  background: transparent;
  text-align: left;
  cursor: pointer;
  color: var(--ink);
  transition: background .15s ease;
}
.row:hover { background: var(--hover); }
.row-body { flex: 1; min-width: 0; }
.row-top { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
.row-title { font-size: 14.5px; font-weight: 600; letter-spacing: .01em; }
.row-pct { font-size: 11px; color: var(--muted); font-variant-numeric: tabular-nums; }
.row-meta {
  display: block;
  margin-top: 4px;
  font-size: 11.5px;
  line-height: 1.5;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.chev { flex: none; color: var(--faint); }
.track { display: block; height: 2px; margin-top: 10px; background: var(--line-soft); overflow: hidden; }
.track i { display: block; height: 100%; background: var(--accent); }
.row.custom { padding: 16px 0; }
.row-ic {
  flex: none;
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border: 1px solid var(--line);
  border-radius: 9px;
  color: var(--muted);
}
.row.custom .row-meta { margin-top: 3px; }

.tools { display: grid; grid-template-columns: repeat(3, 1fr); margin-top: 8px; }
.tool {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 0;
  border: 0;
  background: transparent;
  color: var(--muted);
  font-size: 12px;
  letter-spacing: .02em;
  text-align: left;
  cursor: pointer;
}
.tool :deep(svg) { color: var(--faint); }
.tool:hover { color: var(--ink); }
.tool:hover :deep(svg) { color: var(--ink); }
.tool-chev { display: none; }

@media (min-width: 900px) {
  .home {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 280px;
    grid-template-areas:
      'head stats'
      'list tools';
    column-gap: 72px;
    align-items: start;
  }
  .head { grid-area: head; }
  .stats {
    grid-area: stats;
    display: block;
    margin-top: 2px;
    padding: 0;
    border-top: 1px solid var(--line);
    border-bottom: 0;
  }
  .stats-title {
    display: block;
    margin: 0 0 10px;
    font-size: 12px;
    font-weight: 600;
    letter-spacing: .14em;
    color: var(--ink);
  }
  .stat {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    padding: 12px 0;
    border-bottom: 1px solid var(--line-soft);
  }
  .stat:first-of-type { padding-left: 0; }
  .stat + .stat { border-left: 0; }
  .stat b { display: inline; font-size: 19px; }
  .stat span { order: -1; margin: 0; font-size: 12.5px; }
  .display { margin-top: 14px; font-size: 40px; line-height: 1.24; letter-spacing: -.015em; }
  .lead { margin-top: 12px; font-size: 13px; }
  .actions { margin-top: 24px; }
  .btn-ink { padding: 12px 22px; font-size: 14px; }
  .list-section { grid-area: list; }
  .section-head { margin-top: 40px; }
  .section-head h2 { font-size: 11.5px; }
  .row { padding: 17px 0; gap: 16px; }
  .row-title { font-size: 15px; }
  .row-meta { font-size: 12px; margin-top: 5px; }
  .track { margin-top: 12px; }
  .row.custom { padding: 19px 0; }
  .tools-section { grid-area: tools; }
  .tools-section .section-head { margin-top: 40px; }
  .tools {
    display: block;
    margin-top: 12px;
    border-top: 1px solid var(--line);
  }
  .tool {
    width: 100%;
    padding: 13px 0;
    border-bottom: 1px solid var(--line-soft);
    font-size: 13px;
    color: var(--ink);
  }
  .tool-chev { display: block; margin-left: 8px; color: var(--faint); }
}
</style>

<style>
body.home-surface {
  background: #f7f5ef;
}
html[data-theme='dark'] body.home-surface {
  background: #131210;
}
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) body.home-surface {
    background: #131210;
  }
}
body.home-surface #topbar,
body.home-surface #bottom-nav {
  background: #f7f5ef;
  border-color: rgba(26, 25, 22, .11);
}
html[data-theme='dark'] body.home-surface #topbar,
html[data-theme='dark'] body.home-surface #bottom-nav {
  background: #131210;
  border-color: rgba(236, 232, 223, .12);
}
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) body.home-surface #topbar,
  html:not([data-theme='light']) body.home-surface #bottom-nav {
    background: #131210;
    border-color: rgba(236, 232, 223, .12);
  }
}
body.home-surface #topbar h1 {
  text-align: left;
  padding-left: 2px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .16em;
}
body.home-surface #topbar .spacer { display: none; }
body.home-surface #topbar .icon-btn,
body.home-surface #bottom-nav .nav-item.active {
  color: #2e5a49;
}
html[data-theme='dark'] body.home-surface #topbar .icon-btn,
html[data-theme='dark'] body.home-surface #bottom-nav .nav-item.active {
  color: #93b8a7;
}
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) body.home-surface #topbar .icon-btn,
  html:not([data-theme='light']) body.home-surface #bottom-nav .nav-item.active {
    color: #93b8a7;
  }
}
</style>
