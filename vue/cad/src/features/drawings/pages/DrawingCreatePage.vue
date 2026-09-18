<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { windowService } from '@/services/tauri/window.service'

import DemoIcon from '@/components/common/DemoIcon.vue'
import DrawingAttributesForm from '@/components/common/DrawingAttributesForm.vue'
import { useAttributeStore } from '@/stores/attribute.store'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useUiStore } from '@/stores/ui.store'
import { appContainer, drawingFileService } from '@/app/container'
import { readAccessToken } from '@/services/auth/access-token'
import { getApiBaseUrl } from '@/services/api-base.service'
import { lifecycleApi } from '@/services/lifecycle.service'
import { extractCreationTitleBlocks } from '@/services/drawing-title-block.service'
import { DRAWING_2D_ACCEPT, MODEL_FILE_ACCEPT, MODEL_EXTENSIONS, fileFormat, isDrawing2DFile } from '@/utils/model-formats'
import type { Drawing, DrawingFile, StructurePart } from '@/types/domain.types'
import type { DrawingFileIdentity } from '@/modules/drawing'
import type { UploadSessionSnapshot } from '@/types/application.types'
import { directParentDrawingNo, isEquivalentAssemblyNo, isSameDrawingFamily, parseDrawingNumber, parseStandaloneDrawingFileName } from '@/utils/drawing-number-parser'
import {
  CREATE_MODE_OPTIONS,
  NEW_DRAWING_MATERIAL_ACCEPT,
  createModeTitle,
  materialLabel,
  resolveCreateMode,
  type DrawingCreateMode,
} from '@/features/drawings/create/drawing-create-modes'

defineOptions({
  name: 'DrawingCreatePage',
})

const router = useRouter()
const drawingOperationsStore = useDrawingOperationsStore()
const drawingStore = useDrawingStore()
const attributeStore = useAttributeStore()
const uiStore = useUiStore()
const authStore = useAuthStore()
const uploadGateway = appContainer.uploadGateway
// 创建人/上传人取当前登录人，不再写占位符（否则换账号后仍显示旧值）。
const operatorName = authStore.currentUser?.displayName || authStore.currentUser?.account || '未知'

const formProject = ref('')
const formProjectNo = ref('')
const formDrawingNo = ref('')
const formRemark = ref('')
const formNewName = ref('')
const formAttributeValues = ref<Record<string, string>>({})
const isIdentifyingAssembly = ref(false)
const assemblyIdentifyMessage = ref('')
let assemblyIdentifySequence = 0

const route = useRoute()
/** 三种创建方式由入口 query 决定；切换后写回地址栏，刷新仍停留在同一方式。 */
const mode = ref<DrawingCreateMode>(resolveCreateMode(route.query.mode))
const createModeOptions = CREATE_MODE_OPTIONS
const selectedForkSourceNo = ref('')

// 分步向导状态控制：创建新图纸是单屏建档，不需要分步。
const LEGACY_STEPS = [
  { step: 1, title: '2D 工程图纸', sub: '总图识别与零件图批量导入' },
  { step: 2, title: '基础档案', sub: '项目名称、项目号与总图图号' },
  { step: 3, title: '图纸属性', sub: '业务分类与自定义属性' },
  { step: 4, title: '3D 模型与资料', sub: '三维模型装配包与归档凭证' },
]
const FORK_STEPS = [
  { step: 1, title: '源图纸', sub: '选择要继承的老图纸' },
  { step: 2, title: '基础档案', sub: '新项目名称、项目号与新图号' },
  { step: 3, title: '图纸属性', sub: '业务分类与自定义属性' },
  { step: 4, title: '相关资料', sub: '技术协议与设计依据（可选）' },
]
const NEW_STEPS = [
  { step: 1, title: '新图纸信息', sub: '图纸名称、图号、可选总图与图纸材料' }
]
const currentStep = ref(1)
const steps = computed(() => (mode.value === 'new' ? NEW_STEPS : mode.value === 'fork' ? FORK_STEPS : LEGACY_STEPS))
const createTitle = computed(() => (mode.value === 'new' ? '创建新图纸' : mode.value === 'fork' ? '从老图纸分叉' : '新建项目图纸'))
const createSubtitle = computed(() => {
  if (mode.value === 'new') return '只需图纸名称与图号即可建档；建议同时上传总图，其他人才能拆图与认领零件；总图与图纸材料都可以稍后在图纸详情补传。'
  if (mode.value === 'fork') return '选择一张老图纸作为源，完整继承其属性、零件层级与工艺备料文件，生成全新的项目档案。'
  return '分步向导帮助您规范建立图纸档案；支持先立项后补传，或一步到位关联全套工程文件。'
})
const validationErrors = ref<Record<string, string>>({})
const attributeFieldErrors = computed(() => Object.fromEntries(Object.entries(validationErrors.value)
  .filter(([key]) => key.startsWith('drawing-attr-')).map(([key, value]) => [key.slice('drawing-attr-'.length), value])))

function invalidField(step: number, field: string, message: string): false {
  validationErrors.value = { [field]: message }
  currentStep.value = step
  void nextTick(() => {
    const element = document.getElementById(field)
    element?.scrollIntoView({ block: 'center', behavior: 'smooth' })
    element?.focus()
  })
  return false
}
watch([formProject, formProjectNo, formDrawingNo, formNewName, selectedForkSourceNo, formAttributeValues], () => {
  validationErrors.value = {}
}, { deep: true })

/** 地址栏回写当前创建方式，刷新或分享链接后仍停留在同一方式。 */
function syncModeQuery() {
  if (typeof router.replace !== 'function') return
  void router.replace({ query: { ...route.query, mode: mode.value } }).catch(() => undefined)
}

/** 切换创建方式：重置步骤与校验，并清掉不属于该方式的已选文件，避免提交时被静默丢弃。 */
function setMode(next: DrawingCreateMode) {
  if (mode.value === next) return
  mode.value = next
  currentStep.value = 1
  validationErrors.value = {}
  createError.value = ''
  if (next === 'new') {
    const discarded = (assemblyFile.value ? 1 : 0) + partFiles.value.length + modelFiles.value.length
    if (discarded) {
      assemblyFile.value = null
      partFiles.value = []
      modelFiles.value = []
      uiStore.toast('创建新图纸不区分总图与零件图，已选的 2D 图纸与 3D 模型已清空；可在「图纸材料」中重新选择。', 'warn')
    }
    // 新建档案与老图纸导入的项目名称同名，切换时沿用已填写内容，减少重复输入。
    if (!formNewName.value && formProject.value.trim()) formNewName.value = formProject.value.trim()
  } else {
    const clearedAssets = newMaterialFiles.value.length + (newAssemblyFile.value ? 1 : 0)
    if (clearedAssets) {
      newMaterialFiles.value = []
      newAssemblyFile.value = null
      uiStore.toast(`已切换到「${createModeTitle(next)}」，新图纸已选的总图与图纸材料已清空。`, 'warn')
    }
  }
  syncModeQuery()
}

/** 步骤 1：2D 工程图纸（支持上传总图以自动识别项目基本信息）。 */
function validateDrawingStep(): boolean {
  if (isIdentifyingAssembly.value) {
    return invalidField(1, 'create-validation', '正在识别总图文件名，请稍候。')
  }
  return true
}

/** 步骤 1（分叉）：必须选定一张仍然存在的源图纸。 */
function validateForkSource(): boolean {
  if (!selectedForkSourceNo.value) {
    return invalidField(1, 'fork-source-select', '请选择源图纸')
  }
  if (!drawingStore.drawings.some((item) => item.no === selectedForkSourceNo.value)) {
    return invalidField(1, 'fork-source-select', '源图纸已不存在，请重新选择')
  }
  return true
}

/** 步骤 2：项目标识；分叉模式额外要求新图号与源图纸不同。 */
function validateBasicInfo(): boolean {
  const projectName = formProject.value.trim()
  if (!projectName) {
    return invalidField(2, 'create-project-name', '请填写项目名称')
  }
  const projectNo = formProjectNo.value.trim()
  const drawingNo = formDrawingNo.value.trim()
  if (!projectNo) {
    return invalidField(2, 'create-project-no', '请填写项目号')
  }
  if (isIdentifyingAssembly.value) {
    return invalidField(2, 'create-drawing-no', '正在识别总图文件名，请稍候。')
  }
  if (!drawingNo) {
    return invalidField(2, 'create-drawing-no', '请填写总图图号')
  }
  if (mode.value === 'fork') {
    if (!selectedForkSourceNo.value) {
      return invalidField(2, 'fork-source-select', '请选择源图纸')
    }
    if (selectedForkSourceNo.value === drawingNo) {
      return invalidField(2, 'create-drawing-no', '请填写与源图纸不同的新图号')
    }
  }
  return true
}

/** 步骤 3：按后台配置校验必填业务属性。 */
function validateAttributes(): boolean {
  if (attributeStore.loading || attributeStore.error) {
    return invalidField(3, 'create-validation', attributeStore.loading ? '正在加载图纸属性，请稍候。' : '图纸属性加载失败，请重新加载后继续。')
  }
  const attributeErrors = attributeStore.validate(formAttributeValues.value)
  if (attributeErrors.length) {
    const invalid = attributeStore.sortedAttributes.find(attribute => attribute.enabled && (
      (attribute.required && !formAttributeValues.value[attribute.id]) ||
      (formAttributeValues.value[attribute.id] && !attribute.fields.some(field => field.enabled && field.id === formAttributeValues.value[attribute.id]))
    ))
    return invalidField(3, invalid ? `drawing-attr-${invalid.id}` : 'create-validation', attributeErrors[0] || '请完善图纸属性')
  }
  return true
}

/** 创建新图纸：只校验图纸名称与图号，图纸材料可以一份都不传。 */
function validateNewDrawing(): boolean {
  if (!formNewName.value.trim()) {
    return invalidField(1, 'create-new-name', '请填写图纸名称')
  }
  const drawingNo = formDrawingNo.value.trim()
  if (!drawingNo) {
    return invalidField(1, 'create-new-no', '请填写图号')
  }
  if (drawingStore.drawings.some((item) => item.no === drawingNo)) {
    return invalidField(1, 'create-new-no', `图号「${drawingNo}」已存在，请更换图号`)
  }
  return true
}

/** 前进时逐个校验所有前置步骤，保证跳步不会绕过必填项。 */
function validateUpTo(step: number): boolean {
  validationErrors.value = {}
  if (step > 1 && !(mode.value === 'fork' ? validateForkSource() : validateDrawingStep())) return false
  if (step > 2 && !validateBasicInfo()) return false
  if (step > 3 && !validateAttributes()) return false
  return true
}

function nextStep() {
  if (currentStep.value >= steps.value.length) return
  if (!validateUpTo(currentStep.value + 1)) return
  currentStep.value += 1
}

function prevStep() {
  if (currentStep.value > 1) {
    currentStep.value -= 1
  }
}

function goToStep(step: number) {
  if (step === currentStep.value) return
  if (step > currentStep.value) {
    if (!validateUpTo(step)) return
    currentStep.value = step
  } else {
    currentStep.value = step
  }
}

const existingDrawings = computed(() => drawingStore.drawings)

function onForkSourceChange() {
  const source = drawingStore.drawings.find((item) => item.no === selectedForkSourceNo.value)
  if (!source) return
  // 继承源项目的基本信息（均可修改）；总图图号预填源号，提交前必须改成新号。
  formProject.value = `${source.name} (改进版)`
  formProjectNo.value = source.project || ''
  formDrawingNo.value = source.no
  formRemark.value = `分叉自 ${source.no} · 继承图纸、备料与工艺文件及零件结构`
  formAttributeValues.value = { ...(source.attributeValues ?? {}) }
}

interface UploadedAssembly {
  name: string
  size: string
  file?: File
}

interface UploadedPart {
  id: string
  name: string
  size: string
  file?: File
}

interface UploadedModel extends UploadedPart {
  linkTo: string
}

interface IdentifiedPartFile {
  part: UploadedPart
  parsed: ReturnType<typeof parseDrawingNumber>
  material: string
  identified: boolean
}

const assemblyFile = ref<UploadedAssembly | null>(null)
const partFiles = ref<UploadedPart[]>([])
const modelFiles = ref<UploadedModel[]>([])
const modelLinkTargets = computed(() => [
  ...(assemblyFile.value ? [{ value: 'assembly', label: `总图 · ${assemblyFile.value.name}` }] : []),
  ...partFiles.value.map((part) => ({ value: `part:${part.id}`, label: `零件图 · ${part.name}` })),
])
const isDraggingAssembly = ref(false)
const isDraggingParts = ref(false)
const isDraggingModels = ref(false)
const isCreating = ref(false)
const createStatus = ref('正在准备创建')
const createError = ref('')
const savedDrawingNo = ref('')
const canRetryUpload = computed(() => Boolean(drawingOperationsStore.pendingUploadSessionId))
const uploadSnapshot = ref<UploadSessionSnapshot | null>(null)
const readyUploadCount = computed(() => uploadSnapshot.value?.items.filter((item) => item.status === 'ready' || item.status === 'committed').length ?? 0)
const failedUploadCount = computed(() => uploadSnapshot.value?.items.filter((item) => item.status === 'failed').length ?? 0)
const EVIDENCE_CATEGORIES = [
  '客户沟通',
  '确认图',
  '技术要求',
  '备料表',
  '工艺资料',
  '变更依据',
  '验收证明',
  '其他资料',
]

interface UploadedEvidence {
  id: string
  name: string
  size: string
  file: File
  title: string
}

const evidenceCategory = ref(EVIDENCE_CATEGORIES[0] ?? '其他资料')
const evidenceFolderPath = ref('')
const evidenceDescription = ref('')
const evidenceFiles = ref<UploadedEvidence[]>([])
const isDraggingEvidence = ref(false)

interface ConversionProgressItem { id: string; name: string; status: string; error?: string; attempts?: number }
const conversionModalVisible = ref(false)
const conversionBusy = ref(false)
const conversionError = ref('')
let conversionGeneration = 0
let disposed = false
let leavingAfterSave = false
let discardApproved = false
let pendingLeave: Promise<boolean> | null = null
const hasUnsavedChanges = computed(() => savedDrawingNo.value ? evidenceFiles.value.length > 0 : Boolean(
  formProject.value || formProjectNo.value || formDrawingNo.value || formRemark.value || formNewName.value ||
  Object.values(formAttributeValues.value).some(Boolean) || selectedForkSourceNo.value ||
  assemblyFile.value || partFiles.value.length || modelFiles.value.length || evidenceFiles.value.length ||
  newMaterialFiles.value.length || newAssemblyFile.value ||
  evidenceFolderPath.value || evidenceDescription.value || drawingOperationsStore.pendingUploadSessionId,
))

async function confirmLeave(): Promise<boolean> {
  if (leavingAfterSave || discardApproved) return true
  if (isCreating.value && !conversionModalVisible.value) {
    uiStore.toast('正在保存图纸或资料，请等待完成后再离开。', 'warn')
    return false
  }
  if (!hasUnsavedChanges.value) return true
  if (pendingLeave) return pendingLeave
  pendingLeave = (async () => {
    const accepted = await uiStore.askConfirm(
      savedDrawingNo.value ? '还有资料未归档' : '放弃本次创建？',
      savedDrawingNo.value ? '图纸已经创建，未归档的资料需要稍后重新选择并上传。' : '填写的信息和选择的本地文件尚未保存，离开后需要重新填写和选择。',
      savedDrawingNo.value ? '离开，稍后补传' : '放弃创建', '继续编辑',
    )
    if (!accepted) return false
    if (!savedDrawingNo.value) {
      try { await drawingOperationsStore.cancelPendingUploadSession() }
      catch {
        uiStore.toast('暂时无法清理上传进度，请重试。填写内容仍保留在当前页面。', 'warn')
        return false
      }
    }
    discardApproved = true
    return true
  })()
  try { return await pendingLeave } finally { pendingLeave = null }
}
onBeforeRouteLeave(confirmLeave)
const removeLeaveGuard = uiStore.setLeaveGuard(confirmLeave)
function beforeUnload(event: BeforeUnloadEvent) {
  if (discardApproved || leavingAfterSave) return
  if (hasUnsavedChanges.value || (isCreating.value && !conversionModalVisible.value)) {
    event.preventDefault()
    event.returnValue = ''
  }
}
let removeCloseGuard = () => {}
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload)
  void windowService.guardClose(confirmLeave).then(remove => {
    if (disposed) remove()
    else removeCloseGuard = remove
  }).catch(() => { uiStore.toast('窗口关闭保护未能启用，请保存后再关闭。', 'warn') })
})
onBeforeUnmount(() => {
  disposed = true
  conversionGeneration++
  removeLeaveGuard()
  removeCloseGuard()
  window.removeEventListener('beforeunload', beforeUnload)
})
const conversionDrawingNo = ref('')
const conversionItems = ref<ConversionProgressItem[]>([])
const conversionReadyCount = computed(() => conversionItems.value.filter((item) => item.status === 'ready').length)
const conversionFailedCount = computed(() => conversionItems.value.filter((item) => item.status === 'failed').length)
const conversionPendingCount = computed(() => conversionItems.value.filter((item) => !['ready', 'failed'].includes(item.status)).length)

function accessToken(): string {
  return readAccessToken()
}

function flattenConversionFiles(nodes: Array<{ files?: Array<{ id: string; name: string }>; otherFiles?: Array<{ id: string; name: string }>; children?: unknown[] }>): Array<{ id: string; name: string }> {
  return nodes.flatMap((node) => [
    ...(node.files ?? []), ...(node.otherFiles ?? []),
    ...flattenConversionFiles((node.children ?? []) as Array<{ files?: Array<{ id: string; name: string }>; otherFiles?: Array<{ id: string; name: string }>; children?: unknown[] }>),
  ])
}

function isCadFile(name: string): boolean { return /\.(exb|dwg|dxf)$/i.test(name) }

async function readConversionStatus(item: ConversionProgressItem): Promise<ConversionProgressItem> {
  const response = await fetch(`${getApiBaseUrl()}/cad/conversions/${encodeURIComponent(item.id)}`, {
    headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken()}` },
    credentials: 'include',
    signal: AbortSignal.timeout(10000),
  })
  const body = await response.json().catch(() => ({})) as { data?: { status?: string; error?: string; attempts?: number }; message?: string }
  if (!response.ok) throw new Error(body.message || `读取转换状态失败：HTTP ${response.status}`)
  const data = body.data ?? {}
  return { ...item, status: data.status || 'pending', error: data.error, attempts: data.attempts }
}

function wait(milliseconds: number): Promise<void> { return new Promise((resolve) => window.setTimeout(resolve, milliseconds)) }

const titleExtractionFailures = ref<string[]>([])
async function saveCreatedTitleBlocks(files: Array<{ id: string; name: string }>) {
  titleExtractionFailures.value = await extractCreationTitleBlocks(files, (completed, total) => {
    createStatus.value = `正在提取并保存图纸信息（${completed}/${total}）`
  })
  if (titleExtractionFailures.value.length) uiStore.toast(`${titleExtractionFailures.value.length} 个文件的信息尚未保存，可在属性详情中重试：${titleExtractionFailures.value.join('；')}`, 'warn')
}

async function waitForDrawingConversions(drawingNo: string): Promise<boolean> {
  const generation = ++conversionGeneration
  conversionBusy.value = true
  conversionError.value = ''
  conversionDrawingNo.value = drawingNo
  const drawing = drawingStore.getDrawing(drawingNo)
  const structureFiles = flattenConversionFiles(drawingStore.getStructure(drawingNo))
  const files = [...(drawing?.files ?? []), ...(drawing?.otherFiles ?? []), ...structureFiles]
    .filter((file) => isCadFile(file.name))
    .filter((file, index, all) => all.findIndex((candidate) => candidate.id === file.id) === index)
  conversionItems.value = files.map((file) => ({ id: file.id, name: file.name, status: 'pending' }))
  try {
    if (!conversionItems.value.length) return true
    conversionModalVisible.value = true
    while (!disposed && generation === conversionGeneration && conversionModalVisible.value) {
      conversionItems.value = await Promise.all(conversionItems.value.map(readConversionStatus))
      if (disposed || generation !== conversionGeneration) return false
      createStatus.value = `正在生成预览（${conversionReadyCount.value}/${conversionItems.value.length}）`
      if (conversionReadyCount.value === conversionItems.value.length) {
        await saveCreatedTitleBlocks(files)
        return !disposed && generation === conversionGeneration
      }
      if (conversionFailedCount.value > 0) return false
      await wait(2000)
    }
    return false
  } catch (error) {
    if (!disposed && generation === conversionGeneration) conversionError.value = error instanceof Error ? error.message : '读取预览进度失败'
    return false
  } finally {
    if (generation === conversionGeneration) conversionBusy.value = false
  }
}

async function retryFailedConversions() {
  if (conversionBusy.value) return
  conversionBusy.value = true
  conversionError.value = ''
  const generation = conversionGeneration
  try {
    const failed = conversionItems.value.filter((item) => item.status === 'failed')
    await Promise.all(failed.map(async (item) => {
      const response = await fetch(`${getApiBaseUrl()}/cad/conversions/${encodeURIComponent(item.id)}`, {
        method: 'POST', headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken()}` }, credentials: 'include', signal: AbortSignal.timeout(10000),
      })
      if (!response.ok) throw new Error(`「${item.name}」重新转换失败`)
    }))
    if (disposed || generation !== conversionGeneration) return
    conversionItems.value = conversionItems.value.map((item) => item.status === 'failed' ? { ...item, status: 'pending', error: undefined } : item)
    if (await waitForDrawingConversions(savedDrawingNo.value)) await openSavedDrawing()
  } catch (error) {
    if (!disposed && generation === conversionGeneration) conversionError.value = error instanceof Error ? error.message : '重新生成预览失败'
  } finally { conversionBusy.value = false }
}

async function resumeConversionProgress() {
  if (conversionBusy.value) return
  if (await waitForDrawingConversions(savedDrawingNo.value)) await openSavedDrawing()
}

async function openSavedDrawing() {
  if (!savedDrawingNo.value || disposed) return
  leavingAfterSave = evidenceFiles.value.length === 0
  try {
    await router.push({ name: modelFiles.value.length && !assemblyFile.value ? 'drawing-models' : 'drawing-preview', params: { drawingId: savedDrawingNo.value } })
  } finally { leavingAfterSave = false }
}

/** 项目只提交一次；后续刷新、资料归档与预览失败只能重试后续步骤。 */
/** 项目只提交一次；后续刷新、资料归档与预览失败只能重试后续步骤。 */
async function completeSavedDrawing() {
  createError.value = ''
  try {
    await drawingStore.refresh()
    await archiveEvidenceForDrawing(savedDrawingNo.value)
    await archiveNewMaterialsForDrawing(savedDrawingNo.value)
    if (evidenceFiles.value.length || newMaterialFiles.value.length) {
      createError.value = '图纸已创建，部分资料尚未归档。请重试资料归档，或进入详情后补传。'
      return
    }
    if (mode.value === 'fork' || await waitForDrawingConversions(savedDrawingNo.value)) await openSavedDrawing()
  } catch (error) {
    createError.value = `图纸已创建，后续处理未完成：${error instanceof Error ? error.message : '请重试'}`
  }
}

async function refreshUploadSnapshot() {
  const sessionId = drawingOperationsStore.pendingUploadSessionId
  if (!sessionId) {
    uploadSnapshot.value = null
    return
  }
  try {
    const snapshot = await uploadGateway.getSession(sessionId)
    uploadSnapshot.value = snapshot
    await Promise.all(snapshot.items.map(async (item) => {
      if (item.status === 'ready' || item.status === 'committed') {
        drawingOperationsStore.uploadProgress[item.id] = 100
        return
      }
      try {
        const chunks = await uploadGateway.listChunks(sessionId, item.id)
        if (chunks.manifest.chunkSize > 0 && chunks.manifest.totalSize > 0) {
          const count = Math.ceil(chunks.manifest.totalSize / chunks.manifest.chunkSize)
          drawingOperationsStore.uploadProgress[item.id] = Math.round((chunks.parts.length / count) * 100)
        }
      } catch {
        // 普通整文件上传没有分片清单，状态仍由会话快照展示。
      }
    }))
	  } catch (error) {
	    if (isExpiredUploadError(error)) {
	      await drawingOperationsStore.dismissPendingUploadSession()
	      uploadSnapshot.value = null
	      return
	    }
	    console.warn('读取上传会话状态失败', error)
	  }
}

function uploadStatusLabel(item: { status: string; failureStage?: string }): string {
  if (item.status === 'failed' && item.failureStage === 'conversion') return '转换失败'
  return ({ pending: '待上传', uploading: '上传中', ready: '已完成', committed: '已提交', failed: '失败' } as Record<string, string>)[item.status] ?? item.status
}

function uploadRetryLabel(item: { failureStage?: string }): string {
  return item.failureStage === 'conversion' ? '重试转换' : '重试上传'
}

function isExpiredUploadError(error: unknown): boolean {
  return error instanceof Error && (error.message.includes('上传会话已过期') || error.message.includes('HTTP 410'))
}

async function retryUploadItem(itemId: string) {
  if (isCreating.value) return
  isCreating.value = true
  createStatus.value = '正在重试文件'
  try {
    await drawingOperationsStore.retryFailedDrawingUploadItem(itemId)
    await refreshUploadSnapshot()
    uiStore.toast('文件已重新上传', 'ok')
  } catch (error) {
    createError.value = error instanceof Error ? error.message : '重试文件失败'
    uiStore.toast(createError.value, 'warn')
    await refreshUploadSnapshot()
  } finally {
    isCreating.value = false
    createStatus.value = '正在准备创建'
  }
}

onMounted(() => {
  void Promise.all([drawingStore.load(), attributeStore.load(), refreshUploadSnapshot()])
    .then(() => { applyModeFromRoute() })
    .catch(() => {
      uiStore.toast('部分创建数据加载失败，请重试后继续。', 'warn')
    })
})
watch(() => drawingOperationsStore.pendingUploadSessionId, () => { void refreshUploadSnapshot() })

const assemblyFileInput = ref<HTMLInputElement | null>(null)
const partFilesInput = ref<HTMLInputElement | null>(null)
const partFolderInput = ref<HTMLInputElement | null>(null)
const modelFilesInput = ref<HTMLInputElement | null>(null)
const evidenceFilesInput = ref<HTMLInputElement | null>(null)
const newMaterialFilesInput = ref<HTMLInputElement | null>(null)

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function handleAssemblySelected(file: File) {
  if (!isDrawing2DFile(file)) { uiStore.toast('请选择 2D 工程图文件', 'warn'); return }
  assemblyFile.value = {
    name: file.name,
    size: formatFileSize(file.size),
    file,
  }
  const sequence = ++assemblyIdentifySequence
  const fileNameIdentity = parseStandaloneDrawingFileName(file.name)
  formDrawingNo.value = ''
  assemblyIdentifyMessage.value = '正在根据总图文件名识别图号…'
  isIdentifyingAssembly.value = true
  if (!formProject.value) {
    formProject.value = fileNameIdentity.name || file.name.replace(/\.[^/.]+$/, '')
  }
  if (!formProjectNo.value) {
    formProjectNo.value = projectNoFromFile(file)
  }
  if (file && supportsDrawingNumberIdentification(file.name)) {
    try {
      const identity = await drawingFileService.identify(file, file.name)
      if (sequence !== assemblyIdentifySequence) return
      if (identity.partNoSource !== 'filename' || !identity.partNo.trim()) {
        assemblyIdentifyMessage.value = '文件名中未识别到总图图号，请核对后手动填写。'
      } else {
        formDrawingNo.value = identity.partNo.trim()
        assemblyIdentifyMessage.value = '已从总图文件名识别图号。'
      }
    } catch (error) {
      if (sequence !== assemblyIdentifySequence) return
      assemblyIdentifyMessage.value = error instanceof Error ? error.message : '读取总图文件名失败，请手动填写总图图号。'
    } finally {
      if (sequence === assemblyIdentifySequence) isIdentifyingAssembly.value = false
    }
  } else {
    isIdentifyingAssembly.value = false
    assemblyIdentifyMessage.value = '当前文件名无法自动识别图号，请手动填写总图图号。'
  }
  uiStore.toast(`总图 ${file.name} 已选择，现可继续添加零件图`, 'ok')
}

function onAssemblyChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (file) {
    void handleAssemblySelected(file)
  }
}

function onAssemblyDrop(event: DragEvent) {
  isDraggingAssembly.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) {
    void handleAssemblySelected(file)
  }
}

function appendPartFiles(fileList: FileList | null) {
  if (!fileList || !fileList.length) return
  const added: UploadedPart[] = []
  for (let i = 0; i < fileList.length; i++) {
    const file = fileList[i]
    if (!file) continue
    if (!isDrawing2DFile(file)) { uiStore.toast(`不支持的 2D 图纸格式：${file.name}`, 'warn'); continue }
    const exists = partFiles.value.some((p) => p.name === file.name)
    if (!exists) {
      added.push({
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        name: file.name,
        size: formatFileSize(file.size),
        file,
      })
    }
  }
  partFiles.value.push(...added)
  uiStore.toast(`已加入 ${added.length} 个零件图文件`, 'ok')
}

function onPartsChange(event: Event) {
  const target = event.target as HTMLInputElement
  appendPartFiles(target.files)
}

function onFolderChange(event: Event) {
  const target = event.target as HTMLInputElement
  appendPartFiles(target.files)
}

function onPartsDrop(event: DragEvent) {
  isDraggingParts.value = false
  appendPartFiles(event.dataTransfer?.files ?? null)
}

function appendModelFiles(fileList: FileList | null) {
  if (!fileList?.length) return
  const added: UploadedModel[] = []
  for (const file of Array.from(fileList)) {
    if (!MODEL_EXTENSIONS.includes(fileFormat(file.name))) {
      uiStore.toast(`不支持的 3D 模型格式：${file.name}`, 'warn')
      continue
    }
    if (modelFiles.value.some((item) => item.name === file.name)) continue
    added.push({
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      name: file.name,
      size: formatFileSize(file.size),
      file,
      linkTo: '',
    })
  }
  modelFiles.value.push(...added)
  uiStore.toast(`已加入 ${added.length} 个 3D 模型文件`, 'ok')
}

function onModelsChange(event: Event) {
  appendModelFiles((event.target as HTMLInputElement).files)
}

function onModelsDrop(event: DragEvent) {
  isDraggingModels.value = false
  appendModelFiles(event.dataTransfer?.files ?? null)
}

function triggerModelsPick() {
  modelFilesInput.value?.click()
}

function removeModel(id: string) {
  modelFiles.value = modelFiles.value.filter((item) => item.id !== id)
}

function clearAllModels() {
  modelFiles.value = []
}

function appendEvidenceFiles(fileList: FileList | null) {
  if (!fileList?.length) return
  const added: UploadedEvidence[] = []
  for (const file of Array.from(fileList)) {
    if (evidenceFiles.value.some((item) => item.name === file.name && item.file.size === file.size)) continue
    added.push({
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      name: file.name,
      size: formatFileSize(file.size),
      file,
      title: file.name.replace(/\.[^/.]+$/, ''),
    })
  }
  evidenceFiles.value.push(...added)
  uiStore.toast(`已加入 ${added.length} 份相关资料`, 'ok')
}

function onEvidenceChange(event: Event) {
  const target = event.target as HTMLInputElement
  appendEvidenceFiles(target.files)
  target.value = ''
}

function onEvidenceDrop(event: DragEvent) {
  isDraggingEvidence.value = false
  appendEvidenceFiles(event.dataTransfer?.files ?? null)
}

function triggerEvidencePick() {
  evidenceFilesInput.value?.click()
}

function removeEvidence(id: string) {
  evidenceFiles.value = evidenceFiles.value.filter((item) => item.id !== id)
}

function clearAllEvidence() {
  evidenceFiles.value = []
}

/** 创建新图纸时选择的图纸材料；label 由扩展名自动判定，供列表展示。 */
interface UploadedMaterial {
  id: string
  name: string
  size: string
  file: File
  label: string
}

// ---------- 创建新图纸：图纸材料（业务上可选，允许一份都不传） ----------

const newMaterialFiles = ref<UploadedMaterial[]>([])
const isDraggingNewMaterials = ref(false)
/** 资料档案（lifecycle-documents）后端的单文件上限，与资料档案页的文案保持一致。 */
const MATERIAL_MAX_BYTES = 100 * 1024 * 1024

const newMaterialSummary = computed(() => {
  if (!newMaterialFiles.value.length) return '未选择图纸材料（可跳过）'
  const counts = new Map<string, number>()
  for (const item of newMaterialFiles.value) counts.set(item.label, (counts.get(item.label) ?? 0) + 1)
  return [...counts.entries()].map(([label, count]) => `${count} 份${label}`).join(' · ')
})

/** 入口可指定源图纸（例如从图纸详情发起分叉），数据加载完成后立即预填。 */
function applyModeFromRoute() {
  if (mode.value !== 'fork') return
  const querySource = Array.isArray(route.query.source) ? route.query.source[0] : route.query.source
  if (typeof querySource !== 'string' || !querySource) return
  if (!drawingStore.drawings.some((item) => item.no === querySource)) {
    uiStore.toast(`源图纸「${querySource}」不存在或已删除，请重新选择。`, 'warn')
    return
  }
  selectedForkSourceNo.value = querySource
  onForkSourceChange()
}

function appendNewMaterialFiles(fileList: FileList | null) {
  if (!fileList?.length) return
  const added: UploadedMaterial[] = []
  let skipped = 0
  let assemblyDuplicated = 0
  let oversized = 0
  for (const file of Array.from(fileList)) {
    if (!file.size) {
      skipped += 1
      continue
    }
    // 资料档案走 lifecycle-documents，后端按 100MB 截断；先在此拦住，避免图纸建好后才报归档失败。
    if (file.size > MATERIAL_MAX_BYTES) {
      oversized += 1
      continue
    }
    if (newAssemblyFile.value && newAssemblyFile.value.name === file.name && newAssemblyFile.value.file?.size === file.size) {
      assemblyDuplicated += 1
      continue
    }
    if (newMaterialFiles.value.some((item) => item.name === file.name && item.file.size === file.size)) {
      skipped += 1
      continue
    }
    added.push({
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      name: file.name,
      size: formatFileSize(file.size),
      file,
      label: materialLabel(file.name),
    })
  }
  newMaterialFiles.value.push(...added)
  if (added.length) uiStore.toast(`已加入 ${added.length} 份图纸材料`, 'ok')
  if (skipped) uiStore.toast(`${skipped} 份材料已跳过（空文件或重复选择）`, 'warn')
  if (assemblyDuplicated) uiStore.toast(`${assemblyDuplicated} 份材料已作为总图，不重复归档`, 'warn')
  if (oversized) uiStore.toast(`${oversized} 份材料超过 100 MB，资料档案单文件上限 100 MB，未加入`, 'warn')
}

function onNewMaterialsChange(event: Event) {
  const target = event.target as HTMLInputElement
  appendNewMaterialFiles(target.files)
  target.value = ''
}

function onNewMaterialsDrop(event: DragEvent) {
  isDraggingNewMaterials.value = false
  appendNewMaterialFiles(event.dataTransfer?.files ?? null)
}

function triggerNewMaterialsPick() {
  newMaterialFilesInput.value?.click()
}

function removeNewMaterial(id: string) {
  newMaterialFiles.value = newMaterialFiles.value.filter((item) => item.id !== id)
}

function clearAllNewMaterials() {
  newMaterialFiles.value = []
}

// ---------- 创建新图纸：总图（拆图与派活的依据，允许先建档后补） ----------

const newAssemblyFile = ref<UploadedAssembly | null>(null)
const isDraggingNewAssembly = ref(false)
const newAssemblyInput = ref<HTMLInputElement | null>(null)

/** 总图必须是 2D 工程图：其他人要靠它拆出零件并在图纸预览里认领。 */
function handleNewAssemblySelected(file: File) {
  if (!isDrawing2DFile(file)) {
    uiStore.toast('总图需要是 2D 工程图（EXB / DWG / DXF / PDF）', 'warn')
    return
  }
  newAssemblyFile.value = { name: file.name, size: formatFileSize(file.size), file }
  // 同一份文件既当总图又放在图纸材料里，会在创建时重复归档；先选总图时把材料里的那份去掉。
  const duplicated = newMaterialFiles.value.filter((item) => item.name === file.name && item.file.size === file.size)
  if (duplicated.length) {
    newMaterialFiles.value = newMaterialFiles.value.filter((item) => !duplicated.includes(item))
    uiStore.toast(`图纸材料中的「${file.name}」已作为总图，材料列表已去重`, 'warn')
  }
  uiStore.toast(`总图 ${file.name} 已选择，创建后其他人即可在图纸预览中拆图与认领零件`, 'ok')
}

function onNewAssemblyChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (file) handleNewAssemblySelected(file)
  target.value = ''
}

function onNewAssemblyDrop(event: DragEvent) {
  isDraggingNewAssembly.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) handleNewAssemblySelected(file)
}

function triggerNewAssemblyPick() {
  newAssemblyInput.value?.click()
}

function removeNewAssembly() {
  newAssemblyFile.value = null
}

/**
 * 资料档案以图号外键归属，创建前没有可关联的图纸记录；
 * 因此在项目创建成功后再逐份归档，单份失败不影响已建成的项目。
 */
async function archiveEvidenceForDrawing(drawingNo: string): Promise<void> {
  if (!evidenceFiles.value.length) return
  const count = evidenceFiles.value.length
  const drawingId = drawingStore.getDrawing(drawingNo)?.id || ''
  const failures = drawingId
    ? await archiveCreatedEvidence(drawingId)
    : ['未取得新图号的服务端身份，相关资料未归档，请在资料档案中重新上传']
  if (failures.length) {
    uiStore.toast(`${failures.length} 份相关资料未归档：${failures.join('；')}`, 'warn')
  } else {
    uiStore.toast(`${count} 份相关资料已归档到「${drawingNo}」的资料档案`, 'ok')
  }
}

async function archiveCreatedEvidence(drawingId: string): Promise<string[]> {
  const failures: string[] = []
  const archived = new Set<string>()
  for (const [index, item] of evidenceFiles.value.entries()) {
    createStatus.value = `正在归档相关资料（${index + 1}/${evidenceFiles.value.length}）`
    try {
      const form = new FormData()
      form.set('file', item.file)
      form.set('title', item.title.trim() || item.name)
      form.set('category', evidenceCategory.value)
      form.set('description', evidenceDescription.value.trim())
      form.set('folderPath', evidenceFolderPath.value.trim())
      form.set('drawingId', drawingId)
      form.set('source', '图纸创建')
      await lifecycleApi('/lifecycle-documents', { method: 'POST', body: form })
      archived.add(item.id)
    } catch (error) {
      failures.push(`${item.name}：${error instanceof Error ? error.message : '归档失败'}`)
    }
  }
  evidenceFiles.value = evidenceFiles.value.filter(item => !archived.has(item.id))
  return failures
}

// ---------- 图纸材料：创建成功后统一进该图纸的资料档案 ----------

/** 资料档案库的分类取值固定，材料一律按「其他资料」归类，靠来源与目录区分。 */
const MATERIAL_ARCHIVE_CATEGORY = '其他资料'
const MATERIAL_ARCHIVE_FOLDER = '图纸材料'

async function archiveNewMaterialsForDrawing(drawingNo: string): Promise<void> {
  if (!newMaterialFiles.value.length) return
  const count = newMaterialFiles.value.length
  const drawingId = drawingStore.getDrawing(drawingNo)?.id || ''
  const failures = drawingId
    ? await archiveNewMaterials(drawingId)
    : ['未取得新图号的服务端身份，图纸材料未归档，请在资料档案中重新上传']
  if (failures.length) {
    uiStore.toast(`${failures.length} 份图纸材料未归档：${failures.join('；')}`, 'warn')
  } else {
    uiStore.toast(`${count} 份图纸材料已归档到「${drawingNo}」的资料档案`, 'ok')
  }
}

/** 逐份按原件归档：单份失败只留提示，不影响图纸档案与已归档的材料。 */
async function archiveNewMaterials(drawingId: string): Promise<string[]> {
  const failures: string[] = []
  const archived = new Set<string>()
  for (const [index, item] of newMaterialFiles.value.entries()) {
    createStatus.value = `正在把图纸材料归入资料档案（${index + 1}/${newMaterialFiles.value.length}）`
    try {
      const form = new FormData()
      form.set('file', item.file)
      form.set('title', item.name)
      form.set('category', MATERIAL_ARCHIVE_CATEGORY)
      form.set('description', `图纸创建时上传的图纸材料（${item.label}）`)
      form.set('folderPath', MATERIAL_ARCHIVE_FOLDER)
      form.set('drawingId', drawingId)
      form.set('source', '图纸材料')
      await lifecycleApi('/lifecycle-documents', { method: 'POST', body: form })
      archived.add(item.id)
    } catch (error) {
      failures.push(`${item.name}：${error instanceof Error ? error.message : '归档失败'}`)
    }
  }
  newMaterialFiles.value = newMaterialFiles.value.filter((item) => !archived.has(item.id))
  return failures
}

function removeAssembly() {
  assemblyFile.value = null
  partFiles.value = []
  for (const model of modelFiles.value) model.linkTo = ''
  uiStore.toast('已移除 2D 总图，零件图和 3D 关联已重置', 'warn')
}

function removePart(id: string) {
  partFiles.value = partFiles.value.filter((p) => p.id !== id)
  for (const model of modelFiles.value) if (model.linkTo === `part:${id}`) model.linkTo = ''
}

function clearAllParts() {
  const links = new Set(partFiles.value.map((part) => `part:${part.id}`))
  partFiles.value = []
  for (const model of modelFiles.value) if (links.has(model.linkTo)) model.linkTo = ''
}

function supportsDrawingNumberIdentification(name: string): boolean {
  const extension = name.slice(name.lastIndexOf('.')).toLowerCase()
  return ['.exb', '.dwg', '.dxf'].includes(extension)
}

function projectNoFromFile(file: File): string {
  const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || ''
  const pathParts = relativePath.split(/[\\/]/).filter(Boolean)
  const candidates = [...pathParts, file.name]

  // 优先读取路径或文件名中的明确项目号，例如 PRJ-2026-782。
  for (const candidate of candidates) {
    const baseName = candidate.replace(/\.[^/.]+$/, '').trim()
    const projectMatch = baseName.match(/^(PRJ[-_]\d+(?:[-_][A-Za-z0-9]+)*)/i)
    if (projectMatch?.[1]) return projectMatch[1]
  }

  // 没有 PRJ 前缀时，项目号仍取文件名中的工程编号；它可以与总图图号字符串相同，语义由字段区分。
  const fileBaseName = file.name.replace(/\.[^/.]+$/, '').trim()
  const fileIdentity = parseStandaloneDrawingFileName(file.name)
  if (fileIdentity.isStandard) return fileIdentity.no
  return fileBaseName.split(/[（(【\[]/, 1)[0]?.trim() || ''
}

function isDetailListFile(name: string): boolean {
  return /密封件|外购件/.test(name) && name.includes('明细表')
}

function fallbackPartNoFromFileName(name: string): string {
  return name.replace(/\.[^./\\]+$/, '').trim() || name.trim()
}

function fallbackPartIdentity(name: string): ReturnType<typeof parseDrawingNumber> {
  const partNo = fallbackPartNoFromFileName(name)
  return {
    no: partNo,
    name: partNo,
    rootNo: partNo,
    parentNo: null,
    level: 0,
    isStandard: Boolean(partNo),
  }
}

function triggerAssemblyPick() {
  assemblyFileInput.value?.click()
}

function triggerPartsPick() {
  partFilesInput.value?.click()
}

function triggerFolderPick() {
  partFolderInput.value?.click()
}

function handleCancel() {
  void router.push({ name: 'drawing-library' })
}

function handleSubmit() {
  if (isCreating.value) return
  if (!savedDrawingNo.value && !(mode.value === 'new' ? validateNewDrawing() : validateUpTo(4))) return
  isCreating.value = true
  createError.value = ''
  createStatus.value = '正在准备创建'
  void (savedDrawingNo.value ? completeSavedDrawing() : performCreate()).catch(error => {
    createError.value = error instanceof Error ? error.message : '操作未完成，请重试'
    uiStore.toast(createError.value, 'warn')
  }).finally(() => {
    isCreating.value = false
    createStatus.value = '正在准备创建'
  })
}

async function performCreate() {
  if (mode.value === 'new') {
    await performCreateNew()
    return
  }

  const projectName = formProject.value.trim()
  const projectNo = formProjectNo.value.trim()
  const drawingNo = formDrawingNo.value.trim()

  if (mode.value === 'fork') {
    try {
      createStatus.value = '正在继承源图纸结构与文件'
      await drawingOperationsStore.forkDrawing(
        selectedForkSourceNo.value,
        drawingNo,
        projectName,
        undefined,
       formRemark.value.trim(),
         undefined,
         projectNo,
       formAttributeValues.value,
       )
      savedDrawingNo.value = drawingNo
      uiStore.toast(`已基于「${selectedForkSourceNo.value}」成功分叉项目「${projectNo}」，总图图号为「${drawingNo}」`, 'ok')
      await completeSavedDrawing()
      return
    } catch (error) {
      console.error('分叉图纸失败', error)
      uiStore.toast(error instanceof Error ? error.message : '分叉图纸失败，请重试', 'warn')
      return
    }
  }

  const newProjectDrawing: Drawing = {
    no: drawingNo,
    name: projectName,
    kind: '总图',
    project: projectNo,
    material: '—',
    vendor: '内部项目部',
    status: 'draft',
    ver: 'v1.0',
    updated: '刚刚',
    by: operatorName,
    createdBy: operatorName,
    borrow: 0,
    hasFile: Boolean(assemblyFile.value),
     ...(Object.keys(formAttributeValues.value).length ? { attributeValues: { ...formAttributeValues.value } } : {}),
    signers: {},
  }

  const identifiedPartFiles: IdentifiedPartFile[] = []
  const unidentifiedPartNames: string[] = []
  for (const [index, part] of partFiles.value.entries()) {
    if (!part.file) throw new Error(`零件文件「${part.name}」缺少文件内容`)

    createStatus.value = `正在识别零件图号（${index + 1}/${partFiles.value.length}）`
    let identity: DrawingFileIdentity | null = null
    if (supportsDrawingNumberIdentification(part.name) && !isDetailListFile(part.name)) {
      try {
        identity = await drawingFileService.identify(part.file, part.name)
      } catch (error) {
        console.warn(`读取零件图号失败，改用文件名：${part.name}`, error)
      }
    }

    const parsedIdentity = identity ? parseDrawingNumber(identity.partNo) : parseDrawingNumber('')
    const identified = parsedIdentity.isStandard
    if (!identified) unidentifiedPartNames.push(part.name)
    identifiedPartFiles.push({
      part,
      parsed: identified ? parsedIdentity : fallbackPartIdentity(part.name),
      material: identity?.material || identity?.titleBlock?.['材料名称'] || identity?.titleBlock?.['材料'] || identity?.titleBlock?.['材质'] || '—',
      identified,
    })
  }

  const projectFamilyNo = drawingNo
  const parsedPartFiles = identifiedPartFiles.map(({ part, parsed, material, identified }) => ({
    part,
    parsed,
    material,
    isBorrowed: identified && !isSameDrawingFamily(parsed.no, projectFamilyNo),
  }))
  const assemblyNos = [
    drawingNo,
  ].filter(Boolean)
  const structuredPartFiles = parsedPartFiles.filter(({ part, parsed }) => !isDetailListFile(part.name) && !isEquivalentAssemblyNo(parsed.no, projectFamilyNo, assemblyNos))
  const validPartEntries = structuredPartFiles.map(({ part, parsed, isBorrowed, material }, index) => ({
    part,
    parsed,
    isBorrowed,
    material,
    file: {
      id: `${Date.now()}-part-${index}`,
      name: part.name,
      size: part.size,
      role: 'part' as const,
       drawingNo,
      partNo: parsed.no,
      version: 'v1.0',
      uploadedBy: operatorName,
      uploadedAt: '刚刚',
        fileCategory: 'drawing2d' as const,
        previewable: true,
      } satisfies DrawingFile,
  }))

  const groupedPartEntries = [...validPartEntries.reduce((groups, entry) => {
    const group = groups.get(entry.parsed.no) ?? []
    group.push(entry)
    groups.set(entry.parsed.no, group)
    return groups
  }, new Map<string, typeof validPartEntries>()).values()]
  const duplicatePartFileCount = validPartEntries.length - groupedPartEntries.length

  const partsForStructure: StructurePart[] = groupedPartEntries.map((entries) => {
    const firstEntry = entries[0]
    if (!firstEntry) throw new Error('零件文件分组为空')
    const cleanName = firstEntry.part.name.replace(/\.[^/.]+$/, '')
    const partNo = firstEntry.parsed.no
    const localPartNos = new Set(structuredPartFiles.map((entry) => entry.parsed.no))
    const directParentNo = directParentDrawingNo(partNo)
     const parentNo = directParentNo && localPartNos.has(directParentNo) ? directParentNo : drawingNo
    return {
      no: partNo,
      name: firstEntry.parsed.name || cleanName,
       parentNo: firstEntry.isBorrowed ? drawingNo : parentNo,
       project: projectNo,
      material: firstEntry.material || '—',
      spec: '',
      weight: 0,
      surfaceTreatment: '',
      partType: '自制件',
      qty: 1,
      status: 'draft',
      ver: 'v1.0',
      hasFile: true,
      signers: {},
      files: entries.map((entry) => entry.file),
      ...(firstEntry.isBorrowed
        ? { borrowFrom: firstEntry.parsed.rootNo ?? firstEntry.parsed.no }
        : {}),
    }
  })
  const otherFileEntries = parsedPartFiles
    .filter(({ part, parsed }) => isDetailListFile(part.name) || isEquivalentAssemblyNo(parsed.no, projectFamilyNo, assemblyNos))
  const otherDrawingFiles: DrawingFile[] = otherFileEntries
    .map(({ part }, index) => ({
      id: `${Date.now()}-other-${index}`,
      name: part.name,
      size: part.size,
      role: 'other' as const,
       drawingNo,
      version: 'v1.0',
       uploadedBy: operatorName,
      uploadedAt: '刚刚',
      fileCategory: 'drawing2d' as const,
      previewable: true,
    }))

  const assemblyDrawingFile: DrawingFile | undefined = assemblyFile.value
    ? {
        id: `${Date.now()}-assembly`,
        name: assemblyFile.value.name,
        size: assemblyFile.value.size,
        role: 'assembly' as const,
         drawingNo,
        version: 'v1.0',
         uploadedBy: operatorName,
        uploadedAt: '刚刚',
        fileCategory: 'drawing2d' as const,
        previewable: true,
      }
    : undefined

  const partNoByUploadId = new Map(validPartEntries.map((entry) => [entry.part.id, entry.parsed.no]))
  const modelDrawingFiles: DrawingFile[] = modelFiles.value.map((model, index) => {
    const requestedPartNo = model.linkTo.startsWith('part:') ? partNoByUploadId.get(model.linkTo.slice(5)) : undefined
    const linkedPartNo = requestedPartNo && partsForStructure.some((part) => part.no === requestedPartNo) ? requestedPartNo : undefined
    return {
      id: `${Date.now()}-model-${index}`,
      name: model.name,
      size: model.size,
      role: linkedPartNo ? 'part' : model.linkTo === 'assembly' ? 'assembly' : 'other',
      drawingNo,
      ...(linkedPartNo ? { partNo: linkedPartNo } : {}),
      version: 'v1.0',
      uploadedBy: operatorName,
      uploadedAt: '刚刚',
      fileCategory: 'model3d',
      previewable: false,
    }
  })
  for (const model of modelDrawingFiles.filter((file) => file.partNo)) {
    const owner = partsForStructure.find((part) => part.no === model.partNo)
    if (owner) owner.files = [...(owner.files ?? []), model]
  }

  newProjectDrawing.files = [
    ...(assemblyDrawingFile ? [assemblyDrawingFile] : []),
    ...modelDrawingFiles.filter((file) => file.role === 'assembly'),
  ]
  newProjectDrawing.otherFiles = [
    ...otherDrawingFiles,
    ...modelDrawingFiles.filter((file) => file.role === 'other'),
  ]

  const attachments = [
    ...(assemblyFile.value
      ? [{ id: assemblyDrawingFile?.id ?? '', content: assemblyFile.value.file }]
      : []),
    ...validPartEntries.map((entry) => ({ id: entry.file.id, content: entry.part.file })),
    ...otherFileEntries
       .map(({ part }, index) => ({ id: otherDrawingFiles[index]?.id ?? '', content: part.file })),
    ...modelFiles.value.map((model, index) => ({ id: modelDrawingFiles[index]?.id ?? '', content: model.file })),
  ]

  newProjectDrawing.remark = formRemark.value.trim()

  try {
    createStatus.value = '正在保存项目结构并上传图纸文件'
    await drawingOperationsStore.addDrawing(newProjectDrawing, partsForStructure, attachments.filter((item): item is { id: string; content: File } => Boolean(item.id && item.content)))
    savedDrawingNo.value = drawingNo
  } catch (error) {
    console.error('保存新建图纸失败', error)
    createError.value = error instanceof Error ? error.message : '项目创建失败，数据未能保存'
    // 会话 ID 在文件项创建前就会写入；失败时主动刷新，避免界面继续显示
    // watcher 首次读到的“0/0”旧快照。
    await refreshUploadSnapshot()
    uiStore.toast(createError.value, 'warn')
    return
  }

  const borrowedPartCount = groupedPartEntries.filter((entries) => entries[0]?.isBorrowed).length
   const fallbackMessage = unidentifiedPartNames.length
     ? `；${unidentifiedPartNames.join('、')} 未识别出图号，已暂用文件名，可在零件详情页修改`
     : ''
   uiStore.toast(`项目「${projectNo}」已成功创建，总图图号为「${drawingNo}」${borrowedPartCount ? `，${borrowedPartCount} 个借用组件已关联` : ''}${duplicatePartFileCount ? `，${duplicatePartFileCount} 个同图号文件已合并到对应零件` : ''}${otherDrawingFiles.length ? `，${otherDrawingFiles.length} 个文件归入其他文件` : ''}${fallbackMessage}`, unidentifiedPartNames.length ? 'warn' : 'ok')

  await completeSavedDrawing()
}

/**
 * 创建新图纸：只要求图纸名称与图号；项目号留空时与图号相同（后端要求项目字段非空）。
 * 总图作为图纸文件随档案一起归档，图纸材料则在创建成功后统一进该图纸的资料档案。
 */
async function performCreateNew() {
  const name = formNewName.value.trim()
  const drawingNo = formDrawingNo.value.trim()
  const projectNo = formProjectNo.value.trim() || drawingNo
  const remark = formRemark.value.trim()
  const files: DrawingFile[] = []
  const attachments: Array<{ id: string; content: File }> = []

  // 单独上传的总图排在图纸文件首位：拆图与派活都依赖它，必须与图纸档案同时归档。
  const assembly = newAssemblyFile.value
  if (assembly?.file) {
    const assemblyEntry: DrawingFile = {
      id: `${Date.now()}-new-assembly`,
      name: assembly.name,
      size: assembly.size,
      role: 'assembly',
      drawingNo,
      version: 'v1.0',
      uploadedBy: operatorName,
      uploadedAt: '刚刚',
      fileCategory: 'drawing2d',
      previewable: true,
    }
    files.push(assemblyEntry)
    attachments.push({ id: assemblyEntry.id, content: assembly.file })
  }

  const drawing: Drawing = {
    no: drawingNo,
    name,
    kind: '总图',
    project: projectNo,
    material: '—',
    vendor: '内部项目部',
    status: 'draft',
    ver: 'v1.0',
    updated: '刚刚',
    by: operatorName,
    createdBy: operatorName,
    borrow: 0,
    hasFile: files.length > 0,
    signers: {},
    files,
    otherFiles: [],
    ...(remark ? { remark } : {}),
  }

  try {
    createStatus.value = attachments.length ? '正在保存图纸档案并上传总图' : '正在保存图纸档案'
    await drawingOperationsStore.addDrawing(drawing, [], attachments)
    savedDrawingNo.value = drawingNo
  } catch (error) {
    console.error('创建新图纸失败', error)
    createError.value = error instanceof Error ? error.message : '图纸创建失败，数据未能保存'
    // 会话 ID 在文件项创建前就会写入；失败时主动刷新，避免界面继续显示旧快照。
    await refreshUploadSnapshot()
    uiStore.toast(createError.value, 'warn')
    return
  }

  uiStore.toast(
    `图纸「${drawingNo}」已创建${newAssemblyFile.value ? '，总图已归档' : ''}${newMaterialFiles.value.length ? `，${newMaterialFiles.value.length} 份图纸材料将归入资料档案` : newAssemblyFile.value ? '' : '（未上传文件，可稍后补传）'}`,
    'ok',
  )
  await completeSavedDrawing()
}

async function retryFailedUpload() {
  if (isCreating.value || !canRetryUpload.value) return
  isCreating.value = true
  createError.value = ''
  createStatus.value = '正在重试失败文件'
  try {
    const drawingNo = await drawingOperationsStore.retryFailedDrawingUpload()
    savedDrawingNo.value = drawingNo
    uiStore.toast(`上传已恢复，项目「${drawingNo}」创建成功`, 'ok')
    await completeSavedDrawing()
  } catch (error) {
    createError.value = error instanceof Error ? error.message : '重试失败文件时发生错误'
    uiStore.toast(createError.value, 'warn')
    await refreshUploadSnapshot()
  } finally {
    isCreating.value = false
    createStatus.value = '正在准备创建'
  }
}
</script>

<template>
  <div class="page drawing-create-view">
    <!-- 创建中遮罩 -->
    <div v-if="isCreating && !conversionModalVisible" class="create-loading-overlay" role="status" aria-live="polite">
      <div class="create-loading-card">
        <span class="create-spinner" aria-hidden="true"></span>
        <strong>正在创建图纸</strong>
        <span>{{ createStatus }}</span>
        <small>请勿关闭页面或重复点击</small>
      </div>
    </div>

    <!-- 转换进度模态框 -->
    <div v-if="conversionModalVisible" class="create-loading-overlay conversion-progress-overlay" role="dialog" aria-modal="true">
      <div class="create-loading-card conversion-progress-card">
        <strong>图纸已创建，正在生成预览</strong>
        <span>{{ conversionReadyCount }}/{{ conversionItems.length }} 个文件已完成</span>
        <span v-if="conversionPendingCount">还有 {{ conversionPendingCount }} 个文件等待生成预览，可先进入图纸详情。</span>
        <span>已保存的图纸不会因预览失败而丢失，无需重复创建。</span>
        <span v-if="conversionFailedCount" class="conversion-failed-text">{{ conversionFailedCount }} 个文件转换失败</span>
        <p v-if="conversionError" class="conversion-failed-text" role="alert">预览进度读取或重试失败：{{ conversionError }}</p>
        <div class="conversion-progress-list">
          <div v-for="item in conversionItems" :key="item.id" class="conversion-progress-row">
            <span>{{ item.name }}</span>
            <span>{{ item.status === 'ready' ? '已完成' : item.status === 'failed' ? '失败' : ['retry', 'backoff'].includes(item.status) ? '等待重试' : item.status === 'pending' ? '排队中' : '转换中' }}</span>
            <small v-if="item.error" class="conversion-error">{{ item.error }}</small>
          </div>
        </div>
        <button v-if="conversionFailedCount" class="btn" type="button" :disabled="conversionBusy" @click="retryFailedConversions">{{ conversionBusy ? '正在处理…' : '重试失败文件' }}</button>
        <button v-if="conversionError" class="btn" type="button" :disabled="conversionBusy" @click="resumeConversionProgress">重新读取进度</button>
        <button class="btn primary" type="button" @click="openSavedDrawing">进入图纸详情</button>
      </div>
    </div>

    <!-- 会话中断恢复卡片 -->
    <div v-if="canRetryUpload && !savedDrawingNo" class="create-upload-recovery" :role="createError ? 'alert' : undefined">
      <div>
        <strong>{{ failedUploadCount ? '部分文件尚未上传完成' : createError ? '文件已上传，项目尚未提交' : '上传会话进度' }}</strong>
        <span>项目尚未写入数据库，已完成文件会保留在暂存会话中。</span>
        <span v-if="uploadSnapshot">已完成 {{ readyUploadCount }}/{{ uploadSnapshot.items.length }}，失败 {{ failedUploadCount }} 个；会话截止 {{ new Date(uploadSnapshot.session.expiresAt).toLocaleString() }}</span>
        <small v-if="createError">{{ createError }}</small>
      </div>
      <div class="create-upload-recovery-actions">
        <button v-if="uploadSnapshot?.items.length" class="btn sm" type="button" :disabled="isCreating" @click="retryFailedUpload">{{ failedUploadCount ? '仅重试失败文件' : '继续提交' }}</button>
        <button class="btn sm" type="button" :disabled="isCreating" @click="handleCancel">放弃并清理</button>
      </div>
      <div v-if="uploadSnapshot" class="create-upload-recovery-list">
        <div v-for="item in uploadSnapshot.items" :key="item.id" class="create-upload-recovery-item">
          <span class="mono">{{ item.originalName }}</span>
          <span>{{ uploadStatusLabel(item) }}</span>
          <span class="upload-progress-value">{{ drawingOperationsStore.uploadProgress[item.id] ?? (item.status === 'ready' || item.status === 'committed' ? 100 : 0) }}%</span>
          <span class="upload-progress-track" aria-hidden="true"><span class="upload-progress-fill" :style="{ width: `${drawingOperationsStore.uploadProgress[item.id] ?? (item.status === 'ready' || item.status === 'committed' ? 100 : 0)}%` }"></span></span>
          <small v-if="item.errorMessage">{{ item.errorMessage }}</small>
          <button v-if="item.status === 'failed'" class="btn sm" type="button" :disabled="isCreating" @click="retryUploadItem(item.id)">{{ uploadRetryLabel(item) }}</button>
        </div>
      </div>
    </div>

    <section v-if="savedDrawingNo" class="card saved-drawing-state" role="status">
      <h2>图纸「{{ savedDrawingNo }}」已创建</h2>
      <p v-if="createError" role="alert">{{ createError }}</p>
      <p v-else>{{ isCreating ? createStatus : '图纸已保存，可以进入详情继续处理。' }}</p>
      <button v-if="createError" class="btn" type="button" :disabled="isCreating" @click="handleSubmit">重试后续处理</button>
      <button class="btn primary" type="button" :disabled="isCreating && !conversionModalVisible" @click="openSavedDrawing">进入图纸详情</button>
    </section>

    <template v-if="!savedDrawingNo">
    <div v-if="Object.keys(validationErrors).length" id="create-validation" class="create-validation" tabindex="-1" role="alert">
      {{ Object.values(validationErrors)[0] }}
      <button v-if="attributeStore.error && currentStep === 3" class="btn sm" type="button" @click="attributeStore.load().catch(() => undefined)">重新加载属性</button>
    </div>
    <div v-if="createError && !canRetryUpload" class="create-validation" role="alert">{{ createError }}</div>
    <!-- 顶部状态栏 -->
    <div class="create-topbar">
      <div class="topbar-left">
        <button class="btn icon-only" type="button" title="返回图纸库" @click="handleCancel">
          <DemoIcon name="arrow-left" :size="16" />
        </button>
        <div>
          <h1 class="create-title">{{ createTitle }}</h1>
          <p class="create-subtitle">{{ createSubtitle }}</p>
        </div>
      </div>
      <div class="segmented-control mode-switch" role="radiogroup" aria-label="创建方式">
        <label
          v-for="option in createModeOptions"
          :key="option.value"
          class="segment"
          :class="{ active: mode === option.value }"
          :title="option.sub"
        >
          <input
            type="radio"
            name="drawing-create-mode"
            :value="option.value"
            :checked="mode === option.value"
            @change="setMode(option.value)"
          />
          <DemoIcon :name="option.icon" :size="13" />{{ option.title }}
        </label>
      </div>
      <div class="topbar-actions">
        <button class="btn" type="button" :disabled="isCreating" @click="handleCancel">取消</button>
        <button v-if="mode === 'new' || currentStep >= 3" class="btn primary" type="button" :disabled="isCreating" @click="handleSubmit">
          <span v-if="isCreating" class="button-spinner" aria-hidden="true"></span>
          <DemoIcon v-else name="check" :size="14" />{{ isCreating ? '创建中…' : mode === 'new' ? '创建图纸' : '创建草稿' }}
        </button>
      </div>
    </div>

    <!-- 创建新图纸：单屏最小建档，图纸材料可选 -->
    <section v-if="mode === 'new'" class="wizard-step-panel">
      <input
        ref="newMaterialFilesInput"
        type="file"
        :accept="NEW_DRAWING_MATERIAL_ACCEPT"
        multiple
        class="hidden-input"
        @change="onNewMaterialsChange"
      />
      <input
        ref="newAssemblyInput"
        type="file"
        :accept="DRAWING_2D_ACCEPT"
        class="hidden-input"
        @change="onNewAssemblyChange"
      />

      <div class="card wizard-card">
        <div class="section-head">
          <div class="head-icon-box">
            <DemoIcon name="file-plus" :size="16" />
          </div>
          <div class="head-text">
            <div class="title-badge-row">
              <h2>新图纸信息</h2>
              <span class="badge muted-badge">{{ newMaterialFiles.length ? `${newMaterialFiles.length} 份材料` : '图纸材料可选' }}</span>
            </div>
            <p class="head-tip">只输入图纸名称与图号即可建档；建议顺手传一份总图供他人拆图；图纸材料会归档到资料档案，都能稍后在图纸详情补传</p>
          </div>
        </div>

        <div class="wizard-card-body">
          <div class="form-grid new-form-grid">
            <div class="form-item required">
              <label for="create-new-name">图纸名称</label>
              <input
                id="create-new-name"
                :aria-invalid="Boolean(validationErrors['create-new-name'])"
                :aria-describedby="validationErrors['create-new-name'] ? 'create-new-name-error' : undefined"
                v-model="formNewName"
                class="inp"
                placeholder="例如：主轴回转机构装配图"
              />
              <small class="field-help">用于图纸库列表与详情标题</small>
              <small v-if="validationErrors['create-new-name']" id="create-new-name-error" class="field-error">{{ validationErrors['create-new-name'] }}</small>
            </div>

            <div class="form-item required">
              <label for="create-new-no">图号</label>
              <input
                id="create-new-no"
                :aria-invalid="Boolean(validationErrors['create-new-no'])"
                :aria-describedby="validationErrors['create-new-no'] ? 'create-new-no-error' : undefined"
                v-model="formDrawingNo"
                class="inp"
                placeholder="例如：JG9055e-50/32-00"
              />
              <small class="field-help">图号在系统内唯一；留空的项目号会自动取该图号</small>
              <small v-if="validationErrors['create-new-no']" id="create-new-no-error" class="field-error">{{ validationErrors['create-new-no'] }}</small>
            </div>
          </div>

          <details class="new-optional-block">
            <summary>
              <DemoIcon name="sliders-horizontal" :size="13" />更多信息（可选）
            </summary>
            <div class="form-grid new-form-grid">
              <div class="form-item">
                <label for="create-new-project-no">项目号</label>
                <input id="create-new-project-no" v-model="formProjectNo" class="inp" placeholder="留空则与图号相同" />
                <small class="field-help">用于项目分组与目录归类，不是总图图号。</small>
              </div>
              <div class="form-item">
                <label for="create-new-remark">备注</label>
                <input id="create-new-remark" v-model="formRemark" class="inp" placeholder="用途、交付要求或技术交底说明" />
              </div>
            </div>
          </details>

          <!-- 总图：其他人拆图与派活的依据；可以先建档后补，也可以现在就传 -->
          <div class="card panel-card new-assembly-card">
            <div class="section-head">
              <div class="head-icon-box">
                <DemoIcon name="layers" :size="16" />
              </div>
              <div class="head-text">
                <div class="title-badge-row">
                  <h2>总图（拆图依据）</h2>
                  <span class="badge" :class="newAssemblyFile ? 'ok-badge' : 'muted-badge'">{{ newAssemblyFile ? '已选择' : '未上传' }}</span>
                </div>
                <p class="head-tip">总图是拆图与派活的依据：先传上来，其他人才能在图纸预览里按总图拆出零件并认领；也可以先建档，创建后到图纸详情补传</p>
              </div>
            </div>

            <div class="panel-card-body">
              <div
                class="upload-box modern-drop-card assembly-box"
                :class="{ active: isDraggingNewAssembly, 'has-file': Boolean(newAssemblyFile) }"
                @dragover.prevent="isDraggingNewAssembly = true"
                @dragleave.prevent="isDraggingNewAssembly = false"
                @drop.prevent="onNewAssemblyDrop"
              >
                <template v-if="!newAssemblyFile">
                  <div class="upload-icon-wrap">
                    <DemoIcon name="file-up" :size="26" />
                  </div>
                  <div class="upload-texts">
                    <b>拖拽总图文件至此处，或点击按钮选取</b>
                    <p>支持 EXB / DWG / DXF / PDF；创建时一并归档，供其他人拆图</p>
                  </div>
                  <div class="upload-actions">
                    <button class="btn primary" type="button" @click="triggerNewAssemblyPick">
                      <DemoIcon name="upload" :size="14" />选择总图文件
                    </button>
                  </div>
                </template>

                <template v-else>
                  <div class="file-picked-card">
                    <div class="picked-main">
                      <div class="picked-icon">
                        <DemoIcon name="file-check-2" :size="20" />
                      </div>
                      <div class="picked-meta">
                        <div class="picked-name" :title="newAssemblyFile.name">{{ newAssemblyFile.name }}</div>
                        <div class="picked-sub">
                          <span class="tag plain">2D 总图</span>
                          <span>{{ newAssemblyFile.size }}</span>
                        </div>
                      </div>
                    </div>
                    <div class="picked-ops">
                      <button class="btn sm" type="button" @click="triggerNewAssemblyPick">重新选择</button>
                      <button class="btn sm danger" type="button" @click="removeNewAssembly">移除</button>
                    </div>
                  </div>
                </template>
              </div>
            </div>
          </div>

          <div class="card panel-card new-material-card">
            <div class="section-head">
              <div class="head-icon-box">
                <DemoIcon name="archive" :size="16" />
              </div>
              <div class="head-text">
                <div class="title-badge-row">
                  <h2>图纸材料（可选）</h2>
                  <span class="badge muted-badge">{{ newMaterialSummary }}</span>
                </div>
                <p class="head-tip">不限格式，图片、文档、压缩包乃至图纸与 3D 文件都能收（单文件 ≤ 100 MB）；创建成功后统一归档到本图的「资料档案 → 图纸材料」，不占用图纸文件</p>
              </div>
            </div>

            <div class="panel-card-body">
              <div
                class="upload-box modern-drop-card material-box"
                :class="{ active: isDraggingNewMaterials }"
                @dragover.prevent="isDraggingNewMaterials = true"
                @dragleave.prevent="isDraggingNewMaterials = false"
                @drop.prevent="onNewMaterialsDrop"
              >
                <div class="upload-icon-wrap"><DemoIcon name="file-up" :size="26" /></div>
                <div class="upload-texts">
                  <b>拖拽图纸材料至此处，或点击按钮选取</b>
                  <p>不限格式：图片、文档、压缩包，甚至图纸与 3D 文件都能收；可以不上传，上传的会在创建时一并归入资料档案</p>
                </div>
                <div class="upload-actions">
                  <button class="btn sm" type="button" @click="triggerNewMaterialsPick">
                    <DemoIcon name="files" :size="13" />选择图纸材料
                  </button>
                </div>
              </div>

              <div v-if="newMaterialFiles.length" class="parts-list-card modern-list">
                <div class="parts-list-head">
                  <span>待归档图纸材料（{{ newMaterialFiles.length }}）→ 创建后进资料档案</span>
                  <button class="text-btn danger" type="button" @click="clearAllNewMaterials">清空列表</button>
                </div>
                <div class="parts-list-body panel-scroll">
                  <div v-for="item in newMaterialFiles" :key="item.id" class="part-item-row">
                    <DemoIcon name="file" :size="14" />
                    <span class="part-name" :title="item.name">{{ item.name }}</span>
                    <span class="tag plain">{{ item.label }}</span>
                    <span class="part-size">{{ item.size }}</span>
                    <button class="icon-btn xs" type="button" title="移除" @click="removeNewMaterial(item.id)">
                      <DemoIcon name="x" :size="12" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <footer class="panel-actions">
          <span class="step-hint">总图与图纸材料都可以不选；总图没传其他人无法拆图，图纸材料会在创建时归入资料档案，都能稍后补传</span>
          <div class="footer-actions">
            <button class="btn" type="button" :disabled="isCreating" @click="handleCancel">取消</button>
            <button class="btn primary" type="button" :disabled="isCreating" @click="handleSubmit">
              <span v-if="isCreating" class="button-spinner" aria-hidden="true"></span>
              <DemoIcon v-else name="check" :size="15" />{{ isCreating ? '创建中…' : '创建图纸' }}
            </button>
          </div>
        </footer>
      </div>
    </section>

    <template v-else>
    <!-- 现代向导进度步骤指示器 (Stepper) -->
    <div class="wizard-stepper card">
      <div
        v-for="s in steps"
        :key="s.step"
        class="stepper-item"
        role="button"
        tabindex="0"
        :class="{
          active: currentStep === s.step,
          completed: currentStep > s.step,
          clickable: true,
        }"
        @click="goToStep(s.step)"
        @keydown.enter.prevent="goToStep(s.step)"
        @keydown.space.prevent="goToStep(s.step)"
      >
        <div class="stepper-badge">
          <DemoIcon v-if="currentStep > s.step" name="check" :size="14" />
          <span v-else>{{ s.step }}</span>
        </div>
        <div class="stepper-content">
          <div class="stepper-title-row">
            <span class="stepper-title">{{ s.title }}</span>
            <span v-if="s.step === 1 && mode === 'legacy' && (assemblyFile || partFiles.length)" class="stepper-count-tag">
              {{ (assemblyFile ? 1 : 0) + partFiles.length }} 图
            </span>
            <span v-else-if="s.step === 1 && mode === 'fork' && selectedForkSourceNo" class="stepper-count-tag">
              已选源图
            </span>
            <span v-else-if="s.step === 4 && (evidenceFiles.length || (mode === 'legacy' && modelFiles.length))" class="stepper-count-tag">
              {{ evidenceFiles.length + (mode === 'legacy' ? modelFiles.length : 0) }} 份
            </span>
          </div>
          <span class="stepper-sub">{{ s.sub }}</span>
        </div>
        <div v-if="s.step < steps.length" class="stepper-line" aria-hidden="true"></div>
      </div>
    </div>

    <!-- 分叉模式步骤 1：选择要继承的源图纸 -->
    <section v-if="currentStep === 1 && mode === 'fork'" class="wizard-step-panel">
      <div class="card wizard-card">
        <div class="section-head">
          <div class="head-icon-box">
            <DemoIcon name="git-branch" :size="16" />
          </div>
          <div class="head-text">
            <h2>选择要分叉的老图纸</h2>
            <p class="head-tip">分叉会完整继承源图纸的属性、零件层级与工艺备料文件，生成一个全新的项目档案</p>
          </div>
          <span class="badge muted-badge">{{ existingDrawings.length }} 张可选</span>
        </div>

        <div class="wizard-card-body">
          <div class="fork-source-row">
            <label for="fork-source-select">源图纸 *</label>
            <select id="fork-source-select" v-model="selectedForkSourceNo" class="inp" @change="onForkSourceChange">
              <option value="">请选择已有图纸作为模板…</option>
              <option v-for="item in existingDrawings" :key="item.no" :value="item.no">
                {{ item.no }} · {{ item.name }} ({{ item.vendor || '内部项目部' }})
              </option>
            </select>
            <small v-if="validationErrors['fork-source-select']" class="field-error">{{ validationErrors['fork-source-select'] }}</small>
            <p v-if="existingDrawings.length" class="fork-tip">
              <DemoIcon name="info" :size="12" />
              选中后自动带入源图纸的项目信息与属性，下一步可逐项修改；新图号必须与原图号不同。
            </p>
            <p v-else class="fork-tip">
              <DemoIcon name="info" :size="12" />
              图纸库中还没有可分叉的图纸；请先用「上传老图纸」或「创建新图纸」建立第一份档案。
            </p>
          </div>
        </div>

        <footer class="panel-actions">
          <span class="step-hint">分叉不会修改源图纸，源图与衍生图各自独立维护</span>
          <div class="footer-actions">
            <button class="btn primary" type="button" @click="nextStep">
              下一步：基础档案 <DemoIcon name="arrow-right" :size="14" />
            </button>
          </div>
        </footer>
      </div>
    </section>

    <!-- 步骤 1: 2D 工程图纸（最前节点：上传总图自动识别基本信息） -->
    <section v-else-if="currentStep === 1" class="wizard-step-panel">
      <!-- 隐藏的文件选择器 -->
      <input
        ref="assemblyFileInput"
        type="file"
        :accept="DRAWING_2D_ACCEPT"
        class="hidden-input"
        @change="onAssemblyChange"
      />
      <input
        ref="partFilesInput"
        type="file"
        :accept="DRAWING_2D_ACCEPT"
        multiple
        class="hidden-input"
        @change="onPartsChange"
      />
      <input
        ref="partFolderInput"
        type="file"
        :accept="DRAWING_2D_ACCEPT"
        multiple
        webkitdirectory
        class="hidden-input"
        @change="onFolderChange"
      />

      <div class="panel-grid">
        <!-- 2D 总图卡片 -->
        <div class="card panel-card">
          <div class="section-head">
            <div class="head-icon-box">
              <DemoIcon name="layers" :size="16" />
            </div>
            <div class="head-text">
              <div class="title-badge-row">
                <h2>2D 项目总图</h2>
                <span class="badge" :class="assemblyFile ? 'ok-badge' : 'muted-badge'">
                  {{ assemblyFile ? '已就绪' : '推荐先选' }}
                </span>
              </div>
              <p class="head-tip">上传 EXB / DWG / DXF / PDF，自动提取图号与项目信息并带入下一步</p>
            </div>
          </div>

          <div class="panel-card-body">
            <div
              class="upload-box modern-drop-card assembly-box"
              :class="{ active: isDraggingAssembly, 'has-file': Boolean(assemblyFile) }"
              @dragover.prevent="isDraggingAssembly = true"
              @dragleave.prevent="isDraggingAssembly = false"
              @drop.prevent="onAssemblyDrop"
            >
              <template v-if="!assemblyFile">
                <div class="upload-icon-wrap">
                  <DemoIcon name="file-up" :size="26" />
                </div>
                <div class="upload-texts">
                  <b>拖拽总图文件至此处，或点击按钮选取</b>
                  <p>支持 EXB、DWG、DXF 等标准 2D 格式，自动识别项目名、项目号与图号</p>
                </div>
                <div class="upload-actions">
                  <button class="btn primary" type="button" @click="triggerAssemblyPick">
                    <DemoIcon name="upload" :size="14" />选择总图文件
                  </button>
                </div>
              </template>

              <template v-else>
                <div class="file-picked-card">
                  <div class="picked-main">
                    <div class="picked-icon">
                      <DemoIcon name="file-check-2" :size="20" />
                    </div>
                    <div class="picked-meta">
                      <div class="picked-name" :title="assemblyFile.name">{{ assemblyFile.name }}</div>
                      <div class="picked-sub">
                        <span class="tag plain">2D 总图</span>
                        <span>{{ assemblyFile.size }}</span>
                        <span class="text-ok">✓ 解析就绪</span>
                      </div>
                    </div>
                  </div>
                  <div class="picked-ops">
                    <button class="btn sm" type="button" @click="triggerAssemblyPick">重新选择</button>
                    <button class="btn sm danger" type="button" @click="removeAssembly">移除</button>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>

        <!-- 2D 零件图批量上传卡片 -->
        <div class="card panel-card">
          <div class="section-head">
            <div class="head-icon-box">
              <DemoIcon name="boxes" :size="16" />
            </div>
            <div class="head-text">
              <div class="title-badge-row">
                <h2>2D 零件图批量导入</h2>
                <span class="badge muted-badge">{{ partFiles.length }} 个零件</span>
              </div>
              <p class="head-tip">支持多文件选取或直接选择整套图纸文件夹，智能识别父子层级</p>
            </div>
          </div>

          <div class="panel-card-body">
            <div
              class="upload-box modern-drop-card parts-box"
              :class="{ active: isDraggingParts }"
              @dragover.prevent="isDraggingParts = true"
              @dragleave.prevent="isDraggingParts = false"
              @drop.prevent="onPartsDrop($event)"
            >
              <div class="upload-icon-wrap">
                <DemoIcon name="folder-up" :size="26" />
              </div>
              <div class="upload-texts">
                <b>批量添加 2D 零件图</b>
                <p>可直接将文件夹或多个零件图拖入框中进行批量导入</p>
              </div>
              <div class="upload-actions">
                <button class="btn sm" type="button" @click="triggerPartsPick">
                  <DemoIcon name="files" :size="13" />多选文件
                </button>
                <button class="btn sm" type="button" @click="triggerFolderPick">
                  <DemoIcon name="folder-up" :size="13" />选择整文件夹
                </button>
              </div>
            </div>

            <div v-if="partFiles.length" class="parts-list-card modern-list">
              <div class="parts-list-head">
                <span>待入库零件列表 ({{ partFiles.length }})</span>
                <button class="text-btn danger" type="button" @click="clearAllParts">清空列表</button>
              </div>
              <div class="parts-list-body panel-scroll">
                <div v-for="part in partFiles" :key="part.id" class="part-item-row">
                  <DemoIcon name="file" :size="14" />
                  <span class="part-name" :title="part.name">{{ part.name }}</span>
                  <span class="part-size">{{ part.size }}</span>
                  <button class="icon-btn xs" type="button" title="移除" @click="removePart(part.id)">
                    <DemoIcon name="x" :size="12" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <footer class="panel-actions card">
        <span class="step-hint">选入总图后会自动读取项目名与总图图号；若无图纸也可直接进入下一步手动建档</span>
        <div class="footer-actions">
          <button class="btn primary" type="button" @click="nextStep">
            下一步：基础档案 <DemoIcon name="arrow-right" :size="14" />
          </button>
        </div>
      </footer>
    </section>

    <!-- 步骤 2: 基础档案 -->
    <section v-else-if="currentStep === 2" class="wizard-step-panel">
      <div class="card wizard-card">
        <div class="section-head">
          <div class="head-icon-box">
            <DemoIcon name="folder-plus" :size="16" />
          </div>
          <div class="head-text">
            <h2>{{ mode === 'fork' ? '新项目基本信息' : '项目基本信息' }}</h2>
            <p class="head-tip">{{ mode === 'fork'
              ? '已带入源图纸识别信息；请确认新的总图图号与项目号后再提交'
              : '已关联上一步图纸识别信息；可核对修改后再提交' }}</p>
          </div>
        </div>

        <div class="wizard-card-body">
          <div v-if="mode === 'fork'" class="fork-source-row">
            <label>继承来源 *</label>
            <div class="fork-source-readonly">
              <span class="mono">{{ selectedForkSourceNo }}</span>
              <button class="btn sm" type="button" @click="goToStep(1)">重新选择源图纸</button>
            </div>
            <p class="fork-tip">
              <DemoIcon name="info" :size="12" />
              新项目将继承源图纸的零件层级与工艺备料文件；提交前请确认新图号与原图号不同。
            </p>
          </div>
          <div class="form-grid">
            <div class="form-item required">
              <label for="create-project-name">项目名称</label>
              <input
                id="create-project-name"
                :aria-invalid="Boolean(validationErrors['create-project-name'])"
                :aria-describedby="validationErrors['create-project-name'] ? 'create-project-name-error' : undefined"
                v-model="formProject"
                class="inp"
                placeholder="例如：智能回转减速传动装置"
              />
              <small v-if="validationErrors['create-project-name']" id="create-project-name-error" class="field-error">{{ validationErrors['create-project-name'] }}</small>
            </div>

            <div class="form-item required">
              <label for="create-project-no">项目号</label>
              <input
                id="create-project-no"
                :aria-invalid="Boolean(validationErrors['create-project-no'])"
                :aria-describedby="validationErrors['create-project-no'] ? 'create-project-no-error' : undefined"
                v-model="formProjectNo"
                class="inp"
                placeholder="例如：PRJ-2026-081"
              />
              <small class="field-help">用于项目分类和文件夹目录，不是总图图号。</small>
              <small v-if="validationErrors['create-project-no']" id="create-project-no-error" class="field-error">{{ validationErrors['create-project-no'] }}</small>
            </div>

            <div class="form-item required">
              <label for="create-drawing-no">总图图号</label>
              <input
                id="create-drawing-no"
                :aria-invalid="Boolean(validationErrors['create-drawing-no'])"
                :aria-describedby="validationErrors['create-drawing-no'] ? 'create-drawing-no-error' : undefined"
                v-model="formDrawingNo"
                class="inp"
                :placeholder="isIdentifyingAssembly ? '正在识别文件名…' : '例如：JG9055e-50/32-00'"
                :disabled="isIdentifyingAssembly"
              />
              <small
                class="field-help"
                :class="{ error: (mode === 'fork' && formDrawingNo === selectedForkSourceNo) || (assemblyIdentifyMessage && !formDrawingNo && !isIdentifyingAssembly) }"
              >
                {{ mode === 'fork' && formDrawingNo === selectedForkSourceNo ? '分叉需要新的总图图号，请修改后再提交。' : assemblyIdentifyMessage || '图号规范唯一，已关联上一步总图文件名识别结果。' }}
              </small>
              <small v-if="validationErrors['create-drawing-no']" id="create-drawing-no-error" class="field-error">{{ validationErrors['create-drawing-no'] }}</small>
            </div>

            <div class="form-item">
              <label for="create-remark">项目说明与备忘</label>
              <input
                id="create-remark"
                v-model="formRemark"
                class="inp"
                placeholder="填写项目背景、技术交底要求或交付期限等"
              />
            </div>
          </div>
        </div>

        <footer class="panel-actions">
          <button class="btn" type="button" @click="prevStep">
            <DemoIcon name="arrow-left" :size="14" />上一步：{{ mode === 'fork' ? '源图纸' : '2D 工程图纸' }}
          </button>
          <div class="footer-actions">
            <button class="btn primary" type="button" @click="nextStep">
              下一步：图纸属性 <DemoIcon name="arrow-right" :size="14" />
            </button>
          </div>
        </footer>
      </div>
    </section>

    <!-- 步骤 3: 图纸业务属性 -->
    <section v-else-if="currentStep === 3" class="wizard-step-panel">
      <div class="card wizard-card">
        <div class="section-head">
          <div class="head-icon-box">
            <DemoIcon name="sliders-horizontal" :size="16" />
          </div>
          <div class="head-text">
            <h2>图纸业务属性</h2>
            <p class="head-tip">按后台配置录入业务分类，带 * 的为必填项；未选或停用不影响历史数据</p>
          </div>
          <span class="badge muted-badge">{{ attributeStore.sortedAttributes.length }} 项配置</span>
        </div>

        <div class="wizard-card-body">
          <div v-if="attributeStore.sortedAttributes.length" class="attributes-wrapper step-attributes-container">
            <DrawingAttributesForm
              v-model="formAttributeValues"
              :attributes="attributeStore.sortedAttributes"
              :errors="attributeFieldErrors"
              compact
            />
          </div>
          <p v-else class="empty-tip">当前没有启用中的图纸属性，可直接进入下一步。</p>
        </div>

        <footer class="panel-actions">
          <button class="btn" type="button" @click="prevStep">
            <DemoIcon name="arrow-left" :size="14" />上一步：基础档案
          </button>
          <div class="footer-actions">
            <button class="btn" type="button" :disabled="isCreating" @click="handleSubmit">
              <DemoIcon name="check" :size="13" />创建草稿
            </button>
            <button class="btn primary" type="button" @click="nextStep">
              下一步：{{ mode === 'fork' ? '相关资料' : '3D 模型与资料' }} <DemoIcon name="arrow-right" :size="14" />
            </button>
          </div>
        </footer>
      </div>
    </section>

    <!-- 步骤 4: 3D 模型与相关资料 -->
    <section v-else class="wizard-step-panel">
      <!-- 隐藏的文件选择器 -->
      <input
        v-if="mode === 'legacy'"
        ref="modelFilesInput"
        type="file"
        :accept="MODEL_FILE_ACCEPT"
        multiple
        class="hidden-input"
        @change="onModelsChange"
      />
      <input
        ref="evidenceFilesInput"
        type="file"
        multiple
        class="hidden-input"
        @change="onEvidenceChange"
      />

      <div class="panel-grid" :class="{ 'single-column': mode === 'fork' }">
        <!-- 3D 模型上传卡片；分叉模式不支持新增模型，直接隐藏避免选择后被静默丢弃 -->
        <div v-if="mode === 'legacy'" class="card panel-card">
          <div class="section-head">
            <div class="head-icon-box">
              <DemoIcon name="box" :size="16" />
            </div>
            <div class="head-text">
              <div class="title-badge-row">
                <h2>3D 模型文件</h2>
                <span class="badge muted-badge">{{ modelFiles.length }} 个模型</span>
              </div>
              <p class="head-tip">支持 Z3PRT / Z3ASM / STEP / IGES 及主流 3D 装配体</p>
            </div>
          </div>

          <div class="panel-card-body">
            <div
              class="upload-box modern-drop-card model-box"
              :class="{ active: isDraggingModels }"
              @dragover.prevent="isDraggingModels = true"
              @dragleave.prevent="isDraggingModels = false"
              @drop.prevent="onModelsDrop($event)"
            >
              <div class="upload-icon-wrap"><DemoIcon name="box" :size="26" /></div>
              <div class="upload-texts">
                <b>上传 3D 模型或三维装配包（可选）</b>
                <p>可指定该 3D 文件关联至总图或具体零件图</p>
              </div>
              <div class="upload-actions">
                <button class="btn sm" type="button" @click="triggerModelsPick">
                  <DemoIcon name="files" :size="13" />选择 3D 文件
                </button>
              </div>
            </div>

            <div v-if="modelFiles.length" class="parts-list-card modern-list">
              <div class="parts-list-head">
                <span>待上传 3D 模型（{{ modelFiles.length }}）</span>
                <button class="text-btn danger" type="button" @click="clearAllModels">清空列表</button>
              </div>
              <div class="parts-list-body panel-scroll">
                <div v-for="model in modelFiles" :key="model.id" class="part-item-row model-item-row">
                  <DemoIcon name="box" :size="14" />
                  <span class="part-name" :title="model.name">{{ model.name }}</span>
                  <select v-model="model.linkTo" class="inp model-link-select" aria-label="选择关联的 2D 图纸">
                    <option value="">不关联</option>
                    <option v-for="target in modelLinkTargets" :key="target.value" :value="target.value">
                      关联：{{ target.label }}
                    </option>
                  </select>
                  <span class="part-size">{{ model.size }}</span>
                  <button class="icon-btn xs" type="button" title="移除" @click="removeModel(model.id)">
                    <DemoIcon name="x" :size="12" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 相关资料归档卡片 -->
        <div class="card panel-card">
          <div class="section-head">
            <div class="head-icon-box">
              <DemoIcon name="archive" :size="16" />
            </div>
            <div class="head-text">
              <div class="title-badge-row">
                <h2>相关资料归档</h2>
                <span class="badge muted-badge">{{ evidenceFiles.length }} 份资料</span>
              </div>
              <p class="head-tip">技术协议、客户确认图与设计依据；创建后自动入库并保留 SHA-256 指纹</p>
            </div>
          </div>

          <div class="panel-card-body">
            <div class="evidence-meta-grid">
              <div class="form-item">
                <label for="create-evidence-category">资料分类</label>
                <select id="create-evidence-category" v-model="evidenceCategory" class="inp">
                  <option v-for="item in EVIDENCE_CATEGORIES" :key="item" :value="item">{{ item }}</option>
                </select>
              </div>
              <div class="form-item">
                <label for="create-evidence-folder">存放目录</label>
                <input
                  id="create-evidence-folder"
                  v-model="evidenceFolderPath"
                  class="inp"
                  placeholder="例如：技术方案/评审纪要"
                />
              </div>
              <div class="form-item">
                <label for="create-evidence-desc">说明 / 依据备忘</label>
                <input
                  id="create-evidence-desc"
                  v-model="evidenceDescription"
                  class="inp"
                  placeholder="补充资料来源、关键结论或替代关系"
                />
              </div>
            </div>

            <div
              class="upload-box modern-drop-card evidence-box"
              :class="{ active: isDraggingEvidence }"
              @dragover.prevent="isDraggingEvidence = true"
              @dragleave.prevent="isDraggingEvidence = false"
              @drop.prevent="onEvidenceDrop"
            >
              <div class="upload-icon-wrap"><DemoIcon name="archive" :size="26" /></div>
              <div class="upload-texts">
                <b>上传相关材料文件（可选）</b>
                <p>支持 PDF、Word、Excel、图纸、图片、压缩包等格式，单份不超过 100 MB</p>
              </div>
              <div class="upload-actions">
                <button class="btn sm" type="button" @click="triggerEvidencePick">
                  <DemoIcon name="files" :size="13" />选择资料文件
                </button>
              </div>
            </div>

            <div v-if="evidenceFiles.length" class="parts-list-card modern-list">
              <div class="parts-list-head">
                <span>待归档资料（{{ evidenceFiles.length }}）</span>
                <button class="text-btn danger" type="button" @click="clearAllEvidence">清空列表</button>
              </div>
              <div class="parts-list-body panel-scroll">
                <div v-for="item in evidenceFiles" :key="item.id" class="part-item-row evidence-item-row">
                  <DemoIcon name="file" :size="14" />
                  <input v-model="item.title" class="inp evidence-title-input" aria-label="资料标题" placeholder="资料标题" />
                  <span class="part-name" :title="item.name">{{ item.name }}</span>
                  <span class="part-size">{{ item.size }}</span>
                  <button class="icon-btn xs" type="button" title="移除" @click="removeEvidence(item.id)">
                    <DemoIcon name="x" :size="12" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <footer class="panel-actions card">
        <button class="btn" type="button" @click="prevStep">
          <DemoIcon name="arrow-left" :size="14" />上一步：图纸属性
        </button>
        <div class="footer-actions">
          <button class="btn" type="button" :disabled="isCreating" @click="handleCancel">取消</button>
          <button class="btn primary" type="button" :disabled="isCreating" @click="handleSubmit">
            <span v-if="isCreating" class="button-spinner" aria-hidden="true"></span>
            <DemoIcon v-else name="check" :size="15" />{{ isCreating ? '创建中…' : '创建草稿' }}
          </button>
        </div>
      </footer>
    </section>
    </template>
    </template>
  </div>
</template>

<style scoped>
.create-validation, .field-error { color: var(--danger); }
.create-validation { padding: 12px 16px; border: 1px solid var(--danger); border-radius: var(--radius); }
.field-error { display: block; margin-top: 4px; }
.inp[aria-invalid='true'] { border-color: var(--danger); }
.saved-drawing-state { padding: 24px; }
.saved-drawing-state p { margin: 12px 0; }
.saved-drawing-state .btn + .btn { margin-left: 8px; }
.drawing-create-view {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  max-width: 1360px;
  margin: 0 auto;
  width: 100%;
  box-sizing: border-box;
}

/* 顶部栏 */
.create-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 0 8px;
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.create-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 800;
  letter-spacing: -0.2px;
}

.create-subtitle {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.topbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 现代分步向导 Stepper */
.wizard-stepper {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  border-radius: 12px;
  background: var(--panel);
  gap: 8px;
  flex-shrink: 0;
  min-height: 52px;
  box-sizing: border-box;
}

.stepper-item {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 1;
  position: relative;
  cursor: pointer;
  padding: 4px 10px;
  border-radius: 8px;
  transition: all 0.2s ease;
  user-select: none;
  min-width: 0;
}

.stepper-item:hover {
  background: var(--panel-2);
}

.stepper-item.active {
  background: color-mix(in srgb, var(--accent-soft) 40%, var(--panel));
}

.stepper-badge {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 1.5px solid var(--line);
  background: var(--panel-2);
  color: var(--text-3);
  font-weight: 700;
  font-size: 12px;
  flex-shrink: 0;
  transition: all 0.25s ease;
}

.stepper-item.active .stepper-badge {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--accent-ink);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.stepper-item.completed .stepper-badge {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}

.stepper-content {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  flex: 1;
}

.stepper-title-row {
  display: flex;
  align-items: center;
  gap: 6px;
  line-height: 1.25;
}

.stepper-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-2);
  white-space: nowrap;
}

.stepper-item.active .stepper-title {
  color: var(--accent);
  font-weight: 700;
}

.stepper-item.completed .stepper-title {
  color: var(--text-1);
}

.stepper-sub {
  font-size: 10.5px;
  color: var(--text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.25;
}

.stepper-count-tag {
  padding: 1px 5px;
  border-radius: 99px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 10px;
  font-weight: 600;
}

.stepper-line {
  position: absolute;
  right: -6px;
  top: 50%;
  transform: translateY(-50%);
  width: 12px;
  height: 1px;
  background: var(--line);
  pointer-events: none;
}

@media (max-width: 860px) {
  .wizard-stepper {
    flex-direction: column;
    align-items: stretch;
  }
  .stepper-line {
    display: none;
  }
}

/* 步骤容器与两栏网格自适应 */
.wizard-step-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  flex: 1;
  min-height: 0;
}

.wizard-card {
  display: flex;
  flex-direction: column;
  padding: 16px 20px;
  border-radius: var(--radius);
  flex: 1;
  min-height: 0;
}

.wizard-card-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-right: 4px;
}

.panel-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  flex: 1;
  min-height: 0;
}

.panel-card {
  display: flex;
  flex-direction: column;
  padding: 14px 16px;
  border-radius: var(--radius);
  min-height: 0;
}

.panel-card-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-right: 2px;
}

.panel-scroll {
  max-height: 160px;
  overflow-y: auto;
}

/* 统一底栏控制台 */
.panel-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  border-top: 1px solid var(--line);
  margin-top: 8px;
  flex-shrink: 0;
  gap: 12px;
}

.panel-actions.card {
  margin-top: 0;
  border-radius: var(--radius);
  border-top: 1px solid var(--line);
}

.step-hint {
  font-size: 11.5px;
  color: var(--text-3);
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 头部设计 */
.section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  flex-shrink: 0;
}

.head-icon-box {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--accent-soft);
  color: var(--accent);
  flex-shrink: 0;
}

.head-text {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.section-head h2 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
  color: var(--text-1);
}

.title-badge-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.head-tip {
  margin: 0;
  color: var(--text-3);
  font-size: 11.5px;
}

/* 紧凑型 Segmented Control 分段控制器 */
.segmented-control {
  display: inline-flex;
  align-items: center;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 2px;
  gap: 2px;
  margin-left: auto;
}

.segment {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 10px;
  font-size: 12px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-2);
  transition: all 0.2s ease;
  user-select: none;
}

.segment input {
  display: none;
}

.segment.active {
  background: var(--panel);
  color: var(--accent);
  font-weight: 600;
  box-shadow: 0 1px 3px rgb(0 0 0 / 10%);
}

/* 分叉源图纸提示卡片 */
.fork-source-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line));
  background: color-mix(in srgb, var(--accent-soft) 30%, var(--panel-2));
}

.fork-source-row label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-1);
}

.fork-tip {
  margin: 0;
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: var(--text-2);
  line-height: 1.4;
}

.fork-tip svg {
  color: var(--accent);
  flex-shrink: 0;
}

/* 表单布局 */
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px 14px;
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-item label {
  color: var(--text-2);
  font-size: 11.5px;
  font-weight: 600;
}

.form-item.required label::after {
  content: ' *';
  color: var(--danger);
}

.form-item:last-child {
  grid-column: span 2;
}

.field-help {
  display: block;
  margin-top: 2px;
  color: var(--text-3);
  font-size: 10.5px;
}

.field-help.error {
  color: var(--danger);
}

.attributes-wrapper {
  margin-top: 4px;
}

:deep(.step-attributes-container .attributes-form__header) {
  display: none !important;
}

:deep(.step-attributes-container .attributes-form) {
  background: transparent !important;
  border: none !important;
  padding: 0 !important;
}

.empty-tip {
  margin: 16px 0;
  text-align: center;
  color: var(--text-3);
  font-size: 12px;
}

/* 现代化拖拽与卡片上传 */
.hidden-input {
  display: none;
}

.modern-drop-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 24px 20px;
  border: 1.5px dashed var(--line);
  border-radius: 12px;
  background: var(--panel-2);
  transition: all 0.25s ease;
  cursor: pointer;
}

.modern-drop-card:hover,
.modern-drop-card.active {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--accent-soft) 40%, var(--panel-2));
}

.modern-drop-card.has-file {
  padding: 12px 14px;
  border-style: solid;
  border-color: var(--accent);
  background: var(--panel);
  cursor: default;
}

.upload-icon-wrap {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  margin-bottom: 10px;
  border-radius: 50%;
  background: var(--panel);
  color: var(--accent);
}

.upload-texts b {
  font-size: 13.5px;
  color: var(--text-1);
}

.upload-texts p {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
  line-height: 1.45;
}

.upload-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}

/* 已选择总图卡片 */
.file-picked-card {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.picked-main {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.picked-icon {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  flex: none;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
}

.picked-meta {
  min-width: 0;
  text-align: left;
}

.picked-name {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--text-1);
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.picked-sub {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-3);
  font-size: 11px;
  margin-top: 3px;
}

.text-ok {
  color: #10b981;
  font-weight: 600;
}

.picked-ops {
  display: flex;
  gap: 6px;
  flex: none;
}

/* 列表展示 */
.modern-list {
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  overflow: hidden;
}

.parts-list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid var(--line);
  background: var(--panel);
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-2);
}

.parts-list-body {
  max-height: 200px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.part-item-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-bottom: 1px solid var(--line);
  font-size: 12px;
}

.part-item-row:last-child {
  border-bottom: none;
}

.part-item-row svg {
  color: var(--text-3);
  flex: none;
}

.part-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--text-1);
}

.part-size {
  color: var(--text-3);
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  flex: none;
}

.model-item-row {
  display: grid;
  grid-template-columns: auto minmax(100px, 1fr) minmax(150px, 0.9fr) auto auto;
}

.model-link-select {
  min-width: 0;
  padding: 4px 8px;
  font-size: 11px;
}

.evidence-meta-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.evidence-desc-item {
  margin-top: -2px;
}

.evidence-item-row {
  display: grid;
  grid-template-columns: auto minmax(100px, 0.8fr) minmax(100px, 1.2fr) auto auto;
}

.evidence-title-input {
  min-width: 0;
  padding: 4px 8px;
  font-size: 11.5px;
}

.text-btn {
  border: none;
  background: transparent;
  padding: 0;
  cursor: pointer;
  font-size: 11px;
}

.text-btn.danger {
  color: var(--danger);
}

.text-btn.danger:hover {
  text-decoration: underline;
}

.icon-btn.xs {
  width: 20px;
  height: 20px;
  border-radius: 4px;
}

/* 统一向导底栏控制台 */
.wizard-footer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 20px;
  border-radius: var(--radius);
  background: var(--panel);
  gap: 16px;
  box-shadow: var(--shadow);
}

.step-hint {
  font-size: 12px;
  color: var(--text-3);
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.lg-btn {
  padding: 8px 18px;
  font-size: 13.5px;
}

/* 进度遮罩与断点续传卡片保持原逻辑 */
.create-loading-overlay {
  position: fixed;
  z-index: 50;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 24px;
  background: color-mix(in srgb, var(--bg) 72%, transparent);
  backdrop-filter: blur(3px);
}

.create-loading-card {
  display: flex;
  min-width: 240px;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 26px 30px;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--panel);
  box-shadow: 0 18px 50px rgb(0 0 0 / 18%);
  color: var(--text-1);
  text-align: center;
}

.create-loading-card span:not(.create-spinner) {
  color: var(--text-2);
  font-size: 12px;
}

.create-loading-card small {
  color: var(--text-3);
  font-size: 11px;
}

.conversion-progress-card {
  width: min(520px, calc(100vw - 48px));
  align-items: stretch;
  text-align: left;
}

.conversion-progress-card > strong,
.conversion-progress-card > span {
  text-align: center;
}

.conversion-progress-list {
  display: grid;
  gap: 6px;
  width: 100%;
  max-height: min(42vh, 360px);
  margin-top: 8px;
  overflow-y: auto;
  padding: 8px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
}

.conversion-progress-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 16px;
  padding: 6px 8px;
  border-radius: 6px;
  font-size: 12px;
}

.conversion-progress-row span:first-child {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-2);
}

.conversion-error {
  flex-basis: 100%;
  overflow-wrap: anywhere;
  color: var(--danger, #c45b4b);
}

.conversion-progress-row span:nth-child(2) {
  flex: none;
  color: var(--accent);
}

.conversion-failed-text {
  color: var(--danger, #c45b4b) !important;
}

.conversion-progress-card .btn {
  align-self: center;
  margin-top: 4px;
}

.create-upload-recovery {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin: 14px 0;
  padding: 14px 16px;
  border: 1px solid color-mix(in srgb, var(--danger, #c45b4b) 35%, var(--line));
  border-radius: 12px;
  background: color-mix(in srgb, var(--danger, #c45b4b) 8%, var(--panel));
}

.create-upload-recovery > div:first-child {
  display: grid;
  gap: 4px;
}

.create-upload-recovery span,
.create-upload-recovery small {
  color: var(--text-2);
  font-size: 12px;
}

.create-upload-recovery small {
  color: var(--danger, #c45b4b);
}

.create-upload-recovery-list {
  display: grid;
  gap: 8px;
  margin-top: 12px;
}

.create-upload-recovery-item {
  display: grid;
  grid-template-columns: minmax(120px, 1fr) auto 42px minmax(80px, 180px) auto;
  align-items: center;
  gap: 8px;
}

.upload-progress-value {
  text-align: right;
  font-variant-numeric: tabular-nums;
}

.upload-progress-track {
  display: block;
  height: 5px;
  overflow: hidden;
  border-radius: 999px;
  background: color-mix(in srgb, var(--line) 70%, transparent);
}

.upload-progress-fill {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--accent, #5b8def);
  transition: width 180ms ease;
}

.create-upload-recovery-actions {
  display: flex;
  flex-shrink: 0;
  gap: 8px;
}

.create-spinner,
.button-spinner {
  display: inline-block;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: create-spin 0.75s linear infinite;
}

.create-spinner {
  width: 28px;
  height: 28px;
  margin-bottom: 4px;
  color: var(--accent);
}

.button-spinner {
  width: 13px;
  height: 13px;
}

@keyframes create-spin {
  to {
    transform: rotate(360deg);
  }
}

/* 顶部创建方式切换（创建新图纸 / 上传老图纸 / 从老图纸分叉） */
.mode-switch {
  margin-left: 0;
  flex-shrink: 0;
}

/* 创建新图纸：可选信息折叠区与图纸材料卡片 */
.new-form-grid .form-item:last-child {
  grid-column: auto;
}

.new-optional-block {
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  padding: 8px 12px;
}

.new-optional-block > summary {
  display: flex;
  align-items: center;
  gap: 6px;
  cursor: pointer;
  color: var(--text-2);
  font-size: 12px;
  font-weight: 600;
  list-style: none;
}

.new-optional-block > summary::-webkit-details-marker {
  display: none;
}

.new-optional-block > summary::after {
  content: '展开';
  margin-left: auto;
  color: var(--text-3);
  font-size: 10.5px;
  font-weight: 400;
}

.new-optional-block[open] > summary::after {
  content: '收起';
}

.new-optional-block .form-grid {
  margin-top: 10px;
}

.new-material-card,
.new-assembly-card {
  flex: none;
}

.new-material-card .part-item-row .tag.plain,
.new-assembly-card .picked-sub .tag.plain {
  flex: none;
}

/* 分叉模式：步骤 2 的只读继承来源 */
.fork-source-readonly {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--panel);
}

.fork-source-readonly .mono {
  font-weight: 600;
  color: var(--text-1);
}

/* 分叉模式没有 3D 模型卡片，资料卡片占满整行 */
.panel-grid.single-column {
  grid-template-columns: 1fr;
}

@media (max-width: 900px) {
  .create-topbar {
    flex-wrap: wrap;
  }
  .mode-switch {
    order: 3;
    width: 100%;
    justify-content: space-between;
  }
}
</style>
