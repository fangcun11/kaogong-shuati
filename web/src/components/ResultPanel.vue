<script setup lang="ts">
import { computed, ref } from 'vue'

interface ResultItem {
  index: number
  number: number
  state: 'correct' | 'wrong' | 'todo' | 'answered'
}

type Filter = 'all' | 'wrong' | 'todo'

const props = defineProps<{
  total: number
  answered: number
  correct: number
  wrong: number
  accuracy: number
  items: ResultItem[]
  redoMode?: boolean
}>()

const emit = defineEmits<{
  review: [index: number]
  redo: []
  restart: []
  wrongBook: []
  home: []
}>()

const filter = ref<Filter>('all')
const unanswered = computed(() => Math.max(props.total - props.answered, 0))
const wrongTotal = computed(() => props.items.filter((x) => x.state === 'wrong').length)
const shown = computed(() => (filter.value === 'all' ? props.items : props.items.filter((x) => x.state === filter.value)))
const emptyText = computed(() => (filter.value === 'wrong' ? '这一组没有错题' : '每一题都作答了'))
</script>

<template>
  <section class="card result-panel">
    <div class="rp-hero">
      <div class="rp-ring" :style="{ background: `conic-gradient(var(--green) ${accuracy * 3.6}deg, var(--border) 0deg)` }">
        <span class="rp-ring-inner">
          <b>{{ accuracy }}%</b>
          <small>正确率</small>
        </span>
      </div>
      <h2>{{ redoMode ? '错题重练完成' : '练习完成' }}</h2>
      <p class="rp-sub">{{ correct }} 对 · {{ wrong }} 错<span v-if="unanswered"> · {{ unanswered }} 未答</span></p>
    </div>

    <div class="rp-stats">
      <div><b>{{ total }}</b><span>总题数</span></div>
      <div><b>{{ answered }}</b><span>已作答</span></div>
      <div><b class="ok">{{ correct }}</b><span>正确</span></div>
      <div><b class="no">{{ wrong }}</b><span>错误</span></div>
      <div><b>{{ unanswered }}</b><span>未答</span></div>
    </div>

    <div class="rp-filter" role="tablist" aria-label="筛选题目">
      <button :class="{ on: filter === 'all' }" role="tab" :aria-selected="filter === 'all'" @click="filter = 'all'">全部 {{ total }}</button>
      <button :class="{ on: filter === 'wrong' }" role="tab" :aria-selected="filter === 'wrong'" @click="filter = 'wrong'">错题 {{ wrongTotal }}</button>
      <button :class="{ on: filter === 'todo' }" role="tab" :aria-selected="filter === 'todo'" @click="filter = 'todo'">未答 {{ unanswered }}</button>
    </div>

    <div v-if="shown.length" class="rp-grid">
      <button
        v-for="item in shown"
        :key="item.index"
        :class="item.state"
        :title="`第 ${item.number} 题`"
        @click="emit('review', item.index)"
      >{{ item.number }}</button>
    </div>
    <p v-else class="rp-empty">{{ emptyText }}</p>

    <div class="rp-legend">
      <span><i class="correct" />正确</span>
      <span><i class="wrong" />错误</span>
      <span><i class="todo" />未答</span>
      <span class="rp-tip">点题号可回看解析</span>
    </div>

    <div class="rp-actions">
      <button class="btn btn-primary btn-block" :disabled="!wrongTotal" @click="emit('redo')">重做本次错题（{{ wrongTotal }}）</button>
      <button class="btn btn-ghost btn-block" @click="emit('restart')">{{ redoMode ? '再练一遍' : '再来一组' }}</button>
      <div class="rp-actions-row">
        <button class="btn btn-ghost" @click="emit('wrongBook')">去错题本</button>
        <button class="btn btn-ghost" @click="emit('home')">返回首页</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.result-panel {
  max-width: 560px;
  padding: 26px 18px 20px;
  margin: 6px auto 0;
}
.rp-hero { text-align: center; }
.rp-ring {
  width: 112px;
  height: 112px;
  padding: 9px;
  margin: 0 auto 12px;
  border-radius: 50%;
  box-sizing: border-box;
}
.rp-ring-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  border-radius: 50%;
  background: var(--card);
}
.rp-ring b { font-size: 27px; line-height: 1.1; font-variant-numeric: tabular-nums; }
.rp-ring small { margin-top: 2px; color: var(--text-3); font-size: 10px; }
.rp-hero h2 { margin: 0; font-size: 18px; }
.rp-sub { margin: 5px 0 0; color: var(--text-3); font-size: 12px; }
.rp-stats {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 6px;
  margin: 18px 0 16px;
  padding: 12px 0;
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
}
.rp-stats b { display: block; font-size: 18px; font-variant-numeric: tabular-nums; }
.rp-stats b.ok { color: var(--green); }
.rp-stats b.no { color: var(--red); }
.rp-stats span { display: block; margin-top: 2px; color: var(--text-3); font-size: 10px; }
.rp-filter {
  display: flex;
  gap: 4px;
  padding: 4px;
  border-radius: 10px;
  background: var(--card-2);
}
.rp-filter button {
  flex: 1;
  height: 32px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text-2);
  font: inherit;
  font-size: 12.5px;
  cursor: pointer;
}
.rp-filter button.on {
  background: var(--card);
  color: var(--text);
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(15, 23, 42, .1);
}
.rp-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 8px;
  margin-top: 14px;
}
.rp-grid button {
  aspect-ratio: 1;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card);
  color: var(--text-2);
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.rp-grid button.correct { border-color: var(--green); background: var(--green-soft); color: var(--green); font-weight: 700; }
.rp-grid button.wrong { border-color: var(--red); background: var(--red-soft); color: var(--red); font-weight: 700; }
.rp-grid button.todo { border-style: dashed; color: var(--text-3); background: var(--card-2); }
.rp-grid button.answered { border-color: var(--primary); color: var(--primary); }
@media (min-width: 600px) {
  .rp-grid { grid-template-columns: repeat(auto-fill, minmax(52px, 56px)); }
}
.rp-empty { margin: 18px 0 0; color: var(--text-3); font-size: 12.5px; text-align: center; }
.rp-legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
  color: var(--text-3);
  font-size: 11px;
}
.rp-legend i {
  display: inline-block;
  width: 9px;
  height: 9px;
  margin-right: 4px;
  border: 1px solid var(--border);
  border-radius: 3px;
  vertical-align: -1px;
}
.rp-legend i.correct { border-color: var(--green); background: var(--green-soft); }
.rp-legend i.wrong { border-color: var(--red); background: var(--red-soft); }
.rp-legend i.todo { border-style: dashed; background: var(--card-2); }
.rp-tip { margin-left: auto; }
.rp-actions { display: grid; gap: 8px; margin-top: 18px; }
.rp-actions-row { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
</style>
