<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api } from '../api/client'
import { session } from '../stores/session'

interface Agent {
  id: number
  name: string
  role: string
  description: string
  system_prompt: string
  skill: string
  base_url: string
  api_key_masked?: string
  model: string
  temperature: number
  max_tokens: number
  enabled: number
  skill_loaded?: { name: string; files: number; source: string } | null
}

const agents = ref<Agent[]>([])
const selectedId = ref(0)
const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const notice = ref('')
const testResult = ref('')
const apiKey = ref('')
const form = reactive({
  name: '',
  description: '',
  system_prompt: '',
  skill: '',
  base_url: '',
  model: '',
  temperature: 0.3,
  max_tokens: 12000,
  enabled: 1,
})

const isAdmin = () => session.user?.role === 'admin'

function select(agent: Agent) {
  selectedId.value = agent.id
  form.name = agent.name
  form.description = agent.description
  form.system_prompt = agent.system_prompt
  form.skill = agent.skill
  form.base_url = agent.base_url
  form.model = agent.model
  form.temperature = agent.temperature
  form.max_tokens = agent.max_tokens
  form.enabled = agent.enabled
  apiKey.value = ''
  testResult.value = ''
  notice.value = ''
}

async function load() {
  loading.value = true
  try {
    agents.value = await api<Agent[]>('/api/ai/agents')
    if (!selectedId.value && agents.value[0]) select(agents.value[0])
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载 AI 配置失败'
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!isAdmin()) {
    error.value = '只有管理员可以修改 AI 配置'
    return
  }
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const payload: Record<string, unknown> = {
      name: form.name,
      description: form.description,
      system_prompt: form.system_prompt,
      skill: form.skill,
      base_url: form.base_url,
      model: form.model,
      temperature: Number(form.temperature),
      max_tokens: Number(form.max_tokens),
      enabled: Number(form.enabled),
    }
    if (apiKey.value.trim()) payload.api_key = apiKey.value.trim()
    await api(`/api/ai/agents/${selectedId.value}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    })
    notice.value = '已保存'
    apiKey.value = ''
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

async function testAgent() {
  testing.value = true
  testResult.value = ''
  error.value = ''
  try {
    const result = await api<{ content?: string; error?: string }>(`/api/ai/agents/${selectedId.value}/test`, {
      method: 'POST',
      body: JSON.stringify({ content: '请用一句话确认你可以正常工作。' }),
    })
    testResult.value = result.content || result.error || '没有返回内容'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '测试失败'
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="spinner" />
  <div v-else-if="error && !agents.length" class="empty">加载失败：{{ error }}</div>
  <template v-else>
    <div class="vue-ai-layout">
      <aside class="card vue-ai-list">
        <h3>AI 智能体</h3>
        <button
          v-for="a in agents"
          :key="a.id"
          class="vue-ai-item"
          :class="{ active: a.id === selectedId }"
          @click="select(a)"
        >
          <span class="vue-ai-dot" :class="{ on: a.enabled }" />
          <span>
            <b>{{ a.name }}</b>
            <small>{{ a.enabled ? '已启用' : '未启用' }} · {{ a.model }}</small>
          </span>
        </button>
        <p v-if="!isAdmin()" class="muted" style="font-size: 12px; margin-top: 12px">当前为只读模式，仅管理员可修改。</p>
      </aside>

      <section class="card vue-ai-form">
        <div class="vue-ai-form-head">
          <div>
            <h3>{{ form.name || 'AI 配置' }}</h3>
            <p class="muted">{{ agents.find((a) => a.id === selectedId)?.role }}</p>
          </div>
          <label class="vue-toggle">
            <input v-model.number="form.enabled" type="checkbox" :true-value="1" :false-value="0" :disabled="!isAdmin()" />
            启用
          </label>
        </div>

        <label class="field-label">Base URL</label>
        <input v-model="form.base_url" class="field" :disabled="!isAdmin()" />

        <label class="field-label">API Key</label>
        <input v-model="apiKey" class="field" type="password" :placeholder="agents.find((a) => a.id === selectedId)?.api_key_masked || '尚未配置'" :disabled="!isAdmin()" />
        <p class="muted" style="font-size: 12px; margin: 5px 0 0">留空表示不修改已保存的 key。</p>

        <label class="field-label">模型</label>
        <input v-model="form.model" class="field" :disabled="!isAdmin()" />

        <div class="vue-ai-row">
          <div>
            <label class="field-label">温度</label>
            <input v-model.number="form.temperature" class="field" type="number" min="0" max="2" step="0.1" :disabled="!isAdmin()" />
          </div>
          <div>
            <label class="field-label">最大 tokens</label>
            <input v-model.number="form.max_tokens" class="field" type="number" min="1" :disabled="!isAdmin()" />
          </div>
        </div>

        <label class="field-label">Skill</label>
        <input v-model="form.skill" class="field" :disabled="!isAdmin()" />
        <p v-if="agents.find((a) => a.id === selectedId)?.skill_loaded" class="muted" style="font-size: 12px; margin: 5px 0 0">
          已加载：{{ agents.find((a) => a.id === selectedId)?.skill_loaded?.name }} · {{ agents.find((a) => a.id === selectedId)?.skill_loaded?.files }} 个文件
        </p>

        <label class="field-label">System Prompt</label>
        <textarea v-model="form.system_prompt" class="field" rows="12" :disabled="!isAdmin()" />

        <div class="action-row">
          <button class="btn btn-ghost" :disabled="testing || !isAdmin()" @click="testAgent">{{ testing ? '测试中…' : '测试' }}</button>
          <button class="btn btn-primary" :disabled="saving || !isAdmin()" @click="save">{{ saving ? '保存中…' : '保存配置' }}</button>
        </div>
        <div v-if="notice" class="vue-ai-ok">{{ notice }}</div>
        <div v-if="error" class="vue-form-error">{{ error }}</div>
        <div v-if="testResult" class="answer-box" style="margin-top: 14px">
          <div class="ab-title">测试结果</div>
          <div class="ab-body">{{ testResult }}</div>
        </div>
      </section>
    </div>
  </template>
</template>

<style scoped>
.vue-ai-layout { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 12px; align-items: start; }
.vue-ai-list { padding: 14px; position: sticky; top: 76px; }
.vue-ai-list h3 { margin: 0 0 10px; }
.vue-ai-item { display: flex; align-items: flex-start; gap: 9px; width: 100%; border: 0; background: none; color: var(--text); text-align: left; padding: 9px 7px; border-radius: 9px; cursor: pointer; }
.vue-ai-item:hover, .vue-ai-item.active { background: var(--primary-soft); }
.vue-ai-item b { display: block; font-size: 13px; }
.vue-ai-item small { display: block; margin-top: 3px; color: var(--text-3); font-size: 11px; }
.vue-ai-dot { width: 8px; height: 8px; border-radius: 50%; background: var(--text-3); margin-top: 5px; flex: 0 0 auto; }
.vue-ai-dot.on { background: var(--green); }
.vue-ai-form { padding: 18px; }
.vue-ai-form-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.vue-ai-form-head h3 { margin: 0 0 3px; }
.vue-toggle { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--text-2); }
.vue-ai-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.vue-ai-ok { color: var(--green); font-size: 13px; margin-top: 10px; }
.vue-form-error { color: var(--red); font-size: 13px; margin-top: 10px; }
@media (max-width: 720px) {
  .vue-ai-layout { grid-template-columns: 1fr; }
  .vue-ai-list { position: static; }
}
</style>
