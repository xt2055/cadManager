<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { assertCadWorkerAssets, getCadWorkerUrls } from './cad-worker-assets'
import { findWipeoutMasks } from './cad-entity-filters'
import { installCadFontDiagnostics, normalizeCadToleranceEntities, preloadCadSymbolFonts, resolveCadFontsBaseUrl } from '@/services/cad-fonts.service'
import { registerCadConverters } from '@/services/cad-converters.service'
import { compareEntities, snapshotDrawing, type DrawingDifference, type CompareBounds } from './cad-compare'
import { collectTitleSpaces } from './cad-title-block'
import { panViewBox, zoomViewBox } from './cad-viewport-gestures'
import { readAccessToken } from '@/services/auth/access-token'
import type { AnnotationViewport, Point } from '@/features/reviews/annotation-model'

interface Props {
  dxfUrl?: string | null
  fileName?: string | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'layers-loaded', layers: Array<{ name: string; color: string; visible: boolean }>): void
  (event: 'zoom-change', zoom: number): void
  (event: 'ready'): void
  (event: 'load-error', message: string): void
}>()
const containerRef = ref<HTMLDivElement | null>(null)
const loading = ref(false)
const errorMessage = ref('')

let manager: any = null
let managerGeneration = 0
let openGeneration = 0
let disposed = false
let destroyPromise: Promise<void> | null = null
const compareMarks = ref<Array<{ id: number; kind: string; x: number; y: number; width: number; height: number }>>([])
let differences: DrawingDifference[] = []
let comparisonBounds: CompareBounds | undefined
let markFrame = 0
let compareGeneration = 0
const selectedDifference = ref<number | null>(null)

function clearComparison() {
  compareGeneration++
  cancelAnimationFrame(markFrame)
  differences = []
  comparisonBounds = undefined
  compareMarks.value = []
  selectedDifference.value = null
  manager?.clearOverlays()
  manager?.setCompareDisplay({ enabled: false })
}

function updateCompareMarks() {
  const view = manager?.curView
  if (!view) return
  compareMarks.value = differences.flatMap(diff => {
    const boxes = [diff.before?.bounds, diff.after?.bounds].filter(box => !!box)
    if (!boxes.length) return []
    const a = view.worldToScreen({ x: Math.min(...boxes.map(b => b.minX)), y: Math.min(...boxes.map(b => b.minY)) })
    const b = view.worldToScreen({ x: Math.max(...boxes.map(b => b.maxX)), y: Math.max(...boxes.map(b => b.maxY)) })
    return [{ id: diff.id, kind: diff.kind, x: Math.min(a.x, b.x) - 6, y: Math.min(a.y, b.y) - 6, width: Math.max(12, Math.abs(b.x - a.x) + 12), height: Math.max(12, Math.abs(b.y - a.y) + 12) }]
  })
  markFrame = requestAnimationFrame(updateCompareMarks)
}

async function compareDrawing(fileName: string, content: ArrayBuffer): Promise<DrawingDifference[]> {
  if (!manager || loading.value) throw new Error('请等待基准图纸加载完成')
  clearComparison()
  const generation = compareGeneration
  const currentManager = manager
  const { AcDbDatabase, AcDbFileType, AcDbDxfFiler } = await import('@mlightcad/data-model')
  const database = new AcDbDatabase()
  await database.read(content, { readOnly: true }, /\.dxf$/i.test(fileName) ? AcDbFileType.DXF : AcDbFileType.DWG)
  if (database.lastOpenError) throw new Error('待对比图纸解析失败')
  if (disposed || manager !== currentManager || generation !== compareGeneration) throw new Error('对比已取消')
  normalizeCadToleranceEntities(database)
  findWipeoutMasks(database)
  const before = currentManager.curDocument.database
  const leftEntities = snapshotDrawing(before, () => new AcDbDxfFiler({ database: before, precision: 10 }))
  const rightEntities = snapshotDrawing(database, () => new AcDbDxfFiler({ database, precision: 10 }))
  const result = compareEntities(leftEntities, rightEntities)
  currentManager.curView.activeLayoutBtrId = before.tables.blockTable.modelSpace.objectId
  const overlayId = await currentManager.registerOverlayDatabase(database)
  if (disposed || manager !== currentManager || generation !== compareGeneration) {
    currentManager.removeOverlay(overlayId)
    throw new Error('对比已取消')
  }
  currentManager.setCompareDisplay({ enabled: true, overrides: result.flatMap(d => d.before ? [{ objectId: d.before.id, role: d.kind }] : []) })
  currentManager.setOverlayCompareDisplay(overlayId, { enabled: true, overrides: result.flatMap(d => d.after ? [{ objectId: d.after.id, role: d.kind }] : []) })
  differences = result
  comparisonBounds = unionBounds([...leftEntities, ...rightEntities].map(entity => entity.bounds))
  await resetView()
  if (disposed || manager !== currentManager || generation !== compareGeneration) throw new Error('对比已取消')
  updateCompareMarks()
  return result
}

async function focusDifference(id: number) {
  const difference = differences.find(item => item.id === id)
  const bounds = unionBounds([difference?.before?.bounds, difference?.after?.bounds])
  if (!bounds || !manager) return
  const { AcGeBox2d } = await import('@mlightcad/data-model')
  const padding = Math.max(bounds.maxX - bounds.minX, bounds.maxY - bounds.minY, 1) * 0.3
  manager?.curView.zoomTo(new AcGeBox2d({ x: bounds.minX - padding, y: bounds.minY - padding }, { x: bounds.maxX + padding, y: bounds.maxY + padding }))
  selectedDifference.value = id
}

function unionBounds(values: Array<CompareBounds | undefined>): CompareBounds | undefined {
  let result: CompareBounds | undefined
  for (const box of values) {
    if (!box) continue
    if (!result) result = { ...box }
    else {
      result.minX = Math.min(result.minX, box.minX)
      result.minY = Math.min(result.minY, box.minY)
      result.maxX = Math.max(result.maxX, box.maxX)
      result.maxY = Math.max(result.maxY, box.maxY)
    }
  }
  return result
}

function useMainThreadCadDraw() {
  if (!import.meta.env.DEV || typeof window === 'undefined') return false
  const queryEnabled = new URLSearchParams(window.location.search).get('cad-main-thread') === '1'
  const storageEnabled = window.localStorage.getItem('cad_use_main_thread_draw') !== '0'
  return queryEnabled || storageEnabled
}

async function finishInitialRender(currentManager: any, isCurrent: () => boolean) {
  const view = currentManager?.curView
  if (!view) return

  // openDocument() 可能早于字体和延迟文字几何完成返回。等待场景空闲后
  // 再适配一次，否则形位公差等 MTEXT/DIMENSION 可能要等用户点击才出现。
  if (typeof view.waitUntilIdle === 'function') {
    await view.waitUntilIdle(60_000)
  }
  if (!isCurrent()) return

  view.zoomToFitDrawing(60_000)
  if (typeof view.waitUntilIdle === 'function') {
    await view.waitUntilIdle(60_000)
  }
  // The SDK applies this fit in a 300ms condition waiter. Let it finish
  // before a comparison frames both drawings, including new outer geometry.
  await new Promise(resolve => setTimeout(resolve, 350))
  if (!isCurrent()) return
  view.isDirty = true
  view.isHtmlDirty = true
}

function getAccessToken() {
  return readAccessToken()
}

async function destroyViewer() {
  clearComparison()
  if (destroyPromise) return destroyPromise
  const currentManager = manager
  manager = null
  if (!currentManager) return
  destroyPromise = (async () => {
    try {
      // MLightCAD 1.6.3 的 destroy 内部可能产生一个脱离返回 Promise 的
      // Worker 终止异常，因此同时挂接同步和异步两层保护。
      const result = currentManager.destroy()
      await Promise.resolve(result).catch(() => undefined)
    } catch (error: any) {
      if (!/renderer terminated/i.test(String(error?.message || error))) {
        console.warn('销毁 MLightCAD 查看器失败', error)
      }
    } finally {
      destroyPromise = null
    }
  })()
  return destroyPromise
}

async function loadViewer(url: string) {
  const container = containerRef.value
  if (!container || disposed) return

  const generation = ++openGeneration
  await destroyViewer()
  if (generation !== openGeneration || disposed || !containerRef.value) return

  loading.value = true
  errorMessage.value = ''

  try {
    const token = getAccessToken()
    const response = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const buffer = await response.arrayBuffer()
    if (generation !== openGeneration || disposed || !containerRef.value) return

    const { AcApDocManager, AcEdOpenMode } = await import('@mlightcad/cad-simple-viewer')
    const workerUrls = getCadWorkerUrls()
    await assertCadWorkerAssets(workerUrls)
    if (!(await AcApDocManager.checkWebworkerReadiness(workerUrls))) {
      throw new Error(`CAD Worker 不可访问：${JSON.stringify(workerUrls)}`)
    }
    await registerCadConverters(workerUrls.dwgParser)
    const baseUrl = await resolveCadFontsBaseUrl()
    if (generation !== openGeneration || disposed || !containerRef.value) return
    const mainThreadDraw = useMainThreadCadDraw()
    console.info('[CAD][字体诊断] 绘制线程', JSON.stringify({ mainThreadDraw }))
    const currentManager = AcApDocManager.createInstance({
      container: containerRef.value,
      autoResize: true,
      baseUrl,
      // 与官方示例保持一致，使用 Worker 绘制复杂标注块。
      useMainThreadDraw: mainThreadDraw,
      webworkerFileUrls: {
        ...workerUrls,
      },
      checkWorkersOnInit: true,
      openDocumentDefaults: {
        minimumChunkSize: 1000,
        mode: AcEdOpenMode.Read,
        progressiveRendering: false,
        sysVars: {
          lwdisplay: true,
        },
      },
    })
    if (!currentManager) throw new Error('MLightCAD 初始化失败')
    manager = currentManager
    managerGeneration = generation
    const isCurrent = () => generation === openGeneration && !disposed && manager === currentManager

    await installCadFontDiagnostics(currentManager)
    if (!isCurrent()) return

    // 先加载 GDT/SHX，再打开图纸，避免 TOLERANCE 首帧被普通字母字体替代。
    await preloadCadSymbolFonts(currentManager)

    if (!isCurrent()) return

    // 按原始图纸名称交给 MLightCAD，让它依据 .dwg 后缀选择 DWG 解析器。
    const fileName = props.fileName || url.split('?')[0]?.split('/').pop() || 'drawing.dwg'
    const opened = await currentManager.openDocument(fileName, buffer, {
      minimumChunkSize: 1000,
      mode: AcEdOpenMode.Read,
      drawNoPlotLayers: false,
      progressiveRendering: false,
      sysVars: {
        lwdisplay: true,
      },
    })
    if (!isCurrent()) return
    if (!opened) throw new Error(`MLightCAD 无法解析图纸（传入文件名: ${fileName}）`)
    normalizeCadToleranceEntities(currentManager.curDocument.database)
    const wipeoutMasks = findWipeoutMasks(currentManager.curDocument.database)
    const layers = currentManager.curDocument.layerStore.getLayers().map((layer: any) => ({
      name: layer.name,
      color: layer.cssColor || '#00f3ff',
      visible: layer.isOn && !layer.isFrozen,
    }))
    emit('layers-loaded', layers)
    await finishInitialRender(currentManager, isCurrent)
    if (!isCurrent()) return
    if (wipeoutMasks.length > 0) {
      currentManager.curView.removeEntity(wipeoutMasks)
      currentManager.curView.isDirty = true
      currentManager.curView.isHtmlDirty = true
    }
    emit('zoom-change', 1)
    await nextTick()
    if (generation === openGeneration && !disposed) {
      loading.value = false
      emit('ready')
    }
  } catch (error: any) {
    if (managerGeneration === generation) await destroyViewer()
    if (generation === openGeneration && !disposed) {
      errorMessage.value = `MLightCAD 加载失败: ${error?.message || error}`
      emit('load-error', errorMessage.value)
    }
  } finally {
    if (generation === openGeneration) loading.value = false
  }
}

function setLayerVisibility(layerName: string, visible: boolean) {
  manager?.curDocument.layerStore.setLayerOn(layerName, visible)
}

// 批注覆盖层会挡住画布上的原生滚轮缩放与中键平移，这里改为直接调整视图相机。
// 旧的 sendStringToExecute('zoom\n2x\n') 走 ZOOM 命令的关键字流程，不接受 "2x" 这类
// 比例输入，最终什么都不做，所以使用批注工具时滚轮缩放完全没有反应。
const ZOOM_NOTCH = 1.15
let dataModelPromise: Promise<typeof import('@mlightcad/data-model')> | null = null
let viewActionQueue: Promise<unknown> = Promise.resolve()

function loadDataModel() {
  dataModelPromise ??= import('@mlightcad/data-model')
  return dataModelPromise
}

/** 串行执行视图手势，避免连续滚轮事件都基于同一份旧视图状态计算而互相覆盖。 */
function queueViewAction(action: () => Promise<void>) {
  viewActionQueue = viewActionQueue.then(action, action).catch(() => undefined)
}

/** 当前可见的图纸世界坐标范围，用于把缩放/平移换算成目标视框。 */
function visibleWorldBox(view: any) {
  const topLeft = view.screenToWorld({ x: 0, y: 0 })
  const bottomRight = view.screenToWorld({ x: view.width, y: view.height })
  return {
    minX: Math.min(topLeft.x, bottomRight.x), minY: Math.min(topLeft.y, bottomRight.y),
    maxX: Math.max(topLeft.x, bottomRight.x), maxY: Math.max(topLeft.y, bottomRight.y),
  }
}

/** 缩放：factor > 1 放大；anchor 为屏幕坐标锚点，缺省围绕视图中心缩放。 */
function zoomBy(factor: number, anchor?: Point) {
  if (!Number.isFinite(factor) || factor <= 0) return
  const view = manager?.curView
  if (!view || loading.value) return
  queueViewAction(async () => {
    if (manager?.curView !== view) return
    const { AcGeBox2d } = await loadDataModel()
    if (manager?.curView !== view) return
    const next = zoomViewBox(visibleWorldBox(view), factor, anchor ? view.screenToWorld({ x: anchor.x, y: anchor.y }) : undefined)
    if (!next) return
    view.zoomTo(new AcGeBox2d({ x: next.minX, y: next.minY }, { x: next.maxX, y: next.maxY }), 1)
  })
}

/** 平移：dx/dy 为屏幕像素位移，图纸内容跟随手势移动。 */
function panBy(dx: number, dy: number) {
  if (!Number.isFinite(dx) || !Number.isFinite(dy) || (!dx && !dy)) return
  const view = manager?.curView
  if (!view || loading.value) return
  queueViewAction(async () => {
    if (manager?.curView !== view) return
    const { AcGeBox2d } = await loadDataModel()
    if (manager?.curView !== view) return
    // 用相邻像素的图纸坐标差推出单位向量，避免依赖 y 轴方向假设。
    const origin = view.screenToWorld({ x: 0, y: 0 })
    const unit = (point: Point) => ({ x: point.x - origin.x, y: point.y - origin.y })
    const next = panViewBox(
      visibleWorldBox(view), dx, dy,
      unit(view.screenToWorld({ x: 1, y: 0 })),
      unit(view.screenToWorld({ x: 0, y: 1 })),
    )
    if (!next) return
    view.zoomTo(new AcGeBox2d({ x: next.minX, y: next.minY }, { x: next.maxX, y: next.maxY }), 1)
  })
}

function zoomIn() {
  zoomBy(2)
}

function zoomOut() {
  zoomBy(0.5)
}

async function resetView() {
  if (comparisonBounds) {
    const bounds = comparisonBounds
    const { AcGeBox2d } = await import('@mlightcad/data-model')
    const padding = Math.max(bounds.maxX - bounds.minX, bounds.maxY - bounds.minY, 1) * 0.05
    manager?.curView.zoomTo(new AcGeBox2d({ x: bounds.minX - padding, y: bounds.minY - padding }, { x: bounds.maxX + padding, y: bounds.maxY + padding }))
  } else manager?.curView.zoomToFitDrawing(60_000)
  emit('zoom-change', 1)
}

function extractTitleBlock() {
  if (!manager?.curDocument?.database || loading.value || errorMessage.value) throw new Error('请等待图纸加载完成后再提取')
  return { spaces: collectTitleSpaces(manager.curDocument.database), activeSpaceId: String(manager.curView.activeLayoutBtrId) }
}

function annotationViewport(): AnnotationViewport {
  const view = manager?.curView
  if (!view || loading.value) throw new Error('请等待图纸加载完成')
  return {
    toScreen: (point: Point) => view.worldToScreen(point),
    toDrawing: (point: Point) => view.screenToWorld(point),
    layout: () => String(view.activeLayoutBtrId),
    subscribe(callback) {
      view.events.viewChanged.addEventListener(callback)
      return () => view.events.viewChanged.removeEventListener(callback)
    },
    focus(points) {
      if (!points.length) return
      const xs = points.map(p => p.x), ys = points.map(p => p.y)
      const x = Math.min(...xs), y = Math.min(...ys), width = Math.max(...xs) - x, height = Math.max(...ys) - y
      const padding = Math.max(width, height, 10) * 0.8
      void import('@mlightcad/data-model').then(({ AcGeBox2d }) => {
        if (manager?.curView === view) view.zoomTo(new AcGeBox2d({ x: x - padding, y: y - padding }, { x: x + width + padding, y: y + height + padding }))
      })
    },
    // direction 为滚轮档数（>0 放大），anchor 为屏幕坐标锚点，缩放围绕鼠标位置进行。
    zoom(direction, anchor) { if (!Number.isFinite(direction) || !direction) return; zoomBy(Math.pow(ZOOM_NOTCH, direction), anchor) },
    pan(dx, dy) { panBy(dx, dy) },
  }
}

defineExpose({ setLayerVisibility, zoomIn, zoomOut, resetView, compareDrawing, focusDifference, clearComparison, extractTitleBlock, annotationViewport })

watch(() => props.dxfUrl, (url) => {
  if (url) void loadViewer(url)
  else {
    openGeneration++
    loading.value = false
    void destroyViewer()
  }
})

onMounted(() => {
  disposed = false
  console.info('[MlightCadViewer] 组件已挂载', {
    url: props.dxfUrl,
    fileName: props.fileName,
  })
  if (props.dxfUrl) void loadViewer(props.dxfUrl)
})

onUnmounted(() => {
  disposed = true
  openGeneration++
  void destroyViewer()
})
</script>

<template>
  <div ref="containerRef" class="mlightcad-viewer">
    <div v-if="loading" class="mlightcad-status">正在加载 MLightCAD 渲染引擎...</div>
    <div v-if="errorMessage" class="mlightcad-error">{{ errorMessage }}</div>
    <svg v-if="compareMarks.length" class="compare-marks" aria-label="图纸差异标注">
      <g v-for="mark in compareMarks" :key="mark.id" :class="[mark.kind, { selected: mark.id === selectedDifference }]">
        <rect :x="mark.x" :y="mark.y" :width="mark.width" :height="mark.height" rx="4" />
        <text :x="mark.x + 4" :y="mark.y - 4">{{ mark.id }}</text>
      </g>
    </svg>
  </div>
</template>

<style scoped>
.compare-marks { position: absolute; inset: 0; width: 100%; height: 100%; pointer-events: none; z-index: 3; }
.compare-marks g { fill: none; stroke: #f59e0b; stroke-width: 1.5; stroke-dasharray: 6 3; }
.compare-marks .added { stroke: #22c55e; }
.compare-marks .deleted { stroke: #e11d48; }
.compare-marks .selected { stroke-width: 3; stroke-dasharray: none; }
.compare-marks text { fill: white; stroke: #111; stroke-width: 3; paint-order: stroke; font: bold 13px sans-serif; }
.mlightcad-viewer {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 400px;
  overflow: hidden;
  background: #000;
}

.mlightcad-status,
.mlightcad-error {
  position: absolute;
  inset: 0;
  z-index: 5;
  display: grid;
  place-items: center;
  padding: 20px;
  color: #cbd5e1;
  background: rgba(10, 13, 20, 0.86);
}

.mlightcad-error {
  color: #f87171;
}
</style>
