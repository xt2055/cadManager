<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { AcApDocManager, AcEdOpenMode } from '@mlightcad/cad-simple-viewer'
import { MlCadViewer } from '@mlightcad/cad-viewer'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { auditService } from '@/app/container'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import { windowService } from '@/services/tauri/window.service'
import { getApiBaseUrl } from '@/services/api-base.service'
import { installCadFontDiagnostics, normalizeCadToleranceEntities, preloadCadSymbolFonts, resolveCadFontsBaseUrl } from '@/services/cad-fonts.service'
import { registerCadConverters } from '@/services/cad-converters.service'
import { exportEditorDxf } from '@/services/cad-editor-export'
import type { FileView } from '@/modules/drawing'
import { assertCadWorkerAssets, getCadWorkerUrls } from '../detail-tabs/preview/cad-worker-assets'
import { findWipeoutMasks } from '../detail-tabs/preview/cad-entity-filters'

defineOptions({
  name: 'DrawingEditorPage',
})

const route = useRoute()
const router = useRouter()
const drawingOperationsStore = useDrawingOperationsStore()
const drawingStore = useDrawingStore()
const uiStore = useUiStore()

const drawingId = computed(() => String(route.params.drawingId ?? ''))
const fileId = computed(() => String(route.query.fileId ?? ''))

const currentDrawing = computed(() => drawingStore.getDrawing(drawingId.value) ?? drawingStore.getPart(drawingId.value))
const isAssembly = computed(() => !currentDrawing.value || !('parentNo' in currentDrawing.value))

const targetFile = ref<FileView | null>(null)
const cadOriginalFile = ref<File | null>(null)
const cadSourceFileName = ref<string | null>(null)
const cadOriginalError = ref('')
const isReady = ref(false)
const cadFontsRoot = ref('')
const useMainThreadCadDraw = import.meta.env.DEV
  && (new URLSearchParams(window.location.search).get('cad-main-thread') === '1'
    || window.localStorage.getItem('cad_use_main_thread_draw') !== '0')
const isSaving = ref(false)
const savedVersion = ref('')
// 存档变更工单在线编辑上下文：requiresTicket 为真时，保存走工单工作版本通道而非正式替换。
const requiresTicket = ref(false)
const changeRequestId = ref('')
const workVersionId = ref('')
const editRevision = ref<number>()
let editorDocumentActivatedListener: ((payload: { doc: any }) => void) | null = null
let editorWipeoutCleanupTimer: number | null = null

async function prepareCadEditor() {
  // 与只读预览使用同一套离线 CAD 字体，避免 GDT 等符号字体回退为普通字母。
  cadFontsRoot.value = await resolveCadFontsBaseUrl()
  await preloadCadSymbolFonts()
  const workerUrls = getCadWorkerUrls()
  await assertCadWorkerAssets(workerUrls)

  const { AcApDocManager } = await import('@mlightcad/cad-simple-viewer')

  if (!(await AcApDocManager.checkWebworkerReadiness(workerUrls))) {
    throw new Error(`CAD Worker 不可访问：${JSON.stringify(workerUrls)}`)
  }

  await registerCadConverters(workerUrls.dwgParser)
}

function getAccessToken() {
  return localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
}

function clearOriginalFile() {
  cadOriginalFile.value = null
  cadSourceFileName.value = null
}

async function onlineOpen(storageKey: string): Promise<{ requiresTicket: boolean; changeRequestId: string; workVersionId: string; loadUrl: string; fileName: string; revision?: number }> {
  const token = getAccessToken()
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/editing/online-open`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    credentials: 'include',
    body: JSON.stringify({ storageKey }),
  })
  if (!response.ok) {
    let message = `无法打开在线编辑：HTTP ${response.status}`
    try {
      const body: unknown = await response.json()
      if (body && typeof body === 'object' && 'message' in body) {
        const text = (body as { message?: unknown }).message
        if (typeof text === 'string' && text.trim()) message = text
      }
    } catch {
      // 非 JSON 错误响应保留默认提示
    }
    throw new Error(message)
  }
  const body = (await response.json()) as { data?: Record<string, unknown> }
  const data = (body?.data ?? body) as Record<string, unknown>
  if (typeof data.requiresTicket !== 'boolean' || typeof data.loadUrl !== 'string' || !data.loadUrl
    || (data.requiresTicket && !data.changeRequestId)) {
    throw new Error('在线编辑接口返回不完整，未回退到正式版本，请重新打开')
  }
  return {
    requiresTicket: Boolean(data.requiresTicket),
    changeRequestId: typeof data.changeRequestId === 'string' ? data.changeRequestId : '',
    workVersionId: typeof data.workVersionId === 'string' ? data.workVersionId : '',
    loadUrl: typeof data.loadUrl === 'string' ? data.loadUrl : '',
    fileName: typeof data.fileName === 'string' ? data.fileName : '',
    revision: typeof data.revision === 'number' ? data.revision : undefined,
  }
}

async function loadTargetFile() {
  isReady.value = false
  savedVersion.value = ''
  cadOriginalError.value = ''
  requiresTicket.value = false
  changeRequestId.value = ''
  workVersionId.value = ''
  editRevision.value = undefined
  clearOriginalFile()

  await drawingStore.load()

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

  if (file) {
    const isCad = file.name.toLowerCase().endsWith('.exb') || file.name.toLowerCase().endsWith('.dxf') || file.name.toLowerCase().endsWith('.dwg')
    if (isCad && file.storageKey) {
      try {
        const token = getAccessToken()
        const baseUrl = getApiBaseUrl()
        // 先向服务端确认工单归属与应加载内容：存档图纸只能在工作版本上编辑。
        const open = await onlineOpen(file.storageKey)
        if (!open.requiresTicket && (!Number.isSafeInteger(open.revision) || (open.revision ?? 0) < 1)) {
          throw new Error('缺少服务端文件修订号，请确认后端已更新后重新打开编辑器')
        }
        editRevision.value = open.revision
        requiresTicket.value = open?.requiresTicket ?? false
        changeRequestId.value = open?.changeRequestId ?? ''
        workVersionId.value = open?.workVersionId ?? ''
        const sourcePath = open.loadUrl
        const cacheBuster = Date.now()
        const separator = sourcePath.includes('?') ? '&' : '?'
        const response = await fetch(`${baseUrl}${sourcePath}${separator}_t=${cacheBuster}`, {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
          credentials: 'include',
        })
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const loadedName = open.fileName || file.name
        const sourceName = loadedName.replace(/\.exb$/i, '.dwg')
        const contentType = response.headers.get('content-type') || ''
        if (contentType.includes('text/html') || contentType.includes('application/json')) {
          throw new Error(`渲染源接口返回了错误内容类型: ${contentType}`)
        }
        cadOriginalFile.value = new File([await response.blob()], sourceName, {
          type: contentType || 'application/octet-stream',
        })
        cadSourceFileName.value = sourceName
        await prepareCadEditor()
        isReady.value = true
      } catch (error) {
        cadOriginalError.value = error instanceof Error ? error.message : String(error)
      }
    } else if (!isCad) {
      cadOriginalError.value = '当前文件不是可支持的 CAD 工程图'
    } else {
      cadOriginalError.value = '当前图纸缺少 storageKey，无法加载编辑源'
    }

    void auditService.record({
      drawingNo: file.partNo || file.drawingNo,
      targetType: 'file',
      act: 'edit',
      txt: `打开在线 CAD 编辑器 <b>${file.name}</b>`,
      detail: { fileId: file.id, fileName: file.name },
    })
  }
}

async function removeEditorWipeoutMasks(doc?: any) {
  try {
    const manager = AcApDocManager.instance
    const targetDoc = doc ?? manager.curDocument
    const normalizedCount = normalizeCadToleranceEntities(targetDoc.database)
    if (normalizedCount > 0) {
      manager.curView.updateEntity(targetDoc.database)
    }
    const wipeoutMasks = findWipeoutMasks(targetDoc.database)
    // 编辑模式由 AcApContext 监听数据库实体。不能用 removeEntity，
    // 否则上下文会在后续批量渲染时再次把 WIPEOUT 加回场景。
    for (const entity of wipeoutMasks) {
      manager.curView.updateEntityVisibility(entity)
    }
    if (wipeoutMasks.length === 0) return
    manager.curView.isDirty = true
    manager.curView.isHtmlDirty = true
    await manager.curView?.waitUntilIdle?.(60_000)
  } catch (error) {
    console.warn('在线 CAD 编辑器清理矩形填充失败', error)
  }
}

function scheduleEditorWipeoutCleanup() {
  if (editorWipeoutCleanupTimer) {
    window.clearTimeout(editorWipeoutCleanupTimer)
    editorWipeoutCleanupTimer = null
  }

  const delays = [0, 100, 300, 700, 1_500, 3_000, 6_000]
  let index = 0
  const run = () => {
    void removeEditorWipeoutMasks().finally(() => {
      index++
      if (index < delays.length) {
        editorWipeoutCleanupTimer = window.setTimeout(run, delays[index])
      } else {
        editorWipeoutCleanupTimer = null
      }
    })
  }
  editorWipeoutCleanupTimer = window.setTimeout(run, delays[0])
}

async function bindEditorDocumentCleanup() {
  const manager = AcApDocManager.instance
  await installCadFontDiagnostics(manager)
  await preloadCadSymbolFonts(manager)
  if (editorDocumentActivatedListener) {
    manager.events.documentActivated.removeEventListener(editorDocumentActivatedListener)
  }
  editorDocumentActivatedListener = ({ doc }) => {
    void removeEditorWipeoutMasks(doc)
    scheduleEditorWipeoutCleanup()
  }
  manager.events.documentActivated.addEventListener(editorDocumentActivatedListener)
  scheduleEditorWipeoutCleanup()
}

function goBack() {
  router.push({ name: 'drawing-preview', params: { drawingId: drawingId.value } })
}

function openReadonlyView() {
  if (!currentDrawing.value || !targetFile.value) return
  router.push({
    name: 'drawing-viewer',
    params: { drawingId: currentDrawing.value.no },
    query: { fileId: targetFile.value.id },
  })
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function convertDxfToDwgOnServer(dxfBlob: Blob, baseName: string): Promise<Blob> {
  const formData = new FormData()
  formData.append('file', dxfBlob, `${baseName}.dxf`)
  const token = getAccessToken()
  const baseUrl = getApiBaseUrl()
  const response = await fetch(`${baseUrl}/cad/convert-dwg`, {
    method: 'POST',
    headers: {
      Accept: 'application/acad, */*',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: formData,
    credentials: 'include',
  })
  if (!response.ok) {
    let message = `DXF 转换为 DWG 失败：HTTP ${response.status}`
    try {
      const errorBody: unknown = await response.json()
      if (typeof errorBody === 'object' && errorBody !== null && 'message' in errorBody) {
        const text = (errorBody as { message?: unknown }).message
        if (typeof text === 'string' && text.trim()) message = text
      }
    } catch {
      // 非 JSON 错误响应使用默认提示
    }
    throw new Error(message)
  }
  const blob = await response.blob()
  if (!/^AC10\d{2}$/.test(await blob.slice(0, 6).text())) {
    throw new Error('转换服务未返回有效 DWG，未上传保存，请检查转换插件')
  }
  return blob
}

async function onlineSaveDraft(storageKey: string, requestId: string, baseWorkVersionId: string, file: File): Promise<{ workVersionId: string; version: string }> {
  const token = getAccessToken()
  const baseUrl = getApiBaseUrl()
  const formData = new FormData()
  formData.append('storageKey', storageKey)
  formData.append('changeRequestId', requestId)
  formData.append('baseWorkVersionId', baseWorkVersionId)
  formData.append('file', file, file.name)
  const response = await fetch(`${baseUrl}/editing/online-save`, {
    method: 'POST',
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    credentials: 'include',
    body: formData,
  })
  if (!response.ok) {
    let message = `保存工单工作版本失败：HTTP ${response.status}`
    try {
      const body: unknown = await response.json()
      if (body && typeof body === 'object' && 'message' in body) {
        const text = (body as { message?: unknown }).message
        if (typeof text === 'string' && text.trim()) message = text
      }
    } catch {
      // 非 JSON 错误响应保留默认提示
    }
    throw new Error(message)
  }
  const body = (await response.json()) as { data?: Record<string, unknown> }
  const data = (body?.data ?? body) as Record<string, unknown>
  if (typeof data.workVersionId !== 'string' || !data.workVersionId || typeof data.version !== 'string' || !data.version) {
    throw new Error('保存响应不完整，无法确认保存结果，请保留当前页面并核对工单工作版本')
  }
  return {
    workVersionId: typeof data.workVersionId === 'string' ? data.workVersionId : '',
    version: typeof data.version === 'string' ? data.version : '',
  }
}

async function saveAsNewVersion() {
  if (!targetFile.value || !currentDrawing.value || isSaving.value) return

  const document = AcApDocManager.instance.curDocument
  if (!document?.database) {
    cadOriginalError.value = '编辑器尚未完成初始化，暂时无法保存'
    return
  }

  isSaving.value = true
  cadOriginalError.value = ''
  const savingTarget = targetFile.value
  const savingDrawingNo = savingTarget.partNo || savingTarget.drawingNo || currentDrawing.value.no
  const ticketMode = requiresTicket.value
  const savingRequestId = changeRequestId.value
  const savingWorkVersionId = workVersionId.value
  const savingRevision = editRevision.value
  try {
    await AcApDocManager.instance.curView?.waitUntilIdle?.(60_000)
    const dxfContent = exportEditorDxf(document.database)
    const dxfBlob = new Blob(
      [typeof dxfContent === 'string' ? dxfContent : new Uint8Array(dxfContent)],
      { type: 'application/dxf' },
    )
    const baseName = savingTarget.name.replace(/\.[^.]+$/, '') || 'drawing'
    // 浏览器编辑器只能导出 DXF；由后端经 CAXA 统一转换为 DWG 后再保存版本，
    // 保证图纸的当前文件与历史版本始终是可本地编辑/预览的 DWG。
    const dwgBlob = await convertDxfToDwgOnServer(dxfBlob, baseName)
    const savedFile = new File([dwgBlob], `${baseName}.dwg`, { type: 'application/acad' })

    if (ticketMode) {
      // 存档图纸：成果登记为变更工单的工作版本，不触碰正式当前指针，验收后才发布。
      if (!savingTarget.storageKey) throw new Error('缺少工作文件存储键，无法保存工单成果')
      const saved = await onlineSaveDraft(savingTarget.storageKey, savingRequestId, savingWorkVersionId, savedFile)
      workVersionId.value = saved.workVersionId
      savedVersion.value = saved.version
      uiStore.toast('已保存为变更工单工作版本，验收通过后才会发布正式版', 'ok')
      return
    }

    const updatedFile = await drawingOperationsStore.replaceDrawingFile(
      savingDrawingNo,
      savingTarget.id,
      {
        name: savedFile.name,
        size: formatFileSize(savedFile.size),
        replaceReason: '在线编辑保存新版本',
        expectedRevision: savingRevision,
      },
      savedFile,
    )
    cadSourceFileName.value = updatedFile.name
    editRevision.value = updatedFile.revision
    savedVersion.value = updatedFile.version
    try {
      await drawingStore.refresh()
    } catch {
      uiStore.toast('文件已保存，但列表刷新失败，请稍后刷新；无需重复保存', 'warn')
    }
    targetFile.value = [
      ...drawingStore.drawings.flatMap((item) => [...item.files, ...item.otherFiles]),
      ...drawingStore.parts.flatMap((part) => [...part.files, ...part.otherFiles]),
    ].find((file) => file.id === updatedFile.id) ?? savingTarget
  } catch (error) {
    cadOriginalError.value = error instanceof Error ? error.message : String(error)
    uiStore.toast(`保存新版本失败：${cadOriginalError.value}`, 'warn')
  } finally {
    isSaving.value = false
  }
}

function continueEditing() {
  savedVersion.value = ''
}

function canLeaveEditor() {
  if (!isSaving.value) return true
  uiStore.toast('正在保存，请等待完成后再离开', 'warn')
  return false
}

function warnWhileSaving(event: BeforeUnloadEvent) {
  if (!isSaving.value) return
  event.preventDefault()
  event.returnValue = ''
}

onBeforeRouteLeave(canLeaveEditor)
onBeforeRouteUpdate(canLeaveEditor)

async function startDragging() {
  try {
    await windowService.startDragging()
  } catch (error) {
    // 浏览器预览模式没有原生窗口，忽略拖动调用即可。
    if (typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window) {
      console.warn('拖动编辑窗口失败', error)
    }
  }
}

onMounted(() => {
  window.addEventListener('beforeunload', warnWhileSaving)
  void loadTargetFile()
})

onUnmounted(() => {
  window.removeEventListener('beforeunload', warnWhileSaving)
  if (editorWipeoutCleanupTimer) {
    window.clearTimeout(editorWipeoutCleanupTimer)
    editorWipeoutCleanupTimer = null
  }
  try {
    if (editorDocumentActivatedListener) {
      AcApDocManager.instance.events.documentActivated.removeEventListener(editorDocumentActivatedListener)
      editorDocumentActivatedListener = null
    }
  } catch {
    // CAD 编辑器可能已经先于页面卸载销毁文档管理器。
  }
  clearOriginalFile()
})

watch([drawingId, fileId], () => {
  void loadTargetFile()
})
</script>

<template>
  <div class="drawing-standalone-editor">
    <header class="editor-header" data-tauri-drag-region @mousedown="startDragging">
      <div class="header-left">
        <button class="back-btn" type="button" title="返回图纸详情" @mousedown.stop @click="goBack">
          <DemoIcon name="arrow-left" :size="15" />
          <span>返回图纸</span>
        </button>
        <div class="divider"></div>
        <div class="drawing-meta">
          <span class="tag tag-no-dot" :class="isAssembly ? 'plain' : 'info'">
            {{ isAssembly ? '总图' : '零件图' }}
          </span>
          <span v-if="requiresTicket" class="tag tag-ticket" title="该图纸已存档，正在变更工单的工作版本上编辑，验收后才发布正式版">
            变更工单 · 工作版本
          </span>
          <span class="file-name-highlight">{{ targetFile?.name || currentDrawing?.name }}</span>
          <span class="drawing-no">{{ targetFile?.partNo || targetFile?.drawingNo || currentDrawing?.no }}</span>
        </div>
      </div>

      <div class="header-right">
        <button class="btn sm primary" type="button" :disabled="isSaving || !isReady" :title="requiresTicket ? '保存为变更工单工作版本（不影响正式版）' : '保存在线编辑结果并生成新版本'" @mousedown.stop @click="saveAsNewVersion">
          <DemoIcon name="save" :size="14" />
          <span>{{ isSaving ? '保存中...' : (requiresTicket ? '保存工作版本' : '保存新版本') }}</span>
        </button>
        <button class="btn sm" type="button" title="切换到只读浏览模式" @mousedown.stop @click="openReadonlyView">
          <DemoIcon name="eye" :size="14" />
          <span>只读浏览</span>
        </button>
      </div>
    </header>

    <div class="editor-body">
      <main class="editor-workspace" :inert="isSaving">
        <MlCadViewer
          v-if="isReady && cadOriginalFile"
          locale="zh"
          :local-file="cadOriginalFile"
          :base-url="cadFontsRoot"
          :mode="AcEdOpenMode.Write"
          :use-main-thread-draw="useMainThreadCadDraw"
          theme="dark"
          @create="bindEditorDocumentCleanup"
        />
        <div v-else class="empty-prompt">
          <DemoIcon name="file-question" :size="48" />
          <p>{{ targetFile ? (cadOriginalError || '正在初始化完整版 CAD 编辑器...') : '暂无选中的图纸文件' }}</p>
        </div>
      </main>
    </div>

    <div v-if="savedVersion" class="save-success-backdrop">
      <section class="save-success-dialog" role="dialog" aria-modal="true" aria-labelledby="save-success-title">
        <div class="success-icon">
          <DemoIcon name="check-circle-2" :size="30" />
        </div>
        <h2 id="save-success-title">保存成功</h2>
        <p>当前编辑内容已保存为{{ requiresTicket ? '工单工作版本' : '新版本' }} <strong>{{ savedVersion }}</strong></p>
        <div class="save-success-note">
          <DemoIcon name="shield-check" :size="16" />
          <span>{{ requiresTicket ? '正式版本暂未改动，验收通过后才会发布。' : '原版本已完整保留，可在历史版本中查看和回溯。' }}</span>
        </div>
        <div class="save-success-actions">
          <button class="btn" type="button" @click="continueEditing">继续编辑</button>
          <button class="btn primary" type="button" @click="goBack">返回图纸</button>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.drawing-standalone-editor {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  width: 100vw;
  height: 100vh;
  min-width: 0;
  min-height: 0;
  background: #0f131a;
  color: var(--text-1);
  overflow: hidden;
  position: relative;
}

.editor-header {
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
  flex: 0 0 50px;
  user-select: none;
  -webkit-user-select: none;
  cursor: move;
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
}

.back-btn:hover {
  background: var(--hover);
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

.tag-ticket {
  font-size: 11px;
  color: #b45309;
  background: rgba(245, 158, 11, 0.14);
  border: 1px solid rgba(245, 158, 11, 0.4);
  padding: 2px 8px;
  border-radius: 999px;
  white-space: nowrap;
}

.file-name-highlight {
  font-weight: 600;
  color: var(--text-1);
  font-size: 14px;
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

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.editor-body {
  flex: 1;
  position: relative;
  display: flex;
  overflow: hidden;
  min-height: 0;
  height: calc(100vh - 50px);
}

.editor-workspace {
  flex: 1;
  min-width: 0;
  min-height: 0;
  height: 100%;
  position: relative;
  background: #000;
  overflow: hidden;
}

/* 完整版编辑器内部默认按 viewport 计算高度，深层路由中会越过业务容器。 */
.editor-workspace :deep(.ml-cad-viewer-container),
.editor-workspace :deep(.ml-cad-layout),
.editor-workspace :deep(.ml-cad-container) {
  width: 100% !important;
  min-width: 0 !important;
  min-height: 0 !important;
}

.editor-workspace :deep(.ml-cad-viewer-container) {
  position: absolute !important;
  inset: 0 !important;
  height: 100% !important;
  overflow: hidden !important;
}

.editor-workspace :deep(.ml-cad-layout) {
  height: 100% !important;
  overflow: hidden !important;
}

.editor-workspace :deep(.ml-cad-container) {
  height: 100% !important;
  overflow: hidden !important;
}

.empty-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  gap: 12px;
  color: var(--text-3);
}

.save-success-backdrop {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgb(3 7 18 / 62%);
  backdrop-filter: blur(4px);
}

.save-success-dialog {
  width: min(420px, 100%);
  padding: 30px 30px 24px;
  border: 1px solid var(--line-strong);
  border-radius: 16px;
  background: var(--panel);
  box-shadow: 0 24px 80px rgb(0 0 0 / 42%);
  text-align: center;
}

.success-icon {
  display: grid;
  place-items: center;
  width: 58px;
  height: 58px;
  margin: 0 auto 14px;
  border: 1px solid var(--ok);
  border-radius: 50%;
  background: rgb(52 211 153 / 12%);
  color: var(--ok);
}

.save-success-dialog h2 {
  margin: 0;
  color: var(--text-1);
  font-size: 20px;
}

.save-success-dialog p {
  margin: 10px 0 18px;
  color: var(--text-2);
  font-size: 13px;
}

.save-success-dialog strong {
  color: var(--accent);
  font-family: 'JetBrains Mono', monospace;
}

.save-success-note {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid rgb(52 211 153 / 30%);
  border-radius: 8px;
  background: rgb(52 211 153 / 8%);
  color: var(--ok);
  font-size: 12px;
  text-align: left;
}

.save-success-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 24px;
}
</style>
