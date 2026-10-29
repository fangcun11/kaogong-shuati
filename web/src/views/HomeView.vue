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

const orderedSubjects = computed(() => {
  const rank = (name: string) => {
    const i = SUBJECT_ORDER.indexOf(name)
    return i < 0 ? SUBJECT_ORDER.length : i
  }
  return [...subjects.value].sort((a, b) => rank(a.subjectName) - rank(b.subjectName))
})

const hasData = computed(() => !!stats.value && stats.value.total > 0)
const hasSubjects = computed(() => orderedSubjects.value.length > 0)
const continueSubject = computed(() => {
  const started = orderedSubjects.value.filter((s) => (s.done || 0) > 0)
  return started.sort((a, b) => b.done - a.done)[0] || orderedSubjects.value[0]
})
const activeSubject = computed(() => continueSubject.value?.subjectName || '')
const customTotal = computed(() => batches.value.reduce((sum, b) => sum + b.count, 0))
const customTitle = computed(() => (batches.value.length ? `${batches.value.length} 个批次` : '导入你的题库'))
const customSub = computed(() => (batches.value.length ? `${format(customTotal.value)} 题` : 'PDF / Excel / Word'))
const dateLine = computed(() => {
  const d = new Date()
  const week = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'][d.getDay()]
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日 · ${week}`
})
const headLead = computed(() => {
  if (!hasSubjects.value) return '先导入一份题库，就可以开始练习了'
  if (!hasData.value) return '从一个科目开始，或者导入自己的资料'
  return `已练 ${format(stats.value?.total)} 题 · 正确率 ${stats.value?.rate || 0}%`
})

function format(n: number | undefined | null) {
  return Number(n || 0).toLocaleString()
}

function percent(s: Subject) {
  if (!s.questions) return 0
  return Math.round(((s.done || 0) / s.questions) * 100)
}

function metaLine(s: Subject) {
  const done = s.done || 0
  return `${s.papers} 套试卷 · ${format(s.questions)} 题 · ${done ? `已做 ${format(done)}` : '未开始'}`
}

function tabLabel(name: string) {
  return name.replace(/^(公务员|事业编)·/, '')
}

function openSubject(name: string) {
  router.push({ name: 'subject', params: { name } })
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
    <div v-else class="content">
      <header class="page-head">
        <div>
          <h1>练习</h1>
          <p>{{ headLead }}</p>
        </div>
        <span class="date">{{ dateLine }}</span>
      </header>

      <nav v-if="hasSubjects" class="tabs" aria-label="科目">
        <button
          v-for="s in orderedSubjects"
          :key="s.subjectName"
          class="tab"
          :class="{ active: activeSubject === s.subjectName }"
          @click="openSubject(s.subjectName)"
        >
          {{ tabLabel(s.subjectName) }}
        </button>
        <button class="tab tab-more" @click="openQuick('custom')">自定义题库</button>
      </nav>

      <button class="banner" @click="openQuick('papers')">
        <span class="banner-copy">
          <span class="kicker">智能组卷</span>
          <strong>{{ hasData ? '今天做 15 道' : '先来 15 道热身' }}</strong>
          <span class="banner-sub">{{ hasData ? `当前正确率 ${stats?.rate || 0}%，按弱项出题` : '自己定科目和题量，随时开练' }}</span>
          <span class="banner-cta">去组卷 <Icon name="arrow" :size="12" :stroke="2" /></span>
        </span>
        <span class="banner-art" aria-hidden="true">
          <i class="orbit o1" />
          <i class="orbit o2" />
          <i class="dot d1" />
          <i class="dot d2" />
          <b class="art-num">15</b>
          <span class="art-unit">题</span>
        </span>
      </button>

      <nav class="quick" aria-label="快捷入口">
        <button class="quick-item" @click="openQuick('wrong')">
          <span class="quick-ico">
            <Icon name="book" :size="19" :stroke="1.7" />
            <span v-if="(stats?.wrong || 0) > 0" class="badge">{{ (stats?.wrong || 0) > 99 ? '99+' : stats?.wrong }}</span>
          </span>
          <span>错题本</span>
        </button>
        <button class="quick-item" @click="openQuick('favorites')">
          <span class="quick-ico"><Icon name="star" :size="19" :stroke="1.7" /></span>
          <span>收藏</span>
        </button>
        <button class="quick-item" @click="openQuick('notes')">
          <span class="quick-ico"><Icon name="note" :size="19" :stroke="1.7" /></span>
          <span>笔记</span>
        </button>
        <button class="quick-item" @click="openQuick('stats')">
          <span class="quick-ico"><Icon name="chart" :size="19" :stroke="1.7" /></span>
          <span>学习统计</span>
        </button>
      </nav>

      <div class="promos">
        <button class="promo promo-custom" @click="openQuick('custom')">
          <span class="promo-kicker">自定义题库</span>
          <strong>{{ customTitle }}</strong>
          <span class="promo-sub">{{ customSub }}</span>
          <Icon class="promo-ico" name="database" :size="34" :stroke="1.3" />
        </button>
        <button class="promo promo-stats" @click="openQuick('stats')">
          <span class="promo-kicker">学习概览</span>
          <strong>{{ hasData ? `${stats?.rate || 0}%` : '暂无记录' }}</strong>
          <span class="promo-sub">{{ hasData ? `累计 ${format(stats?.total)} 题 · 待订正 ${format(stats?.wrong)}` : '做几道题就有数据了' }}</span>
          <Icon class="promo-ico" name="chart" :size="34" :stroke="1.3" />
        </button>
      </div>

      <section class="bank">
        <header class="bank-head">
          <h2>题库</h2>
          <span v-if="hasSubjects">{{ orderedSubjects.length }} 个科目</span>
        </header>
        <div class="rows">
          <button
            v-for="s in orderedSubjects"
            :key="s.subjectName"
            class="subject-row"
            :class="{ active: activeSubject === s.subjectName }"
            @click="openSubject(s.subjectName)"
          >
            <span class="row-ico">
              <Icon :name="(s.done || 0) > 0 ? 'book' : 'target'" :size="16" :stroke="1.7" />
            </span>
            <span class="row-body">
              <span class="row-top">
                <span class="row-name">{{ s.subjectName }}</span>
                <span v-if="activeSubject === s.subjectName" class="row-go">
                  {{ (s.done || 0) > 0 ? '继续上次' : '开始练习' }}
                  <Icon name="chevronRight" :size="12" :stroke="2.4" />
                </span>
                <span v-else class="row-pct">{{ percent(s) }}%</span>
              </span>
              <span class="row-meta">{{ metaLine(s) }}</span>
              <span class="bar"><i :style="{ width: percent(s) + '%' }" /></span>
            </span>
          </button>
          <div v-if="!hasSubjects" class="bank-empty">
            <p>还没有可练习的题库，先导入一份真题或自己的资料。</p>
            <button class="btn-primary" @click="openQuick('custom')">导入题库</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.home {
  --canvas: #f5f7fb;
  --card: #ffffff;
  --ink: #1b2434;
  --muted: #7b879c;
  --faint: #a7b0c0;
  --line: #edf1f7;
  --primary: #3d7bf7;
  --primary-strong: #2f6bea;
  --primary-soft: #eef4ff;
  --primary-softer: #f7faff;
  --red: #f04438;
  --shadow: 0 2px 10px rgba(23, 43, 77, .05);
  color: var(--ink);
  padding-bottom: 6px;
}

html[data-theme='dark'] .home {
  --canvas: #0f141c;
  --card: #171e29;
  --ink: #eaf0fa;
  --muted: #8e9bb0;
  --faint: #6c788c;
  --line: rgba(255, 255, 255, .07);
  --primary: #5c8dff;
  --primary-strong: #7aa3ff;
  --primary-soft: #1b2a44;
  --primary-softer: #152034;
  --red: #ff6b5e;
  --shadow: none;
}

@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) .home {
    --canvas: #0f141c;
    --card: #171e29;
    --ink: #eaf0fa;
    --muted: #8e9bb0;
    --faint: #6c788c;
    --line: rgba(255, 255, 255, .07);
    --primary: #5c8dff;
    --primary-strong: #7aa3ff;
    --primary-soft: #1b2a44;
    --primary-softer: #152034;
    --red: #ff6b5e;
    --shadow: none;
  }
}

/* ---------- 页头 ---------- */
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 14px;
}
.page-head h1 {
  margin: 0;
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -.02em;
  color: var(--ink);
}
.page-head p {
  margin: 7px 0 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--muted);
  font-variant-numeric: tabular-nums;
}
.date {
  flex: none;
  padding-bottom: 3px;
  font-size: 11.5px;
  color: var(--faint);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

/* ---------- 科目 tab ---------- */
.tabs {
  display: flex;
  gap: 24px;
  margin-top: 18px;
  border-bottom: 1px solid var(--line);
  overflow-x: auto;
  scrollbar-width: none;
}
.tabs::-webkit-scrollbar { display: none; }
.tab {
  position: relative;
  flex: none;
  padding: 2px 0 11px;
  border: 0;
  background: none;
  font-size: 14px;
  color: var(--muted);
  white-space: nowrap;
  cursor: pointer;
  transition: color .15s ease;
}
.tab:hover { color: var(--ink); }
.tab.active {
  color: var(--ink);
  font-weight: 600;
}
.tab.active::after {
  content: '';
  position: absolute;
  left: 50%;
  bottom: -1px;
  width: 22px;
  height: 3px;
  border-radius: 2px;
  background: var(--primary);
  transform: translateX(-50%);
}
.tab-more { color: var(--faint); font-size: 13px; }
.tab-more::after { display: none; }

/* ---------- 组卷 banner ---------- */
.banner {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  margin-top: 16px;
  padding: 18px 12px 18px 20px;
  border: 0;
  border-radius: 18px;
  background: linear-gradient(120deg, #4e8dff 0%, #2f6bea 100%);
  color: #fff;
  text-align: left;
  cursor: pointer;
  overflow: hidden;
  box-shadow: 0 12px 26px -16px rgba(47, 107, 234, .7);
}
.banner::before {
  content: '';
  position: absolute;
  right: -46px;
  top: -72px;
  width: 210px;
  height: 210px;
  border-radius: 50%;
  background: rgba(255, 255, 255, .1);
}
.banner::after {
  content: '';
  position: absolute;
  left: -30px;
  bottom: -92px;
  width: 180px;
  height: 180px;
  border-radius: 50%;
  background: rgba(255, 255, 255, .07);
}
.banner-copy {
  position: relative;
  flex: 1;
  min-width: 0;
}
.kicker {
  font-size: 11px;
  letter-spacing: .16em;
  opacity: .86;
}
.banner strong {
  display: block;
  margin-top: 7px;
  font-size: 19px;
  font-weight: 600;
  letter-spacing: .01em;
}
.banner-sub {
  display: block;
  margin-top: 6px;
  font-size: 11.5px;
  line-height: 1.5;
  opacity: .82;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.banner-cta {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 13px;
  padding: 6px 12px;
  border-radius: 999px;
  background: #fff;
  color: var(--primary-strong);
  font-size: 11.5px;
  font-weight: 600;
}
.banner-art {
  position: relative;
  flex: none;
  width: 116px;
  height: 110px;
  margin-right: 2px;
}
.orbit {
  position: absolute;
  border: 1px solid rgba(255, 255, 255, .36);
  border-radius: 50%;
}
.o1 { inset: 4px; }
.o2 { inset: 21px; border-style: dashed; opacity: .8; }
.dot {
  position: absolute;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(255, 255, 255, .85);
}
.d1 { top: 12px; right: 14px; }
.d2 { bottom: 16px; left: 10px; width: 4px; height: 4px; opacity: .7; }
.art-num {
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  transform: translateY(-54%);
  text-align: center;
  font-size: 42px;
  font-weight: 600;
  letter-spacing: -.03em;
  line-height: 1;
}
.art-unit {
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  margin-top: 22px;
  text-align: center;
  font-size: 10.5px;
  letter-spacing: .3em;
  text-indent: .3em;
  opacity: .85;
}

/* ---------- 快捷入口 ---------- */
.quick {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  margin-top: 14px;
  padding: 14px 2px 12px;
  border-radius: 16px;
  background: var(--card);
  box-shadow: var(--shadow);
}
.quick-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: none;
  color: var(--muted);
  font-size: 11.5px;
  cursor: pointer;
}
.quick-ico {
  position: relative;
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  border-radius: 13px;
  background: var(--primary-soft);
  color: var(--primary);
  transition: transform .15s ease;
}
.quick-item:active .quick-ico { transform: scale(.94); }
.badge {
  position: absolute;
  top: -5px;
  right: -9px;
  min-width: 17px;
  height: 17px;
  padding: 0 4px;
  border-radius: 9px;
  background: var(--red);
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  line-height: 17px;
  text-align: center;
  font-variant-numeric: tabular-nums;
}

/* ---------- 两张推广卡 ---------- */
.promos {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-top: 10px;
}
.promo {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  min-height: 96px;
  padding: 14px;
  border: 0;
  border-radius: 16px;
  text-align: left;
  overflow: hidden;
  cursor: pointer;
}
.promo-custom {
  background: linear-gradient(125deg, #fff9ec 0%, #ffedc9 100%);
  color: #7a4b12;
}
.promo-stats {
  background: linear-gradient(125deg, #eef5ff 0%, #dbe9ff 100%);
  color: #274a86;
}
.promo-kicker {
  font-size: 10.5px;
  letter-spacing: .1em;
  opacity: .66;
}
.promo strong {
  font-size: 15px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.promo-sub {
  max-width: 100%;
  font-size: 11px;
  opacity: .72;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-variant-numeric: tabular-nums;
}
.promo-ico {
  position: absolute;
  right: -6px;
  bottom: -8px;
  opacity: .18;
  pointer-events: none;
}
html[data-theme='dark'] .promo-custom {
  background: linear-gradient(125deg, #2a2317 0%, #3a2e19 100%);
  color: #f0c98a;
}
html[data-theme='dark'] .promo-stats {
  background: linear-gradient(125deg, #16233a 0%, #1b2f50 100%);
  color: #a9c5f5;
}
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) .promo-custom {
    background: linear-gradient(125deg, #2a2317 0%, #3a2e19 100%);
    color: #f0c98a;
  }
  html:not([data-theme='light']) .promo-stats {
    background: linear-gradient(125deg, #16233a 0%, #1b2f50 100%);
    color: #a9c5f5;
  }
}

/* ---------- 题库列表 ---------- */
.bank {
  margin-top: 20px;
  padding: 2px 14px 6px;
  border-radius: 18px;
  background: var(--card);
  box-shadow: var(--shadow);
}
.bank-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding: 13px 2px 9px;
}
.bank-head h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: .01em;
}
.bank-head span {
  font-size: 11px;
  color: var(--faint);
  font-variant-numeric: tabular-nums;
}
.rows { display: flex; flex-direction: column; }
.subject-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  padding: 13px 2px;
  border: 0;
  border-top: 1px solid var(--line);
  background: none;
  text-align: left;
  color: var(--ink);
  cursor: pointer;
}
.row-ico {
  flex: none;
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border-radius: 12px;
  background: var(--primary-soft);
  color: var(--primary);
}
.subject-row.active .row-ico {
  background: var(--primary);
  color: #fff;
}
.row-body { flex: 1; min-width: 0; }
.row-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.row-name {
  font-size: 14px;
  font-weight: 600;
  letter-spacing: .01em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.row-go {
  display: inline-flex;
  align-items: center;
  gap: 1px;
  flex: none;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--primary);
}
.row-pct {
  flex: none;
  font-size: 11.5px;
  color: var(--muted);
  font-variant-numeric: tabular-nums;
}
.row-meta {
  display: block;
  margin-top: 4px;
  font-size: 11.5px;
  line-height: 1.5;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-variant-numeric: tabular-nums;
}
.bar {
  display: block;
  height: 4px;
  margin-top: 9px;
  border-radius: 2px;
  background: var(--line);
  overflow: hidden;
}
.bar i {
  display: block;
  height: 100%;
  border-radius: 2px;
  background: var(--primary);
  transition: width .3s ease;
}
.bank-empty {
  padding: 24px 6px 28px;
  text-align: center;
}
.bank-empty p {
  margin: 0 0 14px;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--muted);
}
.btn-primary {
  padding: 9px 22px;
  border: 0;
  border-radius: 999px;
  background: var(--primary);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.btn-primary:hover { background: var(--primary-strong); }

/* ---------- 桌面两栏 ---------- */
@media (min-width: 900px) {
  .content {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 304px;
    column-gap: 36px;
  }
  .page-head,
  .tabs,
  .banner,
  .quick,
  .bank { grid-column: 1; }
  .promos {
    grid-column: 2;
    grid-row: 1 / span 5;
    align-self: start;
    grid-template-columns: 1fr;
    margin-top: 0;
    gap: 14px;
  }
  .promo { min-height: 128px; padding: 18px; }
  .promo strong { font-size: 17px; }
  .promo-kicker { font-size: 11px; }
  .promo-sub { font-size: 11.5px; }
  .promo-ico { right: -4px; bottom: -10px; opacity: .22; }

  .page-head h1 { font-size: 32px; }
  .page-head p { font-size: 13px; margin-top: 9px; }
  .date { font-size: 12px; padding-bottom: 5px; }
  .tabs { margin-top: 24px; gap: 30px; }
  .tab { font-size: 14.5px; padding-bottom: 13px; }
  .banner { margin-top: 22px; padding: 24px 24px 24px 28px; border-radius: 22px; }
  .banner strong { font-size: 24px; margin-top: 9px; }
  .banner-sub { font-size: 12.5px; margin-top: 8px; }
  .banner-cta { margin-top: 18px; padding: 7px 15px; font-size: 12px; }
  .banner-art { width: 152px; height: 146px; }
  .art-num { font-size: 58px; }
  .art-unit { margin-top: 31px; font-size: 11.5px; }
  .o2 { inset: 27px; }
  .quick { margin-top: 18px; padding: 18px 6px 16px; }
  .quick-ico { width: 46px; height: 46px; border-radius: 14px; }
  .quick-item { font-size: 12px; gap: 9px; }
  .bank { margin-top: 24px; padding: 4px 18px 8px; }
  .bank-head { padding: 15px 2px 10px; }
  .bank-head h2 { font-size: 16px; }
  .subject-row { padding: 15px 2px; gap: 14px; }
  .row-ico { width: 42px; height: 42px; }
  .row-name { font-size: 14.5px; }
  .row-meta { font-size: 12px; }
}
</style>

<style>
body.home-surface {
  background: #f5f7fb;
}
body.home-surface #topbar,
body.home-surface #bottom-nav {
  background: #fff;
  border-color: #edf1f7;
}
body.home-surface #topbar h1 {
  text-align: left;
  padding-left: 2px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: .04em;
  color: #1b2434;
}
body.home-surface #topbar .spacer { display: none; }
body.home-surface #bottom-nav .nav-item.active {
  color: #3d7bf7;
}
html[data-theme='dark'] body.home-surface {
  background: #0f141c;
}
html[data-theme='dark'] body.home-surface #topbar,
html[data-theme='dark'] body.home-surface #bottom-nav {
  background: #171e29;
  border-color: rgba(255, 255, 255, .07);
}
html[data-theme='dark'] body.home-surface #topbar h1 {
  color: #eaf0fa;
}
html[data-theme='dark'] body.home-surface #bottom-nav .nav-item.active {
  color: #5c8dff;
}
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) body.home-surface {
    background: #0f141c;
  }
  html:not([data-theme='light']) body.home-surface #topbar,
  html:not([data-theme='light']) body.home-surface #bottom-nav {
    background: #171e29;
    border-color: rgba(255, 255, 255, .07);
  }
  html:not([data-theme='light']) body.home-surface #topbar h1 {
    color: #eaf0fa;
  }
  html:not([data-theme='light']) body.home-surface #bottom-nav .nav-item.active {
    color: #5c8dff;
  }
}
</style>
