<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { post } from '../api/client'
// The legacy parser is a plain browser ES module shared with the old frontend.
// @ts-expect-error untyped shared browser module
import { parseTxt, parseExcel, docxToText, deChineseSpace, aiStructure, dedupeQuestions } from '../../../public/lib/custom-parser.js'

interface DraftQuestion {
  prompt: string
  material?: string
  options: string[]
  answer: string
  answer_index: number
  analysis?: string
  failed?: boolean
}

interface AiStructureResult {
  text?: string
  notice?: string
}

const router = useRouter()
const name = ref('')
const subject = ref('行测')
const raw = ref('')
const error = ref('')
const importing = ref(false)
const parsing = ref(false)
const progress = ref('')
const mode = ref<'ai' | 'local'>('ai')
const drafts = ref<DraftQuestion[]>([])

const preview = computed(() => drafts.value.slice(0, 5))
const failedCount = computed(() => drafts.value.filter((q) => q.failed).length)

function toDrafts(list: unknown): DraftQuestion[] {
  if (!Array.isArray(list)) return []
  return list
    .map((raw: any) => {
      const options = Array.isArray(raw?.options) ? raw.options.map((x: unknown) => String(x)) : []
      const answer = String(raw?.answer ?? '').trim()
      return {
        prompt: String(raw?.prompt ?? raw?.content ?? '').trim(),
        material: String(raw?.material ?? '').trim(),
        options,
        answer,
        answer_index: Number.isFinite(Number(raw?.answer_index)) ? Number(raw.answer_index) : -1,
        analysis: String(raw?.analysis ?? '').trim(),
        failed: Boolean(raw?.failed) || !options.length && !answer,
      }
    })
    .filter((q: DraftQuestion) => q.prompt)
}

/** AI 结构化：与旧版 customAiStructureText 同序——AI 优先，失败或空结果回退本地规则 */
async function structureText(text: string): Promise<unknown[]> {
  const trimmed = text.trim()
  if (!trimmed) return []
  if (mode.value === 'local') return parseTxt(trimmed)
  const chunks = trimmed.split(/\n{2,}/).map((s) => s.trim()).filter(Boolean)
  let ai: unknown[] = []
  let aiFailed = false
  if (chunks.length) {
    try {
      ai = await aiStructure(chunks, async (input: string) => {
        const res = await post<AiStructureResult>('/api/ai/structure', { text: input })
        if (!res || !res.text) throw new Error(res?.notice || '解析失败')
        return res.text
      })
    } catch {
      aiFailed = true
    }
  }
  const usable = (ai as any[]).filter((q) => String(q?.prompt || '').trim() && !q?.failed)
  if (!aiFailed && usable.length) return ai
  const local = parseTxt(trimmed)
  if (local.length) return local
  return aiFailed ? ai : []
}

/** PDF 文本层：AI 模式下先试本地规则，结构完整时优先本地（旧版同策略），否则交给 AI */
async function structurePdfText(text: string): Promise<unknown[]> {
  const local = parseTxt(text)
  if (mode.value === 'local') return local
  const usable = local.filter((q: any) => q.options?.length || q.answer).length
  if (usable >= 5) return local
  return structureText(text)
}

function parseJsonText(text: string): unknown[] {
  const json = JSON.parse(text)
  return Array.isArray(json) ? json : json.questions
}

let xlsxLoading: Promise<any> | null = null

/** XLSX 是旧版 index.html 的全局脚本，Vue 页面按需注入 */
async function loadXlsx(): Promise<any> {
  const w = window as any
  if (w.XLSX) return w.XLSX
  if (!xlsxLoading) {
    xlsxLoading = new Promise((resolve, reject) => {
      const script = document.createElement('script')
      script.src = '/vendor/xlsx/xlsx.full.min.js'
      script.onload = () => (w.XLSX ? resolve(w.XLSX) : reject(new Error('Excel 解析库未初始化')))
      script.onerror = () => reject(new Error('Excel 解析库加载失败，请刷新重试'))
      document.head.appendChild(script)
    }).catch((e) => {
      xlsxLoading = null
      throw e
    })
  }
  return xlsxLoading
}

async function parseExcelFile(file: File): Promise<unknown[]> {
  const XLSX = await loadXlsx()
  const wb = XLSX.read(await file.arrayBuffer())
  const sheet = wb.Sheets[wb.SheetNames[0]]
  if (!sheet) throw new Error('Excel 中没有工作表')
  const rows = XLSX.utils.sheet_to_json(sheet, { header: 1, defval: '' })
  const parsed = parseExcel(rows)
  if (parsed.fixedCols && parsed.questions.length && parsed.questions.some((q: any) => q.prompt)) {
    return parsed.questions
  }
  const freeText = String(parsed.freeText || rows.map((r: any[]) => r.filter(Boolean).join(' | ')).filter(Boolean).join('\n'))
  if (freeText.trim().length <= 20) return parsed.questions
  return structureText(freeText)
}

async function renderPdfPageImage(page: any): Promise<string> {
  const viewport = page.getViewport({ scale: 2 })
  const canvas = document.createElement('canvas')
  canvas.width = Math.min(Math.round(viewport.width), 2400)
  canvas.height = Math.min(Math.round(viewport.height), 2600)
  await page.render({ canvasContext: canvas.getContext('2d'), viewport }).promise
  return canvas.toDataURL('image/jpeg', 0.85)
}

async function parsePdfFile(file: File): Promise<unknown[]> {
  // 运行时从 public/vendor 加载（Go 静态服务与 Vite dev 都在站点根路径提供）
  const pdfjsURL = '/vendor/pdfjs/pdf.min.mjs'
  const pdfjs: any = await import(/* @vite-ignore */ pdfjsURL)
  pdfjs.GlobalWorkerOptions.workerSrc = '/vendor/pdfjs/pdf.worker.min.mjs'
  const doc = await pdfjs.getDocument({ data: await file.arrayBuffer() }).promise
  const texts: string[] = []
  const scanned: string[] = []
  try {
    for (let p = 1; p <= doc.numPages; p++) {
      const page = await doc.getPage(p)
      const tc = await page.getTextContent()
      const pageText = tc.items.map((i: any) => i.str + (i.hasEOL ? '\n' : ' ')).join('').replace(/[ \t]+\n/g, '\n').trim()
      const pt = deChineseSpace(pageText)
      if (pt.length > 20) {
        texts.push(pt)
      } else {
        scanned.push(await renderPdfPageImage(page))
      }
    }
  } finally {
    try { await doc.destroy() } catch { /* ignore */ }
  }

  const out: unknown[] = []
  if (texts.length) {
    const cleaned = texts.join('\n\n')
      .split('\n')
      .filter((line) => !/^\s*\d{4}\s*年.*(?:国考|联考|行政执法).*(?:卷|资料分析)/.test(line))
      .join('\n')
    out.push(...(await structurePdfText(cleaned)))
  }
  for (let i = 0; i < scanned.length; i++) {
    progress.value = `识别扫描页（${i + 1}/${scanned.length}）…`
    try {
      const res = await post<AiStructureResult>('/api/ai/ocr', { image: scanned[i] })
      if (!res?.text) throw new Error(res?.notice || '未返回识别文本')
      out.push(...(await structureText(res.text)))
    } catch (e) {
      out.push({
        prompt: `【扫描页 ${i + 1} 解析失败】${e instanceof Error ? e.message : '识图失败'}`,
        options: [], answer: '', answer_index: -1, failed: true,
      })
    }
  }
  return dedupeQuestions(out as any[])
}

async function parseFile(file: File): Promise<unknown[]> {
  const ext = (file.name.split('.').pop() || '').toLowerCase()
  if (ext === 'txt' || ext === 'md') return structureText(await file.text())
  if (ext === 'json') return parseJsonText(await file.text())
  if (ext === 'docx') return structureText(await docxToText(file))
  if (ext === 'doc') throw new Error('旧版 .doc 请用 Word 另存为 .docx 或 TXT 后再导入')
  if (ext === 'xlsx' || ext === 'xls') return parseExcelFile(file)
  if (ext === 'pdf') return parsePdfFile(file)
  throw new Error(`不支持的格式：${ext || '未知'}`)
}

function ensureName(fileNames: string[]) {
  if (name.value.trim() || !fileNames.length) return
  name.value = fileNames.map((n) => n.replace(/\.[^.]+$/, '')).join('+') || `导入题库 ${new Date().toLocaleDateString('zh-CN')}`
}

async function parseInput() {
  error.value = ''
  drafts.value = []
  const text = raw.value.trim()
  if (!text) {
    error.value = '请先粘贴题目文本或选择文件'
    return
  }
  parsing.value = true
  progress.value = '解析中…'
  try {
    const parsed = text.startsWith('[') || text.startsWith('{') ? parseJsonText(text) : await structureText(text)
    drafts.value = toDrafts(parsed)
    if (!drafts.value.length) {
      error.value = '没有解析到题目，请检查格式'
      return
    }
    ensureName([])
  } catch (e) {
    error.value = e instanceof Error ? e.message : '解析失败'
  } finally {
    parsing.value = false
    progress.value = ''
  }
}

async function selectFiles(event: Event) {
  const input = event.target as HTMLInputElement
  const files = [...(input.files || [])]
  if (!files.length) return
  error.value = ''
  drafts.value = []
  parsing.value = true
  const collected: unknown[] = []
  try {
    for (let i = 0; i < files.length; i++) {
      progress.value = `解析 ${files[i].name}（${i + 1}/${files.length}）…`
      try {
        collected.push(...(await parseFile(files[i])))
      } catch (e) {
        collected.push({
          prompt: `【解析失败】${files[i].name}：${e instanceof Error ? e.message : '未知错误'}`,
          options: [], answer: '', answer_index: -1, failed: true,
        })
      }
    }
    drafts.value = toDrafts(collected)
    ensureName(files.map((f) => f.name))
    if (!drafts.value.length) error.value = '文件里没有解析出题目，请检查内容或改用旧版导入'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '解析失败'
  } finally {
    parsing.value = false
    progress.value = ''
    input.value = ''
  }
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
    <p class="muted">支持 PDF、Excel（.xlsx/.xls）、Word（.docx）、TXT、JSON。一次导入 = 一个模块。</p>

    <div class="vue-mode-bar">
      <span class="muted">解析方式</span>
      <button :class="['btn', mode === 'ai' ? 'btn-primary' : 'btn-ghost']" :disabled="parsing" @click="mode = 'ai'">AI 智能导入</button>
      <button :class="['btn', mode === 'local' ? 'btn-primary' : 'btn-ghost']" :disabled="parsing" @click="mode = 'local'">本地规则</button>
    </div>
    <p class="muted vue-mode-hint">
      AI 模式会把杂乱文本交给「题目解析员」结构化，未配置或失败时自动回退本地规则；扫描版 PDF 的识图依赖已启用的「识图转写员」。
    </p>

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
      <label class="btn btn-ghost vue-file-btn" :class="{ disabled: parsing }">
        选择文件
        <input
          type="file"
          accept=".pdf,.xlsx,.xls,.docx,.txt,.md,.json"
          multiple
          hidden
          :disabled="parsing"
          @change="selectFiles"
        />
      </label>
      <button class="btn btn-ghost" :disabled="parsing" @click="parseInput">解析文字</button>
    </div>
    <div v-if="parsing || progress" class="vue-import-progress">
      <span class="spinner" />
      <span>{{ progress || '解析中…' }}</span>
    </div>

    <div v-if="preview.length" class="vue-preview">
      <div class="vue-preview-head">
        预览 {{ drafts.length }} 题
        <span v-if="failedCount" class="tag warn">{{ failedCount }} 题待人工修正</span>
      </div>
      <div v-for="(q, i) in preview" :key="i" class="vue-preview-item" :class="{ failed: q.failed }">
        <b>{{ i + 1 }}. {{ q.prompt }}</b>
        <div class="muted">{{ q.options.join(' · ') || '（无选项）' }}</div>
        <small>答案：{{ q.answer || '未识别' }}</small>
      </div>
      <div v-if="drafts.length > preview.length" class="muted" style="font-size: 12px">还有 {{ drafts.length - preview.length }} 题未展示</div>
    </div>

    <button class="btn btn-primary btn-block" :disabled="importing || parsing || !drafts.length" @click="importBatch">
      {{ importing ? '导入中…' : `导入 ${drafts.length || ''} 题` }}
    </button>
    <div v-if="error" class="vue-form-error">{{ error }}</div>
    <button class="btn btn-ghost btn-block" style="margin-top: 8px" @click="router.back()">返回</button>
  </div>
</template>

<style scoped>
.vue-import { padding: 20px 18px; }
.vue-import h3 { margin: 0 0 5px; }
.vue-mode-bar { display: flex; align-items: center; gap: 8px; margin-top: 14px; }
.vue-mode-bar .btn { flex: 0 0 auto; padding: 7px 12px; font-size: 12.5px; }
.vue-mode-hint { margin: 8px 0 0; font-size: 12px; line-height: 1.6; }
.vue-import-text { min-height: 210px; resize: vertical; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 13px; line-height: 1.6; }
.vue-import-tools { display: flex; gap: 8px; margin-top: 10px; }
.vue-file-btn { display: inline-flex; align-items: center; justify-content: center; flex: 1; cursor: pointer; }
.vue-file-btn.disabled { opacity: 0.55; cursor: default; }
.vue-import-progress { display: flex; align-items: center; gap: 9px; margin-top: 12px; color: var(--text-2); font-size: 13px; }
.vue-import-progress .spinner { width: 16px; height: 16px; }
.vue-preview { margin: 16px 0; padding: 12px; border: 1px solid var(--border); border-radius: 12px; background: var(--card-2); }
.vue-preview-head { display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 13px; margin-bottom: 9px; }
.vue-preview-item { padding: 8px 0; border-top: 1px solid var(--border); font-size: 13px; line-height: 1.55; }
.vue-preview-item b { font-weight: 600; }
.vue-preview-item.failed { color: var(--red); }
.vue-preview-item small { color: var(--green); }
.vue-preview-item.failed small { color: inherit; }
.vue-form-error { color: var(--red); font-size: 13px; margin-top: 10px; }
</style>
