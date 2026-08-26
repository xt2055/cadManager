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
let hatchRecoveredCount = 0
let hatchDiagPrinted = false
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

// 注册自定义 HATCH 实体解析器（解析 CAD 标注箭头、剖面填充与实心多边形）
const HATCH_RELEVANT_CODES = new Set([
  // 实体属性与边界路径控制字段
  2, 8, 62, 70, 71, 72, 73, 75, 76, 77, 78, 91, 92, 93, 97, 98, 99,
  // 直线、圆弧和多段线边界坐标/参数
  10, 11, 20, 21, 30, 31, 40, 42, 50, 51, 52, 53, 63,
  94, 95, 96, 210, 220, 230,
])

class HatchEntityHandler {
  ForEntityName = 'HATCH'

  parseEntity(scanner: any, curr: any) {
    const entity: any = { type: 'HATCH', polygons: [] }
    const groups: any[] = []

    curr = scanner.next()
    while (!scanner.isEOF() && curr.code !== 0) {
      // HATCH 边界只依赖上述字段。过滤 XDATA 和转换器产生的异常组码，
      // 避免它们插入 91/92/93 结构后使边界读取位置错位。
      if (HATCH_RELEVANT_CODES.has(curr.code)) {
        groups.push(curr)
      }
      curr = scanner.next()
    }

    let index = 0
    let boundaryPathCount = 0

    const readPoint = (startIndex: number, xCode: number, yCode: number) => {
      const xGroup = groups[startIndex]
      const yGroup = groups[startIndex + 1]
      if (!xGroup || !yGroup || xGroup.code !== xCode || yGroup.code !== yCode) {
        return { point: null as { x: number; y: number } | null, nextIndex: startIndex }
      }
      return {
        point: { x: xGroup.value, y: yGroup.value },
        nextIndex: startIndex + 2,
      }
    }

    const appendPoint = (polygon: Array<{ x: number; y: number }>, point: { x: number; y: number }) => {
      const previous = polygon[polygon.length - 1]
      if (!previous || previous.x !== point.x || previous.y !== point.y) {
        polygon.push(point)
      }
    }

    const samePoint = (left: { x: number; y: number }, right: { x: number; y: number }) =>
      Math.abs(left.x - right.x) < 1e-8 && Math.abs(left.y - right.y) < 1e-8

    const orderEdges = (edges: Array<{ start: { x: number; y: number }; end: { x: number; y: number } }>) => {
      if (edges.length === 0) return [] as Array<{ x: number; y: number }>

      const first = edges[0]
      if (!first) return [] as Array<{ x: number; y: number }>
      const remaining = edges.slice(1)
      const polygon: Array<{ x: number; y: number }> = [first.start, first.end]

      while (remaining.length > 0) {
        const tail = polygon[polygon.length - 1]
        if (!tail) break
        const nextIndex = remaining.findIndex((edge) => samePoint(edge.start, tail) || samePoint(edge.end, tail))
        if (nextIndex < 0) break

        const next = remaining.splice(nextIndex, 1)[0]
        if (!next) break
        const nextPoint = samePoint(next.start, tail) ? next.end : next.start
        polygon.push(nextPoint)
      }

      return polygon
    }

    const recoverSolidPolygon = () => {
      const lineEdges: Array<{ start: { x: number; y: number }; end: { x: number; y: number } }> = []

      // 部分 CAXA/LibreDWG 输出会在 HATCH 边界控制字段中插入扩展数据，
      // 但箭头的几何坐标仍保持标准的 10/20 -> 11/21 直线边格式。
      for (let groupIndex = 0; groupIndex < groups.length - 3; groupIndex++) {
        const start = readPoint(groupIndex, 10, 20)
        if (!start.point) continue
        const end = readPoint(start.nextIndex, 11, 21)
        if (end.point) {
          lineEdges.push({ start: start.point, end: end.point })
          groupIndex = end.nextIndex - 1
        }
      }

      const ordered = orderEdges(lineEdges)
      if (ordered.length >= 3) return ordered

      // 某些文件使用多段线边界，只保留连续的 10/20 顶点。
      const vertices: Array<{ x: number; y: number }> = []
      for (let groupIndex = 0; groupIndex < groups.length - 1; groupIndex++) {
        const point = readPoint(groupIndex, 10, 20)
        if (!point.point) continue
        appendPoint(vertices, point.point)
        groupIndex = point.nextIndex - 1
      }
      if (vertices.length >= 3) {
        const first = vertices[0]
        const last = vertices[vertices.length - 1]
        if (first && last && !samePoint(first, last)) vertices.push(first)
        return vertices
      }

      return [] as Array<{ x: number; y: number }>
    }

    while (index < groups.length) {
      const group = groups[index]
      switch (group.code) {
        case 2:
          entity.patternName = group.value
          index += 1
          break
        case 8:
          entity.layer = group.value
          index += 1
          break
        case 62:
          entity.colorIndex = group.value
          index += 1
          break
        case 70:
          entity.solidFill = group.value === 1
          index += 1
          break
        case 91: {
          boundaryPathCount = group.value
          index += 1

          for (let pathIndex = 0; pathIndex < boundaryPathCount && index < groups.length; pathIndex++) {
            const pathFlags = groups[index]?.code === 92 ? groups[index].value : 0
            index += groups[index]?.code === 92 ? 1 : 0
            const polygon: Array<{ x: number; y: number }> = []

            if ((pathFlags & 2) !== 0) {
              // 多段线边界：93 后只读取指定数量的 10/20 顶点，不能把后续填充数据当成顶点。
              index += groups[index]?.code === 72 ? 1 : 0
              const isClosed = groups[index]?.code === 73 ? groups[index].value !== 0 : true
              index += groups[index]?.code === 73 ? 1 : 0
              const vertexCount = groups[index]?.code === 93 ? groups[index].value : 0
              index += groups[index]?.code === 93 ? 1 : 0

              for (let vertexIndex = 0; vertexIndex < vertexCount; vertexIndex++) {
                const result = readPoint(index, 10, 20)
                if (!result.point) break
                appendPoint(polygon, result.point)
                index = result.nextIndex
                if (groups[index]?.code === 42) index += 1
              }

              if (!isClosed && polygon.length > 1) {
                polygon.push(polygon[0] as { x: number; y: number })
              }
            } else {
              // 独立边界：93 后读取指定数量的边；当前重点支持直线边组成的箭头。
              const edgeCount = groups[index]?.code === 93 ? groups[index].value : 0
              index += groups[index]?.code === 93 ? 1 : 0
              const lineEdges: Array<{ start: { x: number; y: number }; end: { x: number; y: number } }> = []

              for (let edgeIndex = 0; edgeIndex < edgeCount && index < groups.length; edgeIndex++) {
                const edgeType = groups[index]?.code === 72 ? groups[index].value : 0
                index += groups[index]?.code === 72 ? 1 : 0

                if (edgeType === 1) {
                  const start = readPoint(index, 10, 20)
                  index = start.nextIndex
                  const end = readPoint(index, 11, 21)
                  index = end.nextIndex
                  if (start.point && end.point) {
                    lineEdges.push({ start: start.point, end: end.point })
                  }
                } else if (edgeType === 2) {
                  // 圆弧边界：离散采样，保证实心箭头和圆弧填充不会丢失。
                  const center = readPoint(index, 10, 20)
                  index = center.nextIndex
                  const radius = groups[index]?.code === 40 ? groups[index].value : 0
                  index += groups[index]?.code === 40 ? 1 : 0
                  const startAngle = groups[index]?.code === 50 ? groups[index].value : 0
                  index += groups[index]?.code === 50 ? 1 : 0
                  const endAngle = groups[index]?.code === 51 ? groups[index].value : startAngle
                  index += groups[index]?.code === 51 ? 1 : 0
                  const counterClockwise = groups[index]?.code === 73 ? groups[index].value !== 0 : false
                  index += groups[index]?.code === 73 ? 1 : 0

                  if (center.point && radius > 0) {
                    let span = endAngle - startAngle
                    if (counterClockwise && span < 0) span += Math.PI * 2
                    if (!counterClockwise && span > 0) span -= Math.PI * 2
                    const sampleCount = Math.max(8, Math.ceil(Math.abs(span) * radius * 2))
                    for (let sampleIndex = 0; sampleIndex <= sampleCount; sampleIndex++) {
                      const angle = startAngle + (span * sampleIndex) / sampleCount
                      appendPoint(polygon, {
                        x: center.point.x + Math.cos(angle) * radius,
                        y: center.point.y + Math.sin(angle) * radius,
                      })
                    }
                  }
                } else {
                  // 未支持的边类型无法安全猜测长度，跳过当前路径，避免污染后续实体。
                  break
                }
              }

              for (const point of orderEdges(lineEdges)) {
                appendPoint(polygon, point)
              }
            }

            if (polygon.length >= 3) {
              entity.polygons.push(polygon)
            }
          }
          break
        }
        default:
          // 跳过 HATCH 标高、图案比例等非边界数据。
          index += 1
          break
      }
    }

    if (entity.solidFill === true && entity.polygons.length === 0) {
      const recovered = recoverSolidPolygon()
      if (recovered.length >= 3) {
        entity.polygons.push(recovered)
        hatchRecoveredCount++
      }
    }

    return entity
  }
}

// 预处理 DXF 字节流：仅精准缝合 MTEXT 超长文本连续组码（组码 3 紧跟组码 1 或 3）的跨行中文字节断裂，绝不误伤其它组码与图层颜色定义
function mergeDxfMTextSplitBytes(buffer: ArrayBuffer): ArrayBuffer {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  const chunkSize = 32768
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode.apply(null, Array.from(bytes.subarray(i, i + chunkSize)))
  }

  // 仅匹配组码 3 文本行后紧接着出现组码 1 或 3 的接续情况，移除中间的分割标记使字节流连续
  const pattern = /(\r?\n[ \t]*3\r?\n[^\r\n]*)\r?\n[ \t]*[13]\r?\n/g
  let prev = ''
  while (prev !== binary) {
    prev = binary
    binary = binary.replace(pattern, (match, p1) => p1)
  }

  const outBytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    outBytes[i] = binary.charCodeAt(i) & 0xff
  }
  return outBytes.buffer
}

// GDT 字体形位公差符号映射表（AutoCAD/CAXA 标准机械形位公差字符映射）
const GDT_CHAR_MAP: Record<string, string> = {
  'a': '—',   // 直线度 (Straightness)
  'b': '⏢',   // 平面度 (Flatness)
  'c': '○',   // 圆度 (Circularity)
  'd': '⌭',   // 圆柱度 (Cylindricity)
  'e': '⌒',   // 线轮廓度 (Profile of a line)
  'f': '⌓',   // 面轮廓度 (Profile of a surface)
  'g': '∥',   // 平行度 (Parallelism)
  'h': '⊥',   // 垂直度 (Perpendicularity)
  'i': '∠',   // 倾斜度 (Angularity)
  'j': '◎',   // 同轴度/同心度 (Concentricity)
  'k': '⌯',   // 对称度 (Symmetry)
  'l': '⌖',   // 位置度 (Position)
  'm': '↗',   // 圆跳动 (Circular runout)
  'n': '⌰',   // 全跳动 (Total runout)
  'p': 'Ⓜ',   // 最大实体 MMC
  's': 'Ⓛ',   // 最小实体 LMC
  'r': 'Ⓢ',   // 自由状态 RFS
  'v': 'Φ',   // 直径符号
  'w': '□',   // 正方形符号
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

  // 3. 处理 GDT 形位公差字体块（将 {\Fgdt...;h} / \Fgdt;h 等字体字符精准映射为机械公差符号 ⟂, ∥, ◎, ⏢ 等）
  str = str.replace(/\{\\F[gG][dD][tT][^;]*;([^}]+)\}/gi, (_, inner) => {
    let res = ''
    for (const ch of inner) {
      const lower = ch.toLowerCase()
      res += GDT_CHAR_MAP[lower] || ch
    }
    return res
  })
  str = str.replace(/\\F[gG][dD][tT][^;]*;([A-Za-z0-9]+)/gi, (_, inner) => {
    let res = ''
    for (const ch of inner) {
      const lower = ch.toLowerCase()
      res += GDT_CHAR_MAP[lower] || ch
    }
    return res
  })

  // 4. 处理 CAXA/AutoCAD 特殊形位公差转义符
  str = str
    .replace(/%%v/gi, 'Φ')
    .replace(/%%h/gi, '⊥')
    .replace(/%%g/gi, '∥')
    .replace(/%%b/gi, '⏢')
    .replace(/%%a/gi, '—')
    .replace(/%%c/gi, '○')
    .replace(/%%d/gi, '⌭')
    .replace(/%%j/gi, '◎')
    .replace(/%%k/gi, '⌯')
    .replace(/%%l/gi, '⌖')
    .replace(/%%m/gi, '↗')
    .replace(/%%n/gi, '⌰')

  // 5. 提取局部字号缩放比例 \H...x; 或 \H...;
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

  // 6. 优先处理 CAXA 自定义上下标公差格式如 {\D\H0.7x;+0.1^+0.2|a;} -> (+0.1 / +0.2)
  str = str.replace(/\{\\D\\H[0-9.]+x;([^|}]+)\^([^|}]+)\|a;\}/gi, '  ($1 / $2)')

  // 7. 处理 AutoCAD 标注堆叠公差如 \S+0.035^ 0; 或 \S-0.043^-0.083; 或 \S+0.1^;
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

  // 8. 标准工程特殊符号替换
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

  // 9. 清理 AutoCAD MTEXT 各种格式控制指令（彻底清除带管道符 |b0|i0|c134|p2 的字体参数，防止残留乱码）
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

  const polygonArea = (polygon: Array<{ x: number; y: number }>) => {
    let area = 0
    for (let i = 0; i < polygon.length; i++) {
      const current = polygon[i]
      const next = polygon[(i + 1) % polygon.length]
      if (!current || !next) continue
      area += current.x * next.y - next.x * current.y
    }
    return Math.abs(area) / 2
  }

  // 递归渲染实体
  let hatchEntityCount = 0
  let hatchPolygonCount = 0
  const hatchFailSamples: Array<Record<string, unknown>> = []
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
        let raw = e.text || e.string || e.value || ''
        
        // 如果文本为单个字符（如 'h', 'g', 'j', 'b' 等），且图层属于标注/公差图层，映射为对应形位公差符号
        if (/^[a-np-w]$/i.test(raw.trim()) && (layerName.includes('尺寸') || layerName.includes('DIM') || layerName.includes('TOL') || layerName.includes('公差') || e.style === 'GDT' || e.textStyle === 'GDT')) {
          raw = GDT_CHAR_MAP[raw.trim().toLowerCase()] || raw
        }

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
      } else if (e.type === 'HATCH') {
        // 渲染 CAD 标注箭头、剖面填充与实心多边形
        hatchEntityCount++
        const validPolygons = (e.polygons || []).filter(
          (poly: Array<{ x: number; y: number }>) => poly.length >= 3 && polygonArea(poly) > 1e-10,
        )
        if (validPolygons.length > 0) {
          ctx.save()
          ctx.globalCompositeOperation = 'source-over'
          ctx.fillStyle = color
          ctx.strokeStyle = color
          ctx.lineJoin = 'round'
          ctx.lineCap = 'round'
          for (const poly of validPolygons) {
            hatchPolygonCount++
            ctx.beginPath()
            const p0 = toScreen(transform.apply(poly[0]))
            ctx.moveTo(p0.x, p0.y)
            for (let i = 1; i < poly.length; i++) {
              const pt = toScreen(transform.apply(poly[i]))
              ctx.lineTo(pt.x, pt.y)
            }
            ctx.closePath()
            if (e.solidFill !== false) {
              ctx.fill()
              // 实心箭头额外描边，避免缩放较小时纯填充边缘不明显。
              ctx.stroke()
            } else {
              ctx.stroke()
            }
          }
          ctx.restore()
        } else if (hatchFailSamples.length < 5) {
          // 收集解析失败的 HATCH 样本，用于定位真实图纸箭头结构差异
          hatchFailSamples.push({
            layer: layerName,
            pattern: e.patternName,
            solid: e.solidFill,
            polygonCount: e.polygons?.length ?? -1,
            firstPolygonPoints: e.polygons?.[0]?.length ?? -1,
          })
        }
      } else if (e.type === 'SOLID' || e.type === 'TRACE') {
        // 渲染 3 点或 4 点实心箭头/填充面 (AutoCAD SOLID 点序为 0, 1, 3, 2)
        if (e.points && e.points.length >= 3) {
          ctx.save()
          ctx.beginPath()
          const p0 = toScreen(transform.apply(e.points[0]))
          const p1 = toScreen(transform.apply(e.points[1]))
          const p2 = e.points[3] ? toScreen(transform.apply(e.points[3])) : toScreen(transform.apply(e.points[2]))
          const p3 = e.points[2] ? toScreen(transform.apply(e.points[2])) : null
          ctx.moveTo(p0.x, p0.y)
          ctx.lineTo(p1.x, p1.y)
          ctx.lineTo(p2.x, p2.y)
          if (p3) ctx.lineTo(p3.x, p3.y)
          ctx.closePath()
          ctx.fill()
          ctx.restore()
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

  // 诊断日志：确认浏览器运行的代码版本与 HATCH 箭头实际绘制数量（打开 F12 控制台可见）
  console.log(`[CAD] 箭头诊断 v5: HATCH实体=${hatchEntityCount}, 已绘制多边形=${hatchPolygonCount}, 恢复多边形=${hatchRecoveredCount}, 缩放=${viewScale.value.toFixed(4)}`)
  if (hatchFailSamples.length > 0 && !hatchDiagPrinted) {
    hatchDiagPrinted = true
    console.log('[CAD] 解析失败的 HATCH 样本:', JSON.stringify(hatchFailSamples))
  }
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

      // HATCH 实心箭头/填充顶点也必须纳入包围盒，避免视口计算偏差
      if (e.polygons) {
        for (const poly of e.polygons) {
          for (const v of poly) {
            const pt = transform.apply(v)
            if (Number.isFinite(pt.x) && Number.isFinite(pt.y)) {
              minX = Math.min(minX, pt.x); maxX = Math.max(maxX, pt.x)
              minY = Math.min(minY, pt.y); maxY = Math.max(maxY, pt.y)
              count++
            }
          }
        }
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
  hatchRecoveredCount = 0
  hatchDiagPrinted = false
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
    parser.registerEntityHandler(HatchEntityHandler as any)
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
