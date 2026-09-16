<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Stage as VStage, Layer as VLayer, Group as VGroup, Line as VLine, Arrow as VArrow, Rect as VRect, Ellipse as VEllipse, Text as VText } from 'vue-konva'
import type { KonvaEventObject } from 'konva/lib/Node'
import type { AnnotationMark, AnnotationTool, AnnotationViewport, Point } from '../annotation-model'
import { labelLayout, labelStyle, pointBox, translatedMark } from '../annotation-model'

const props = defineProps<{
  viewport: AnnotationViewport; marks: AnnotationMark[]; editableIds: string[]
  tool: AnnotationTool; color: string; width: number; text: string; selected: string; visible: boolean
}>()
const emit = defineEmits<{ add: [mark: AnnotationMark]; replace: [mark: AnnotationMark]; select: [id: string]; hint: [message: string] }>()
const host = ref<HTMLElement | null>(null)
const size = ref({ width: 1, height: 1 })
const frame = ref(0)
const draft = ref<AnnotationMark | null>(null)
let observer: ResizeObserver | undefined
let unsubscribe: (() => void) | undefined
let raf = 0
let startScreen: Point | null = null
const panning = ref(false)
let panPointer: Point | null = null
const interactive = computed(() => props.tool !== 'browse')
const layout = computed(() => { frame.value; return props.viewport.layout() })
const displayMarks = computed(() => {
  frame.value
  return [...(props.visible ? props.marks : []), ...(draft.value ? [draft.value] : [])]
    .filter(mark => mark.layout === layout.value).map(mark => ({ mark, points: mark.points.map(p => props.viewport.toScreen(p)) }))
})
function refresh() { cancelAnimationFrame(raf); raf = requestAnimationFrame(() => { frame.value++ }) }
watch(() => props.viewport, viewport => { unsubscribe?.(); unsubscribe = viewport.subscribe(refresh); refresh() }, { immediate: true })
watch(() => props.tool, () => { draft.value = null; startScreen = null })
onMounted(() => {
  observer = new ResizeObserver(entries => { const rect = entries[0]?.contentRect; if (rect) size.value = { width: rect.width, height: rect.height }; refresh() })
  if (host.value) observer.observe(host.value)
  window.addEventListener('pointerup', finish)
  window.addEventListener('pointercancel', cancel)
})
function pointer(event: KonvaEventObject<PointerEvent>): Point | null { return event.target.getStage()?.getPointerPosition() ?? null }
onBeforeUnmount(() => { observer?.disconnect(); unsubscribe?.(); cancelAnimationFrame(raf); endPan(); window.removeEventListener('pointerup', finish); window.removeEventListener('pointercancel', cancel) })
function down(event: KonvaEventObject<PointerEvent>) {
  if (props.tool === 'browse') return
  // 批注层挡住了画布，中键平移需要在覆盖层上自行转发。
  if (event.evt.button === 1) { beginPan(event); return }
  if (event.evt.button !== 0) return
  const p = pointer(event); if (!p) return
  if (props.tool === 'select') { if (event.target === event.target.getStage()) emit('select', ''); return }
  if (props.tool === 'text' && !props.text.trim()) { emit('hint', '先点击右侧话术，或填写批注文字'); return }
  const mark: AnnotationMark = { id: crypto.randomUUID(), kind: props.tool, layout: layout.value, points: [props.viewport.toDrawing(p)], text: props.tool === 'text' ? props.text.trim() : '', color: props.color, width: props.width }
  if (['text', 'check', 'cross'].includes(props.tool)) { emit('add', mark); return }
  mark.points.push({ ...mark.points[0]! }); draft.value = mark; startScreen = p
}
function move(event: KonvaEventObject<PointerEvent>) {
  if (panning.value) return
  const p = pointer(event); if (!draft.value || !p) return
  if (draft.value.kind === 'pen') {
    const last = props.viewport.toScreen(draft.value.points.at(-1)!)
    if (Math.hypot(p.x - last.x, p.y - last.y) < 2 || draft.value.points.length >= 10000) return
    draft.value.points.push(props.viewport.toDrawing(p))
  } else draft.value.points[1] = props.viewport.toDrawing(p)
}
function finish() {
  if (!draft.value) return
  const end = props.viewport.toScreen(draft.value.points.at(-1)!)
  if (startScreen && (draft.value.kind === 'pen' || Math.hypot(end.x - startScreen.x, end.y - startScreen.y) > 3)) emit('add', JSON.parse(JSON.stringify(draft.value)))
  draft.value = null; startScreen = null
}
function cancel() { draft.value = null; startScreen = null; endPan() }
function beginPan(event: KonvaEventObject<PointerEvent>) {
  // 指针压在可拖拽的批注上时把中键让给 Konva，避免与拖动标记冲突。
  if (event.target !== event.target.getStage()) return
  event.evt.preventDefault()
  panPointer = { x: event.evt.clientX, y: event.evt.clientY }
  panning.value = true
  window.addEventListener('pointermove', panMove)
  window.addEventListener('pointerup', endPan)
  window.addEventListener('pointercancel', endPan)
}
function panMove(event: PointerEvent) {
  if (!panPointer) return
  const next = { x: event.clientX, y: event.clientY }
  const dx = next.x - panPointer.x, dy = next.y - panPointer.y
  panPointer = next
  if (dx || dy) props.viewport.pan(dx, dy)
}
function endPan() {
  panPointer = null
  panning.value = false
  window.removeEventListener('pointermove', panMove)
  window.removeEventListener('pointerup', endPan)
  window.removeEventListener('pointercancel', endPan)
}
function shapeStyle(mark: AnnotationMark) { return { stroke: mark.color, strokeWidth: mark.width, lineCap: 'round' as const, lineJoin: 'round' as const, hitStrokeWidth: 14 } }
function ellipse(points: Point[]) { const b = pointBox(points); return { x: b.x + b.width / 2, y: b.y + b.height / 2, radiusX: b.width / 2, radiusY: b.height / 2 } }
function stamp(mark: AnnotationMark, point: Point) {
  return mark.kind === 'check' ? [point.x - 9, point.y, point.x - 2, point.y + 8, point.x + 13, point.y - 11] : [point.x - 9, point.y - 9, point.x + 9, point.y + 9]
}
function select(mark: AnnotationMark) { if (props.tool === 'select') emit('select', mark.id) }
function dragged(mark: AnnotationMark, event: KonvaEventObject<DragEvent>) {
  const origin = props.viewport.toScreen(mark.points[0]!)
  const destination = { x: origin.x + event.target.x(), y: origin.y + event.target.y() }
  event.target.position({ x: 0, y: 0 })
  emit('replace', translatedMark(mark, props.viewport.toDrawing(origin), props.viewport.toDrawing(destination)))
}
function wheel(event: WheelEvent) {
  if (!interactive.value) return
  event.preventDefault()
  if (draft.value || !event.deltaY) return
  // 以鼠标位置为锚点缩放，滚轮放大缩小才不会把图纸拖离视野。
  const rect = host.value?.getBoundingClientRect()
  const anchor = rect ? { x: event.clientX - rect.left, y: event.clientY - rect.top } : undefined
  props.viewport.zoom(event.deltaY < 0 ? 1 : -1, anchor)
}
</script>

<template>
  <div ref="host" class="annotation-layer" :class="{ interactive, drawing: interactive && tool !== 'select', panning }" @wheel="wheel">
    <VStage :config="size" @pointerdown="down" @pointermove="move" @pointerup="finish">
      <VLayer>
        <VGroup v-for="item in displayMarks" :key="item.mark.id" :config="{ draggable: tool === 'select' && editableIds.includes(item.mark.id), listening: tool === 'select' }" @click="select(item.mark)" @tap="select(item.mark)" @dragend="dragged(item.mark, $event)">
          <VRect v-if="selected === item.mark.id" :config="{ x: pointBox(item.points).x - 8, y: pointBox(item.points).y - 8, width: Math.max(24, pointBox(item.points).width + 16), height: Math.max(24, pointBox(item.points).height + 16), stroke: '#72B7FF', strokeWidth: 1, dash: [4, 4], listening: false }" />
          <VLine v-if="item.mark.kind === 'pen'" :config="{ ...shapeStyle(item.mark), points: item.points.flatMap(p => [p.x, p.y]) }" />
          <VArrow v-else-if="item.mark.kind === 'arrow'" :config="{ ...shapeStyle(item.mark), points: item.points.flatMap(p => [p.x, p.y]), fill: item.mark.color, pointerLength: 10, pointerWidth: 8 }" />
          <VRect v-else-if="item.mark.kind === 'rect'" :config="{ ...shapeStyle(item.mark), ...pointBox(item.points) }" />
          <VEllipse v-else-if="item.mark.kind === 'ellipse'" :config="{ ...shapeStyle(item.mark), ...ellipse(item.points) }" />
          <template v-else-if="item.mark.kind === 'check' || item.mark.kind === 'cross'">
            <VLine :config="{ ...shapeStyle(item.mark), points: stamp(item.mark, item.points[0]!) }" />
            <VLine v-if="item.mark.kind === 'cross'" :config="{ ...shapeStyle(item.mark), points: [item.points[0]!.x - 9, item.points[0]!.y + 9, item.points[0]!.x + 9, item.points[0]!.y - 9] }" />
          </template>
          <VText v-if="item.mark.text" :config="{ text: item.mark.text, fill: item.mark.color, fontFamily: 'Microsoft YaHei, sans-serif', wrap: 'char', verticalAlign: 'middle', fontSize: labelStyle.fontSize, lineHeight: labelStyle.lineHeight, padding: labelStyle.padding, ...labelLayout(item.mark.kind, item.points, item.mark.text) }" />
        </VGroup>
      </VLayer>
    </VStage>
  </div>
</template>

<style scoped>
.annotation-layer { position: absolute; inset: 0; pointer-events: none; }
.annotation-layer.interactive { pointer-events: auto; }
.annotation-layer.drawing { cursor: crosshair; touch-action: none; }
.annotation-layer.panning { cursor: grabbing; touch-action: none; }
</style>
