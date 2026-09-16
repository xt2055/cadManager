<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { auditService } from '@/app/container'
// 已废弃：前端不再使用 Canvas DXF 渲染器，保留原组件引用以便回溯。
// import CadVectorViewer from '@/features/drawings/detail-tabs/preview/CadVectorViewer.vue'
import MlightCadViewer from '@/features/drawings/detail-tabs/preview/MlightCadViewer.vue'
import DrawingInfoPanel from '@/features/drawings/components/detail/DrawingInfoPanel.vue'
import type { TitleSpace } from '@/features/drawings/detail-tabs/preview/cad-title-block'
import { useDrawingStore } from '@/stores/drawing.store'
import { getApiBaseUrl } from '@/services/api-base.service'
import type { FileView } from '@/modules/drawing'
import ReviewAnnotationBoard from '@/features/reviews/components/ReviewAnnotationBoard.vue'
import type { AnnotationWorkspace, AnnotationViewport } from '@/features/reviews/annotation-model'
import { reviewAnnotationService } from '@/services/review-annotation.service'
import { reviewCaseService } from '@/services/review-case.service'

defineOptions({
  name: 'DrawingViewerPage',
})

const route = useRoute()
const router = useRouter()
const drawingStore = useDrawingStore()

const drawingId = computed(() => String(route.params.drawingId ?? ''))
const fileId = computed(() => String(route.query.fileId ?? ''))
// 历史版本在线浏览：携带 versionId/versionKey 时按版本精确加载该版本内容，
// 不会误用当前最新文件，也不会把 EXB 版本直接丢给浏览器 CAD 引擎。
const versionId = computed(() => String(route.query.versionId ?? ''))
const versionKey = computed(() => String(route.query.versionKey ?? ''))
const fromReview = computed(() => route.query.from === 'review')
const annotationVisible = ref(fromReview.value)
const annotationWorkspace = shallowRef<AnnotationWorkspace | null>(null)
const annotationView = shallowRef<AnnotationViewport | null>(null)
const annotationError = ref('')
let loadGeneration = 0

function handleViewerReady() {
  viewerReady.value = true
  annotationView.value = mlightCadViewerRef.value?.annotationViewport() ?? null
}
async function reloadAnnotations() {
  const scope = annotationWorkspace.value
  if (!scope) { void loadTargetFile(); return }
  try { annotationWorkspace.value = await reviewAnnotationService.load(scope.caseId, scope.attachmentId); annotationError.value = '' }
  catch (e) { annotationError.value = e instanceof Error ? e.message : '读取批注失败' }
}

const currentDrawing = computed(() => drawingStore.getDrawing(drawingId.value) ?? drawingStore.getPart(drawingId.value))
const isAssembly = computed(() => !currentDrawing.value || !('parentNo' in currentDrawing.value))

// 当前指定查看的图纸文件
const targetFile = ref<FileView | null>(null)
// 已废弃：Canvas DXF 地址保留，不再参与详情页渲染。
// const cadDxfUrl = ref<string | null>(null)
const cadOriginalUrl = ref<string | null>(null)
const cadSourceFileName = ref<string | null>(null)
const cadOriginalError = ref('')
const conversionState = ref('')
let conversionTimer: ReturnType<typeof setTimeout> | undefined
let disposed = false
async function checkConversion(retry = false) {
  const id = targetFile.value?.id
  if (!id || disposed || versionId.value || versionKey.value) return
  if (conversionTimer) clearTimeout(conversionTimer)
  try {
    const response = await fetch(`${getApiBaseUrl()}/cad/conversions/${encodeURIComponent(id)}`, {
      method: retry ? 'POST' : 'GET', headers: { Authorization: `Bearer ${getAccessToken()}` },
      signal: AbortSignal.timeout(10000),
    })
    const body = await response.json()
    if (disposed || targetFile.value?.id !== id) return
    if (!response.ok) throw new Error(body.message || '读取转换状态失败')
    const data = body.data ?? body
    conversionState.value = data.status
    if (data.status === 'ready') { await loadTargetFile(); return }
    cadOriginalError.value = ({ pending: '文件已上传，正在排队转换', processing: '正在转换，完成后自动加载', retry: '转换失败，等待自动重试', backoff: '转换失败，等待重试', failed: '转换失败，请点击重新转换', missing: '未找到转换任务' } as Record<string,string>)[data.status] || '转换状态不可用'
    if (['pending','processing','retry','backoff'].includes(data.status)) conversionTimer = setTimeout(() => void checkConversion(), 3000)
  } catch (error) { cadOriginalError.value = error instanceof Error ? error.message : String(error) }
}
// 已废弃：Canvas 查看器引用保留，不再挂载。
// const cadViewerRef = ref<InstanceType<typeof CadVectorViewer> | null>(null)
const mlightCadViewerRef = ref<InstanceType<typeof MlightCadViewer> | null>(null)
const viewerReady = ref(false)
const titleResult = ref<{ spaces: TitleSpace[]; activeSpaceId: string } | null>(null)
const titleError = ref('')

function extractDrawingInfo() {
  titleError.value = ''
  try {
    titleResult.value = mlightCadViewerRef.value?.extractTitleBlock() ?? null
  } catch (error) {
    titleResult.value = null
    titleError.value = error instanceof Error ? error.message : '提取图纸信息失败'
  }
}

// 视图与图层控制
const layerPanelVisible = ref(true)
const dynamicLayers = ref<Array<{ name: string; color: string; visible: boolean }>>([])
const zoomLevel = ref(1)
// 已废弃：渲染引擎切换已固定为 MLightCAD，保留状态供旧逻辑回溯。
const renderEngine = ref<'canvas' | 'mlightcad'>('mlightcad')
const renderEngineKey = ref(0)

const zoomText = computed(() => `${Math.round(zoomLevel.value * 100)}%`)

function getAccessToken() {
  return localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
}

function revokeOriginalUrl() {
  if (cadOriginalUrl.value) URL.revokeObjectURL(cadOriginalUrl.value)
  cadOriginalUrl.value = null
  cadSourceFileName.value = null
}

async function loadTargetFile() {
  const generation = ++loadGeneration
  viewerReady.value = false
  annotationView.value = null
  annotationWorkspace.value = null
  annotationError.value = ''
  titleResult.value = null
  titleError.value = ''
  if (conversionTimer) clearTimeout(conversionTimer)
  conversionState.value = ''
  await drawingStore.load()
  if (generation !== loadGeneration || disposed) return

  // 优先按 fileId 在当前总图及其全部零件中精确查找，避免零件文件回退到总图。
  let file: FileView | undefined
  const currentFiles = currentDrawing.value
    ? [...currentDrawing.value.files, ...currentDrawing.value.otherFiles]
    : []
  const structureFiles = drawingStore.parts.flatMap((part) => [...part.files, ...part.otherFiles])

  if (fileId.value) {
    file = [...currentFiles, ...structureFiles].find((candidate) => candidate.id === fileId.value)
  } else if (currentFiles.length > 0) {
    file = currentFiles[0]
  }

  targetFile.value = file || null
  dynamicLayers.value = []
  cadOriginalError.value = ''
  revokeOriginalUrl()

  if (file) {
    let pinnedVersion = versionId.value
    try {
      const cases = await reviewCaseService.list()
      if (generation !== loadGeneration || disposed) return
      const requestedCase = String(route.query.reviewCaseId ?? '')
      const review = requestedCase ? cases.find(c => c.id === requestedCase) : cases.find(c => c.drawingNo === (currentDrawing.value?.no || drawingId.value))
      if (review) {
        const scope = await reviewAnnotationService.load(review.id, file.id)
        if (generation !== loadGeneration || disposed) return
        if (versionKey.value || (versionId.value && versionId.value !== scope.versionId) || (!fromReview.value && !versionId.value && file.currentVersionId !== scope.versionId)) {
          annotationError.value = '当前查看的版本与本轮审核版本不同，请从审核工作台打开对应文件。'
        } else { annotationWorkspace.value = scope; pinnedVersion = scope.versionId }
      } else annotationError.value = '此图纸尚未发起审核，发起审核后可在这里添加批注。'
    } catch (e) { if (generation !== loadGeneration || disposed) return; annotationError.value = e instanceof Error ? e.message : '读取审核批注失败' }
    const isCad = file.name.toLowerCase().endsWith('.exb') || file.name.toLowerCase().endsWith('.dxf') || file.name.toLowerCase().endsWith('.dwg')
    const storageKeyValue = file.storageKey || ''
    if (isCad && (storageKeyValue || pinnedVersion)) {
      const baseUrl = getApiBaseUrl()
      if (storageKeyValue || pinnedVersion) {
        try {
          const token = getAccessToken()
          const cacheBuster = Date.now()
          // 历史版本按存储键精确读取该版本；无版本记录（旧数据）时回退当前文件源。
          let response: Response | null = null
          if (versionKey.value) {
            response = await fetch(`${baseUrl}/file-versions/source?storageKey=${encodeURIComponent(versionKey.value)}&_t=${cacheBuster}`, {
              headers: token ? { Authorization: `Bearer ${token}` } : {},
              credentials: 'include',
            })
            if (!response.ok) response = null
          }
          if (!response) {
            if (pinnedVersion) {
              response = await fetch(`${baseUrl}/file-versions/${encodeURIComponent(pinnedVersion)}/source?_t=${cacheBuster}`, {
                headers: token ? { Authorization: `Bearer ${token}` } : {},
                credentials: 'include',
              })
            } else if (storageKeyValue) {
              response = await fetch(`${baseUrl}/cad/source?attachmentId=${encodeURIComponent(file.id)}&_t=${cacheBuster}`, {
                headers: token ? { Authorization: `Bearer ${token}` } : {},
                credentials: 'include',
              })
            }
          }
          if (!response) {
            throw new Error('无法确定 CAD 源文件地址')
          }
           if (!response.ok) {
             if (response.status === 409 && !pinnedVersion && !versionKey.value) { void checkConversion(); return }
             const body = await response.json().catch(() => ({}))
             throw new Error(body.message || `HTTP ${response.status}`)
           }
          const sourceName = `${file.name.replace(/\.(exb|dxf|dwg)$/i, '')}.dwg`
          const contentType = response.headers.get('content-type') || ''
          if (contentType.includes('text/html') || contentType.includes('application/json')) {
            throw new Error(`渲染源接口返回了错误内容类型: ${contentType}`)
          }
           const blob = await response.blob()
           if (generation !== loadGeneration || disposed) return
           cadOriginalUrl.value = URL.createObjectURL(blob)
           cadSourceFileName.value = sourceName
           console.info('[DrawingViewerPage] 原始 CAD 已加载', { sourceName, storageKey: file.storageKey, versionId: versionId.value })
        } catch (error) {
          if (generation !== loadGeneration || disposed) return
          cadOriginalError.value = error instanceof Error ? error.message : String(error)
          console.warn('读取原始 CAD 文件失败，MLightCAD 将不可用', error)
        }
      } else {
        cadOriginalError.value = '当前图纸没有 storageKey，无法读取原始 DWG'
      }
    } else if (isCad) {
      cadOriginalError.value = '当前图纸没有 storageKey，无法读取原始 DWG'
    } else {
      // 已废弃：Canvas DXF 地址清理逻辑保留，不再执行。
      // cadDxfUrl.value = null
      revokeOriginalUrl()
    }

    void auditService.record({
      drawingNo: file.partNo || file.drawingNo,
      targetType: 'file',
      act: 'view',
      txt: `独立浏览 CAD 图纸 <b>${file.name}</b>`,
      detail: { fileId: file.id, fileName: file.name },
    })
  }
}

function handleLayersLoaded(layers: Array<{ name: string; color: string; visible: boolean }>) {
  dynamicLayers.value = layers
}

function toggleDynamicLayer(layer: { name: string; color: string; visible: boolean }) {
  mlightCadViewerRef.value?.setLayerVisibility(layer.name, layer.visible)
  // 已废弃：Canvas 图层控制逻辑保留，不再执行。
  // if (renderEngine.value === 'canvas') {
  //   cadViewerRef.value?.setLayerVisibility(layer.name, layer.visible)
  // } else {
  //   mlightCadViewerRef.value?.setLayerVisibility(layer.name, layer.visible)
  // }
}

function handleZoomChange(val: number) {
  zoomLevel.value = val
}

function zoomIn() {
  mlightCadViewerRef.value?.zoomIn()
  // 已废弃：Canvas 缩放逻辑保留，不再执行。
  // if (renderEngine.value === 'canvas') cadViewerRef.value?.zoomIn()
  // else mlightCadViewerRef.value?.zoomIn()
}

function zoomOut() {
  mlightCadViewerRef.value?.zoomOut()
  // 已废弃：Canvas 缩放逻辑保留，不再执行。
  // if (renderEngine.value === 'canvas') cadViewerRef.value?.zoomOut()
  // else mlightCadViewerRef.value?.zoomOut()
}

function resetView() {
  mlightCadViewerRef.value?.resetView()
  // 已废弃：Canvas 复位逻辑保留，不再执行。
  // if (renderEngine.value === 'canvas') cadViewerRef.value?.resetView()
  // else mlightCadViewerRef.value?.resetView()
}

// 已废弃：Canvas/MLightCAD 切换逻辑保留，不再提供切换入口。
// function selectRenderEngine(engine: 'canvas' | 'mlightcad') {
//   if (renderEngine.value === engine) return
//   console.info('[DrawingViewerPage] 切换 CAD 引擎', {
//     engine,
//     hasOriginalCad: Boolean(cadOriginalUrl.value),
//     originalError: cadOriginalError.value || undefined,
//   })
//   renderEngine.value = engine
//   renderEngineKey.value++
//   zoomLevel.value = 1
// }

function goBack() {
  if (fromReview.value) {
    router.push({ name: 'review-workspace', params: { drawingNo: drawingId.value } })
    return
  }
  router.push({ name: 'drawing-preview', params: { drawingId: drawingId.value } })
}

onMounted(() => {
  void loadTargetFile()
})

onUnmounted(() => {
  disposed = true
  if (conversionTimer) clearTimeout(conversionTimer)
  revokeOriginalUrl()
})

watch([drawingId, fileId, versionId, versionKey, () => route.query.reviewCaseId], () => {
  void loadTargetFile()
})
</script>

<template>
  <div class="drawing-standalone-viewer">
    <!-- 顶部导航与操作栏：与全局主题严格协同融合 -->
    <header class="viewer-header">
      <div class="header-left">
        <button class="back-btn" type="button" :title="fromReview ? '返回审核工作台' : '返回图纸文件列表'" @click="goBack">
          <DemoIcon name="arrow-left" :size="15" />
          <span>{{ fromReview ? '返回审核' : '返回图纸' }}</span>
        </button>
        <div class="divider"></div>
        <div class="drawing-meta">
          <span class="tag tag-no-dot" :class="isAssembly ? 'plain' : 'info'">
            {{ isAssembly ? '总图' : '零件图' }}
          </span>
          <span class="file-name-highlight">{{ targetFile?.name || currentDrawing?.name }}</span>
          <span class="drawing-no">{{ targetFile?.partNo || targetFile?.drawingNo || currentDrawing?.no }}</span>
        </div>
      </div>

      <div class="header-right">
        <button class="toggle-btn" :class="{ active: annotationVisible }" type="button" :aria-pressed="annotationVisible" @click="annotationVisible = !annotationVisible; if (annotationVisible) layerPanelVisible = false">审核批注</button>
        <button class="toggle-btn" type="button" :disabled="!viewerReady" @click="extractDrawingInfo">提取图纸信息</button>
        <button class="toggle-btn" type="button" @click="router.push({ name: 'drawing-compare', params: { drawingId }, query: { fileId, versionId: versionId || undefined, versionKey: versionKey || undefined } })">图纸对比</button>
        <!-- 已废弃：Canvas/MLightCAD 切换入口保留，不再显示，当前固定使用 MLightCAD。 -->
        <!--
        <div class="engine-switcher" role="group" aria-label="CAD 渲染引擎">
          <button
            class="engine-btn"
            :class="{ active: renderEngine === 'canvas' }"
            type="button"
            @click="selectRenderEngine('canvas')"
          >
            Canvas
          </button>
          <button
            class="engine-btn"
            :class="{ active: renderEngine === 'mlightcad' }"
            type="button"
            @click="selectRenderEngine('mlightcad')"
          >
            MLightCAD
          </button>
        </div>
        -->

        <div class="view-controls">
          <button class="ctrl-btn" type="button" title="缩小" @click="zoomOut">
            <DemoIcon name="zoom-out" :size="15" />
          </button>
          <span class="zoom-value">{{ zoomText }}</span>
          <button class="ctrl-btn" type="button" title="放大" @click="zoomIn">
            <DemoIcon name="zoom-in" :size="15" />
          </button>
          <button class="ctrl-btn" type="button" title="自适应居中" @click="resetView">
            <DemoIcon name="maximize" :size="15" />
          </button>
        </div>

        <div class="divider"></div>

        <button
          class="toggle-btn"
          :class="{ active: layerPanelVisible }"
          type="button"
          title="切换图层面板"
          @click="layerPanelVisible = !layerPanelVisible"
        >
          <DemoIcon name="layers" :size="15" />
          <span>图层 ({{ dynamicLayers.length }})</span>
        </button>
      </div>
    </header>

    <p v-if="titleError" role="alert" class="extraction-error">{{ titleError }}</p>

    <!-- 核心画布工作区 -->
    <div class="viewer-body">
      <!-- 中间 CAD 矢量图画板 -->
      <main class="viewer-canvas-container">
        <ReviewAnnotationBoard :workspace="annotationWorkspace" :viewport="annotationView" :enabled="annotationVisible" :error="annotationError" @reload="reloadAnnotations">
        <!-- 已废弃：Canvas DXF 渲染组件保留，不再挂载。 -->
        <!--
        <CadVectorViewer
          v-if="cadDxfUrl && renderEngine === 'canvas'"
          :key="`canvas-${renderEngineKey}`"
          ref="cadViewerRef"
          :dxf-url="cadDxfUrl"
          :file-name="targetFile?.name"
          @layers-loaded="handleLayersLoaded"
          @zoom-change="handleZoomChange"
        />
        -->
        <MlightCadViewer
          v-if="cadOriginalUrl && renderEngine === 'mlightcad'"
          :key="`mlightcad-${renderEngineKey}`"
          ref="mlightCadViewerRef"
          :dxf-url="cadOriginalUrl"
          :file-name="cadSourceFileName"
          @ready="handleViewerReady"
          @load-error="viewerReady = false; titleResult = null; annotationView = null"
          @layers-loaded="handleLayersLoaded"
          @zoom-change="handleZoomChange"
        />
        <div v-else class="empty-prompt">
          <DemoIcon name="file-question" :size="48" />
          <p>{{ targetFile ? (cadOriginalError || '原始 CAD 文件不可用，无法使用 MLightCAD') : '暂无选中的图纸文件' }}</p>
          <button v-if="['failed','backoff','retry'].includes(conversionState)" class="btn primary" @click="checkConversion(true)">重新转换</button>
        </div>
        </ReviewAnnotationBoard>
      </main>

      <!-- 右侧图层管理器面板（可收起） -->
      <DrawingInfoPanel v-if="titleResult" :spaces="titleResult.spaces" :active-space-id="titleResult.activeSpaceId" :file-name="targetFile?.name || ''" @close="titleResult = null" />
      <aside v-else-if="layerPanelVisible && !annotationVisible" class="side-panel layer-panel">
        <div class="panel-header">
          <DemoIcon name="layers" :size="14" />
          <span>图层控制 ({{ dynamicLayers.length }})</span>
        </div>
        <div class="panel-content">
          <div v-if="dynamicLayers.length === 0" class="no-layers">
            正在读取 CAD 图层拓扑...
          </div>
          <label
            v-for="layer in dynamicLayers"
            :key="layer.name"
            class="layer-row"
          >
            <input
              v-model="layer.visible"
              type="checkbox"
              class="layer-checkbox"
              @change="toggleDynamicLayer(layer)"
            />
            <span class="layer-name" :title="layer.name">{{ layer.name }}</span>
            <span class="layer-color-dot" :style="{ background: layer.color }"></span>
          </label>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.extraction-error { margin: 0; padding: 10px 16px; color: var(--text-1); background: var(--panel); border-bottom: 1px solid var(--line); }
.toggle-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.drawing-standalone-viewer {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: var(--cad-bg, #1a1d24);
  color: var(--text-1);
  overflow: hidden;
  position: relative;
}

/* 顶部导航栏，采用系统主题的 --panel 和 --line 变量 */
.viewer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 50px;
  padding: 0 16px;
  background: var(--panel-top, var(--panel));
  border-bottom: 1px solid var(--line);
  z-index: 20;
  flex-shrink: 0;
  gap: 16px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.back-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: var(--radius-sm, 6px);
  background: var(--panel-2);
  border: 1px solid var(--line);
  color: var(--text-1);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.back-btn:hover {
  background: var(--hover);
  border-color: var(--accent);
  color: var(--accent);
}

.divider {
  width: 1px;
  height: 18px;
  background: var(--line);
}

.drawing-meta {
  display: flex;
  align-items: center;
  gap: 10px;
}

.file-name-highlight {
  font-size: 14.5px;
  font-weight: 600;
  color: var(--text-1);
  max-width: 320px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.drawing-no {
  font-size: 12px;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  background: var(--panel-2);
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid var(--line);
}

/* 头部右侧操作区 */
.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

/* 已废弃：Canvas/MLightCAD 切换入口样式保留，不再使用。
.engine-switcher {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm, 6px);
  background: var(--panel-2);
}

.engine-btn {
  padding: 5px 9px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--text-3);
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.engine-btn:hover {
  color: var(--text-1);
  background: var(--hover);
}

.engine-btn.active {
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
}
*/

.view-controls {
  display: flex;
  align-items: center;
  gap: 4px;
  background: var(--panel-2);
  padding: 2px 4px;
  border-radius: var(--radius-sm, 6px);
  border: 1px solid var(--line);
}

.ctrl-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 4px;
  background: transparent;
  border: none;
  color: var(--text-2);
  cursor: pointer;
  transition: all 0.15s ease;
}

.ctrl-btn:hover {
  background: var(--hover);
  color: var(--accent);
}

.zoom-value {
  font-size: 12px;
  font-family: 'JetBrains Mono', monospace;
  padding: 0 6px;
  color: var(--accent);
  min-width: 44px;
  text-align: center;
}

.toggle-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: var(--radius-sm, 6px);
  background: var(--panel-2);
  border: 1px solid var(--line);
  color: var(--text-2);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.toggle-btn:hover {
  color: var(--text-1);
  background: var(--hover);
  border-color: var(--accent);
}

.toggle-btn.active {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--accent);
  font-weight: 500;
}

/* 主画布区与右侧图层面板 */
.viewer-body {
  display: flex;
  flex: 1;
  width: 100%;
  height: calc(100% - 50px);
  overflow: hidden;
  position: relative;
}

.side-panel {
  width: 220px;
  height: 100%;
  background: var(--panel);
  border-left: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  z-index: 10;
  flex-shrink: 0;
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-3);
  border-bottom: 1px solid var(--line);
  letter-spacing: 0.5px;
}

.panel-content {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* 图层列表项 */
.layer-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 12.5px;
  color: var(--text-2);
  cursor: pointer;
  user-select: none;
  transition: all 0.15s ease;
}

.layer-row:hover {
  background: var(--hover);
  color: var(--text-1);
}

.layer-checkbox {
  accent-color: var(--accent);
  cursor: pointer;
}

.layer-name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.layer-color-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 1px solid rgba(255, 255, 255, 0.2);
  flex-shrink: 0;
}

.no-layers {
  font-size: 12px;
  color: var(--text-3);
  text-align: center;
  padding: 24px 0;
}

/* 画布容器 */
.viewer-canvas-container {
  flex: 1;
  height: 100%;
  background: #0a0d14;
  position: relative;
  overflow: hidden;
}

.empty-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-3);
  gap: 12px;
  font-size: 14px;
}
</style>
