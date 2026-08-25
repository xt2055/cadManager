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

// 注册自定义 ATTRIB 实体解析器（修复 DXF-Parser 遗漏块属性导致图号、材料、签名丢失的问题）
class AttribEntityHandler {
  ForEntityName = 'ATTRIB'
  parseEntity(scanner: any, curr: any) {
    const entity: any = { type: curr.value }
    curr = scanner.next()
    while (!scanner.isEOF()) {
      if (curr.code === 0) break
      switch (curr.code) {
        case 1: entity.text = curr.value; break
        case 2: entity.tag = curr.value; break
        case 3: entity.prompt = curr.value; break
        case 10:
          entity.startPoint = { x: curr.value, y: 0, z: 0 }
          break
        case 20:
          if (entity.startPoint) entity.startPoint.y = curr.value
          break
        case 30:
          if (entity.startPoint) entity.startPoint.z = curr.value
          break
        case 11:
          entity.endPoint = { x: curr.value, y: 0, z: 0 }
          break
        case 21:
          if (entity.endPoint) entity.endPoint.y = curr.value
          break
        case 31:
          if (entity.endPoint) entity.endPoint.z = curr.value
          break
        case 40: entity.textHeight = curr.value; break
        case 41: entity.scale = curr.value; break
        case 50: entity.rotation = curr.value; break
        case 70: entity.flags = curr.value; break
        case 71: entity.generationFlags = curr.value; break
        case 72: entity.halign = curr.value; break
        case 74: entity.valign = curr.value; break
        case 8: entity.layer = curr.value; break
        case 62: entity.colorIndex = curr.value; break
      }
      curr = scanner.next()
    }
    return entity
  }
}

// 预处理 DXF 字节流：缝合因 DXF 单行 250 字节限制被拆分在组码 3 与组码 1 之间的多字节中文字符（防止汉字字节被换行切成两半产生雪崩乱码）
function mergeDxfMTextSplitBytes(buffer: ArrayBuffer): ArrayBuffer {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  const chunkSize = 32768
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode.apply(null, Array.from(bytes.subarray(i, i + chunkSize)))
  }

  const pattern = /(\r?\n[ \t]*3\r?\n[^\r\n]*)\r?\n[ \t]*[13]\r?\n/g
  let prev = ''
  while (prev !== binary) {
    prev = binary
    binary = binary.replace(pattern, (match, p1) => p1)
  }
  binary = binary.replace(/(\r?\n[ \t]*)3(\r?\n)/g, (match, p1, p2) => p1 + '1' + p2)

  const outBytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    outBytes[i] = binary.charCodeAt(i) & 0xff
  }
  return outBytes.buffer
}

// 解析 CAD 文字并提取字号缩放及清洗后的文本
function parseCadText(raw: string): { lines: string[]; fontScale: number; isTolerance: boolean } {
  if (!raw) return { lines: [], fontScale: 1, isTolerance: false }
  let str = raw
  let isTol = false

  // 1. 解码 AutoCAD Unicode 转义字符 \U+XXXX (如 \U+6280 -> 技, \U+00B0 -> °, \U+00B1 -> ±, \U+2205 -> Φ)
  str = str.replace(/\\U\+([0-9A-Fa-f]{4})/gi, (_, hex) => {
    try {
      return String.fromCharCode(parseInt(hex, 16))
    } catch {
      return ''
    }
  })

  // 2. 解码 AutoCAD 多字节/MIF 转义 \M+1XXXX 或 \M+5XXXX 等
  str = str.replace(/\\M\+[0-9A-Fa-f]{5}/gi, '')

  // 3. 提取局部字号缩放比例 \H...x; 或 \H...;
  let fontScale = 1
  const hMatch = str.match(/\\H([0-9.]+)x;/i) || str.match(/\\H([0-9.]+);/i)
  if (hMatch && hMatch[1]) {
    const val = parseFloat(hMatch[1])
    if (!isNaN(val) && val > 0) {
      fontScale = val
      if (fontScale < 0.95) {
        isTol = true
      }
    }
  }

  // 4. 优先处理 CAXA 自定义上下标公差格式如 {\D\H0.7x;+0.1^+0.2|a;} -> (+0.1 / +0.2)
  str = str.replace(/\{\\D\\H[0-9.]+x;([^|}]+)\^([^|}]+)\|a;\}/gi, '  ($1 / $2)')

  // 5. 处理 AutoCAD 标注堆叠公差如 \S+0.035^ 0; 或 \S-0.043^-0.083; 或 \S+0.1^;
  // 在主尺寸与公差之间保留充分安全距离，防止数字遮挡正负号
  str = str.replace(/\\S([^;^/#]+)\^([^;]*);?/gi, (match, top, btm) => {
    top = top ? top.trim() : ''
    btm = btm ? btm.trim() : ''
    if (top && btm) return `  (${top} / ${btm})`
    if (top) return `  (${top})`
    if (btm) return `  (${btm})`
    return ''
  })
  str = str.replace(/\\S\^([^;]+);?/gi, (match, btm) => `  (${btm.trim()})`)
  str = str.replace(/\\S([^;]+)\^;?/gi, (match, top) => `  (${top.trim()})`)
  str = str.replace(/\\S([^;/#]+)[/#]([^;]+);?/gi, (match, top, btm) => `  (${top.trim()} / ${btm.trim()})`)

  // 6. 标准工程特殊符号替换
  str = str
    .replace(/\\P/gi, '\n')
    .replace(/\\X/gi, '\n')
    .replace(/\\~/g, ' ')
    .replace(/\\{/g, '{')
    .replace(/\\}/g, '}')
    .replace(/\\\\/g, '\\')
    .replace(/%%C/gi, 'Φ').replace(/%C/gi, 'Φ')
    .replace(/%%D/gi, '°').replace(/%D/gi, '°')
    .replace(/%%P/gi, '  ±').replace(/%P/gi, '  ±')
    .replace(/%%U/gi, '')
    .replace(/%%O/gi, '')
    .replace(/%%%/gi, '%')

  // 7. 清理 AutoCAD MTEXT 各种格式控制指令（彻底清除带管道符 |b0|i0|c134|p2 的字体参数，防止残留乱码）
  // 匹配 \fFontName|b0|i0|c134|p2; 或 \Ffont.shx,gbcbig.shx;
  str = str.replace(/\\[fF][^;]*;/g, '')
  // 清理字高、宽度、倾斜、字距、颜色、对齐、段落控制等带分号指令
  str = str.replace(/\\[hHwWqQtTcCkKaApP][^;]*;/g, '')
  // 清理其它所有带分号的控制指令 \AnyCode;
  str = str.replace(/\\[A-Za-z0-9.]+;/g, '')
  // 清理下划线、上划线、删除线开关指令（\L, \l, \O, \o, \K, \k）
  str = str.replace(/\\[LlOoKk]/g, '')
  // 清理大括号分组 {}
  str = str.replace(/[{}]/g, '')

  const lines = str.split('\n').map(l => l.trim()).filter(l => l.length > 0)
  if (lines.length > 0) {
    const first = lines[0] ?? ''
    if (/^[+\-±]/.test(first) || first.startsWith('(')) {
      isTol = true
    }
  }

  return {
    lines,
    fontScale,
    isTolerance: isTol,
  }
}

// 绘制主渲染逻辑
function redraw() {
  const canvas = canvasRef.value
  if (!canvas || !parsedDxf) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const width = canvas.width
  const height = canvas.height

  // 1. 专业 CAD 纯黑高对比度背景
  ctx.fillStyle = '#0a0d14'
  ctx.fillRect(0, 0, width, height)

  // 绘制工程网格背景
  ctx.lineWidth = 1
  ctx.strokeStyle = 'rgba(255, 255, 255, 0.04)'
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
      } else if (e.type === 'TEXT' || e.type === 'MTEXT' || e.type === 'ATTRIB') {
        const raw = e.text || e.string || e.value || ''
        const { lines, fontScale, isTolerance } = parseCadText(raw)
        
        // AutoCAD 对齐点决策：对于 TEXT/ATTRIB，如果有对齐方式(halign/valign)，DXF 规范以 endPoint (组码 11) 为基准点，否则以 startPoint (组码 10)
        let posRaw = e.position || e.startPoint
        const halign = e.halign !== undefined ? e.halign : 0
        const valign = e.valign !== undefined ? e.valign : 0
        if (e.endPoint && (halign > 0 || valign > 0)) {
          posRaw = e.endPoint
        }
        
        if (lines.length > 0 && posRaw) {
          const pos = toScreen(transform.apply(posRaw))
          const textHeight = (e.height || e.textHeight || 3.5) * fontScale
          const fontSize = Math.max(6, textHeight * viewScale.value * Math.abs(transform.d))
          
          if (fontSize >= 3) {
            ctx.save()
            ctx.font = `${fontSize}px "Noto Sans SC", "Microsoft YaHei", "SimSun", sans-serif`
            
            // 垂直与水平对齐映射
            if (e.type === 'MTEXT') {
              const ap = e.attachmentPoint || 1
              // 1: TopLeft, 2: TopCenter, 3: TopRight
              // 4: MiddleLeft, 5: MiddleCenter, 6: MiddleRight
              // 7: BottomLeft, 8: BottomCenter, 9: BottomRight
              ctx.textAlign = [1, 4, 7].includes(ap) ? 'left' : [2, 5, 8].includes(ap) ? 'center' : 'right'
              ctx.textBaseline = [1, 2, 3].includes(ap) ? 'top' : [4, 5, 6].includes(ap) ? 'middle' : 'bottom'
            } else {
              // TEXT / ATTRIB 规范
              // halign: 0=Left, 1=Center, 2=Right, 3=Aligned, 4=Middle, 5=Fit
              // valign: 0=Baseline, 1=Bottom, 2=Middle, 3=Top
              ctx.textAlign = (halign === 1 || halign === 4) ? 'center' : (halign === 2 ? 'right' : 'left')
              ctx.textBaseline = valign === 3 ? 'top' : (valign === 2 || halign === 4) ? 'middle' : (valign === 1 ? 'bottom' : 'alphabetic')
            }

            // 计算文字旋转角度（MTEXT 优先采用 directionVector 方向向量，TEXT 采用 rotation）
            let rotDeg = e.rotation || 0
            if (e.type === 'MTEXT' && e.directionVector) {
              const dx = e.directionVector.x || 0
              const dy = e.directionVector.y || 0
              if (dx !== 0 || dy !== 0) {
                rotDeg = (Math.atan2(dy, dx) * 180) / Math.PI
              }
            }

            // 如果是公差类文本（以 + / - / ± 开头或有缩放），沿文字书写方向自动增加安全间距，防止主尺寸数字遮挡正负号
            const tolOffset = isTolerance ? Math.max(6, fontSize * 0.45) : 0

            if (rotDeg !== 0) {
              ctx.translate(pos.x, pos.y)
              ctx.rotate((-rotDeg * Math.PI) / 180)
              lines.forEach((line, idx) => {
                ctx.fillText(line, tolOffset, idx * (fontSize * 1.25))
              })
            } else {
              lines.forEach((line, idx) => {
                ctx.fillText(line, pos.x + tolOffset, pos.y + idx * (fontSize * 1.25))
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
    loadingStage.value = '正在智能探测并解码 CAD 图纸编码...'
    const rawBuffer = await res.arrayBuffer()
    if (!rawBuffer || rawBuffer.byteLength === 0) {
      throw new Error('获取到的 CAD 图纸数据为空')
    }

    // 缝合 DXF 超长文本（组码 3 与组码 1）被换行切断的中文字节，防止跨行断字乱码
    const buffer = mergeDxfMTextSplitBytes(rawBuffer)

    let dxfText = ''
    // 智能编码探测：
    // 1. 检查 DXF 文件头部的 $DWGCODEPAGE（前 8KB 区域）
    const headerSample = new TextDecoder('ascii').decode(buffer.slice(0, Math.min(buffer.byteLength, 8192)))
    if (/ANSI_936|GB2312|GBK|CP936|GB18030/i.test(headerSample)) {
      dxfText = new TextDecoder('gb18030').decode(buffer)
    } else {
      // 2. 尝试严格 UTF-8 解码，如果出现非 UTF-8 字节（如 GBK 双字节）则 fatal 抛错降级到 GB18030
      try {
        dxfText = new TextDecoder('utf-8', { fatal: true }).decode(buffer)
      } catch {
        dxfText = new TextDecoder('gb18030').decode(buffer)
      }
    }

    if (!dxfText || dxfText.trim().length === 0) {
      throw new Error('CAD 图纸文本内容为空')
    }

    loadingProgress.value = 70
    loadingStage.value = '正在解析图元拓扑与块表格...'
    const parser = new DxfParser()
    parser.registerEntityHandler(AttribEntityHandler as any)
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
  background: var(--cad-bg, #1a1d24);
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
  background: color-mix(in srgb, var(--panel, #1e2228) 85%, transparent);
  backdrop-filter: blur(4px);
  color: var(--text-1, #f8fafc);
  z-index: 10;
}

.spin-icon {
  animation: spin 1s linear infinite;
  color: var(--accent, #38bdf8);
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.progress-card {
  width: min(440px, calc(100% - 48px));
  padding: 20px 24px;
  border-radius: var(--radius, 12px);
  background: var(--panel-top, var(--panel, #252a31));
  border: 1px solid var(--line, rgba(255, 255, 255, 0.1));
  box-shadow: var(--shadow-lg, 0 10px 30px rgba(0, 0, 0, 0.5));
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
  color: var(--text-1, #f8fafc);
}

.progress-bar-bg {
  width: 100%;
  height: 6px;
  border-radius: 3px;
  background: var(--panel-2, rgba(255, 255, 255, 0.08));
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  border-radius: 3px;
  background: linear-gradient(90deg, var(--accent, #0284c7), var(--accent-2, #38bdf8));
  transition: width 0.3s ease;
}

.progress-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
}

.progress-stage {
  color: var(--text-3, #94a3b8);
  max-width: 80%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.progress-percent {
  font-weight: 700;
  font-family: 'JetBrains Mono', monospace;
  color: var(--accent, #38bdf8);
}

.error-overlay {
  color: var(--danger, #f87171);
}

.retry-btn {
  margin-top: 8px;
  padding: 6px 16px;
  border-radius: var(--radius-sm, 4px);
  background: var(--accent, #38bdf8);
  color: var(--accent-ink, #0f172a);
  font-weight: 600;
  border: none;
  cursor: pointer;
}
</style>
