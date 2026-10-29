<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { api } from '../api/client'
import Icon from '../components/Icon.vue'

interface ChapterStat { chapter: string; c: number; ok: number }
interface Stats {
  total: number
  correct: number
  wrong: number
  rate: number
  byChapter: ChapterStat[]
}
interface ActivitySubject { name: string; c: number; ok: number; graded: number }
interface ActivityDay { d: string; c: number; ok: number; graded: number; ms: number; subjects: ActivitySubject[] }
interface Activity {
  from: string
  to: string
  startedOn: string
  activeDays: number
  allActiveDays: number
  currentStreak: number
  longestStreak: number
  total: number
  correct: number
  graded: number
  ms: number
  days: ActivityDay[]
  subjects: ActivitySubject[]
}
interface Cell { date: string; count: number; level: number; future: boolean }
interface WeekColumn { cells: Cell[] }

const WEEK_LABELS = ['日', '一', '二', '三', '四', '五', '六']
const WEEK_SHOWN = [false, true, false, true, false, true, false]
const WEEKS = 53

const stats = ref<Stats | null>(null)
const activity = ref<Activity | null>(null)
const loading = ref(true)
const error = ref('')
const selected = ref('')
const scroller = ref<HTMLElement | null>(null)

const today = startOfToday()
const thisSunday = addDays(today, -today.getDay())
const gridStart = addDays(thisSunday, -(WEEKS - 1) * 7)
const todayKey = ymd(today)

const dayMap = computed(() => {
  const map = new Map<string, ActivityDay>()
  for (const day of activity.value?.days || []) map.set(day.d, day)
  return map
})

const thresholds = computed(() => {
  const counts = (activity.value?.days || [])
    .map((d) => d.c)
    .filter((c) => c > 0)
    .sort((a, b) => a - b)
  if (!counts.length) return [1, 1, 1, 1]
  const at = (p: number) => counts[Math.min(counts.length - 1, Math.max(0, Math.floor(p * (counts.length - 1))))]
  return [at(0.25), at(0.5), at(0.75), counts[counts.length - 1]]
})

function levelOf(count: number) {
  if (count <= 0) return 0
  const t = thresholds.value
  if (count <= t[0]) return 1
  if (count <= t[1]) return 2
  if (count <= t[2]) return 3
  return 4
}

const weeks = computed<WeekColumn[]>(() => {
  const out: WeekColumn[] = []
  for (let w = 0; w < WEEKS; w++) {
    const cells: Cell[] = []
    for (let j = 0; j < 7; j++) {
      const date = addDays(gridStart, w * 7 + j)
      const key = ymd(date)
      const entry = dayMap.value.get(key)
      const count = entry?.c || 0
      cells.push({
        date: key,
        count,
        level: levelOf(count),
        future: date.getTime() > today.getTime(),
      })
    }
    out.push({ cells })
  }
  return out
})

const months = computed(() => {
  const labels: { col: number; label: string }[] = []
  let previous = -1
  weeks.value.forEach((week, col) => {
    const month = parseYmd(week.cells[0].date).getMonth()
    if (month !== previous) {
      const last = labels[labels.length - 1]
      // Skip the clipped first month, and never stack labels closer than 3 weeks.
      if (col > 0 && (!last || col - last.col >= 3)) {
        labels.push({ col, label: `${month + 1}月` })
      }
      previous = month
    }
  })
  return labels
})

const selectedDay = computed(() => (selected.value ? dayMap.value.get(selected.value) : undefined))
const selectedLabel = computed(() => dayLabel(selected.value || todayKey))
const canPrev = computed(() => !!selected.value && selected.value > ymd(gridStart))
const canNext = computed(() => !!selected.value && selected.value < todayKey)
const isToday = computed(() => selected.value === todayKey)

const subjectRows = computed(() => activity.value?.subjects || [])
const weakChapters = computed(() =>
  [...(stats.value?.byChapter || [])]
    .filter((c) => c.c >= 5 && rateOf(c.ok, c.c) < 60)
    .sort((a, b) => rateOf(a.ok, a.c) - rateOf(b.ok, b.c))
    .slice(0, 6),
)

const rangeText = computed(() => {
  const a = parseYmd(ymd(gridStart))
  const b = today
  return `${a.getFullYear()} 年 ${a.getMonth() + 1} 月 – ${b.getFullYear()} 年 ${b.getMonth() + 1} 月`
})

const subtitle = computed(() => {
  if (!stats.value || !stats.value.total) return '从第一题开始，慢慢积累。'
  return `${format(stats.value.total)} 道题，落在 ${activity.value?.allActiveDays || 0} 个日子里。`
})

const todayText = computed(() => {
  const week = WEEK_LABELS[today.getDay()]
  return `${today.getMonth() + 1} 月 ${today.getDate()} 日 · 周${week}`
})

function startOfToday() {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  return d
}

function addDays(date: Date, amount: number) {
  const next = new Date(date)
  next.setDate(next.getDate() + amount)
  return next
}

function ymd(date: Date) {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

function parseYmd(raw: string) {
  const [y, m, d] = raw.split('-').map(Number)
  return new Date(y, (m || 1) - 1, d || 1)
}

function dayLabel(raw: string) {
  const d = parseYmd(raw)
  const week = WEEK_LABELS[d.getDay()]
  if (raw === todayKey) return `今天 · ${d.getMonth() + 1} 月 ${d.getDate()} 日`
  if (raw === ymd(addDays(today, -1))) return `昨天 · ${d.getMonth() + 1} 月 ${d.getDate()} 日`
  return `${d.getMonth() + 1} 月 ${d.getDate()} 日 · 周${week}`
}

function cellTitle(cell: Cell) {
  if (cell.future) return ''
  return cell.count ? `${cell.date} · ${cell.count} 题` : `${cell.date} · 没有练习`
}

function shift(delta: number) {
  const base = parseYmd(selected.value || todayKey)
  const next = addDays(base, delta)
  const key = ymd(next)
  if (key < ymd(gridStart) || key > todayKey) return
  selected.value = key
}

function format(value: number | undefined | null) {
  return Number(value || 0).toLocaleString('zh-CN')
}

function rateOf(ok: number, graded: number) {
  if (!graded) return 0
  return Math.round((ok * 100) / graded)
}

function tone(rate: number) {
  if (rate >= 80) return 'good'
  if (rate < 60) return 'low'
  return ''
}

function duration(ms: number) {
  if (!ms) return ''
  const minutes = Math.round(ms / 60000)
  if (minutes < 1) return '不到 1 分钟'
  if (minutes < 60) return `${minutes} 分钟`
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest ? `${hours} 小时 ${rest} 分` : `${hours} 小时`
}

function defaultSelected(data: Activity) {
  if (data.days.some((d) => d.d === todayKey)) return todayKey
  const latest = [...data.days].reverse().find((d) => d.c > 0)
  return latest?.d || todayKey
}

onMounted(async () => {
  try {
    const [statsData, activityData] = await Promise.all([
      api<Stats>('/api/records/stats'),
      api<Activity>('/api/records/activity'),
    ])
    stats.value = statsData
    activity.value = activityData
    selected.value = defaultSelected(activityData)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载统计失败'
  } finally {
    loading.value = false
  }
  // The wall only exists after loading flips, so scroll once it is rendered.
  await nextTick()
  if (scroller.value) scroller.value.scrollLeft = scroller.value.scrollWidth
})
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <div v-else class="stats">
    <header class="head">
      <div>
        <h1>学习足迹</h1>
        <p>{{ subtitle }}</p>
      </div>
      <span class="today">{{ todayText }}</span>
    </header>

    <template v-if="stats?.total">
      <section class="metrics">
        <div class="metric">
          <b>{{ format(stats?.total) }}</b>
          <span>累计做题</span>
        </div>
        <div class="metric">
          <b>{{ stats?.rate || 0 }}<small>%</small></b>
          <span>正确率</span>
        </div>
        <div class="metric hot">
          <b>{{ activity?.currentStreak || 0 }}<small>天</small></b>
          <span>
            连续练习<template v-if="(activity?.longestStreak || 0) > (activity?.currentStreak || 0)">
              · 最长 {{ activity?.longestStreak }} 天</template>
          </span>
        </div>
        <div class="metric">
          <b>{{ activity?.activeDays || 0 }}<small>天</small></b>
          <span>近一年活跃</span>
        </div>
      </section>

      <section class="card wall">
      <header class="wall-head">
        <div>
          <h3>近一年</h3>
          <p>{{ rangeText }}</p>
        </div>
        <div class="wall-sum">
          <b>{{ format(activity?.total) }}</b> 题
          <span>·</span>
          {{ activity?.activeDays || 0 }} 天
        </div>
      </header>

      <div ref="scroller" class="wall-scroll">
        <div class="wall-grid">
          <div class="months">
            <span v-for="m in months" :key="m.col" :style="{ gridColumn: String(m.col + 1) }">{{ m.label }}</span>
          </div>
          <div class="grid">
            <div class="weekdays" aria-hidden="true">
              <span v-for="(label, i) in WEEK_LABELS" :key="i" class="wd">{{ WEEK_SHOWN[i] ? label : '' }}</span>
            </div>
            <div class="weeks">
              <div v-for="(week, wi) in weeks" :key="wi" class="week">
                <button
                  v-for="cell in week.cells"
                  :key="cell.date"
                  type="button"
                  class="cell"
                  :class="[`lvl${cell.level}`, { sel: cell.date === selected, future: cell.future }]"
                  :disabled="cell.future"
                  :title="cellTitle(cell)"
                  :aria-label="cellTitle(cell)"
                  @click="selected = cell.date"
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="legend" aria-hidden="true">
        <span>少</span>
        <i class="lvl0" /><i class="lvl1" /><i class="lvl2" /><i class="lvl3" /><i class="lvl4" />
        <span>多</span>
      </div>

      <div class="day">
        <div class="day-head">
          <div class="day-nav">
            <button class="nav-btn" :disabled="!canPrev" aria-label="前一天" @click="shift(-1)">
              <Icon name="chevronLeft" :size="14" :stroke="2.4" />
            </button>
            <strong>{{ selectedLabel }}</strong>
            <button class="nav-btn" :disabled="!canNext" aria-label="后一天" @click="shift(1)">
              <Icon name="chevronRight" :size="14" :stroke="2.4" />
            </button>
            <button v-if="!isToday" class="today-btn" type="button" @click="selected = todayKey">回到今天</button>
          </div>
          <p v-if="selectedDay" class="day-meta">
            {{ selectedDay.c }} 题<template v-if="selectedDay.graded"> · 正确率 {{ rateOf(selectedDay.ok, selectedDay.graded) }}%</template><template
              v-if="selectedDay.ms"
            > · {{ duration(selectedDay.ms) }}</template>
          </p>
          <p v-else class="day-meta">没有练习</p>
        </div>

        <div v-if="selectedDay && selectedDay.subjects.length" class="day-subjects">
          <div v-for="s in selectedDay.subjects" :key="s.name" class="day-subject">
            <div class="ds-top">
              <span class="ds-name"><i class="dot" />{{ s.name }}</span>
              <b>{{ rateOf(s.ok, s.graded) }}%</b>
            </div>
            <div class="ds-meta">{{ s.c }} 题 · 对 {{ s.ok }}</div>
            <span class="bar"><i :style="{ width: rateOf(s.ok, s.graded) + '%' }" :class="tone(rateOf(s.ok, s.graded))" /></span>
          </div>
        </div>
        <p v-else class="day-empty">这一天休息了，没有做题记录。</p>
      </div>
    </section>

    <div class="pairs">
      <section class="card">
        <header class="card-head">
          <h3>科目表现</h3>
          <span>近一年</span>
        </header>
        <div v-if="subjectRows.length" class="rows">
          <div v-for="s in subjectRows" :key="s.name" class="row">
            <div class="row-top">
              <span class="name">{{ s.name }}</span>
              <b>{{ rateOf(s.ok, s.graded) }}%</b>
            </div>
            <div class="row-meta">{{ format(s.c) }} 题 · 对 {{ format(s.ok) }}</div>
            <span class="bar"><i :style="{ width: rateOf(s.ok, s.graded) + '%' }" :class="tone(rateOf(s.ok, s.graded))" /></span>
          </div>
        </div>
        <p v-else class="hint">近一年还没有练习记录。</p>
      </section>

      <section class="card">
        <header class="card-head">
          <h3>薄弱章节</h3>
          <span>正确率低于 60%</span>
        </header>
        <div v-if="weakChapters.length" class="rows">
          <div v-for="c in weakChapters" :key="c.chapter" class="row">
            <div class="row-top">
              <span class="name">{{ c.chapter || '未分类' }}</span>
              <b>{{ rateOf(c.ok, c.c) }}%</b>
            </div>
            <div class="row-meta">{{ format(c.c) }} 题 · 对 {{ format(c.ok) }}</div>
            <span class="bar"><i :style="{ width: rateOf(c.ok, c.c) + '%' }" class="low" /></span>
          </div>
        </div>
        <p v-else class="hint">暂时没有明显薄弱的章节。</p>
      </section>
    </div>
    </template>

    <section v-else class="card empty-start">
      <span class="empty-ico"><Icon name="chart" :size="26" :stroke="1.6" /></span>
      <h2>从第一道题开始</h2>
      <p>每天的练习会点亮一格。连续天数、科目正确率和薄弱章节，都会在这里慢慢长出来。</p>
      <RouterLink class="go" to="/papers">去组卷</RouterLink>
    </section>
  </div>
</template>

<style scoped>
.stats {
  --wall-cell: 10px;
  --wall-gap: 3px;
  padding-bottom: 8px;
}

/* ---------- 标题 ---------- */
.head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}
.head h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
  letter-spacing: -.02em;
  color: var(--text);
}
.head p {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-2);
  font-variant-numeric: tabular-nums;
}
.head .today {
  flex: none;
  padding-bottom: 3px;
  font-size: 11px;
  color: var(--text-3);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

/* ---------- 指标条 ---------- */
.metrics {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1px;
  margin-bottom: 14px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--border);
  overflow: hidden;
}
.metric {
  padding: 13px 15px;
  background: var(--card);
}
.metric b {
  display: block;
  font-size: 23px;
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: -.02em;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}
.metric small {
  margin-left: 2px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}
.metric span {
  display: block;
  margin-top: 5px;
  font-size: 11px;
  color: var(--text-3);
}
.metric.hot b { color: var(--primary); }

/* ---------- 空状态 ---------- */
.empty-start {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 34px 26px 28px;
  text-align: center;
}
.empty-ico {
  display: grid;
  place-items: center;
  width: 52px;
  height: 52px;
  border-radius: 16px;
  background: var(--primary-soft);
  color: var(--primary);
}
.empty-start h2 {
  margin: 14px 0 0;
  font-size: 16px;
  font-weight: 700;
  color: var(--text);
}
.empty-start p {
  margin: 8px 0 18px;
  max-width: 300px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-2);
}
.empty-start .go {
  padding: 9px 24px;
  border-radius: 999px;
  background: var(--primary);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
}

/* ---------- 足迹墙 ---------- */
.wall { padding: 16px 14px 15px; }
.wall-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}
.wall-head h3 { margin: 0; font-size: 15px; font-weight: 700; color: var(--text); }
.wall-head p {
  margin: 4px 0 0;
  font-size: 10.5px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.wall-sum {
  flex: none;
  padding-top: 1px;
  font-size: 11px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.wall-sum b { font-size: 14px; color: var(--text); font-weight: 700; }
.wall-sum span { margin: 0 3px; }

.wall-scroll {
  margin-top: 12px;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
  scrollbar-color: var(--border) transparent;
  -webkit-overflow-scrolling: touch;
}
.wall-scroll::-webkit-scrollbar { height: 4px; }
.wall-scroll::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

.wall-grid { width: max-content; }
.months {
  display: grid;
  grid-template-columns: repeat(53, calc(var(--wall-cell) + var(--wall-gap)));
  margin-left: calc(16px + var(--wall-gap));
  height: 15px;
}
.months span {
  grid-row: 1;
  font-size: 9px;
  line-height: 15px;
  color: var(--text-3);
  white-space: nowrap;
}
.grid { display: flex; }
.weekdays {
  display: grid;
  grid-template-rows: repeat(7, var(--wall-cell));
  gap: var(--wall-gap);
  width: 16px;
  margin-right: var(--wall-gap);
}
.wd {
  font-size: 8.5px;
  line-height: var(--wall-cell);
  text-align: right;
  color: var(--text-3);
}
.weeks { display: flex; gap: var(--wall-gap); }
.week {
  display: grid;
  grid-template-rows: repeat(7, var(--wall-cell));
  gap: var(--wall-gap);
}
.cell {
  display: block;
  width: var(--wall-cell);
  height: var(--wall-cell);
  padding: 0;
  border: 0;
  border-radius: 2.5px;
  cursor: pointer;
  touch-action: manipulation;
}
.cell:disabled { cursor: default; }
.cell.future { opacity: .32; }
.cell.sel {
  position: relative;
  z-index: 1;
  outline: 1.5px solid var(--text);
  outline-offset: 1px;
}
.cell.lvl0, .legend .lvl0 { background: #e8ecf3; }
.cell.lvl1, .legend .lvl1 { background: #cfe0ff; }
.cell.lvl2, .legend .lvl2 { background: #9fc0ff; }
.cell.lvl3, .legend .lvl3 { background: #5f93f5; }
.cell.lvl4, .legend .lvl4 { background: #2f6bea; }
[data-theme='dark'] .cell.lvl0, [data-theme='dark'] .legend .lvl0 { background: #262b34; }
[data-theme='dark'] .cell.lvl1, [data-theme='dark'] .legend .lvl1 { background: #14345e; }
[data-theme='dark'] .cell.lvl2, [data-theme='dark'] .legend .lvl2 { background: #1d4c8e; }
[data-theme='dark'] .cell.lvl3, [data-theme='dark'] .legend .lvl3 { background: #2f6fce; }
[data-theme='dark'] .cell.lvl4, [data-theme='dark'] .legend .lvl4 { background: #5f9bff; }
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) .cell.lvl0, html:not([data-theme='light']) .legend .lvl0 { background: #262b34; }
  html:not([data-theme='light']) .cell.lvl1, html:not([data-theme='light']) .legend .lvl1 { background: #14345e; }
  html:not([data-theme='light']) .cell.lvl2, html:not([data-theme='light']) .legend .lvl2 { background: #1d4c8e; }
  html:not([data-theme='light']) .cell.lvl3, html:not([data-theme='light']) .legend .lvl3 { background: #2f6fce; }
  html:not([data-theme='light']) .cell.lvl4, html:not([data-theme='light']) .legend .lvl4 { background: #5f9bff; }
}

.legend {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  margin-top: 12px;
  font-size: 9.5px;
  color: var(--text-3);
}
.legend i { width: 10px; height: 10px; border-radius: 2.5px; }
.legend span { margin: 0 2px; }

/* ---------- 选中日 ---------- */
.day {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--border);
}
.day-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px 10px;
}
.day-nav { display: flex; align-items: center; gap: 6px; min-width: 0; }
.day-nav strong {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
}
.nav-btn {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card);
  color: var(--text-2);
  cursor: pointer;
}
.nav-btn:disabled { opacity: .35; cursor: default; }
.today-btn {
  margin-left: 2px;
  padding: 3px 8px;
  border: 0;
  border-radius: 999px;
  background: none;
  color: var(--primary);
  font-size: 10.5px;
  font-weight: 600;
  cursor: pointer;
}
.today-btn:hover { background: var(--primary-soft); }
.day-meta {
  margin: 0;
  font-size: 11.5px;
  color: var(--text-2);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.day-subjects {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
  margin-top: 12px;
}
.day-subject {
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--card-2);
}
.ds-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.ds-name {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 12px;
  color: var(--text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ds-name .dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
}
.ds-top b {
  flex: none;
  font-size: 12.5px;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}
.ds-meta {
  margin-top: 3px;
  font-size: 10.5px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.day-empty {
  margin: 12px 0 0;
  font-size: 11.5px;
  color: var(--text-3);
}

.bar {
  display: block;
  height: 5px;
  margin-top: 8px;
  border-radius: 999px;
  background: var(--bg-soft);
  overflow: hidden;
}
.bar i {
  display: block;
  height: 100%;
  min-width: 2px;
  border-radius: inherit;
  background: var(--primary);
}
.bar i.good { background: var(--green); }
.bar i.low { background: var(--amber); }

/* ---------- 科目 / 章节 ---------- */
.pairs {
  display: grid;
  grid-template-columns: 1fr;
  gap: 14px;
}
.pairs .card { margin-bottom: 0; }
.card-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
}
.card-head h3 { margin: 0; font-size: 14px; font-weight: 700; color: var(--text); }
.card-head span { font-size: 10.5px; color: var(--text-3); }
.rows { display: flex; flex-direction: column; margin-top: 4px; }
.row { padding: 10px 0; border-top: 1px solid var(--border); }
.row:first-child { border-top: 0; padding-top: 10px; }
.row-top {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
}
.row-top .name {
  min-width: 0;
  font-size: 12.5px;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.row-top b { flex: none; font-size: 12.5px; color: var(--text); font-variant-numeric: tabular-nums; }
.row-meta {
  margin-top: 2px;
  font-size: 10.5px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.hint {
  margin: 12px 0 4px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-3);
}

@media (min-width: 560px) {
  .day-subjects { grid-template-columns: 1fr 1fr; }
}

@media (min-width: 900px) {
  .stats { --wall-cell: 12px; --wall-gap: 3px; }
  .head h1 { font-size: 28px; }
  .head p { font-size: 13px; margin-top: 8px; }
  .metrics { grid-template-columns: repeat(4, 1fr); margin-bottom: 16px; }
  .metric { padding: 16px 20px; }
  .metric b { font-size: 27px; }
  .metric span { font-size: 11.5px; }
  .wall { padding: 20px; }
  .wall-head h3 { font-size: 16px; }
  .wall-head p { font-size: 11px; }
  .wall-sum { font-size: 12px; }
  .wall-sum b { font-size: 15px; }
  .months { height: 17px; }
  .months span { font-size: 9.5px; line-height: 17px; }
  .wd { font-size: 9px; }
  .day-subjects { grid-template-columns: 1fr 1fr; gap: 10px; }
  .day-nav strong { font-size: 14.5px; }
  .day-meta { font-size: 12px; }
  .pairs { grid-template-columns: 1fr 1fr; gap: 16px; }
  .card-head h3 { font-size: 15px; }
  .row-top .name { font-size: 13px; }
  .row-top b { font-size: 13px; }
  .row-meta { font-size: 11px; }
}

@media (min-width: 1000px) {
  .stats { --wall-cell: 13px; --wall-gap: 3.5px; }
}
</style>
