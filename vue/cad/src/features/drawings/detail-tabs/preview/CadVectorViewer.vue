<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import DxfParser from 'dxf-parser'
import DemoIcon from '@/components/common/DemoIcon.vue'

interface Props {
  dxfUrl?: string | null
  fileName?: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'layers-loaded', layers: Array<{ name: string; color: string; visible: boolean }>): void
  (e: 'zoom-change', zoom: number): void
}>()

const containerRef = ref<HTMLDivElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)

const loading = ref(false)
const loadingStage = ref('正在准备 CAD 画布...')
const loadingProgress = ref(0)
const errorMessage = ref('')

// CAD 图形数据
let parsedDxf: any = null
const layerMap = ref<Map<string, { name: string; color: string; visible: boolean }>>(new Map())

// 视图变换状态 (CAD 坐标 -> Canvas 屏幕坐标)
const viewCenter = ref({ x: 0, y: 0 })
const viewScale = ref(1) // 像素 / CAD 单位
const defaultScale = ref(1)

let isDragging = false
let dragStartPos = { x: 0, y: 0 }
let dragStartCenter = { x: 0, y: 0 }
let resizeObserver: ResizeObserver | null = null

// ACI 颜色表
const ACI_COLORS: Record<number, string> = {
  1: '#ff0000', // 红
  2: '#ffff00', // 黄
  3: '#00ff00', // 绿
  4: '#00ffff', // 青
  5: '#0000ff', // 蓝
  6: '#ff00ff', // 洋红
  7: '#ffffff', // 白
  8: '#808080',
  9: '#c0c0c0',
}

function getLayerColor(layerName: string): string {
  const layer = parsedDxf?.tables?.layer?.layers?.[layerName]
  if (!layer) return '#00f3ff'
  if (layer.colorNumber !== undefined && ACI_COLORS[layer.colorNumber]) {
    return ACI_COLORS[layer.colorNumber] || '#00f3ff'
  }
  if (layer.color !== undefined && layer.color !== 0) {
    if (ACI_COLORS[layer.color]) return ACI_COLORS[layer.color] || '#00f3ff'
    return '#' + (layer.color & 0xffffff).toString(16).padStart(6, '0')
  }
  return '#00f3ff'
}

function getEntityColor(entity: any, inheritedColor?: string): string {
  if (entity.color !== undefined && entity.color !== 0 && entity.color !== 256) {
    if (ACI_COLORS[entity.color]) return ACI_COLORS[entity.color] || '#00f3ff'
    return '#' + (entity.color & 0xffffff).toString(16).padStart(6, '0')
  }
  if (entity.colorIndex !== undefined && entity.colorIndex !== 0 && entity.colorIndex !== 256) {
    if (ACI_COLORS[entity.colorIndex]) return ACI_COLORS[entity.colorIndex] || '#00f3ff'
  }
  if (inheritedColor) return inheritedColor
  return getLayerColor(entity.layer || '0')
}

// 2D 仿射变换类
class Transform2D {
  a = 1; b = 0; c = 0; d = 1; tx = 0; ty = 0;
  constructor(tx = 0, ty = 0, sx = 1, sy = 1, rotationDeg = 0) {
    const rad = (rotationDeg * Math.PI) / 180
    const cos = Math.cos(rad)
    const sin = Math.sin(rad)
    this.a = cos * sx
    this.b = sin * sx
    this.c = -sin * sy
    this.d = cos * sy
    this.tx = tx
    this.ty = ty
  }

  multiply(t: Transform2D): Transform2D {
    const res = new Transform2D()
    res.a = this.a * t.a + this.c * t.b
    res.b = this.b * t.a + this.d * t.b
    res.c = this.a * t.c + this.c * t.d
    res.d = this.b * t.c + this.d * t.d
    res.tx = this.a * t.tx + this.c * t.ty + this.tx
    res.ty = this.b * t.tx + this.d * t.ty + this.ty
    return res
  }

  apply(pt: { x: number; y: number }): { x: number; y: number } {
    return {
      x: this.a * pt.x + this.c * pt.y + this.tx,
      y: this.b * pt.x + this.d * pt.y + this.ty,
    }
  }
}

// 文字格式清洗并按换行拆分
function parseCadTextLines(raw: string): string[] {
  if (!raw) return []
  const formatted = raw
    .replace(/\\P/gi, '\n')
    .replace(/%%C/gi, 'Φ').replace(/%C/gi, 'Φ')
    .replace(/%%D/gi, '°').replace(/%D/gi, '°')
    .replace(/%%P/gi, '±').replace(/%P/gi, '±')
    // 提取公差上下标格式如 {\D\H0.7x;+0.1^+0.2|a;} -> (+0.1 / +0.2)
    .replace(/\{\\D\\H[0-9.]+x;([^|}]+)\^([^|}]+)\|a;\}/gi, '($1/$2)')
    .replace(/\\A[0-9];/gi, '')
    .replace(/\\[A-Za-z][^;]*;/g, '')
    .replace(/[{}]/g, '')

  return formatted.split('\n').map(l => l.trim()).filter(l => l.length > 0)
}

// 绘制主渲染逻辑
function redraw() {
  const canvas = canvasRef.value
  if (!canvas || !parsedDxf) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const width = canvas.width
  const height = canvas.height

  // 1. 清空背景 (现代深色 CAD 工业蓝灰背景)
  ctx.fillStyle = '#12151c'
  ctx.fillRect(0, 0, width, height)

  // 绘制工程网格背景
  ctx.lineWidth = 1
  ctx.strokeStyle = '#1a202c'
  const gridSize = 50 * viewScale.value
  if (gridSize > 20 && gridSize < 300) {
    const offsetX = (width / 2 - viewCenter.value.x * viewScale.value) % gridSize
    const offsetY = (height / 2 + viewCenter.value.y * viewScale.value) % gridSize
    ctx.beginPath()
    for (let x = offsetX; x < width; x += gridSize) {
      ctx.moveTo(x, 0); ctx.lineTo(x, height)
    }
    for (let y = offsetY; y < height; y += gridSize) {
      ctx.moveTo(0, y); ctx.lineTo(width, y)
    }
    ctx.stroke()
  }

  // CAD -> 屏幕坐标转换函数 (CAD Y 轴向上，Canvas Y 轴向下)
  const toScreen = (pt: { x: number; y: number }) => {
    return {
      x: (pt.x - viewCenter.value.x) * viewScale.value + width / 2,
      y: -(pt.y - viewCenter.value.y) * viewScale.value + height / 2,
    }
  }

  // 递归渲染实体
  function drawEntities(entities: any[], transform: Transform2D, inheritedLayer?: string, inheritedColor?: string) {
    if (!ctx) return
    for (const e of entities) {
      const layerName = e.layer || inheritedLayer || '0'
      const layerConfig = layerMap.value.get(layerName)
      if (layerConfig && !layerConfig.visible) {
        continue // 图层已关闭
      }

      const color = getEntityColor(e, inheritedColor)
      ctx.strokeStyle = color
      ctx.fillStyle = color
      ctx.lineWidth = Math.max(1, (e.lineweight || 1) * 0.05 * viewScale.value)

      if (e.type === 'LINE') {
        if (e.vertices && e.vertices.length >= 2) {
          const p1 = toScreen(transform.apply(e.vertices[0]))
          const p2 = toScreen(transform.apply(e.vertices[1]))
          ctx.beginPath()
          ctx.moveTo(p1.x, p1.y)
          ctx.lineTo(p2.x, p2.y)
          ctx.stroke()
        }
      } else if (e.type === 'LWPOLYLINE' || e.type === 'POLYLINE') {
        if (e.vertices && e.vertices.length > 1) {
          ctx.beginPath()
          const first = toScreen(transform.apply(e.vertices[0]))
          ctx.moveTo(first.x, first.y)
          for (let i = 1; i < e.vertices.length; i++) {
            const pt = toScreen(transform.apply(e.vertices[i]))
            ctx.lineTo(pt.x, pt.y)
          }
          if (e.shape) {
            ctx.lineTo(first.x, first.y)
          }
          ctx.stroke()
        }
      } else if (e.type === 'CIRCLE') {
        if (e.center && e.radius) {
          const center = toScreen(transform.apply(e.center))
          const r = e.radius * viewScale.value * Math.abs(transform.a)
          ctx.beginPath()
          ctx.arc(center.x, center.y, r, 0, Math.PI * 2)
          ctx.stroke()
        }
      } else if (e.type === 'ARC') {
        if (e.center && e.radius) {
          const center = toScreen(transform.apply(e.center))
          const r = e.radius * viewScale.value * Math.abs(transform.a)
          // 修正 ARC 弧度：dxf-parser 输出的 startAngle/endAngle 为弧度
          const startAngle = -(e.endAngle || 0)
          const endAngle = -(e.startAngle || 0)
          ctx.beginPath()
          ctx.arc(center.x, center.y, r, startAngle, endAngle, false)
          ctx.stroke()
        }
      } else if (e.type === 'SPLINE') {
        if (e.controlPoints && e.controlPoints.length > 1) {
          ctx.beginPath()
          const p0 = toScreen(transform.apply(e.controlPoints[0]))
          ctx.moveTo(p0.x, p0.y)
          for (let i = 1; i < e.controlPoints.length; i++) {
            const pt = toScreen(transform.apply(e.controlPoints[i]))
            ctx.lineTo(pt.x, pt.y)
          }
          ctx.stroke()
        }
      } else if (e.type === 'TEXT' || e.type === 'MTEXT' || e.type === 'ATTDEF' || e.type === 'ATTRIB') {
        const raw = e.text || e.string || e.tag || e.value || ''
        const lines = parseCadTextLines(raw)
        const posRaw = e.position || e.startPoint || e.endPoint || e.alignmentPoint
        
        if (lines.length > 0 && posRaw) {
          const pos = toScreen(transform.apply(posRaw))
          const textHeight = e.height || e.textHeight || 3.5
          const fontSize = Math.max(8, textHeight * viewScale.value * Math.abs(transform.d))
          
          if (fontSize >= 4) {
            ctx.save()
            ctx.font = `${fontSize}px "Noto Sans SC", "Microsoft YaHei", "SimSun", sans-serif`
            ctx.textBaseline = 'middle'
            ctx.textAlign = (e.halign === 2 || e.horizontalJustification === 2) ? 'right' : (e.halign === 1 || e.horizontalJustification === 1) ? 'center' : 'left'

            const rotDeg = e.rotation || 0
            if (rotDeg !== 0) {
              ctx.translate(pos.x, pos.y)
              ctx.rotate((-rotDeg * Math.PI) / 180)
              lines.forEach((line, idx) => {
                ctx.fillText(line, 0, idx * (fontSize * 1.3))
              })
            } else {
              lines.forEach((line, idx) => {
                ctx.fillText(line, pos.x, pos.y + idx * (fontSize * 1.3))
              })
            }
            ctx.restore()
          }
        }
      } else if (e.type === 'INSERT') {
        const block = parsedDxf.blocks?.[e.name]
        if (block && block.entities) {
          const insertTransform = new Transform2D(
            e.position?.x || 0,
            e.position?.y || 0,
            e.xScale || 1,
            e.yScale || 1,
            e.rotation || 0,
          )
          const combined = transform.multiply(insertTransform)
          drawEntities(block.entities, combined, layerName, color)
        }
      } else if (e.type === 'DIMENSION') {
        const block = parsedDxf.blocks?.[e.block]
        if (block && block.entities) {
          drawEntities(block.entities, transform, layerName, color)
        }
      }
    }
  }

  // 从顶层实体开始绘制
  drawEntities(parsedDxf.entities || [], new Transform2D())
}

// 计算所有几何体包围盒自适应居中
function fitView() {
  if (!parsedDxf) return
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity
  let count = 0

  function scanBounds(entities: any[], transform: Transform2D) {
    for (const e of entities) {
      if (e.vertices) {
        for (const v of e.vertices) {
          const pt = transform.apply(v)
          if (Number.isFinite(pt.x) && Number.isFinite(pt.y)) {
            minX = Math.min(minX, pt.x); maxX = Math.max(maxX, pt.x)
            minY = Math.min(minY, pt.y); maxY = Math.max(maxY, pt.y)
            count++
          }
        }
      } else if (e.center && e.radius) {
        const pt = transform.apply(e.center)
        const r = e.radius
        minX = Math.min(minX, pt.x - r); maxX = Math.max(maxX, pt.x + r)
        minY = Math.min(minY, pt.y - r); maxY = Math.max(maxY, pt.y + r)
        count++
      } else if (e.position || e.startPoint) {
        const pt = transform.apply(e.position || e.startPoint)
        minX = Math.min(minX, pt.x); maxX = Math.max(maxX, pt.x)
        minY = Math.min(minY, pt.y); maxY = Math.max(maxY, pt.y)
        count++
      }

      if (e.type === 'INSERT') {
        const block = parsedDxf.blocks?.[e.name]
        if (block && block.entities) {
          const insertTransform = new Transform2D(
            e.position?.x || 0,
            e.position?.y || 0,
            e.xScale || 1,
            e.yScale || 1,
            e.rotation || 0,
          )
          scanBounds(block.entities, transform.multiply(insertTransform))
        }
      } else if (e.type === 'DIMENSION') {
        const block = parsedDxf.blocks?.[e.block]
        if (block && block.entities) {
          scanBounds(block.entities, transform)
        }
      }
    }
  }

  scanBounds(parsedDxf.entities || [], new Transform2D())

  if (count > 0 && Number.isFinite(minX) && Number.isFinite(maxX)) {
    const canvas = canvasRef.value
    const w = canvas?.clientWidth || 800
    const h = canvas?.clientHeight || 600
    const bboxW = maxX - minX || 100
    const bboxH = maxY - minY || 100

    const scaleX = (w * 0.85) / bboxW
    const scaleY = (h * 0.85) / bboxH
    viewScale.value = Math.min(scaleX, scaleY)
    defaultScale.value = viewScale.value
    viewCenter.value = {
      x: (minX + maxX) / 2,
      y: (minY + maxY) / 2,
    }
    emit('zoom-change', 1)
  }
  redraw()
}

// 加载 DXF 矢量图
async function loadDxf(url: string) {
  loading.value = true
  errorMessage.value = ''
  loadingProgress.value = 10
  loadingStage.value = '正在请求 CAD 图纸数据...'

  try {
    const token = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
    const res = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!res.ok) {
      let msg = `HTTP ${res.status}`
      try {
        const errJson = await res.json()
        if (errJson?.message) msg = errJson.message
      } catch {}
      throw new Error(msg)
    }

    loadingProgress.value = 40
    loadingStage.value = '正在以 GB18030 中文编码解码图纸...'
    const buffer = await res.arrayBuffer()
    let dxfText = ''
    try {
      dxfText = new TextDecoder('gb18030').decode(buffer)
    } catch {
      dxfText = new TextDecoder('utf-8').decode(buffer)
    }

    loadingProgress.value = 70
    loadingStage.value = '正在解析图元拓扑与块表格...'
    const parser = new DxfParser()
    parsedDxf = parser.parseSync(dxfText)
    if (!parsedDxf || !parsedDxf.entities) {
      throw new Error('未解析到有效的 DXF 实体')
    }

    // 提取图层列表
    const newLayers = new Map<string, { name: string; color: string; visible: boolean }>()
    if (parsedDxf.tables?.layer?.layers) {
      for (const [name, lyr] of Object.entries<any>(parsedDxf.tables.layer.layers)) {
        newLayers.set(name, {
          name,
          color: getLayerColor(name),
          visible: !lyr.frozen,
        })
      }
    }
    if (!newLayers.has('0')) {
      newLayers.set('0', { name: '0', color: '#00f3ff', visible: true })
    }
    layerMap.value = newLayers
    emit('layers-loaded', Array.from(newLayers.values()))

    loadingProgress.value = 100
    loadingStage.value = '渲染完成'
    
    // 初始化自适应视口
    fitView()
  } catch (err: any) {
    console.error('加载 CAD 图纸失败', err)
    errorMessage.value = `CAD 图纸加载失败: ${err.message || err}`
  } finally {
    setTimeout(() => {
      loading.value = false
    }, 150)
  }
}

// 鼠标交互控制：平移与缩放
function onMouseDown(e: MouseEvent) {
  if (e.button !== 0 && e.button !== 1) return
  isDragging = true
  dragStartPos = { x: e.clientX, y: e.clientY }
  dragStartCenter = { ...viewCenter.value }
}

function onMouseMove(e: MouseEvent) {
  if (!isDragging) return
  const dx = e.clientX - dragStartPos.x
  const dy = e.clientY - dragStartPos.y
  viewCenter.value = {
    x: dragStartCenter.x - dx / viewScale.value,
    y: dragStartCenter.y + dy / viewScale.value,
  }
  redraw()
}

function onMouseUp() {
  isDragging = false
}

function onWheel(e: WheelEvent) {
  e.preventDefault()
  const canvas = canvasRef.value
  if (!canvas) return
  const rect = canvas.getBoundingClientRect()
  const mouseX = e.clientX - rect.left
  const mouseY = e.clientY - rect.top

  // 鼠标处的 CAD 坐标
  const cadX = (mouseX - canvas.width / 2) / viewScale.value + viewCenter.value.x
  const cadY = -(mouseY - canvas.height / 2) / viewScale.value + viewCenter.value.y

  const factor = e.deltaY < 0 ? 1.15 : 1 / 1.15
  viewScale.value *= factor

  // 保持鼠标指向的 CAD 点位置不变
  viewCenter.value = {
    x: cadX - (mouseX - canvas.width / 2) / viewScale.value,
    y: cadY + (mouseY - canvas.height / 2) / viewScale.value,
  }

  emit('zoom-change', viewScale.value / (defaultScale.value || 1))
  redraw()
}

// 暴露给外层工具栏调用的方法
function setLayerVisibility(layerName: string, visible: boolean) {
  const item = layerMap.value.get(layerName)
  if (item) {
    item.visible = visible
    redraw()
  }
}

function zoomIn() {
  viewScale.value *= 1.25
  emit('zoom-change', viewScale.value / (defaultScale.value || 1))
  redraw()
}

function zoomOut() {
  viewScale.value /= 1.25
  emit('zoom-change', viewScale.value / (defaultScale.value || 1))
  redraw()
}

function resetView() {
  fitView()
}

defineExpose({
  setLayerVisibility,
  zoomIn,
  zoomOut,
  resetView,
})

watch(() => props.dxfUrl, (url) => {
  if (url) loadDxf(url)
})

onMounted(() => {
  const updateSize = () => {
    if (containerRef.value && canvasRef.value) {
      canvasRef.value.width = containerRef.value.clientWidth
      canvasRef.value.height = containerRef.value.clientHeight
      redraw()
    }
  }

  resizeObserver = new ResizeObserver(updateSize)
  if (containerRef.value) {
    resizeObserver.observe(containerRef.value)
    updateSize()
  }

  if (props.dxfUrl) {
    loadDxf(props.dxfUrl)
  }
})

onUnmounted(() => {
  resizeObserver?.disconnect()
})
</script>

<template>
  <div ref="containerRef" class="cad-2d-viewer">
    <canvas
      ref="canvasRef"
      class="cad-canvas"
      :class="{ dragging: isDragging }"
      @mousedown="onMouseDown"
      @mousemove="onMouseMove"
      @mouseup="onMouseUp"
      @mouseleave="onMouseUp"
      @wheel="onWheel"
    ></canvas>

    <!-- 加载中动画 -->
    <div v-if="loading" class="overlay loading-overlay">
      <div class="progress-card">
        <div class="progress-header">
          <DemoIcon name="loader" :size="24" class="spin-icon" />
          <span class="progress-title">CAD 矢量图纸加载</span>
        </div>
        <div class="progress-bar-bg">
          <div class="progress-bar-fill" :style="{ width: `${loadingProgress}%` }"></div>
        </div>
        <div class="progress-footer">
          <span class="progress-stage">{{ loadingStage }}</span>
          <span class="progress-percent">{{ loadingProgress }}%</span>
        </div>
      </div>
    </div>

    <!-- 错误提示 -->
    <div v-if="errorMessage" class="overlay error-overlay">
      <DemoIcon name="alert-circle" :size="32" />
      <p>{{ errorMessage }}</p>
      <button class="btn retry-btn" type="button" @click="props.dxfUrl ? loadDxf(props.dxfUrl) : null">
        重试
      </button>
    </div>
  </div>
</template>

<style scoped>
.cad-2d-viewer {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 400px;
  background: #12151c;
  overflow: hidden;
  user-select: none;
}

.cad-canvas {
  display: block;
  width: 100%;
  height: 100%;
  cursor: grab;
}

.cad-canvas.dragging {
  cursor: grabbing;
}

.overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: rgba(18, 21, 28, 0.88);
  backdrop-filter: blur(4px);
  color: #f8fafc;
  z-index: 10;
}

.spin-icon {
  animation: spin 1s linear infinite;
  color: #00f3ff;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.progress-card {
  width: min(440px, calc(100% - 48px));
  padding: 20px 24px;
  border-radius: 12px;
  background: rgba(24, 29, 41, 0.96);
  border: 1px solid rgba(56, 189, 248, 0.25);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.progress-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.progress-title {
  font-size: 14.5px;
  font-weight: 600;
  color: #f8fafc;
}

.progress-bar-bg {
  width: 100%;
  height: 8px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  border-radius: 4px;
  background: linear-gradient(90deg, #0284c7, #38bdf8, #00f3ff);
  transition: width 0.3s ease;
}

.progress-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
}

.progress-stage {
  color: #94a3b8;
  max-width: 80%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.progress-percent {
  font-weight: 700;
  font-family: 'JetBrains Mono', monospace;
  color: #38bdf8;
}

.error-overlay {
  color: #f87171;
}

.retry-btn {
  margin-top: 8px;
  padding: 6px 16px;
  border-radius: 4px;
  background: #38bdf8;
  color: #0f172a;
  font-weight: 600;
  border: none;
  cursor: pointer;
}
</style>
