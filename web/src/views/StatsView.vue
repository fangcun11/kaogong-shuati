<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
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
interface ActivitySubject { name: string; c: number; ok: number; graded: number; ms: number }
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
interface TrendPoint { date: string; c: number; ok: number; graded: number }
interface DotPoint { date: string; x: number; y: number }
interface DetailRow { d: string; subject: string; c: number; ok: number; graded: number; ms: number }

type Tab = 'footprint' | 'trend'
type WindowSize = 7 | 14 | 30 | 90

const WEEK_LABELS = ['日', '一', '二', '三', '四', '五', '六']
const WEEK_SHOWN = [false, true, false, true, false, true, false]
const WEEKS = 53
const WINDOWS: WindowSize[] = [7, 14, 30, 90]

const stats = ref<Stats | null>(null)
const activity = ref<Activity | null>(null)
const loading = ref(true)
const error = ref('')
const selected = ref('')
const scroller = ref<HTMLElement | null>(null)

const tab = ref<Tab>('footprint')
const trendWindow = ref<WindowSize>(30)
const trendSubject = ref('')
const hoverIndex = ref(-1)

const detailMonth = ref('')
const detailSubject = ref('')
const detailRate = ref('')
const detailKeyword = ref('')

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

/* ---------------- 趋势 ---------------- */

const trendPoints = computed<TrendPoint[]>(() => {
  const window = trendWindow.value
  const start = addDays(today, -(window - 1))
  const subject = trendSubject.value
  const points: TrendPoint[] = []
  for (let i = 0; i < window; i++) {
    const key = ymd(addDays(start, i))
    const day = dayMap.value.get(key)
    let c = 0
    let ok = 0
    let graded = 0
    if (day) {
      if (!subject) {
        c = day.c
        ok = day.ok
        graded = day.graded
      } else {
        const row = day.subjects.find((s) => s.name === subject)
        if (row) {
          c = row.c
          ok = row.ok
          graded = row.graded
        }
      }
    }
    points.push({ date: key, c, ok, graded })
  }
  return points
})

const trendMax = computed(() => trendPoints.value.reduce((max, p) => Math.max(max, p.c), 1))

const trendSummary = computed(() => {
  let c = 0
  let ok = 0
  let graded = 0
  let active = 0
  for (const p of trendPoints.value) {
    c += p.c
    ok += p.ok
    graded += p.graded
    if (p.c > 0) active++
  }
  return {
    c,
    active,
    rate: rateOf(ok, graded),
    avg: Math.round(c / trendWindow.value),
  }
})

const lastActiveIndex = computed(() => {
  const points = trendPoints.value
  for (let i = points.length - 1; i >= 0; i--) {
    if (points[i].c > 0) return i
  }
  return points.length - 1
})

const focusIndex = computed(() => (hoverIndex.value >= 0 ? hoverIndex.value : lastActiveIndex.value))
const focusPoint = computed(() => trendPoints.value[focusIndex.value])
const focusLabel = computed(() => (focusPoint.value ? dayLabel(focusPoint.value.date) : ''))

function barHeight(p: TrendPoint) {
  if (!p.c) return '0%'
  const ratio = p.c / trendMax.value
  return `${Math.max(3, Math.round(ratio * 100))}%`
}

const lineSegments = computed<string[]>(() => {
  const points = trendPoints.value
  const n = points.length
  const segments: string[] = []
  let current: string[] = []
  points.forEach((p, i) => {
    if (!p.graded) {
      if (current.length) {
        segments.push(segmentPath(current))
        current = []
      }
      return
    }
    const x = ((i + 0.5) / n) * 100
    const y = Math.min(99, Math.max(1, 100 - rateOf(p.ok, p.graded)))
    current.push(`${x.toFixed(2)},${y.toFixed(2)}`)
  })
  if (current.length) segments.push(segmentPath(current))
  return segments
})

function segmentPath(points: string[]) {
  if (points.length > 1) return points.join(' ')
  // A lone day still deserves a visible tick instead of an invisible dot path.
  const [x, y] = points[0].split(',').map(Number)
  return `${(x - 0.3).toFixed(2)},${y} ${(x + 0.3).toFixed(2)},${y}`
}

const dots = computed<DotPoint[]>(() => {
  if (trendWindow.value > 30) return []
  const n = trendPoints.value.length
  const out: DotPoint[] = []
  trendPoints.value.forEach((p, i) => {
    if (!p.graded) return
    out.push({
      date: p.date,
      x: ((i + 0.5) / n) * 100,
      y: Math.min(99, Math.max(1, 100 - rateOf(p.ok, p.graded))),
    })
  })
  return out
})

const xTicks = computed(() => {
  const points = trendPoints.value
  const n = points.length
  if (n <= 7) return points.map((p, i) => ({ i, label: shortDate(p.date) }))
  const picks = new Set<number>([0, Math.round((n - 1) / 3), Math.round(((n - 1) * 2) / 3), n - 1])
  return [...picks].sort((a, b) => a - b).map((i) => ({ i, label: shortDate(points[i].date) }))
})

function tickLeft(index: number) {
  return `${((index + 0.5) / trendPoints.value.length) * 100}%`
}

function pointLabel(p: TrendPoint) {
  const accuracy = p.graded ? `，正确率 ${rateOf(p.ok, p.graded)}%` : ''
  return `${dayLabel(p.date)}：${p.c} 题${accuracy}`
}

watch([trendWindow, trendSubject], () => {
  hoverIndex.value = -1
})

/* ---------------- 学习明细 ---------------- */

const detailRows = computed<DetailRow[]>(() => {
  const rows: DetailRow[] = []
  for (const day of activity.value?.days || []) {
    let counted = 0
    let ok = 0
    let graded = 0
    let ms = 0
    for (const s of day.subjects) {
      rows.push({ d: day.d, subject: s.name, c: s.c, ok: s.ok, graded: s.graded, ms: s.ms || 0 })
      counted += s.c
      ok += s.ok
      graded += s.graded
      ms += s.ms || 0
    }
    // Records without a subject still exist; keep them visible in the ledger.
    if (day.c > counted) {
      rows.push({
        d: day.d,
        subject: '未分类',
        c: day.c - counted,
        ok: day.ok - ok,
        graded: day.graded - graded,
        ms: day.ms - ms,
      })
    }
  }
  return rows.sort((a, b) => (a.d === b.d ? b.c - a.c : a.d < b.d ? 1 : -1))
})

const monthOptions = computed(() => {
  const seen = new Set<string>()
  for (const row of detailRows.value) seen.add(row.d.slice(0, 7))
  return [...seen].sort().reverse()
})

const subjectOptions = computed(() => {
  const counts = new Map<string, number>()
  for (const row of detailRows.value) counts.set(row.subject, (counts.get(row.subject) || 0) + row.c)
  return [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([name]) => name)
})

const defaultMonth = ref('')

const hasDetailFilter = computed(
  () =>
    detailMonth.value !== defaultMonth.value ||
    !!detailSubject.value ||
    !!detailRate.value ||
    !!detailKeyword.value.trim(),
)

const filteredRows = computed(() => {
  const keyword = detailKeyword.value.trim().toLowerCase()
  return detailRows.value.filter((row) => {
    if (detailMonth.value && !row.d.startsWith(detailMonth.value)) return false
    if (detailSubject.value && row.subject !== detailSubject.value) return false
    if (detailRate.value) {
      if (!row.graded) return false
      const rate = rateOf(row.ok, row.graded)
      if (detailRate.value === 'low' && rate >= 60) return false
      if (detailRate.value === 'mid' && (rate < 60 || rate >= 80)) return false
      if (detailRate.value === 'high' && rate < 80) return false
    }
    if (keyword) {
      const haystack = `${row.subject} ${row.d} ${monthLabel(row.d.slice(0, 7))}`.toLowerCase()
      if (!haystack.includes(keyword)) return false
    }
    return true
  })
})

const detailTotals = computed(() => {
  let c = 0
  let ms = 0
  let timed = 0
  for (const row of filteredRows.value) {
    c += row.c
    if (row.ms > 0) {
      ms += row.ms
      timed += row.c
    }
  }
  return { c, avg: timed ? Math.round(ms / 1000 / timed) : 0 }
})

function monthLabel(ym: string) {
  const [y, m] = ym.split('-')
  return `${y} 年 ${Number(m)} 月`
}

function clearFilters() {
  detailMonth.value = ''
  detailSubject.value = ''
  detailRate.value = ''
  detailKeyword.value = ''
}

function shortDate(raw: string) {
  const [, m, d] = raw.split('-')
  return `${Number(m)}/${Number(d)}`
}

function durationShort(ms: number) {
  const minutes = Math.round(ms / 60000)
  if (!minutes) return '<1 分'
  if (minutes < 60) return `${minutes} 分`
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest ? `${hours} 时 ${rest} 分` : `${hours} 时`
}

function avgSeconds(row: DetailRow) {
  if (!row.ms || !row.c) return 0
  return Math.round(row.ms / 1000 / row.c)
}

function exportJSON() {
  const payload = {
    exportedAt: new Date().toISOString(),
    range: detailMonth.value ? monthLabel(detailMonth.value) : '全部时间',
    subject: detailSubject.value || '全部科目',
    rows: filteredRows.value.map((row) => ({
      date: row.d,
      subject: row.subject,
      count: row.c,
      correct: row.ok,
      graded: row.graded,
      accuracy: row.graded ? rateOf(row.ok, row.graded) : null,
      ms: row.ms,
    })),
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `kaogong-detail-${detailMonth.value || 'all'}.json`
  link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/* ---------------- 足迹 ---------------- */

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
    detailMonth.value = monthOptions.value[0] || ''
    defaultMonth.value = detailMonth.value
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

      <div class="tabs" role="tablist" aria-label="统计视图">
        <button
          type="button"
          role="tab"
          :aria-selected="tab === 'footprint'"
          :class="{ on: tab === 'footprint' }"
          @click="tab = 'footprint'"
        >
          足迹
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="tab === 'trend'"
          :class="{ on: tab === 'trend' }"
          @click="tab = 'trend'"
        >
          趋势与明细
        </button>
      </div>

      <div v-show="tab === 'footprint'" class="tab-panel">
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
      </div>

      <div v-show="tab === 'trend'" class="tab-panel">
        <section class="card chart-card">
          <header class="chart-head">
            <div>
              <h3>正确率与练习题量</h3>
              <p>
                近 {{ trendWindow }} 天<template v-if="trendSubject"> · {{ trendSubject }}</template>
                · 单日最多 {{ trendMax }} 题
              </p>
            </div>
            <div class="windows" role="group" aria-label="时间范围">
              <button
                v-for="n in WINDOWS"
                :key="n"
                type="button"
                :class="{ on: trendWindow === n }"
                @click="trendWindow = n"
              >
                {{ n }}天
              </button>
            </div>
          </header>

          <div class="chips-scroll" role="group" aria-label="科目筛选">
            <button type="button" class="chip" :class="{ on: !trendSubject }" @click="trendSubject = ''">全部</button>
            <button
              v-for="s in subjectRows"
              :key="s.name"
              type="button"
              class="chip"
              :class="{ on: trendSubject === s.name }"
              @click="trendSubject = s.name"
            >
              {{ s.name }}
            </button>
          </div>

          <div class="readout">
            <strong>{{ focusLabel }}</strong>
            <span v-if="focusPoint && focusPoint.c">
              {{ focusPoint.c }} 题<template v-if="focusPoint.graded"> · 正确率 {{ rateOf(focusPoint.ok, focusPoint.graded) }}%</template>
            </span>
            <span v-else>没有练习</span>
          </div>

          <div class="chart-frame">
            <div class="chart" @mouseleave="hoverIndex = -1">
              <div class="grid-lines" aria-hidden="true"><i /><i /><i /></div>
              <div class="cols">
                <button
                  v-for="(p, i) in trendPoints"
                  :key="p.date"
                  type="button"
                  class="col"
                  :class="{ on: i === focusIndex }"
                  :aria-label="pointLabel(p)"
                  @mouseenter="hoverIndex = i"
                  @focus="hoverIndex = i"
                  @click="hoverIndex = i"
                >
                  <span class="cbar" :style="{ height: barHeight(p) }" />
                </button>
              </div>
              <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
                <polyline v-for="(segment, si) in lineSegments" :key="si" :points="segment" />
              </svg>
              <span
                v-for="dot in dots"
                :key="dot.date"
                class="dot"
                :style="{ left: dot.x + '%', top: dot.y + '%' }"
                aria-hidden="true"
              />
            </div>
            <div class="x-labels" aria-hidden="true">
              <span
                v-for="(t, idx) in xTicks"
                :key="t.i"
                :class="{ first: idx === 0, last: idx === xTicks.length - 1 }"
                :style="{ left: tickLeft(t.i) }"
              >{{ t.label }}</span>
            </div>
            <div class="y-labels" aria-hidden="true"><span>100%</span><span>50%</span><span>0</span></div>
          </div>

          <div class="chart-foot">
            <div class="legend-line">
              <span><i class="sw-bar" />题量</span>
              <span><i class="sw-line" />正确率</span>
            </div>
            <span class="sum-line">
              共 {{ format(trendSummary.c) }} 题 · 正确率 {{ trendSummary.rate }}% · 练习 {{ trendSummary.active }} 天 · 日均 {{ trendSummary.avg }} 题
            </span>
          </div>
        </section>

        <section class="card detail">
          <header class="detail-head">
            <div>
              <h3>学习明细</h3>
              <p>按日期和科目汇总</p>
            </div>
            <button type="button" class="export" @click="exportJSON">导出 JSON</button>
          </header>

          <div class="filters">
            <select v-model="detailMonth" aria-label="按月份筛选">
              <option value="">全部月份</option>
              <option v-for="m in monthOptions" :key="m" :value="m">{{ monthLabel(m) }}</option>
            </select>
            <select v-model="detailSubject" aria-label="按科目筛选">
              <option value="">全部科目</option>
              <option v-for="s in subjectOptions" :key="s" :value="s">{{ s }}</option>
            </select>
            <select v-model="detailRate" aria-label="按正确率筛选">
              <option value="">全部正确率</option>
              <option value="low">低于 60%</option>
              <option value="mid">60% – 79%</option>
              <option value="high">80% 以上</option>
            </select>
            <input v-model="detailKeyword" type="search" placeholder="搜索科目或日期" aria-label="搜索科目或日期" />
            <button v-if="hasDetailFilter" type="button" class="clear" @click="clearFilters">清除筛选</button>
          </div>

          <div class="ledger">
            <div v-for="row in filteredRows" :key="row.d + row.subject" class="ledger-row">
              <div class="lr-top">
                <span class="lr-date">{{ shortDate(row.d) }}</span>
                <span class="lr-subj" :title="row.subject">{{ row.subject }}</span>
                <b v-if="row.graded" class="lr-rate" :class="tone(rateOf(row.ok, row.graded))">{{ rateOf(row.ok, row.graded) }}%</b>
                <span v-else class="lr-rate dim">—</span>
              </div>
              <div class="lr-meta">
                {{ row.c }} 题 · 对 {{ row.ok }}<template v-if="row.ms"> · {{ durationShort(row.ms) }} · {{ avgSeconds(row) }} 秒/题</template>
              </div>
            </div>
            <p v-if="!filteredRows.length" class="none">没有符合条件的学习记录。</p>
          </div>

          <div class="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>日期</th>
                  <th>科目</th>
                  <th class="num">练习题</th>
                  <th class="num">答对</th>
                  <th class="num">正确率</th>
                  <th class="num">时长</th>
                  <th class="num">均耗时</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in filteredRows" :key="row.d + row.subject">
                  <td class="date">{{ shortDate(row.d) }}</td>
                  <td class="subj" :title="row.subject">{{ row.subject }}</td>
                  <td class="num">{{ format(row.c) }}</td>
                  <td class="num">{{ format(row.ok) }}</td>
                  <td class="num">
                    <b v-if="row.graded" :class="tone(rateOf(row.ok, row.graded))">{{ rateOf(row.ok, row.graded) }}%</b>
                    <span v-else class="dim">—</span>
                  </td>
                  <td class="num">{{ row.ms ? durationShort(row.ms) : '—' }}</td>
                  <td class="num">{{ row.ms ? avgSeconds(row) + ' 秒' : '—' }}</td>
                </tr>
                <tr v-if="!filteredRows.length">
                  <td colspan="7" class="none">没有符合条件的学习记录。</td>
                </tr>
              </tbody>
            </table>
          </div>

          <footer class="detail-foot">
            共 {{ filteredRows.length }} 条 · {{ format(detailTotals.c) }} 题<template v-if="detailTotals.avg">
              · 平均 {{ detailTotals.avg }} 秒/题</template>
          </footer>
          <p class="foot-note">时长与均耗时只统计有计时数据的记录；备注和逐条编辑需要按题记录，暂未提供。</p>
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
  --chart-bar: #d7e4fb;
  --chart-bar-hi: #5f93f5;
  --chart-line: #2f6bea;
  --chart-grid: rgba(17, 24, 39, .07);
  --chart-guide: rgba(47, 107, 234, .06);
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

/* ---------- 页签 ---------- */
.tabs {
  display: flex;
  gap: 3px;
  width: 100%;
  max-width: 360px;
  margin-bottom: 14px;
  padding: 3px;
  border-radius: var(--radius-sm);
  background: var(--bg-soft);
}
.tabs button {
  flex: 1;
  padding: 7px 10px;
  border: 0;
  border-radius: calc(var(--radius-sm) - 3px);
  background: none;
  color: var(--text-2);
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color .15s ease, color .15s ease;
}
.tabs button.on {
  background: var(--card);
  color: var(--text);
  font-weight: 700;
  box-shadow: 0 1px 3px rgba(0, 0, 0, .08);
}
/* In dark mode --card matches the track, so lift the active pill one step. */
[data-theme='dark'] .tabs button.on { background: var(--card-2); }
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) .tabs button.on { background: var(--card-2); }
}

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

/* ---------- 趋势 ---------- */
.chart-card {
  --chart-h: 150px;
  padding: 16px 14px 14px;
}
.chart-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}
.chart-head h3 { margin: 0; font-size: 15px; font-weight: 700; color: var(--text); }
.chart-head p {
  margin: 4px 0 0;
  font-size: 10.5px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.windows {
  display: flex;
  flex: none;
  gap: 2px;
  padding: 2px;
  border-radius: 9px;
  background: var(--bg-soft);
}
.windows button {
  padding: 4px 6px;
  border: 0;
  border-radius: 7px;
  background: none;
  color: var(--text-2);
  font-size: 10.5px;
  cursor: pointer;
  font-variant-numeric: tabular-nums;
}
.windows button.on {
  background: var(--card);
  color: var(--text);
  font-weight: 700;
  box-shadow: 0 1px 2px rgba(0, 0, 0, .08);
}
[data-theme='dark'] .windows button.on { background: var(--card-2); }
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) .windows button.on { background: var(--card-2); }
}

.chips-scroll {
  display: flex;
  gap: 6px;
  margin-top: 12px;
  padding-bottom: 2px;
  overflow-x: auto;
  scrollbar-width: none;
}
.chips-scroll::-webkit-scrollbar { display: none; }
.chip {
  flex: none;
  padding: 5px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--card);
  color: var(--text-2);
  font-size: 11px;
  white-space: nowrap;
  cursor: pointer;
}
.chip.on {
  border-color: transparent;
  background: var(--primary-soft);
  color: var(--primary);
  font-weight: 600;
}

.readout {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-height: 22px;
  margin-top: 14px;
  font-variant-numeric: tabular-nums;
}
.readout strong { font-size: 13px; font-weight: 700; color: var(--text); }
.readout span { font-size: 11.5px; color: var(--text-2); }

.chart-frame {
  position: relative;
  margin-top: 4px;
  padding-right: 30px;
}
.chart { position: relative; height: var(--chart-h); }
.grid-lines {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}
.grid-lines i { display: block; height: 1px; background: var(--chart-grid); }
.cols {
  position: absolute;
  inset: 0;
  display: flex;
  z-index: 1;
}
.col {
  position: relative;
  flex: 1 1 0;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  min-width: 0;
  padding: 0;
  border: 0;
  background: none;
  cursor: pointer;
}
.col.on { background: var(--chart-guide); }
.cbar {
  display: block;
  width: 68%;
  border-radius: 3px 3px 0 0;
  background: var(--chart-bar);
  transition: height .18s ease, background-color .15s ease;
}
.col.on .cbar { background: var(--chart-bar-hi); }
.col:focus-visible .cbar { outline: 2px solid var(--chart-line); outline-offset: 1px; }

.chart svg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
  pointer-events: none;
  z-index: 2;
}
.chart svg polyline {
  fill: none;
  stroke: var(--chart-line);
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}
.dot {
  position: absolute;
  z-index: 3;
  width: 6px;
  height: 6px;
  margin: -3px 0 0 -3px;
  border: 1.5px solid var(--chart-line);
  border-radius: 50%;
  background: var(--card);
  pointer-events: none;
}

.x-labels {
  position: relative;
  height: 16px;
  margin-top: 4px;
}
.x-labels span {
  position: absolute;
  top: 0;
  transform: translateX(-50%);
  font-size: 9px;
  color: var(--text-3);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.x-labels span.first { transform: none; }
.x-labels span.last { transform: translateX(-100%); }
.y-labels {
  position: absolute;
  top: 0;
  right: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  height: var(--chart-h);
  font-size: 9px;
  line-height: 1;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}

.chart-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 6px 12px;
  margin-top: 10px;
}
.legend-line {
  display: flex;
  gap: 12px;
  font-size: 10px;
  color: var(--text-3);
}
.legend-line span { display: flex; align-items: center; gap: 4px; }
.legend-line .sw-bar {
  width: 9px;
  height: 9px;
  border-radius: 2px;
  background: var(--chart-bar-hi);
}
.legend-line .sw-line {
  width: 14px;
  height: 2px;
  border-radius: 1px;
  background: var(--chart-line);
}
.sum-line {
  font-size: 10.5px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}

[data-theme='dark'] .stats {
  --chart-bar: #1d3a5f;
  --chart-bar-hi: #2f6fce;
  --chart-line: #5f9bff;
  --chart-grid: rgba(255, 255, 255, .08);
  --chart-guide: rgba(95, 155, 255, .08);
}
@media (prefers-color-scheme: dark) {
  html:not([data-theme='light']) .stats {
    --chart-bar: #1d3a5f;
    --chart-bar-hi: #2f6fce;
    --chart-line: #5f9bff;
    --chart-grid: rgba(255, 255, 255, .08);
    --chart-guide: rgba(95, 155, 255, .08);
  }
}

/* ---------- 学习明细 ---------- */
.detail { padding: 16px 14px 12px; }
.detail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
}
.detail-head h3 { margin: 0; font-size: 15px; font-weight: 700; color: var(--text); }
.detail-head p { margin: 4px 0 0; font-size: 10.5px; color: var(--text-3); }
.export {
  flex: none;
  padding: 6px 11px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card);
  color: var(--text-2);
  font-size: 11px;
  cursor: pointer;
}
.export:hover { border-color: var(--primary); color: var(--primary); }

.filters {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
  margin-top: 12px;
}
.filters select,
.filters input {
  width: 100%;
  min-width: 0;
  padding: 7px 8px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--card);
  color: var(--text);
  font-size: 11px;
  font-family: inherit;
}
.filters input::placeholder { color: var(--text-3); }
.filters select:focus,
.filters input:focus { outline: 2px solid var(--primary-soft); border-color: var(--primary); }
.clear {
  justify-self: start;
  padding: 4px 2px;
  border: 0;
  background: none;
  color: var(--primary);
  font-size: 10.5px;
  cursor: pointer;
}

.ledger { margin-top: 6px; }
.ledger-row { padding: 9px 2px; border-top: 1px solid var(--border); }
.ledger-row:first-child { border-top: 0; }
.lr-top { display: flex; align-items: baseline; gap: 8px; }
.lr-date {
  flex: none;
  width: 34px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}
.lr-subj {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  color: var(--text-2);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.lr-rate {
  flex: none;
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text);
  font-variant-numeric: tabular-nums;
}
.lr-rate.good { color: var(--green); }
.lr-rate.low { color: var(--amber); }
.lr-rate.dim { color: var(--text-3); font-weight: 400; }
.lr-meta {
  margin-top: 2px;
  padding-left: 42px;
  font-size: 10.5px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}

.table-scroll {
  display: none;
  margin-top: 10px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}
.detail table {
  width: 100%;
  min-width: 540px;
  border-collapse: collapse;
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
}
.detail th,
.detail td {
  padding: 8px 6px;
  border-top: 1px solid var(--border);
  text-align: left;
  white-space: nowrap;
}
.detail th {
  border-top: 0;
  font-size: 10px;
  font-weight: 600;
  color: var(--text-3);
}
.detail td { color: var(--text-2); }
.detail tbody tr:hover { background: var(--card-2); }
.detail td.date { color: var(--text); font-weight: 600; }
.detail td.subj {
  max-width: 170px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.detail .num { text-align: right; }
.detail td b { font-weight: 600; color: var(--text); }
.detail td b.good { color: var(--green); }
.detail td b.low { color: var(--amber); }
.detail .dim { color: var(--text-3); }
.detail .none {
  padding: 20px 6px;
  text-align: center;
  color: var(--text-3);
}

.detail-foot {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--border);
  font-size: 10.5px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.foot-note {
  margin: 7px 0 0;
  font-size: 10px;
  line-height: 1.5;
  color: var(--text-3);
}

@media (min-width: 560px) {
  .day-subjects { grid-template-columns: 1fr 1fr; }
  .filters { grid-template-columns: 1fr 1fr 1fr; }
  .filters input { grid-column: span 2; }
  .clear { grid-column: span 3; }
  .ledger { display: none; }
  .table-scroll { display: block; }
}

@media (min-width: 900px) {
  .stats { --wall-cell: 12px; --wall-gap: 3px; }
  .head h1 { font-size: 28px; }
  .head p { font-size: 13px; margin-top: 8px; }
  .metrics { grid-template-columns: repeat(4, 1fr); margin-bottom: 16px; }
  .metric { padding: 16px 20px; }
  .metric b { font-size: 27px; }
  .metric span { font-size: 11.5px; }
  .tabs { max-width: 340px; }
  .tabs button { font-size: 13px; padding: 8px 12px; }
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

  .chart-card { --chart-h: 200px; padding: 20px; }
  .chart-head h3 { font-size: 16px; }
  .chart-head p { font-size: 11px; }
  .windows button { padding: 5px 9px; font-size: 11px; }
  .chip { padding: 6px 12px; font-size: 11.5px; }
  .readout strong { font-size: 14px; }
  .readout span { font-size: 12px; }
  .x-labels span { font-size: 9.5px; }
  .y-labels { font-size: 9.5px; }
  .sum-line { font-size: 11px; }

  .detail { padding: 20px; }
  .detail-head h3 { font-size: 16px; }
  .detail-head p { font-size: 11px; }
  .filters {
    grid-template-columns: 150px 190px 130px minmax(160px, 1fr) auto;
    align-items: center;
  }
  .filters input { grid-column: auto; }
  .clear { grid-column: auto; }
  .detail table { font-size: 12px; }
  .detail th, .detail td { padding: 9px 8px; }
  .detail th { font-size: 10.5px; }
  .detail-foot { font-size: 11px; }
  .foot-note { font-size: 10.5px; }
}

@media (min-width: 1000px) {
  .stats { --wall-cell: 13px; --wall-gap: 3.5px; }
}
</style>
