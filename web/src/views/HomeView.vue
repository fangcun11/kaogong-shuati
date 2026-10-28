<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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

const router = useRouter()
const subjects = ref<Subject[]>([])
const stats = ref<Stats | null>(null)
const customCount = ref('加载中…')
const loading = ref(true)
const error = ref('')

const subjectMeta: Record<string, { tint: string; icon: string; desc: string }> = {
  '公务员·行测': { tint: 'tint-orange', icon: 'book', desc: '言语·判断·资料·数量·常识' },
  '公务员·申论': { tint: 'tint-green', icon: 'book', desc: '归纳概括·综合分析·大作文' },
  '事业编·综应': { tint: 'tint-violet', icon: 'book', desc: '归纳概括·综合分析·大作文' },
  '事业编·职测': { tint: 'tint-blue', icon: 'target', desc: '言语·判断·资料·数量·常识' },
}
const quickItems = [
  { label: '随机练习', icon: 'play', cls: 'qg-violet', to: 'random' },
  { label: '错题本', icon: 'book', cls: 'qg-coral', to: 'wrong' },
  { label: '收藏', icon: 'star', cls: 'qg-amber', to: 'favorites' },
  { label: '笔记', icon: 'book', cls: 'qg-green', to: 'notes' },
  { label: 'AI 设置', icon: 'sparkles', cls: 'qg-blue', to: 'ai' },
  { label: '智能组卷', icon: 'target', cls: 'qg-orange', to: 'papers' },
]
const hasData = computed(() => !!stats.value && stats.value.total > 0)

function meta(name: string) {
  return subjectMeta[name] || { tint: 'tint-blue', icon: 'book', desc: '' }
}

function openSubject(name: string) {
  router.push({ name: 'subject', params: { name } })
}

function openQuick(to: string) {
  if (to === 'papers') return router.push({ name: 'papers' })
  if (to === 'wrong') return router.push({ name: 'wrong' })
  if (to === 'favorites') return router.push({ name: 'favorites' })
  if (to === 'random' && subjects.value[0]) return openSubject(subjects.value[0].subjectName)
  if (to === 'ai') return router.push({ name: 'ai' })
  if (to === 'notes') return router.push({ name: 'notes' })
  if (to === 'custom') return router.push({ name: 'custom' })
}

onMounted(async () => {
  try {
    const [subjectData, statData, batchData] = await Promise.all([
      api<Subject[]>('/api/subjects'),
      api<Stats>('/api/records/stats').catch(() => null),
      api<{ batches: Array<{ count: number }> }>('/api/custom/batches').catch(() => ({ batches: [] })),
    ])
    subjects.value = subjectData
    stats.value = statData
    const total = batchData.batches.reduce((sum, b) => sum + b.count, 0)
    customCount.value = batchData.batches.length ? `${batchData.batches.length} 个批次 · ${total} 题` : '暂无批次，点击导入'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error" class="empty">加载失败：{{ error }}</div>
  <template v-else>
    <section class="hero">
      <div class="hero-eyebrow">EXAM ARCHIVE · {{ hasData ? '你的学习档案' : '新档案' }}</div>
      <div class="hero-title">{{ hasData ? '继续保持，坚持就是上岸' : '开始刷第一道题' }}</div>
      <div class="hero-stats">
        <div class="hero-stat"><div class="hs-num">{{ stats?.total || 0 }}</div><div class="hs-label">累计做题</div></div>
        <div class="hero-stat"><div class="hs-num">{{ hasData ? `${stats?.rate || 0}%` : '—' }}</div><div class="hs-label">正确率</div></div>
        <div v-if="hasData" class="hero-stat"><div class="hs-num">{{ stats?.wrong || 0 }}</div><div class="hs-label">待订错题</div></div>
      </div>
      <button class="hero-cta" @click="subjects[0] && openSubject(subjects[0].subjectName)">
        <Icon name="play" :size="17" :stroke="2.2" />
        {{ hasData ? '继续学习' : '开始刷题' }}
      </button>
    </section>

    <section class="subject-grid">
      <div class="subject-card custom-entry" @click="openQuick('custom')">
        <span class="emoji tint-cyan"><Icon name="database" :size="30" /></span>
        <div class="name">自定义题库</div>
        <div class="desc">导入 PDF / Excel / Word / TXT 题目，自由刷题</div>
        <div class="stat-line"><span>{{ customCount }}</span></div>
      </div>
      <div v-for="s in subjects" :key="s.subjectName" class="subject-card" @click="openSubject(s.subjectName)">
        <span class="emoji" :class="meta(s.subjectName).tint"><Icon :name="meta(s.subjectName).icon" :size="30" /></span>
        <div class="name">{{ s.subjectName }}</div>
        <div class="desc">{{ meta(s.subjectName).desc }}</div>
        <div class="stat-line">
          <span><b>{{ s.papers }}</b> 套</span>
          <span><b>{{ s.done || 0 }}/{{ s.questions }}</b> 题</span>
        </div>
      </div>
    </section>

    <section class="card vue-quick-card">
      <h3>快捷入口</h3>
      <div class="quick-grid">
        <button v-for="q in quickItems" :key="q.label" class="quick-item" @click="openQuick(q.to)">
          <span class="qi-ico" :class="q.cls"><Icon :name="q.icon" :size="22" /></span>
          <span class="qi-label">{{ q.label }}</span>
        </button>
      </div>
    </section>
  </template>
</template>

<style scoped>
.vue-quick-card { margin-top: 14px; }
.subject-card { cursor: pointer; }
.custom-entry .stat-line { color: var(--text-2); }
</style>
