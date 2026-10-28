<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'

interface ChapterStat { chapter: string; c: number; ok: number }
interface DayStat { d: string; c: number; ok?: number }
interface Stats {
  total: number
  correct: number
  wrong: number
  rate: number
  byChapter: ChapterStat[]
  last7: DayStat[]
  daily: DayStat[]
}

const stats = ref<Stats | null>(null)
const loading = ref(true)
const error = ref('')
const maxChapter = computed(() => Math.max(1, ...(stats.value?.byChapter || []).map((x) => x.c)))
const maxDay = computed(() => Math.max(1, ...(stats.value?.last7 || []).map((x) => x.c)))

onMounted(async () => {
  try {
    stats.value = await api<Stats>('/api/records/stats')
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载统计失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <template v-else-if="stats">
    <div class="stats-hero">
      <div><b>{{ stats.total }}</b><span>累计做题</span></div>
      <div><b>{{ stats.rate }}%</b><span>正确率</span></div>
      <div><b>{{ stats.wrong }}</b><span>待订错题</span></div>
    </div>

    <section class="card stats-card">
      <h3>最近 7 天</h3>
      <div v-if="stats.last7.length" class="stats-bars">
        <div v-for="d in stats.last7" :key="d.d" class="stats-bar-row">
          <span>{{ d.d.slice(5) }}</span>
          <span class="stats-bar"><i :style="{ width: `${(d.c / maxDay) * 100}%` }" /></span>
          <b>{{ d.c }}</b>
        </div>
      </div>
      <div v-else class="muted">最近 7 天还没有做题记录</div>
    </section>

    <section class="card stats-card">
      <h3>章节分布</h3>
      <div v-if="stats.byChapter.length" class="stats-bars">
        <div v-for="c in stats.byChapter" :key="c.chapter" class="stats-bar-row">
          <span class="stats-chapter">{{ c.chapter || '未分类' }}</span>
          <span class="stats-bar"><i :style="{ width: `${(c.c / maxChapter) * 100}%` }" /></span>
          <b>{{ c.ok }}/{{ c.c }}</b>
        </div>
      </div>
      <div v-else class="muted">还没有做题记录</div>
    </section>
  </template>
</template>

<style scoped>
.stats-hero {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-bottom: 14px;
  padding: 20px;
  color: #fff;
  border-radius: 18px;
  background: var(--primary);
}
.stats-hero b { display: block; font-size: 26px; line-height: 1.1; font-variant-numeric: tabular-nums; }
.stats-hero span { display: block; margin-top: 4px; font-size: 11px; opacity: .8; }
.stats-card { margin-bottom: 12px; }
.stats-bars { display: grid; gap: 10px; margin-top: 12px; }
.stats-bar-row { display: grid; grid-template-columns: 76px 1fr 48px; align-items: center; gap: 9px; font-size: 12px; color: var(--text-2); }
.stats-bar-row b { text-align: right; color: var(--text); font-variant-numeric: tabular-nums; }
.stats-chapter { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.stats-bar { height: 7px; border-radius: 999px; background: var(--bg-soft); overflow: hidden; }
.stats-bar i { display: block; height: 100%; min-width: 2px; border-radius: inherit; background: var(--primary); }
</style>
