<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { post } from '../api/client'
// The legacy parser is a plain browser ES module shared with the old frontend.
// @ts-expect-error untyped shared browser module
import { parseTxt } from '../../../public/lib/custom-parser.js'

interface DraftQuestion {
  prompt: string
  material?: string
  options: string[]
  answer: string
  answer_index: number
  analysis?: string
}

const router = useRouter()
const name = ref('')
const subject = ref('行测')
const raw = ref('')
const error = ref('')
const importing = ref(false)
const drafts = ref<DraftQuestion[]>([])

const preview = computed(() => drafts.value.slice(0, 5))

function parseInput() {
  error.value = ''
  drafts.value = []
  const text = raw.value.trim()
  if (!text) {
    error.value = '请先粘贴题目文本或选择文件'
    return
  }
  try {
    let parsed: unknown
    if (text.startsWith('[') || text.startsWith('{')) {
      const json = JSON.parse(text)
      parsed = Array.isArray(json) ? json : json.questions
    } else {
      parsed = parseTxt(text)
    }
    if (!Array.isArray(parsed) || !parsed.length) {
      error.value = '没有解析到题目，请检查格式'
      return
    }
    drafts.value = parsed
      .map((q: any) => ({
        prompt: String(q.prompt || q.content || '').trim(),
        material: String(q.material || '').trim(),
        options: Array.isArray(q.options) ? q.options.map((x: unknown) => String(x)) : [],
        answer: String(q.answer || '').trim(),
        answer_index: Number.isFinite(Number(q.answer_index)) ? Number(q.answer_index) : -1,
        analysis: String(q.analysis || '').trim(),
      }))
      .filter((q: DraftQuestion) => q.prompt && q.options.length)
    if (!drafts.value.length) {
      error.value = '解析结果缺少题干或选项'
      return
    }
    if (!name.value) name.value = `导入题库 ${new Date().toLocaleDateString('zh-CN')}`
  } catch (e) {
    error.value = e instanceof Error ? e.message : '解析失败'
  }
}

async function selectFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (!/\.(txt|md|json)$/i.test(file.name)) {
    error.value = 'Vue 版目前支持 TXT / Markdown / JSON；Excel、Word、PDF 请先用旧版导入'
    input.value = ''
    return
  }
  raw.value = await file.text()
  if (!name.value) name.value = file.name.replace(/\.[^.]+$/, '')
  parseInput()
}

async function importBatch() {
  if (!drafts.value.length) {
    error.value = '请先解析并预览题目'
    return
  }
  if (!name.value.trim()) {
    error.value = '请填写批次名称'
    return
  }
  importing.value = true
  error.value = ''
  try {
    await post('/api/custom/import', {
      name: name.value.trim(),
      subject: subject.value,
      questions: drafts.value,
    })
    await router.replace({ name: 'custom' })
  } catch (e) {
    error.value = e instanceof Error ? e.message : '导入失败'
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <div class="card vue-import">
    <h3>导入自定义题库</h3>
    <p class="muted">支持题干、材料、选项和答案解析。文本格式与旧版一致。</p>

    <label class="field-label">批次名称</label>
    <input v-model="name" class="field" placeholder="例如：2026 省考错题" />

    <label class="field-label">科目</label>
    <select v-model="subject" class="field">
      <option>行测</option>
      <option>申论</option>
      <option>职测</option>
      <option>综应</option>
      <option>其他</option>
    </select>

    <label class="field-label">题目文本</label>
    <textarea
      v-model="raw"
      class="field vue-import-text"
      rows="12"
      placeholder="1. 题干……&#10;A. 选项一  B. 选项二&#10;答案：A&#10;解析：……"
    />

    <div class="vue-import-tools">
      <label class="btn btn-ghost vue-file-btn">
        选择 TXT / JSON 文件
        <input type="file" accept=".txt,.md,.json" hidden @change="selectFile" />
      </label>
      <button class="btn btn-ghost" @click="parseInput">解析预览</button>
    </div>

    <div v-if="preview.length" class="vue-preview">
      <div class="vue-preview-head">预览 {{ drafts.length }} 题</div>
      <div v-for="(q, i) in preview" :key="i" class="vue-preview-item">
        <b>{{ i + 1 }}. {{ q.prompt }}</b>
        <div class="muted">{{ q.options.join(' · ') }}</div>
        <small>答案：{{ q.answer || '未识别' }}</small>
      </div>
      <div v-if="drafts.length > preview.length" class="muted" style="font-size: 12px">还有 {{ drafts.length - preview.length }} 题未展示</div>
    </div>

    <button class="btn btn-primary btn-block" :disabled="importing || !drafts.length" @click="importBatch">
      {{ importing ? '导入中…' : `导入 ${drafts.length || ''} 题` }}
    </button>
    <div v-if="error" class="vue-form-error">{{ error }}</div>
    <button class="btn btn-ghost btn-block" style="margin-top: 8px" @click="router.back()">返回</button>
  </div>
</template>

<style scoped>
.vue-import { padding: 20px 18px; }
.vue-import h3 { margin: 0 0 5px; }
.vue-import-text { min-height: 210px; resize: vertical; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 13px; line-height: 1.6; }
.vue-import-tools { display: flex; gap: 8px; margin-top: 10px; }
.vue-file-btn { display: inline-flex; align-items: center; justify-content: center; flex: 1; cursor: pointer; }
.vue-preview { margin: 16px 0; padding: 12px; border: 1px solid var(--border); border-radius: 12px; background: var(--card-2); }
.vue-preview-head { font-weight: 700; font-size: 13px; margin-bottom: 9px; }
.vue-preview-item { padding: 8px 0; border-top: 1px solid var(--border); font-size: 13px; line-height: 1.55; }
.vue-preview-item b { font-weight: 600; }
.vue-preview-item small { color: var(--green); }
.vue-form-error { color: var(--red); font-size: 13px; margin-top: 10px; }
</style>
