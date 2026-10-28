<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import Icon from '../components/Icon.vue'

interface GeneratedPaper {
  ok: boolean
  error?: string
  title: string
  subject: string
  total: number
  durationMinutes: number
  questions: unknown[]
  notice?: string
}

const router = useRouter()
const subject = ref('公务员·行测')
const category = ref('联考A类')
const difficulty = ref('balanced')
const weak = ref(false)
const loading = ref(false)
const error = ref('')
const notice = ref('')

const isZhiCe = computed(() => subject.value === '事业编·职测')
const count = computed(() => (isZhiCe.value ? 100 : 130))

async function generate() {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    const payload: Record<string, unknown> = {
      subject: subject.value,
      difficulty: difficulty.value,
      count: count.value,
    }
    if (isZhiCe.value) payload.category = category.value
    if (weak.value) payload.weak = ['资料分析']
    const result = await api<GeneratedPaper>('/api/paper/generate', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    if (!result.ok) {
      error.value = result.error || '组卷失败'
      return
    }
    sessionStorage.setItem('kg_generated_paper', JSON.stringify(result))
    if (result.notice) notice.value = result.notice
    await router.push({
      name: 'practice',
      params: { subject: result.subject },
      query: { generated: '1' },
    })
  } catch (e) {
    error.value = e instanceof Error ? e.message : '组卷失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="card vue-paper-config">
    <div class="vue-paper-title"><Icon name="target" :size="22" /> 智能组卷</div>
    <p class="muted">按官方模块、题源和材料组规则生成一套新卷。</p>

    <label class="field-label">科目</label>
    <div class="chip-row">
      <button class="chip" :class="{ on: subject === '公务员·行测' }" @click="subject = '公务员·行测'">公务员·行测</button>
      <button class="chip" :class="{ on: subject === '事业编·职测' }" @click="subject = '事业编·职测'">事业编·职测</button>
    </div>

    <template v-if="isZhiCe">
      <label class="field-label">职测卷型</label>
      <div class="chip-row">
        <button v-for="c in ['联考A类', '联考B类', '联考C类', '山东']" :key="c" class="chip" :class="{ on: category === c }" @click="category = c">{{ c }}</button>
      </div>
    </template>

    <label class="field-label">难度</label>
    <div class="chip-row">
      <button v-for="d in [['easy', '简单'], ['balanced', '适中'], ['hard', '偏难'], ['random', '随机']]" :key="d[0]" class="chip" :class="{ on: difficulty === d[0] }" @click="difficulty = d[0]">{{ d[1] }}</button>
    </div>

    <label class="vue-check">
      <input v-model="weak" type="checkbox" />
      资料分析弱项加权
    </label>

    <div class="vue-paper-summary">
      <span>{{ subject }}</span>
      <span>{{ isZhiCe ? category : '官方卷型' }}</span>
      <b>{{ count }} 题</b>
    </div>

    <button class="btn btn-primary btn-block" :disabled="loading" @click="generate">
      {{ loading ? '正在组卷…' : `生成 ${count} 题试卷` }}
    </button>
    <div v-if="notice" class="muted" style="margin-top: 10px">{{ notice }}</div>
    <div v-if="error" class="vue-form-error">{{ error }}</div>
  </div>
</template>

<style scoped>
.vue-paper-config { padding: 20px 18px; }
.vue-paper-title { display: flex; align-items: center; gap: 8px; font-size: 20px; font-weight: 800; }
.vue-paper-config .field-label { margin-top: 18px; }
.chip-row { display: flex; flex-wrap: wrap; gap: 8px; }
.chip { border: 1px solid var(--border-strong); background: var(--card); color: var(--text-2); border-radius: 10px; padding: 8px 12px; font: inherit; font-size: 13px; cursor: pointer; }
.chip.on { border-color: var(--primary); background: var(--primary-soft); color: var(--primary); font-weight: 700; }
.vue-check { display: flex; gap: 8px; align-items: center; margin: 18px 0; color: var(--text-2); font-size: 13px; }
.vue-paper-summary { display: flex; justify-content: space-between; gap: 8px; padding: 12px 0 18px; color: var(--text-2); font-size: 13px; border-bottom: 1px solid var(--border); margin-bottom: 16px; }
.vue-paper-summary b { color: var(--text); }
.vue-form-error { margin-top: 10px; color: var(--red); font-size: 13px; }
</style>
