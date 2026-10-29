<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

type Tool = 'pen' | 'eraser'
interface Stroke {
  tool: Tool
  width: number
  points: Array<[number, number]>
}

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const PAPER = '#fbfaf6'
const INK = '#1f2a3c'
const PEN_WIDTH = 2.4
const ERASER_WIDTH = 18

const canvasEl = ref<HTMLCanvasElement | null>(null)
const wrapEl = ref<HTMLElement | null>(null)
const tool = ref<Tool>('pen')
const strokes = ref<Stroke[]>([])
let drawing: Stroke | null = null
let observer: ResizeObserver | null = null

function ctxOf() {
  return canvasEl.value?.getContext('2d') || null
}

function cssSize() {
  const el = wrapEl.value
  return el ? { w: el.clientWidth, h: el.clientHeight } : { w: 0, h: 0 }
}

function paint(ctx: CanvasRenderingContext2D, s: Stroke) {
  const pts = s.points
  if (!pts.length) return
  ctx.strokeStyle = s.tool === 'eraser' ? PAPER : INK
  ctx.fillStyle = ctx.strokeStyle
  ctx.lineWidth = s.width
  if (pts.length === 1) {
    ctx.beginPath()
    ctx.arc(pts[0][0], pts[0][1], s.width / 2, 0, Math.PI * 2)
    ctx.fill()
    return
  }
  ctx.beginPath()
  ctx.moveTo(pts[0][0], pts[0][1])
  for (let i = 1; i < pts.length; i++) ctx.lineTo(pts[i][0], pts[i][1])
  ctx.stroke()
}

function drawAll() {
  const canvas = canvasEl.value
  const ctx = ctxOf()
  const { w } = cssSize()
  if (!canvas || !ctx || !w) return
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.clearRect(0, 0, canvas.width, canvas.height)
  const dpr = canvas.width / w
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  for (const s of strokes.value) paint(ctx, s)
}

function resize() {
  const canvas = canvasEl.value
  const { w, h } = cssSize()
  if (!canvas || w < 2 || h < 2) return
  const dpr = Math.min(window.devicePixelRatio || 1, 2.5)
  const bw = Math.round(w * dpr)
  const bh = Math.round(h * dpr)
  if (canvas.width !== bw || canvas.height !== bh) {
    canvas.width = bw
    canvas.height = bh
  }
  drawAll()
}

function pointFrom(e: PointerEvent): [number, number] {
  const rect = canvasEl.value!.getBoundingClientRect()
  return [e.clientX - rect.left, e.clientY - rect.top]
}

function onDown(e: PointerEvent) {
  if (!props.open) return
  if (e.pointerType === 'mouse' && e.button !== 0) return
  e.preventDefault()
  const canvas = canvasEl.value
  const ctx = ctxOf()
  if (!canvas || !ctx) return
  try {
    canvas.setPointerCapture(e.pointerId)
  } catch {
    /* optional */
  }
  drawing = {
    tool: tool.value,
    width: tool.value === 'eraser' ? ERASER_WIDTH : PEN_WIDTH,
    points: [pointFrom(e)],
  }
  strokes.value.push(drawing)
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  paint(ctx, drawing)
}

function onMove(e: PointerEvent) {
  if (!drawing) return
  e.preventDefault()
  const pt = pointFrom(e)
  const last = drawing.points[drawing.points.length - 1]
  if (Math.hypot(pt[0] - last[0], pt[1] - last[1]) < 1.2) return
  drawing.points.push(pt)
  const ctx = ctxOf()
  if (!ctx) return
  ctx.strokeStyle = drawing.tool === 'eraser' ? PAPER : INK
  ctx.lineWidth = drawing.width
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.beginPath()
  ctx.moveTo(last[0], last[1])
  ctx.lineTo(pt[0], pt[1])
  ctx.stroke()
}

function onUp(e: PointerEvent) {
  if (!drawing) return
  drawing = null
  try {
    canvasEl.value?.releasePointerCapture(e.pointerId)
  } catch {
    /* optional */
  }
}

function undo() {
  if (!strokes.value.length) return
  strokes.value.pop()
  drawAll()
}

function clearAll() {
  if (!strokes.value.length) return
  strokes.value = []
  drawAll()
}

function reset() {
  drawing = null
  strokes.value = []
  drawAll()
}

function close() {
  emit('close')
}

function onKey(e: KeyboardEvent) {
  if (props.open && e.key === 'Escape') close()
}

watch(
  () => props.open,
  async (open) => {
    if (open) {
      document.body.classList.add('scratch-open')
      await nextTick()
      requestAnimationFrame(resize)
    } else {
      document.body.classList.remove('scratch-open')
    }
  },
)

onMounted(() => {
  observer = new ResizeObserver(() => {
    if (props.open) resize()
  })
  if (wrapEl.value) observer.observe(wrapEl.value)
  window.addEventListener('keydown', onKey)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  window.removeEventListener('keydown', onKey)
  document.body.classList.remove('scratch-open')
})

defineExpose({ reset })
</script>

<template>
  <div v-show="open" class="scratch-overlay" @click.self="close">
    <section class="scratch-panel" role="dialog" aria-modal="true" aria-label="草稿纸">
      <header class="scratch-head">
        <div>
          <b>草稿纸</b>
          <span>手写计算 · 离开练习即清空</span>
        </div>
        <button class="scratch-close" @click="close">收起</button>
      </header>

      <div class="scratch-tools">
        <button class="tool-btn" :class="{ on: tool === 'pen' }" @click="tool = 'pen'">笔</button>
        <button class="tool-btn" :class="{ on: tool === 'eraser' }" @click="tool = 'eraser'">橡皮</button>
        <span class="tool-gap" />
        <button class="tool-btn" :disabled="!strokes.length" @click="undo">撤销</button>
        <button class="tool-btn" :disabled="!strokes.length" @click="clearAll">清空</button>
      </div>

      <div ref="wrapEl" class="scratch-canvas-wrap">
        <canvas
          ref="canvasEl"
          class="scratch-canvas"
          aria-label="手写草稿画布"
          @pointerdown="onDown"
          @pointermove="onMove"
          @pointerup="onUp"
          @pointercancel="onUp"
          @pointerleave="onUp"
        />
        <div v-if="!strokes.length" class="scratch-hint">在这里列竖式、算比例、划关键词</div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.scratch-overlay {
  position: fixed;
  inset: 0;
  z-index: 600;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  background: rgba(15, 23, 42, .44);
  backdrop-filter: blur(2px);
}
.scratch-panel {
  display: flex;
  flex-direction: column;
  width: min(760px, 100%);
  height: min(88vh, 760px);
  padding: 14px 16px calc(14px + env(safe-area-inset-bottom, 0px));
  border-radius: 20px 20px 0 0;
  background: var(--card);
  box-shadow: 0 -10px 40px rgba(0, 0, 0, .22);
}
.scratch-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.scratch-head b {
  font-size: 15px;
  color: var(--text);
}
.scratch-head span {
  margin-left: 8px;
  font-size: 11px;
  color: var(--text-3);
}
.scratch-close {
  flex: none;
  padding: 6px 12px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--card);
  color: var(--text-2);
  font-size: 12.5px;
  cursor: pointer;
}
.scratch-close:hover { color: var(--text); }
.scratch-tools {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 12px 0;
}
.tool-btn {
  min-width: 52px;
  height: 32px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--card);
  color: var(--text-2);
  font-size: 12.5px;
  cursor: pointer;
  transition: all .15s ease;
}
.tool-btn:hover:not(:disabled) { color: var(--text); }
.tool-btn.on {
  border-color: var(--primary);
  background: var(--primary-soft);
  color: var(--primary);
  font-weight: 600;
}
.tool-btn:disabled { opacity: .4; cursor: default; }
.tool-gap { flex: 1; }
.scratch-canvas-wrap {
  position: relative;
  flex: 1;
  min-height: 0;
  border: 1px solid var(--border);
  border-radius: 14px;
  overflow: hidden;
  background: #fbfaf6;
  box-shadow: inset 0 1px 3px rgba(0, 0, 0, .04);
}
.scratch-canvas {
  display: block;
  width: 100%;
  height: 100%;
  touch-action: none;
  cursor: crosshair;
}
.scratch-hint {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 20px;
  color: #b9b3a4;
  font-size: 12.5px;
  text-align: center;
  pointer-events: none;
}
</style>

<style>
body.scratch-open {
  overflow: hidden;
}
</style>
