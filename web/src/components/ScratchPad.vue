<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

type Tool = 'pen' | 'eraser'
type Mode = 'sheet' | 'clear'
type ColorId = 'ink' | 'red' | 'blue' | 'green'
interface Stroke {
  tool: Tool
  color: ColorId
  width: number
  points: Array<[number, number]>
}

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const PALETTE: Array<{ id: ColorId; name: string; light: string; dark: string }> = [
  { id: 'ink', name: '墨黑', light: '#1f2a3c', dark: '#e9eef8' },
  { id: 'red', name: '红色', light: '#dc3d43', dark: '#ff7b7b' },
  { id: 'blue', name: '蓝色', light: '#2563eb', dark: '#7aa7ff' },
  { id: 'green', name: '绿色', light: '#157f3c', dark: '#5fd28a' },
]
const PEN_WIDTH = 2.4
const ERASER_WIDTH = 18
const MODE_KEY = 'kg_scratch_mode'
const COLOR_KEY = 'kg_scratch_color'

const canvasEl = ref<HTMLCanvasElement | null>(null)
const wrapEl = ref<HTMLElement | null>(null)
const tool = ref<Tool>('pen')
const mode = ref<Mode>(readMode())
const color = ref<ColorId>(readColor())
const paletteOpen = ref(false)
const strokes = ref<Stroke[]>([])
let drawing: Stroke | null = null
let observer: ResizeObserver | null = null

function readMode(): Mode {
  try {
    return localStorage.getItem(MODE_KEY) === 'clear' ? 'clear' : 'sheet'
  } catch {
    return 'sheet'
  }
}

function readColor(): ColorId {
  try {
    const saved = localStorage.getItem(COLOR_KEY) as ColorId | null
    return PALETTE.some((c) => c.id === saved) ? (saved as ColorId) : 'ink'
  } catch {
    return 'ink'
  }
}

function pickColor(id: ColorId) {
  color.value = id
  paletteOpen.value = false
  try {
    localStorage.setItem(COLOR_KEY, id)
  } catch {
    /* optional */
  }
}

const activeColor = computed(() => PALETTE.find((c) => c.id === color.value) || PALETTE[0])
const swatch = computed(() => inkOf(activeColor.value.id))

function toggleMode() {
  paletteOpen.value = false
  mode.value = mode.value === 'sheet' ? 'clear' : 'sheet'
  try {
    localStorage.setItem(MODE_KEY, mode.value)
  } catch {
    /* optional */
  }
  nextTick(() => requestAnimationFrame(resize))
}

function ctxOf() {
  return canvasEl.value?.getContext('2d') || null
}

function cssSize() {
  const el = wrapEl.value
  return el ? { w: el.clientWidth, h: el.clientHeight } : { w: 0, h: 0 }
}

function isDark() {
  return document.documentElement.getAttribute('data-theme') === 'dark'
}

function inkOf(id: ColorId) {
  // 纸面弹层始终是浅色纸；只有深色主题的半透明模式才用浅色笔迹
  const c = PALETTE.find((x) => x.id === id) || PALETTE[0]
  return isDark() && mode.value === 'clear' ? c.dark : c.light
}

function paint(ctx: CanvasRenderingContext2D, s: Stroke) {
  const pts = s.points
  if (!pts.length) return
  ctx.globalCompositeOperation = s.tool === 'eraser' ? 'destination-out' : 'source-over'
  ctx.strokeStyle = inkOf(s.color)
  ctx.fillStyle = inkOf(s.color)
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
    color: color.value,
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
  ctx.globalCompositeOperation = drawing.tool === 'eraser' ? 'destination-out' : 'source-over'
  ctx.strokeStyle = inkOf(drawing.color)
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
  paletteOpen.value = false
  if (!strokes.value.length) return
  strokes.value.pop()
  drawAll()
}

function clearAll() {
  paletteOpen.value = false
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
  paletteOpen.value = false
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
      paletteOpen.value = false
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
  <div v-show="open" class="scratch-overlay" :class="`mode-${mode}`" @click.self="close">
    <section class="scratch-panel" :class="`mode-${mode}`" role="dialog" aria-modal="true" aria-label="草稿纸">
      <header class="scratch-head">
        <div class="scratch-title">
          <b>草稿纸</b>
          <span class="scratch-sub">手写计算 · 离开练习即清空</span>
        </div>
        <div class="scratch-head-actions">
          <button
            class="mode-btn"
            :title="mode === 'sheet' ? '改为半透明覆盖，能看见题目' : '改为纸面弹层，遮挡页面'"
            @click="toggleMode"
          >
            {{ mode === 'sheet' ? '半透明' : '纸面' }}
          </button>
          <button class="scratch-close" @click="close">收起</button>
        </div>
      </header>

      <div class="scratch-tools">
        <button class="tool-btn" :class="{ on: tool === 'pen' }" @click="tool = 'pen'; paletteOpen = false">笔</button>
        <button class="tool-btn" :class="{ on: tool === 'eraser' }" @click="tool = 'eraser'; paletteOpen = false">橡皮</button>
        <span class="tool-gap" />
        <button
          class="tool-btn color-btn"
          :class="{ on: paletteOpen }"
          title="笔迹颜色"
          aria-label="笔迹颜色"
          :aria-expanded="paletteOpen"
          @click="paletteOpen = !paletteOpen"
        >
          <span class="swatch-dot" :style="{ background: swatch }" />
        </button>
        <button class="tool-btn" :disabled="!strokes.length" @click="undo">撤销</button>
        <button class="tool-btn" :disabled="!strokes.length" @click="clearAll">清空</button>
        <div v-if="paletteOpen" class="color-pop" role="menu" aria-label="笔迹颜色">
          <button
            v-for="c in PALETTE"
            :key="c.id"
            class="color-swatch-btn"
            :class="{ on: color === c.id }"
            :style="{ background: inkOf(c.id) }"
            :title="c.name"
            :aria-label="c.name"
            :aria-pressed="color === c.id"
            @click="pickColor(c.id)"
          />
        </div>
      </div>
      <div v-if="paletteOpen" class="color-mask" @click="paletteOpen = false" />

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
  position: relative;
  z-index: 3;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.scratch-title {
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.scratch-head b {
  font-size: 15px;
  color: var(--text);
  white-space: nowrap;
}
.scratch-sub {
  margin-left: 8px;
  font-size: 11px;
  color: var(--text-3);
}
.scratch-head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}
.mode-btn {
  flex: none;
  padding: 6px 11px;
  border: 1px solid var(--primary);
  border-radius: 999px;
  background: var(--primary-soft);
  color: var(--primary);
  font-size: 12.5px;
  font-weight: 600;
  white-space: nowrap;
  cursor: pointer;
}
.mode-btn:hover { background: var(--primary); color: #fff; }
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
  position: relative;
  z-index: 3;
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
.color-btn {
  min-width: 44px;
  padding: 0 10px;
}
.swatch-dot {
  display: inline-block;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  box-shadow: inset 0 0 0 1px rgba(15, 23, 42, .18);
  vertical-align: middle;
}
.color-pop {
  position: absolute;
  top: calc(100% + 8px);
  left: 50%;
  z-index: 5;
  display: flex;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 14px;
  background: var(--card);
  box-shadow: 0 10px 30px rgba(15, 23, 42, .18);
  transform: translateX(-50%);
}
.color-swatch-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  border: 2px solid transparent;
  border-radius: 50%;
  box-shadow: inset 0 0 0 1px rgba(15, 23, 42, .14);
  cursor: pointer;
}
.color-swatch-btn.on {
  border-color: var(--card);
  box-shadow: 0 0 0 2px var(--primary);
}
.color-mask {
  position: fixed;
  inset: 0;
  z-index: 2;
}
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

/* 半透明覆盖：整页铺满，能看见下面的题目，工具栏和抬头浮在画布上 */
.scratch-overlay.mode-clear {
  background: transparent;
  backdrop-filter: none;
}
.scratch-panel.mode-clear {
  position: relative;
  width: 100%;
  max-width: none;
  height: 100%;
  padding: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}
.scratch-panel.mode-clear .scratch-head {
  position: absolute;
  top: calc(10px + env(safe-area-inset-top, 0px));
  left: 12px;
  right: 12px;
  z-index: 3;
  padding: 9px 10px 9px 14px;
  border-radius: 14px;
  background: var(--card);
  box-shadow: 0 6px 22px rgba(15, 23, 42, .14);
}
.scratch-panel.mode-clear .scratch-tools {
  position: absolute;
  left: 50%;
  bottom: calc(14px + env(safe-area-inset-bottom, 0px));
  z-index: 3;
  margin: 0;
  padding: 6px;
  border-radius: 12px;
  background: var(--card);
  box-shadow: 0 6px 22px rgba(15, 23, 42, .14);
  transform: translateX(-50%);
}
.scratch-panel.mode-clear .color-pop {
  top: auto;
  bottom: calc(100% + 8px);
}
.scratch-panel.mode-clear .scratch-canvas-wrap {
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}
.scratch-panel.mode-clear .scratch-hint { color: rgba(80, 74, 62, .5); }
/* 深色主题下垫一层深色玻璃，笔迹改用浅色，保证两边都看得清 */
html[data-theme='dark'] .scratch-panel.mode-clear .scratch-canvas-wrap {
  background: rgba(10, 15, 24, .58);
}
html[data-theme='dark'] .scratch-panel.mode-clear .scratch-hint { color: rgba(226, 230, 240, .42); }
</style>

<style>
body.scratch-open {
  overflow: hidden;
}
</style>
