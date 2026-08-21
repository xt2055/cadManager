<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({
  name: 'DrawingPreviewTab',
})

const demoStore = useDemoStore()
const uiStore = useUiStore()
const canvasRef = ref<HTMLElement | null>(null)
const layerPanel = ref(true)
const zoom = ref(1)
const rotation = ref(0)
const offset = ref({ x: 0, y: 0 })
const measuring = ref(false)
const measureStart = ref<{ x: number; y: number } | null>(null)
const measureEnd = ref<{ x: number; y: number } | null>(null)
const dragging = ref(false)
const lastPointer = ref({ x: 0, y: 0 })
const moved = ref(false)
const visibleLayers = ref<Record<string, boolean>>({
  'ly-outline': true,
  'ly-center': true,
  'ly-dim': true,
  'ly-hatch': true,
  'ly-frame': true,
})

const drawing = computed(() => demoStore.currentDrawing ?? demoStore.drawings[0])
const zoomText = computed(() => `${Math.round(zoom.value * 100)}%`)
const transform = computed(() => `translate(${offset.value.x}px, ${offset.value.y}px) rotate(${rotation.value}deg) scale(${zoom.value})`)

function fitView() {
  const canvas = canvasRef.value
  if (!canvas) return
  const rect = canvas.getBoundingClientRect()
  const rotated = rotation.value % 180 !== 0
  const width = rotated ? 660 : 1000
  const height = rotated ? 1000 : 660
  zoom.value = Math.min((rect.width - 70) / width, (rect.height - 70) / height)
  const radians = (rotation.value * Math.PI) / 180
  const sx = zoom.value * 500
  const sy = zoom.value * 330
  offset.value = {
    x: rect.width / 2 - (sx * Math.cos(radians) - sy * Math.sin(radians)),
    y: rect.height / 2 - (sx * Math.sin(radians) + sy * Math.cos(radians)),
  }
}

function zoomAtCenter(factor: number) {
  zoom.value = Math.min(8, Math.max(0.2, zoom.value * factor))
}

function zoomAt(event: WheelEvent) {
  const canvas = canvasRef.value
  if (!canvas) return
  const rect = canvas.getBoundingClientRect()
  const nextZoom = Math.min(8, Math.max(0.2, zoom.value * (event.deltaY < 0 ? 1.15 : 1 / 1.15)))
  if (nextZoom === zoom.value) return
  const radians = (rotation.value * Math.PI) / 180
  const dx = event.clientX - rect.left - offset.value.x
  const dy = event.clientY - rect.top - offset.value.y
  const ux = (dx * Math.cos(radians) + dy * Math.sin(radians)) / zoom.value
  const uy = (-dx * Math.sin(radians) + dy * Math.cos(radians)) / zoom.value
  const sx = nextZoom * ux
  const sy = nextZoom * uy
  offset.value = {
    x: event.clientX - rect.left - (sx * Math.cos(radians) - sy * Math.sin(radians)),
    y: event.clientY - rect.top - (sx * Math.sin(radians) + sy * Math.cos(radians)),
  }
  zoom.value = nextZoom
}

function rotate() {
  rotation.value = (rotation.value + 90) % 360
}

function screenToSvg(event: MouseEvent) {
  const canvas = canvasRef.value
  if (!canvas) return { x: 0, y: 0 }
  const rect = canvas.getBoundingClientRect()
  const radians = (rotation.value * Math.PI) / 180
  const dx = event.clientX - rect.left - offset.value.x
  const dy = event.clientY - rect.top - offset.value.y
  return {
    x: (dx * Math.cos(radians) + dy * Math.sin(radians)) / zoom.value,
    y: (-dx * Math.sin(radians) + dy * Math.cos(radians)) / zoom.value,
  }
}

function toggleMeasure() {
  measuring.value = !measuring.value
  if (!measuring.value) {
    measureStart.value = null
    measureEnd.value = null
  } else {
    uiStore.toast('测量模式：在图纸上点击两点（Esc 退出）· 按 1:2 比例换算', 'info')
  }
}

function handleCanvasClick(event: MouseEvent) {
  if (!measuring.value || moved.value) return
  const point = screenToSvg(event)
  if (!measureStart.value || measureEnd.value) {
    measureStart.value = point
    measureEnd.value = null
    uiStore.toast('已记录测量起点，点击终点完成测量', 'info')
  } else {
    measureEnd.value = point
  }
}

function handlePointerDown(event: PointerEvent) {
  dragging.value = true
  moved.value = false
  lastPointer.value = { x: event.clientX, y: event.clientY }
  canvasRef.value?.setPointerCapture(event.pointerId)
}

function handlePointerMove(event: PointerEvent) {
  if (!dragging.value) return
  const dx = event.clientX - lastPointer.value.x
  const dy = event.clientY - lastPointer.value.y
  if (Math.abs(dx) + Math.abs(dy) > 3) moved.value = true
  offset.value.x += dx
  offset.value.y += dy
  lastPointer.value = { x: event.clientX, y: event.clientY }
}

function handlePointerUp(event: PointerEvent) {
  dragging.value = false
  canvasRef.value?.releasePointerCapture(event.pointerId)
}

function toggleFullscreen() {
  const viewer = document.querySelector('.viewer')
  if (!viewer) return
  if (!document.fullscreenElement) {
    void viewer.requestFullscreen?.()
  } else {
    void document.exitFullscreen?.()
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && measuring.value) toggleMeasure()
}

onMounted(async () => {
  window.addEventListener('keydown', handleKeydown)
  await nextTick()
  fitView()
})

watch(() => demoStore.currentDrawing?.no, () => {
  void nextTick().then(fitView)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div class="viewer">
    <div class="v-toolbar">
      <div class="v-group">
        <button class="v-btn" :class="{ on: layerPanel }" type="button" title="图层面板" @click="layerPanel = !layerPanel"><DemoIcon name="layers" :size="15" /></button>
      </div>
      <div class="v-group">
        <button class="v-btn" type="button" title="缩小" @click="zoomAtCenter(1 / 1.3)"><DemoIcon name="zoom-out" :size="15" /></button>
        <span class="v-zoomlbl">{{ zoomText }}</span>
        <button class="v-btn" type="button" title="放大" @click="zoomAtCenter(1.3)"><DemoIcon name="zoom-in" :size="15" /></button>
        <button class="v-btn" type="button" title="适合窗口" @click="fitView"><DemoIcon name="maximize" :size="15" /></button>
        <button class="v-btn" type="button" title="旋转 90°" @click="rotate"><DemoIcon name="rotate-cw" :size="15" /></button>
      </div>
      <div class="v-group">
        <button class="v-btn" :class="{ on: measuring }" type="button" title="测量" @click="toggleMeasure"><DemoIcon name="ruler" :size="15" /></button>
      </div>
      <div class="v-group last-group">
        <button class="v-btn" type="button" title="下载原始文件" @click="uiStore.toast('当前图纸暂无可下载文件', 'info')"><DemoIcon name="download" :size="15" /></button>
        <button class="v-btn" type="button" title="打印" @click="uiStore.toast('已发送至打印服务（Demo）', 'info')"><DemoIcon name="printer" :size="15" /></button>
        <button class="v-btn" type="button" title="全屏" @click="toggleFullscreen"><DemoIcon name="maximize-2" :size="15" /></button>
      </div>
    </div>

    <div class="v-main">
      <aside class="layer-panel" :class="{ hidden: !layerPanel }">
        <div class="lp-title">LAYERS · 图层</div>
        <label v-for="item in [
          ['ly-outline', '轮廓线', 'var(--cad-line)'],
          ['ly-center', '中心线', 'var(--cad-center)'],
          ['ly-dim', '标注', 'var(--cad-dim)'],
          ['ly-hatch', '剖面线', 'var(--cad-hatch)'],
          ['ly-frame', '图框', 'var(--cad-frame)'],
        ]" :key="item[0]" class="layer-item">
          <input v-model="visibleLayers[item[0]]" type="checkbox" />
          <span>{{ item[1] }}</span>
          <span class="ly-swatch" :style="{ background: item[2] }"></span>
        </label>
        <div class="lp-title layer-view-title">视图</div>
        <label class="layer-item"><input v-model="visibleLayers['ly-outline']" type="checkbox" /><span>主视图 + 剖视</span></label>
      </aside>

      <div
        ref="canvasRef"
        class="v-canvas"
        :class="{ dragging, measuring }"
        @wheel.prevent="zoomAt"
        @pointerdown="handlePointerDown"
        @pointermove="handlePointerMove"
        @pointerup="handlePointerUp"
        @pointercancel="handlePointerUp"
        @click="handleCanvasClick"
      >
        <div class="cad-stage" :style="{ transform }">
          <CadDrawing :visible-layers="visibleLayers" :measure-start="measureStart" :measure-end="measureEnd" />
        </div>
        <div class="v-scanline"></div>
        <div class="v-hud">{{ drawing?.no }} · {{ drawing?.ver }} · 比例 1:2 · A2</div>
      </div>
    </div>

    <div class="v-info">
      <span class="fi"><DemoIcon name="file" :size="13" />{{ drawing?.no }} · {{ drawing?.ver }}</span>
      <span class="fi"><DemoIcon name="refresh-cw" :size="13" />等待文件转换</span>
      <span class="fi"><DemoIcon name="hard-drive" :size="13" />文件大小待获取</span>
      <span class="fi"><DemoIcon name="clock" :size="13" />上传信息待获取</span>
      <span class="fi hash-ok"><DemoIcon name="shield-check" :size="13" />校验信息待获取</span>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, h, type PropType } from 'vue'

import { useDemoStore as useCadDemoStore } from '@/stores/demo.store'

export const CadDrawing = defineComponent({
  name: 'CadDrawing',
  props: {
    visibleLayers: { type: Object, required: true },
    measureStart: { type: Object as PropType<{ x: number; y: number } | null>, default: null },
    measureEnd: { type: Object as PropType<{ x: number; y: number } | null>, default: null },
  },
  setup(props) {
    const demoStore = useCadDemoStore()
    const holes = Array.from({ length: 8 }, (_, index) => {
      const angle = ((22.5 + index * 45) * Math.PI) / 180
      return [250 + 85 * Math.cos(angle), 280 + 85 * Math.sin(angle)]
    })

    return () => h('svg', { id: 'cadSvg', width: 1000, height: 660, viewBox: '0 0 1000 660' }, [
      h('defs', [
        h('marker', { id: 'arr', viewBox: '0 0 10 10', refX: 9.5, refY: 5, markerWidth: 7.5, markerHeight: 7.5, orient: 'auto-start-reverse' }, [h('path', { class: 'm-arr', d: 'M0,0 L10,5 L0,10 z' })]),
        h('pattern', { id: 'hatch', width: 8, height: 8, patternTransform: 'rotate(45)', patternUnits: 'userSpaceOnUse' }, [h('line', { class: 'h-line', x1: 0, y1: 0, x2: 0, y2: 8 })]),
      ]),
      h('g', { id: 'ly-frame', style: { display: props.visibleLayers['ly-frame'] ? '' : 'none' } }, [
        h('rect', { class: 'fline', x: 8, y: 8, width: 984, height: 644, 'stroke-width': 1 }),
        h('rect', { class: 'fline', x: 26, y: 26, width: 948, height: 608, 'stroke-width': 2.2 }),
        h('g', { class: 'fline', 'stroke-width': 1 }, [h('rect', { x: 560, y: 494, width: 414, height: 120 }), h('line', { x1: 560, y1: 524, x2: 974, y2: 524 }), h('line', { x1: 560, y1: 554, x2: 974, y2: 554 }), h('line', { x1: 560, y1: 584, x2: 974, y2: 584 }), h('line', { x1: 740, y1: 524, x2: 740, y2: 614 })]),
        h('text', { class: 'txt', x: 767, y: 515, 'font-size': 15, 'text-anchor': 'middle', 'font-weight': 700, style: { fontFamily: 'Noto Sans SC' } }, demoStore.currentDrawing?.name ?? 'CAD 图纸预览'),
        h('text', { class: 'txt-lbl', x: 570, y: 543 }, '图号'), h('text', { class: 'txt', x: 598, y: 543 }, demoStore.currentDrawing?.no ?? ''), h('text', { class: 'txt-lbl', x: 750, y: 543 }, '比例'), h('text', { class: 'txt', x: 778, y: 543 }, '—'),
        h('text', { class: 'txt-lbl', x: 570, y: 573 }, '设计'), h('text', { class: 'txt', x: 598, y: 573 }, '—'), h('text', { class: 'txt-lbl', x: 750, y: 573 }, '校对'), h('text', { class: 'txt', x: 778, y: 573 }, '—'),
        h('text', { class: 'txt-lbl', x: 570, y: 603 }, '材料'), h('text', { class: 'txt', x: 598, y: 603 }, demoStore.currentDrawing?.material ?? ''), h('text', { class: 'txt-lbl', x: 750, y: 603 }, '批准'), h('text', { class: 'txt', x: 778, y: 603 }, '—'),
      ]),
      h('g', { id: 'ly-hatch', style: { display: props.visibleLayers['ly-hatch'] ? '' : 'none' } }, [h('path', { d: 'M550,140 H830 V420 H550 Z M590,180 H790 V420 H590 Z', fill: 'url(#hatch)', 'fill-rule': 'evenodd' })]),
      h('g', { id: 'ly-center', style: { display: props.visibleLayers['ly-center'] ? '' : 'none' } }, [h('line', { class: 'ctr', x1: 115, y1: 280, x2: 425, y2: 280 }), h('line', { class: 'ctr', x1: 250, y1: 115, x2: 250, y2: 445 }), h('circle', { class: 'ctr', cx: 250, cy: 280, r: 85 }), h('line', { class: 'ctr', x1: 535, y1: 280, x2: 845, y2: 280 }), h('line', { class: 'ctr', x1: 690, y1: 125, x2: 690, y2: 435 }), h('line', { class: 'ctr', x1: 605, y1: 130, x2: 605, y2: 190 }), h('line', { class: 'ctr', x1: 775, y1: 130, x2: 775, y2: 190 })]),
      h('g', { id: 'ly-outline', style: { display: props.visibleLayers['ly-outline'] ? '' : 'none' } }, [h('circle', { class: 'ln thick', cx: 250, cy: 280, r: 140 }), h('circle', { class: 'ln thick', cx: 250, cy: 280, r: 115 }), h('circle', { class: 'ln thick', cx: 250, cy: 280, r: 55 }), ...holes.map(([x, y]) => h('circle', { class: 'ln thin', cx: x, cy: y, r: 9 })), h('rect', { class: 'ln thick', x: 550, y: 140, width: 280, height: 280 }), h('path', { class: 'ln thick', d: 'M590,180 H790 V420' }), h('rect', { class: 'hole-fill', x: 596, y: 140, width: 18, height: 40 }), h('rect', { class: 'hole-fill', x: 766, y: 140, width: 18, height: 40 })]),
      h('g', { id: 'ly-dim', style: { display: props.visibleLayers['ly-dim'] ? '' : 'none' } }, [
        h('line', { class: 'dim', x1: 151, y1: 181, x2: 100, y2: 128, 'marker-end': 'url(#arr)' }), h('line', { class: 'dim', x1: 100, y1: 128, x2: 62, y2: 128 }), h('text', { class: 'txt', x: 62, y: 121 }, 'Φ280'),
        h('line', { class: 'dim', x1: 331, y1: 199, x2: 386, y2: 146, 'marker-end': 'url(#arr)' }), h('line', { class: 'dim', x1: 386, y1: 146, x2: 420, y2: 146 }), h('text', { class: 'txt', x: 384, y: 139 }, 'Φ230 h9'),
        h('line', { class: 'dim', x1: 329, y1: 313, x2: 392, y2: 382, 'marker-end': 'url(#arr)' }), h('line', { class: 'dim', x1: 392, y1: 382, x2: 432, y2: 382 }), h('text', { class: 'txt', x: 384, y: 402 }, '8×Φ18 均布'),
        h('line', { class: 'dim', x1: 289, y1: 319, x2: 336, y2: 418, 'marker-end': 'url(#arr)' }), h('text', { class: 'txt', x: 300, y: 440 }, 'Φ110'),
        h('line', { class: 'dim', x1: 550, y1: 140, x2: 518, y2: 140 }), h('line', { class: 'dim', x1: 550, y1: 420, x2: 518, y2: 420 }), h('line', { class: 'dim', x1: 524, y1: 140, x2: 524, y2: 420, 'marker-start': 'url(#arr)', 'marker-end': 'url(#arr)' }), h('text', { class: 'txt', x: 516, y: 284, transform: 'rotate(-90 516 284)', 'text-anchor': 'middle' }, '280'),
        h('line', { class: 'dim', x1: 550, y1: 118, x2: 830, y2: 118, 'marker-start': 'url(#arr)', 'marker-end': 'url(#arr)' }), h('text', { class: 'txt', x: 690, y: 111, 'text-anchor': 'middle' }, '280'),
        h('line', { class: 'dim', x1: 830, y1: 140, x2: 858, y2: 140 }), h('line', { class: 'dim', x1: 830, y1: 180, x2: 858, y2: 180 }), h('line', { class: 'dim', x1: 852, y1: 140, x2: 852, y2: 180, 'marker-start': 'url(#arr)', 'marker-end': 'url(#arr)' }), h('text', { class: 'txt', x: 862, y: 164 }, '40'),
        h('line', { class: 'dim', x1: 790, y1: 300, x2: 812, y2: 300 }), h('text', { class: 'txt', x: 806, y: 294 }, '20'), h('line', { class: 'dim', x1: 590, y1: 440, x2: 790, y2: 440, 'marker-start': 'url(#arr)', 'marker-end': 'url(#arr)' }), h('text', { class: 'txt', x: 690, y: 458, 'text-anchor': 'middle' }, '200'),
        h('text', { class: 'txt-lbl tech-title', x: 46, y: 480 }, '技术要求'), h('text', { class: 'txt-lbl', x: 46, y: 500 }, '待加载真实图纸文件'),
      ]),
      props.measureStart ? h('g', { class: 'measure-layer' }, [
        h('circle', { cx: props.measureStart.x, cy: props.measureStart.y, r: 5, fill: 'var(--accent)', stroke: 'var(--cad-bg)', 'stroke-width': 2 }),
        props.measureEnd ? h('g', [h('line', { x1: props.measureStart.x, y1: props.measureStart.y, x2: props.measureEnd.x, y2: props.measureEnd.y, stroke: 'var(--accent)', 'stroke-width': 1.6, 'stroke-dasharray': '7 4' }), h('circle', { cx: props.measureEnd.x, cy: props.measureEnd.y, r: 5, fill: 'var(--accent)', stroke: 'var(--cad-bg)', 'stroke-width': 2 })]) : null,
      ]) : null,
    ])
  },
})

export default CadDrawing
</script>

<style scoped>
.viewer {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--panel);
  box-shadow: var(--shadow);
}

.v-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 9px 13px;
  border-bottom: 1px solid var(--line);
  flex-wrap: wrap;
}

.v-group {
  display: flex;
  align-items: center;
  gap: 3px;
  margin-right: 6px;
  padding: 0 8px 0 0;
  border-right: 1px solid var(--line);
}

.v-group.last-group {
  margin-left: auto;
  margin-right: 0;
  padding-right: 0;
  border-right: none;
}

.v-btn {
  display: grid;
  width: 31px;
  height: 31px;
  place-items: center;
  border-radius: 8px;
  color: var(--text-2);
  transition: all 0.2s;
}

.v-btn:hover,
.v-btn.on {
  background: var(--active);
  color: var(--accent);
}

html[data-skin='tech'] .v-btn.on {
  box-shadow: inset 0 0 8px var(--glow);
}

.v-zoomlbl {
  min-width: 44px;
  color: var(--text-2);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
  text-align: center;
}

.v-main {
  display: flex;
  position: relative;
  flex: 1;
  min-height: 0;
}

.layer-panel {
  width: 172px;
  flex: none;
  padding: 13px 12px;
  overflow-y: auto;
  border-right: 1px solid var(--line);
  transition: width 0.3s, padding 0.3s;
}

.layer-panel.hidden {
  width: 0;
  padding: 0;
  overflow: hidden;
  border-right: none;
}

.lp-title {
  padding: 0 6px 9px;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  letter-spacing: 2px;
}

.layer-view-title {
  margin-top: 14px;
}

.layer-item {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 8px 9px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12.5px;
  transition: background 0.18s;
  user-select: none;
}

.layer-item:hover {
  background: var(--hover);
}

.layer-item span:nth-child(2) {
  flex: 1;
}

.ly-swatch {
  width: 16px;
  height: 4px;
  flex: none;
  border-radius: 99px;
}

.v-canvas {
  position: relative;
  flex: 1;
  overflow: hidden;
  background: var(--cad-bg);
  cursor: grab;
  touch-action: none;
  user-select: none;
}

.v-canvas.dragging {
  cursor: grabbing;
}

.v-canvas.measuring {
  cursor: crosshair;
}

.cad-stage {
  position: absolute;
  top: 0;
  left: 0;
  width: 1000px;
  height: 660px;
  transform-origin: 0 0;
}

.v-hud {
  position: absolute;
  bottom: 11px;
  left: 13px;
  color: var(--cad-dim);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  opacity: 0.85;
  pointer-events: none;
}

.v-scanline {
  position: absolute;
  right: 0;
  left: 0;
  height: 90px;
  background: linear-gradient(180deg, transparent, var(--accent-soft), transparent);
  pointer-events: none;
  animation: scan-move 5.5s linear infinite;
}

html[data-skin='elegant'] .v-scanline,
html[data-theme='light'][data-skin='tech'] .v-scanline {
  display: none;
}

@keyframes scan-move {
  from { top: -90px; }
  to { top: 100%; }
}

.v-info {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 9px 16px;
  border-top: 1px solid var(--line);
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  flex-wrap: wrap;
}

.fi {
  display: flex;
  align-items: center;
  gap: 6px;
}

.fi svg {
  color: var(--accent);
}

.hash-ok {
  margin-left: auto;
  color: var(--ok);
}

:deep(#cadSvg) {
  display: block;
}

:deep(#cadSvg .ln) { stroke: var(--cad-line); fill: none; stroke-linecap: round; }
:deep(#cadSvg .thick) { stroke-width: 2.1; }
:deep(#cadSvg .thin) { stroke-width: 0.95; }
:deep(#cadSvg .ctr) { stroke: var(--cad-center); fill: none; stroke-width: 0.9; stroke-dasharray: 15 4 3 4; }
:deep(#cadSvg .dim) { stroke: var(--cad-dim); fill: none; stroke-width: 0.8; }
:deep(#cadSvg .txt) { fill: var(--cad-text); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
:deep(#cadSvg .txt-lbl) { fill: var(--cad-dim); font-family: 'Noto Sans SC', sans-serif; font-size: 9.5px; }
:deep(#cadSvg .fline) { stroke: var(--cad-frame); fill: none; }
:deep(#cadSvg .m-arr) { fill: var(--cad-dim); stroke: none; }
:deep(#cadSvg .h-line) { stroke: var(--cad-hatch); stroke-width: 0.75; }
:deep(#cadSvg .hole-fill) { fill: var(--cad-bg); stroke: var(--cad-line); stroke-width: 1; }
:deep(#cadSvg .tech-title) { font-size: 12px; font-weight: 700; }

@media (max-width: 760px) {
  .layer-panel { width: 136px; }
  .v-info { gap: 8px; padding: 7px 10px; font-size: 9px; }
  .hash-ok { margin-left: 0; }
}
</style>
