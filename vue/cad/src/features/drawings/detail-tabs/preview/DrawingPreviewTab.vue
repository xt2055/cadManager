<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { drawingFileService, editingService } from '@/app/container'
import type { ActiveEditSessionInfo, EditSessionOpenResult } from '@/types/application.types'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useReviewStore } from '@/stores/review.store'
import { useUiStore } from '@/stores/ui.store'
import { CAXA_NOT_FOUND_PREFIX } from '@/modules/editing'
import type { DrawingFile } from '@/types/domain.types'
import type { DrawingSummaryView, FileView, PartView, StructureNodeView } from '@/modules/drawing'
import { parseDrawingNumber } from '@/utils/drawing-number-parser'
import { formatReadableDateTime } from '@/utils/date-time'
import { isTauri } from '@tauri-apps/api/core'
import { saveDownloadFile } from '@/services/tauri/cad-edit.service'
import { convertCadToPdfBlob } from '@/services/cad-pdf-export.service'
import { changeRequestService } from '@/services/change-request.service'
import { editableChangeTargets } from '../../components/detail/change-edit-access'
import { canDeleteDrawingFiles } from './drawing-file-delete'
import { isModelFile } from '@/utils/model-formats'
import { versionDisplayLabel } from '@/modules/versioning/versioning-service'

defineOptions({
  name: 'DrawingPreviewTab',
})

const router = useRouter()
const route = useRoute()
const drawingOperationsStore = useDrawingOperationsStore()
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const uiStore = useUiStore()
const authStore = useAuthStore()

const currentItem = computed<DrawingSummaryView | PartView | null>(() => {
  const id = String(route.params.drawingId ?? '')
  return drawingStore.getDrawing(id) ?? drawingStore.getPart(id)
})
const isAssembly = computed(() => !currentItem.value || !('parentNo' in currentItem.value))
const rootDrawingNo = computed(() => {
  if (!currentItem.value) return ''
  if (!('parentNo' in currentItem.value)) return currentItem.value.no

  let currentNo = currentItem.value.no
  const visited = new Set<string>()
  while (!visited.has(currentNo)) {
    visited.add(currentNo)
    const part = drawingStore.parts.find((item) => item.no === currentNo)
    if (!part) return currentItem.value.parentNo
    if (drawingStore.drawings.some((drawing) => drawing.no === part.parentNo)) return part.parentNo
    currentNo = part.parentNo
  }
  return currentItem.value.parentNo
})

type ProjectDrawingFile = DrawingFile & {
  ownerNo: string
  ownerName: string
}

function toDrawingFile(file: FileView): DrawingFile {
  return {
    ...file,
    rawName: file.name,
    rawStorageKey: file.storageKey,
    history: file.history.map((item) => ({ ...item })),
  }
}

function flattenStructure(nodes: StructureNodeView[]): PartView[] {
  return nodes.flatMap((node) => [node, ...flattenStructure(node.children)])
}

// 总图页是项目级文件清单：总图与结构树内全部零件图都必须在这里出现。
// 文件的归属从结构关系取得，而不依赖 CAD 标题栏或文件角色的历史数据。
const allFiles = computed<ProjectDrawingFile[]>(() => {
  if (!currentItem.value) return []
  const rootNo = rootDrawingNo.value
  const files: ProjectDrawingFile[] = []
  const appendOwnerFiles = (owner: DrawingSummaryView | PartView) => {
    for (const file of [...(owner.files ?? []), ...(owner.otherFiles ?? [])]) {
      files.push({
        ...toDrawingFile(file),
        ownerNo: owner.no,
        ownerName: owner.name,
        ...(file.partNo ? { partNo: file.partNo } : {}),
        drawingNo: file.drawingNo || ('parentNo' in owner ? rootDrawingNo.value : owner.no),
      })
    }
  }

  // 如果是总图，递归汇总总图和所有后代零件。
  if (isAssembly.value) {
    const mainDrawing = currentItem.value as DrawingSummaryView
    appendOwnerFiles(mainDrawing)
    const belongsToDrawing = (part: PartView): boolean => {
      if (part.parentNo === mainDrawing.no || part.no.startsWith(`${mainDrawing.no}-`)) return true

      const visited = new Set<string>()
      let parentNo = part.parentNo
      while (parentNo && !visited.has(parentNo)) {
        if (parentNo === mainDrawing.no) return true
        visited.add(parentNo)
        const parent = drawingStore.parts.find((candidate) => candidate.no === parentNo)
        if (!parent) return part.no.startsWith(`${mainDrawing.no}-`)
        parentNo = parent.parentNo
      }
      return false
    }

    // 优先使用服务端归属到当前总图的结构树；外部借用件、图号前缀不规范的零件
    // 与多层子件都能被递归汇总。旧快照没有结构树时才回退为父级链判断。
    const structureParts = flattenStructure(drawingStore.getStructure(mainDrawing.no))
    const projectParts = structureParts.length
      ? structureParts
      : drawingStore.parts.filter(belongsToDrawing)
    projectParts.forEach(appendOwnerFiles)
  } else {
    // 如果当前选中的就是零件图
    appendOwnerFiles(currentItem.value as PartView)
  }

  return sortProjectFiles(files.filter((file) => !isModelFile(file)))
})

function sortProjectFiles(files: ProjectDrawingFile[]): ProjectDrawingFile[] {
  const uniqueFiles = files.filter((file, index, sourceFiles) => sourceFiles.findIndex((candidate) => candidate.id === file.id) === index)
  return uniqueFiles.sort((left, right) => {
    const roleRank = (file: ProjectDrawingFile) => file.role === 'assembly' ? 0 : file.role === 'part' ? 1 : 2
    return roleRank(left) - roleRank(right) || left.ownerNo.localeCompare(right.ownerNo) || left.name.localeCompare(right.name)
  })
}

const hasAssemblyFile = computed(() => {
  if (isAssembly.value) {
    const mainDrawing = currentItem.value as DrawingSummaryView
    return Boolean(mainDrawing?.files?.some((f) => f.role === 'assembly'))
  }
  return true // 零件图单独查看时
})

// 文件上传 input ref
const assemblyInput = ref<HTMLInputElement | null>(null)
const partInput = ref<HTMLInputElement | null>(null)
const replaceInput = ref<HTMLInputElement | null>(null)

// 替换弹窗与正在替换的目标文件
const isReplacing = ref(false)
const targetReplaceFile = ref<DrawingFile | null>(null)
const replaceReasonInput = ref('')
const selectedReplaceBlob = ref<File | null>(null)

// 借用零件弹窗与多维度智能选型系统
const isBorrowing = ref(false)
const isCreatingDrawing = ref(false)
const creatingDrawing = ref(false)
type DrawingCreationStage = 'preparing' | 'uploading' | 'converting' | 'opening'
const drawingCreationStage = ref<DrawingCreationStage>('preparing')
const drawingCreationError = ref('')
const newDrawingName = ref('')
const newDrawingNo = ref('')
const drawingCreationProgress = computed(() => ({
  preparing: { step: 1, title: '正在准备空白图纸', detail: '正在校验 CAXA 模板，请稍候…' },
  uploading: { step: 2, title: '正在创建图纸', detail: '正在创建草稿零件并保存空白模板…' },
  converting: { step: 3, title: '正在同步新建图纸', detail: '正在读取刚创建的零件和附件信息…' },
  opening: { step: 4, title: '正在准备并打开 CAXA', detail: '正在等待 DWG 就绪并启动本地编辑，首次可能需要几十秒…' },
})[drawingCreationStage.value])
const canCreateDrawing = computed(() => {
  const project = drawingStore.getDrawing(rootDrawingNo.value)
  const user = authStore.currentUser
  return Boolean(project && user && ['draft', 'published'].includes(project.status)
    && (user.roles?.includes('admin') || project.createdBy === user.displayName))
})

function openCreateDrawing() {
  newDrawingName.value = ''
  newDrawingNo.value = ''
  drawingCreationError.value = ''
  drawingCreationStage.value = 'preparing'
  isCreatingDrawing.value = true
}

function closeCreateDrawing() {
  if (creatingDrawing.value) return
  isCreatingDrawing.value = false
  drawingCreationError.value = ''
}

function guardDrawingCreationUnload(event: BeforeUnloadEvent) {
  if (!creatingDrawing.value) return
  event.preventDefault()
  event.returnValue = ''
}

async function createDrawing() {
  if (creatingDrawing.value || !canCreateDrawing.value) return
  const name = newDrawingName.value.trim()
  const no = newDrawingNo.value.trim()
  const parentNo = rootDrawingNo.value
  if (!name || !no) {
    uiStore.toast('请填写名称和图号', 'warn')
    return
  }
  const normalize = (value: string) => value.replace(/[\s/\\]/g, '').toLowerCase()
  if ([...drawingStore.drawings, ...drawingStore.parts].some((item) => normalize(item.no) === normalize(no))) {
    uiStore.toast('图号已存在，请使用其他图号', 'warn')
    return
  }
  drawingCreationError.value = ''
  drawingCreationStage.value = 'preparing'
  creatingDrawing.value = true
  let created = false
  try {
    const response = await fetch(`${import.meta.env.BASE_URL}templates/blank.exb`)
    if (!response.ok) throw new Error('空白 EXB 模板读取失败')
    const content = await response.blob()
    const signature = new Uint8Array(await content.slice(0, 8).arrayBuffer())
    if (signature.length !== 8 || ![0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1].every((byte, index) => signature[index] === byte)) {
      throw new Error('空白 EXB 模板无效')
    }
    const filename = `${no}(${name})`.replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_') + '.exb'
    const file: DrawingFile = {
      id: crypto.randomUUID(), name: filename, size: formatFileSize(content.size),
      role: 'part', drawingNo: parentNo, partNo: no, version: 'v1.0',
      uploadedBy: authStore.currentUser?.displayName || '', uploadedAt: formatCurrentTime(), previewable: true,
    }
    drawingCreationStage.value = 'uploading'
    await drawingOperationsStore.createPartWithFile(parentNo, {
      no, name, parentNo, project: drawingStore.getDrawing(parentNo)?.project || parentNo,
      material: '—', spec: '', weight: 0, surfaceTreatment: '', partType: '自制件',
      qty: 1, status: 'draft', ver: 'v1.0', hasFile: true, files: [file],
    }, file, content)
    created = true
    drawingCreationStage.value = 'converting'
    await drawingStore.refresh()
    // 新建零件仅有这张附件；EXB 转换完成后展示名可能已经变为 DWG。
    const savedFile = allFiles.value.find((entry) => entry.ownerNo === no && entry.role === 'part')
    uiStore.toast(`图纸「${name}」已创建`, 'ok')
    if (savedFile) {
      drawingCreationStage.value = 'opening'
      await openEditor(savedFile)
    }
    else uiStore.toast('图纸已创建，请刷新列表后点击「本地编辑」', 'warn')
    isCreatingDrawing.value = false
  } catch (error) {
    const message = created ? '图纸已创建，但自动打开失败；请刷新列表后点击「本地编辑」' : error instanceof Error ? error.message : '新建图纸失败'
    if (created) isCreatingDrawing.value = false
    else drawingCreationError.value = message
    uiStore.toast(message, 'warn')
  } finally {
    creatingDrawing.value = false
  }
}
const isReidentifyingAll = ref(false)
const HISTORY_READ_STORAGE_KEY = 'cad:read-file-history:v1'
const borrowSearchMode = ref<'by-project' | 'global-part'>('by-project')
const projectSearchQuery = ref('')
const partSearchQuery = ref('')
const selectedSourceProjectNo = ref('')
const selectedSourcePartNo = ref('')
interface LocalActiveEditSession {
  sessionId: string
  fileId: string
  fileName: string
  drawingNo: string
  storageKey?: string
  uncPath: string
  openUrl: string
  startedAt: string
  lastHeartbeatAt?: number
}

// 编辑会话状态持久化：切换页签/路由导致本组件卸载重建时，“编辑中”状态栏不能丢。
const LOCAL_SESSIONS_STORAGE_KEY = 'cad:active-edit-sessions:v1'

function loadLocalSessions(): LocalActiveEditSession[] {
  try {
    const raw = window.localStorage.getItem(LOCAL_SESSIONS_STORAGE_KEY)
    const list = raw ? JSON.parse(raw) : []
    if (!Array.isArray(list)) return []
    return list.filter((item): item is LocalActiveEditSession => Boolean(item && item.sessionId && item.fileId))
  } catch {
    return []
  }
}

function persistLocalSessions() {
  try {
    window.localStorage.setItem(LOCAL_SESSIONS_STORAGE_KEY, JSON.stringify(myActiveSessions.value))
  } catch {
    // 本地存储不可用时忽略：仅影响状态栏恢复
  }
}

let sessionPollTimer: number | null = null
let heartbeatTimer: number | null = null
const editingFileId = ref<string | null>(null)
const readonlyFileId = ref<string | null>(null)
const activeSessionList = ref<ActiveEditSessionInfo[]>([])
const myActiveSessions = ref<LocalActiveEditSession[]>(loadLocalSessions())
const closingSessionIds = ref<Set<string>>(new Set())
const closedSessions = ref<{ sessionId: string; fileName: string; savedAt: string }[]>([])
const borrowReasonInput = ref('')
const caxaHelpVisible = ref(false)
const caxaHelpDetail = ref('')
const isSavingCaxaPath = ref(false)
let pendingCadRetry: (() => Promise<void>) | null = null

/** 统一处理本地 CAD 打开失败：找不到 CAXA 时弹出可操作的弹窗，其余仅提示。 */
function handleCadOpenError(error: unknown, retry: () => Promise<void>) {
  const message = error instanceof Error ? error.message : String(error)
  if (message.startsWith(CAXA_NOT_FOUND_PREFIX)) {
    caxaHelpDetail.value = message.slice(CAXA_NOT_FOUND_PREFIX.length)
    pendingCadRetry = retry
    caxaHelpVisible.value = true
    return
  }
  uiStore.toast(message, 'warn')
}

function closeCaxaHelpModal() {
  caxaHelpVisible.value = false
  pendingCadRetry = null
}

async function pickAndSaveCaxa() {
  if (isSavingCaxaPath.value) return
  isSavingCaxaPath.value = true
  try {
    const picked = await editingService.pickCaxa()
    if (!picked) return
    await editingService.saveCaxaPath(picked)
    uiStore.toast(`已记住本机 CAXA 程序：${picked}`, 'ok')
    caxaHelpVisible.value = false
    const retry = pendingCadRetry
    pendingCadRetry = null
    await retry?.()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '保存 CAXA 路径失败', 'warn')
  } finally {
    isSavingCaxaPath.value = false
  }
}

async function openSystemDefaultApps() {
  try {
    await editingService.openDefaultApps()
    uiStore.toast('已打开系统「默认应用」设置，请为图纸扩展名配置打开方式', 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '打开系统设置失败', 'warn')
  }
}

// 获取除当前项目外的所有可选项目（支持名称、图号、厂商模糊过滤）
const candidateProjects = computed(() => {
  const curNo = currentItem.value?.no || ''
  const q = projectSearchQuery.value.trim().toLowerCase()
  const list = drawingStore.drawings.filter((d) => d.no !== curNo)
  if (!q) return list
  return list.filter((d) =>
    d.no.toLowerCase().includes(q) ||
    d.name.toLowerCase().includes(q) ||
    (d.vendor && d.vendor.toLowerCase().includes(q)) ||
    (d.project && d.project.toLowerCase().includes(q))
  )
})

// 当前选中的源项目对象
const selectedProjectDetail = computed(() => {
  if (!selectedSourceProjectNo.value) return candidateProjects.value[0] || null
  return drawingStore.drawings.find((d) => d.no === selectedSourceProjectNo.value) || null
})

// 根据模式和筛选条件获取零件列表
const candidateParts = computed(() => {
  const q = partSearchQuery.value.trim().toLowerCase()

  if (borrowSearchMode.value === 'global-part') {
    // 全库全局穿透搜索（排除当前项目自身的零件）
    const curNo = currentItem.value?.no || ''
    const allOtherParts = drawingStore.parts.filter((p) => p.parentNo !== curNo && isBorrowablePart(p))
    if (!q) return allOtherParts.slice(0, 100) // 默认展示前 100 项
    return allOtherParts.filter((p) =>
      p.no.toLowerCase().includes(q) ||
      p.name.toLowerCase().includes(q) ||
      (p.material && p.material.toLowerCase().includes(q)) ||
      (p.spec && p.spec.toLowerCase().includes(q)) ||
      (p.parentNo && p.parentNo.toLowerCase().includes(q))
    )
  }

  // 按项目浏览选型
  const pNo = selectedSourceProjectNo.value || candidateProjects.value[0]?.no
  if (!pNo) return []

  const parts = drawingStore.parts.filter((p) => isBorrowablePart(p) && (p.parentNo === pNo || p.no.startsWith(`${pNo}-`)))
  if (!q) return parts

  return parts.filter((p) =>
    p.no.toLowerCase().includes(q) ||
    p.name.toLowerCase().includes(q) ||
    (p.material && p.material.toLowerCase().includes(q)) ||
    (p.spec && p.spec.toLowerCase().includes(q))
  )
})

// 计算各个项目的零件数量
function isBorrowablePart(part: PartView): boolean {
  return part.status === 'published' || part.status === 'archived'
}

function getProjectPartCount(pNo: string): number {
  return drawingStore.parts.filter((p) => isBorrowablePart(p) && (p.parentNo === pNo || p.no.startsWith(`${pNo}-`))).length
}

// 获取零件所属项目的名称
function getPartProjectName(part: PartView): string {
  const p = drawingStore.drawings.find((d) => d.no === part.parentNo)
  return p ? p.name : (part.parentNo || '未知项目')
}

const selectedPartDetail = computed(() => {
  if (!selectedSourcePartNo.value) return null
  return drawingStore.parts.find((p) => p.no === selectedSourcePartNo.value) || null
})

function openBorrowModal() {
  if (archivedProject.value) {
    uiStore.confirm('图纸已存档，无法直接借用零件', '存档图纸处于只读保护。如需把其他项目的零件挂到当前图纸，请先发起变更工单，经管理员审批后再修改。', {
      confirmText: '去发起变更工单',
      onConfirm: () => {
        const drawingId = String(route.params.drawingId ?? currentItem.value?.no ?? '')
        if (!drawingId) return
        router.push({ name: 'drawing-changes', params: { drawingId } })
      },
    })
    return
  }
  isBorrowing.value = true
  borrowSearchMode.value = 'by-project'
  projectSearchQuery.value = ''
  partSearchQuery.value = ''
  selectedSourceProjectNo.value = candidateProjects.value[0]?.no || ''
  selectedSourcePartNo.value = ''
  borrowReasonInput.value = ''
}

const isSubmittingBorrow = ref(false)
const borrowIdempotencyKey = ref('')
const borrowKeyBoundSignature = ref('')

function cancelBorrowModal() {
  if (isSubmittingBorrow.value) return
  isBorrowing.value = false
  borrowIdempotencyKey.value = ''
  borrowKeyBoundSignature.value = ''
  projectSearchQuery.value = ''
  partSearchQuery.value = ''
  selectedSourcePartNo.value = ''
}

function selectProject(projNo: string) {
  selectedSourceProjectNo.value = projNo
  selectedSourcePartNo.value = ''
}

async function confirmBorrowPart() {
  if (isSubmittingBorrow.value) return
  if (!selectedSourcePartNo.value || !currentItem.value) return
  const curNo = currentItem.value.no
  const reason = borrowReasonInput.value.trim() || '跨项目工程设计借用'
  const signature = `${curNo}::${selectedSourcePartNo.value}::${reason}`

  if (!borrowIdempotencyKey.value || borrowKeyBoundSignature.value !== signature) {
    borrowIdempotencyKey.value = `borrow-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
    borrowKeyBoundSignature.value = signature
  }

  isSubmittingBorrow.value = true
  let borrowedPartName = ''
  let borrowedPartNo = ''
  try {
    const borrowed = await drawingOperationsStore.borrowPartToProject(
      curNo,
      selectedSourcePartNo.value,
      reason,
      borrowIdempotencyKey.value,
    )
    borrowedPartName = borrowed.name
    borrowedPartNo = borrowed.no
    try {
      await drawingStore.refresh()
    } catch (refreshErr) {
      console.warn('借用后刷新图纸列表失败，但不影响借用结果:', refreshErr)
    }
    uiStore.toast(`成功借用零件「${borrowedPartName} (${borrowedPartNo})」到当前项目`, 'ok')
    borrowIdempotencyKey.value = ''
    borrowKeyBoundSignature.value = ''
    isBorrowing.value = false
    selectedSourcePartNo.value = ''
  } catch (err: unknown) {
    console.error('借用失败', err)
    const message = err instanceof Error ? err.message : '借用零件失败，请重试'
    if (archivedProject.value || /存档|归档|变更工单/.test(message)) {
      uiStore.confirm('图纸已存档，无法直接借用零件', '存档图纸处于只读保护。如需把其他项目的零件挂到当前图纸，请先发起变更工单，经管理员审批后再修改。', {
        confirmText: '去发起变更工单',
        onConfirm: () => {
          const drawingId = String(route.params.drawingId ?? currentItem.value?.no ?? '')
          if (!drawingId) return
          router.push({ name: 'drawing-changes', params: { drawingId } })
        },
      })
      return
    }
    uiStore.toast(message, 'warn')
  } finally {
    isSubmittingBorrow.value = false
  }
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatCurrentTime(): string {
  const current = new Date()
  const year = current.getFullYear()
  const month = String(current.getMonth() + 1).padStart(2, '0')
  const day = String(current.getDate()).padStart(2, '0')
  const hours = String(current.getHours()).padStart(2, '0')
  const minutes = String(current.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}`
}

function openBrowse(file: DrawingFile) {
  if (!currentItem.value) return
  router.push({
    name: 'drawing-viewer',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

function openOnlineEditor(file: DrawingFile) {
  if (!currentItem.value) return
  if (!file.storageKey) {
    uiStore.toast('该文件尚未保存物理存储，无法使用在线编辑器打开', 'warn')
    return
  }
  router.push({
    name: 'drawing-editor',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

async function refreshActiveSessions() {
  const drawingNo = currentItem.value?.no
  if (!drawingNo) {
    activeSessionList.value = []
    return
  }
  try {
    const list = await editingService.listSessions(drawingNo)
    activeSessionList.value = list

    // 同步更新 myActiveSessions：只校验当前图纸的会话（服务端已关闭则剔除）；
    // 其他图纸的会话不由本次轮询裁决，否则在项目间切换时会误删编辑状态。
    const validIds = new Set(list.filter((s) => s.isCurrent).map((s) => s.id))
    const filtered = myActiveSessions.value.filter((s) => s.drawingNo !== drawingNo || validIds.has(s.sessionId))
    if (filtered.length !== myActiveSessions.value.length) {
      myActiveSessions.value = filtered
      persistLocalSessions()
    }
  } catch {
    // 轮询静默失败
  }
}

function getFileLockInfo(file: DrawingFile): ActiveEditSessionInfo | undefined {
  // 服务端会话记录的是原始存储键（EXB 上传场景），优先用原始键匹配。
  const originalKey = file.rawStorageKey || file.storageKey
  if (!originalKey) return undefined
  return activeSessionList.value.find((s) => s.attachmentId ? s.attachmentId === file.id : s.storageKey === originalKey || s.storageKey === file.storageKey)
}

function isFileLockedByOther(file: DrawingFile): boolean {
  const lock = getFileLockInfo(file)
  return Boolean(lock && !lock.isCurrent)
}

function isFileEditingByMe(file: DrawingFile): boolean {
  if (myActiveSessions.value.some((s) => s.fileId === file.id)) return true
  const lock = getFileLockInfo(file)
  return Boolean(lock && lock.isCurrent)
}

async function copyEditLink(value: string, label: string) {
  try {
    await navigator.clipboard.writeText(value)
    uiStore.toast(`${label}已复制`, 'ok')
  } catch {
    uiStore.toast(`无法复制${label}，请手动选择文本`, 'warn')
  }
}

async function stopSession(session: ActiveEditSessionInfo | { sessionId: string }) {
  const targetId = 'id' in session ? session.id : session.sessionId
  if (!targetId || closingSessionIds.value.has(targetId)) return

  const targetFileName = ('fileName' in session && session.fileName)
    || myActiveSessions.value.find((s) => s.sessionId === targetId)?.fileName
    || '当前文件'
  uiStore.confirm(
    '结束本地编辑',
    `确定要结束「${targetFileName}」的编辑吗？\n结束后端会等待图纸落盘并自动生成新版本，通常需要数秒，请耐心等待。`,
    {
      confirmText: '结束编辑',
      danger: true,
      onConfirm: () => doStopSession(targetId, targetFileName),
    },
  )
}

async function doStopSession(targetId: string, targetFileName: string) {
  if (closingSessionIds.value.has(targetId)) return
  closingSessionIds.value.add(targetId)
  try {
    const result = await editingService.closeSession(targetId)
    myActiveSessions.value = myActiveSessions.value.filter((s) => s.sessionId !== targetId)
    persistLocalSessions()
    await refreshActiveSessions()
    // 后端已完成文件稳定等待与版本捕获；有改动时后端已生成新版本并切换当前指针，
    // 从后端重新加载业务文档，保证文件列表/版本号以服务端数据库为准。
    const successRecord = { sessionId: targetId, fileName: targetFileName, savedAt: new Date().toLocaleTimeString() }
    closedSessions.value = [...closedSessions.value, successRecord]
    window.setTimeout(() => {
      closedSessions.value = closedSessions.value.filter((item) => item.sessionId !== targetId)
    }, 5000)
    if (result.changed && result.version) {
      uiStore.toast(`编辑已结束，已保存新版本 ${result.version}`, 'ok')
    } else {
      uiStore.toast('编辑已结束，图纸无改动', 'ok')
    }
    await drawingStore.refresh()
  } catch (error) {
    // 捕获失败时后端保留编辑会话，用户可重试结束编辑，不会丢失工作内容。
    uiStore.toast(error instanceof Error ? error.message : '释放编辑会话失败', 'warn')
  } finally {
    closingSessionIds.value.delete(targetId)
  }
}

async function relaunchEditor(session: LocalActiveEditSession) {
  // 打开票据是一次性的（消费即失效，5 分钟过期），不能复用首次呼出时的旧 URL；
  // 重新呼出必须重新调 openSession 幂等认领，服务端会签发新票据且不覆盖工作文件。
  const file = allFiles.value.find((item) => item.id === session.fileId)
  const storageKey = session.storageKey || file?.rawStorageKey || file?.storageKey
  if (!storageKey) {
    uiStore.toast('本地会话缺少存储键，无法重新呼出；请改点文件行的「本地编辑」重新认领', 'warn')
    return
  }
  if (editingFileId.value) return
  editingFileId.value = session.fileId
  try {
    const result = await editingService.openSession(storageKey, session.fileId)
    session.openUrl = result.openUrl
    session.uncPath = result.uncPath
    session.lastHeartbeatAt = Date.now()
    persistLocalSessions()
    await editingService.openCad(result)
    uiStore.toast('已重新呼出本地 CAD', 'ok')
  } catch (error) {
    handleCadOpenError(error, () => relaunchEditor(session))
  } finally {
    editingFileId.value = null
  }
}

async function relaunchEditorForFile(file: DrawingFile) {
  const session = myActiveSessions.value.find((s) => s.fileId === file.id)
  if (session) {
    await relaunchEditor(session)
    return
  }
  // 页面关闭后重开的恢复场景：服务端会话仍在占用中，重新认领（幂等）并呼出 CAD。
  const storageKey = file.rawStorageKey || file.storageKey
  if (!storageKey) return
  if (editingFileId.value) return
  editingFileId.value = file.id
  try {
    const result = await editingService.openSession(storageKey, file.id)
    myActiveSessions.value = [
      ...myActiveSessions.value.filter((s) => s.sessionId !== result.sessionId),
      {
        sessionId: result.sessionId,
        fileId: file.id,
        fileName: file.name,
        drawingNo: currentItem.value?.no || '',
        uncPath: result.uncPath,
        openUrl: result.openUrl,
        startedAt: new Date().toLocaleTimeString(),
        lastHeartbeatAt: Date.now(),
      },
    ]
    persistLocalSessions()
    await editingService.openCad(result)
    await refreshActiveSessions()
    uiStore.toast('已重新认领编辑会话并呼出本地 CAD', 'ok')
  } catch (error) {
    handleCadOpenError(error, () => relaunchEditorForFile(file))
  } finally {
    editingFileId.value = null
  }
}

// 总图清单包含零件文件：按所属项目读取工单，再按附件 ID 判定授权。
const changeTargetIds = ref<Set<string>>(new Set())
const archivedProject = computed(() => drawingStore.getDrawing(rootDrawingNo.value)?.status === 'archived' || currentItem.value?.status === 'archived')
let changeAccessSequence = 0
async function refreshChangeEditAccess() {
  const sequence = ++changeAccessSequence
  const current = authStore.currentUser
  changeTargetIds.value = new Set()
  if (!archivedProject.value || !current) return
  const drawingId = drawingStore.getDrawing(rootDrawingNo.value)?.id
  if (!drawingId) return
  try {
    const list = await changeRequestService.listByDrawing(drawingId)
    const details = await Promise.all(list.filter((entry) => entry.status === 'executing' && entry.executorId === current.id)
      .map((entry) => changeRequestService.get(entry.id)))
    if (sequence === changeAccessSequence) changeTargetIds.value = editableChangeTargets(details, current.id)
  } catch {
    if (sequence === changeAccessSequence) changeTargetIds.value = new Set()
  }
}
watch([() => currentItem.value, rootDrawingNo, archivedProject, () => authStore.currentUser?.id], () => { void refreshChangeEditAccess() }, { immediate: true })

// 编辑权限矩阵（前端显隐；后端 editing.Open 同步强校验）：
// 草稿/生产 → 创建者或管理员；审核中 → 当前节点责任人或管理员；
// 存档 → 仅当当前用户持有执行中的变更工单时可编辑。
function canEditFile(file: DrawingFile): boolean {
  const item = currentItem.value
  const current = authStore.currentUser
  if (!item || !current) return false
  const admin = current.roles?.includes('admin') ?? false
  if (archivedProject.value) return changeTargetIds.value.has(file.id)
  if (item.status === 'reviewing') {
    if (admin) return true
    return reviewStore.myPendingReviews().some((reviewCase) => reviewCase.no === item.no)
  }
  const creator = (('createdBy' in item && item.createdBy) || ('by' in item ? item.by : '')) === current.displayName
  return creator || admin
}

const canDeleteFiles = computed(() => {
  const item = currentItem.value
  const current = authStore.currentUser
  if (!item || !current) return false
  const rawCreator = (item as { createdBy?: unknown }).createdBy
  const creator = typeof rawCreator === 'string' ? rawCreator : ('by' in item && typeof item.by === 'string' ? item.by : '')
  return canDeleteDrawingFiles({
    status: item.status,
    creator,
    userName: current.displayName,
    admin: current.roles?.includes('admin') ?? false,
  })
})
// 心跳新鲜度：30s 一次心跳，90s 内有成功记录视为保护生效中。
function isHeartbeatFresh(session: LocalActiveEditSession): boolean {
  return Boolean(session.lastHeartbeatAt && Date.now() - session.lastHeartbeatAt < 90_000)
}

/** 本地只读查看：临时副本在本机打开，退出即销毁，不回传服务器。 */
async function openReadonly(file: DrawingFile) {
  // 服务端附件记录与编辑会话都以原始存储键为准；合并后的当前键可能指向转换产物。
  const storageKey = file.rawStorageKey || file.storageKey
  if (!storageKey) {
    uiStore.toast('该文件尚未保存物理存储，无法本地查看', 'warn')
    return
  }
  if (readonlyFileId.value) return
  readonlyFileId.value = file.id
  try {
    await editingService.openReadonly({ storageKey })
    uiStore.toast(`已用本机 CAD 打开「${file.name}」（只读副本，关闭后自动销毁）`, 'ok')
  } catch (error) {
    handleCadOpenError(error, () => openReadonly(file))
  } finally {
    readonlyFileId.value = null
  }
}

async function openEditor(file: DrawingFile) {
  if (!currentItem.value || !canEditFile(file)) return
  if (editingFileId.value) return
  if (!file.storageKey) {
    uiStore.toast('该文件尚未保存物理存储，无法使用本地 CAD 打开', 'warn')
    return
  }

  // 检查是否已被他人锁定
  const lock = getFileLockInfo(file)
  if (lock && !lock.isCurrent) {
    uiStore.toast(`该图纸正由「${lock.userName || lock.userAccount}」编辑中，已被协同锁定`, 'warn')
    return
  }

  // 如果自己已经打开了该文件，直接重新唤醒 CAD
  const existingMySession = myActiveSessions.value.find((s) => s.fileId === file.id)
  if (existingMySession) {
    await relaunchEditor(existingMySession)
    return
  }
  // 服务端仍保留自己的会话（例如关闭页面后重新打开）：重新认领并呼出 CAD，不新建占用。
  const ownServerSession = activeSessionList.value.find((s) => s.isCurrent && (s.attachmentId ? s.attachmentId === file.id : s.storageKey === file.rawStorageKey || s.storageKey === file.storageKey))
  if (ownServerSession) {
    await relaunchEditorForFile(file)
    return
  }

  editingFileId.value = file.id
  let sessionId: string | null = null
  try {
    // 会话占用与票据都以原始存储键为准（EXB 上传场景，当前键可能指向转换产物）。
    const session = await editingService.openSession(file.rawStorageKey || file.storageKey, file.id)
    sessionId = session.sessionId
    window.localStorage.setItem('cad_last_edit_url', session.openUrl)

    const newLocalSession: LocalActiveEditSession = {
      sessionId: session.sessionId,
      fileId: file.id,
      fileName: file.name,
      drawingNo: currentItem.value?.no || '',
      storageKey: file.rawStorageKey || file.storageKey,
      uncPath: session.uncPath,
      openUrl: session.openUrl,
      startedAt: new Date().toLocaleTimeString(),
      lastHeartbeatAt: Date.now(),
    }

    myActiveSessions.value = [
      ...myActiveSessions.value.filter((s) => s.sessionId !== session.sessionId && s.fileId !== file.id),
      newLocalSession,
    ]
    persistLocalSessions()

    await editingService.openCad(session)
    await refreshActiveSessions()
    uiStore.toast(`已在本地 CAD 中打开「${file.name}」，支持多开协同编辑`, 'ok')
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    // 本机未找到 CAXA 时保留服务端会话：文件已唤醒到工作区，用户在弹窗中选择程序后可直接重试。
    if (sessionId && !message.startsWith(CAXA_NOT_FOUND_PREFIX)) {
      await editingService.closeSession(sessionId).catch(() => undefined)
    }
    handleCadOpenError(error, () => openEditor(file))
  } finally {
    editingFileId.value = null
  }
}

watch(
  () => currentItem.value?.no,
  () => {
    void refreshActiveSessions()
  },
  { immediate: true },
)

onMounted(() => {
  window.addEventListener('beforeunload', guardDrawingCreationUnload)
  void Promise.all([drawingStore.load(), reviewStore.load()])
  sessionPollTimer = window.setInterval(() => {
    void refreshActiveSessions()
  }, 10_000)

  // 统一心跳轮询：对当前正在编辑的多开图纸批量保活，并记录最近成功时间用于状态展示
  heartbeatTimer = window.setInterval(() => {
    for (const session of myActiveSessions.value) {
      void editingService.heartbeat(session.sessionId)
        .then(() => {
          session.lastHeartbeatAt = Date.now()
        })
        .catch((error) => {
          console.warn(`会话 ${session.sessionId} 心跳失败`, error)
        })
    }
  }, 30_000)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', guardDrawingCreationUnload)
  if (heartbeatTimer) {
    window.clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
  if (sessionPollTimer) {
    window.clearInterval(sessionPollTimer)
    sessionPollTimer = null
  }
})

function openHistory(file: DrawingFile) {
  if (!currentItem.value) return
  markHistoryAsRead(file.id)
  router.push({
    name: 'drawing-file-history',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

function readHistoryIds(): Set<string> {
  try {
    const raw = window.localStorage.getItem(HISTORY_READ_STORAGE_KEY)
    const ids = raw ? JSON.parse(raw) : []
    return new Set(Array.isArray(ids) ? ids.filter((id): id is string => typeof id === 'string') : [])
  } catch {
    return new Set()
  }
}

function markHistoryAsRead(fileId: string) {
  const ids = readHistoryIds()
  ids.add(fileId)
  window.localStorage.setItem(HISTORY_READ_STORAGE_KEY, JSON.stringify([...ids]))
}

function isHistoryUnread(file: DrawingFile): boolean {
  return Boolean(file.history?.length) && !readHistoryIds().has(file.id)
}

function triggerReplace(file: DrawingFile) {
  targetReplaceFile.value = file
  replaceReasonInput.value = ''
  selectedReplaceBlob.value = null
  replaceInput.value?.click()
}

function onReplaceFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file || !targetReplaceFile.value) return
  selectedReplaceBlob.value = file
  isReplacing.value = true
  target.value = ''
}

async function confirmReplace() {
  if (!targetReplaceFile.value || !selectedReplaceBlob.value) return
  const cur = targetReplaceFile.value
  const targetNo = cur.partNo || cur.drawingNo || currentItem.value?.no || ''

  try {
    const updated = await drawingOperationsStore.replaceDrawingFile(
      targetNo,
      cur.id,
      {
        name: selectedReplaceBlob.value.name,
        size: formatFileSize(selectedReplaceBlob.value.size),
        replaceReason: replaceReasonInput.value.trim() || '版本替换更新',
      },
      selectedReplaceBlob.value,
    )
    await drawingStore.refresh()
    uiStore.toast(`文件已成功替换为 ${updated.version} · 历史版本已归档留痕`, 'ok')
    isReplacing.value = false
    targetReplaceFile.value = null
    selectedReplaceBlob.value = null
  } catch (error) {
    console.error('替换文件失败', error)
    uiStore.toast('替换文件失败，请重试', 'warn')
  }
}

function cancelReplace() {
  isReplacing.value = false
  targetReplaceFile.value = null
  selectedReplaceBlob.value = null
}

async function handleDeleteFile(file: DrawingFile) {
  if (!currentItem.value) return
  uiStore.confirm(
    '删除图纸文件',
    `确定要删除图纸文件「${file.name}」吗？`,
    {
      confirmText: '删除',
      danger: true,
      onConfirm: () => doDeleteFile(file),
    },
  )
}

async function doDeleteFile(file: DrawingFile) {
  if (!currentItem.value) return
  try {
    const targetNo = file.partNo || file.drawingNo || currentItem.value.no
    if (file.role === 'other') {
      await drawingOperationsStore.deleteOtherFile(targetNo, file.id)
    } else {
      await drawingOperationsStore.deleteDrawingFile(targetNo, file.id)
    }
    await drawingStore.refresh()
    uiStore.toast(`已删除文件 ${file.name}`)
  } catch (error) {
    console.error('删除文件失败', error)
    uiStore.toast('删除文件失败，请重试', 'warn')
  }
}

// ===== 批量下载：选择文件与格式（EXB 原始 / DWG / PDF），zip 打包下载 =====
type DownloadFormat = 'exb' | 'dwg' | 'pdf'

interface DownloadCandidate {
  file: DrawingFile
  exbKey: string
  dwgKey: string
  pdfKey: string
  canConvertToPdf: boolean
}

const isDownloadOpen = ref(false)
const downloadFormat = ref<DownloadFormat>('dwg')
const downloadFileIds = ref<Set<string>>(new Set())
const isDownloading = ref(false)
const downloadProgress = ref('')

const downloadCandidates = computed<DownloadCandidate[]>(() =>
  allFiles.value.map((file) => {
    const isDirectPdf = /\.pdf$/i.test(file.name || '') || /\.pdf$/i.test(file.storageKey || '')
    const exbKey = file.rawStorageKey || (/\.exb$/i.test(file.storageKey || '') ? file.storageKey! : '')
    const dwgKey = file.currentStorageKey || (/\.dwg$/i.test(file.storageKey || '') ? file.storageKey! : '')
    const pdfKey = isDirectPdf ? (file.storageKey || file.currentStorageKey || '') : ''
    const canConvertToPdf = Boolean(dwgKey || (/\.dxf$/i.test(file.storageKey || '') ? file.storageKey : ''))

    return {
      file,
      exbKey,
      dwgKey,
      pdfKey,
      canConvertToPdf,
    }
  }),
)

const allDownloadSelected = computed(() =>
  downloadCandidates.value.length > 0
  && downloadCandidates.value.filter((item) => candidateHasFormat(item, downloadFormat.value)).every((item) => downloadFileIds.value.has(item.file.id)),
)

function candidateHasFormat(item: DownloadCandidate, format: DownloadFormat): boolean {
  if (format === 'exb') return Boolean(item.exbKey)
  if (format === 'dwg') return Boolean(item.dwgKey)
  if (format === 'pdf') return Boolean(item.pdfKey || item.canConvertToPdf)
  return false
}

function openDownloadModal() {
  downloadFormat.value = 'dwg'
  downloadFileIds.value = new Set(downloadCandidates.value.filter((item) => candidateHasFormat(item, 'dwg')).map((item) => item.file.id))
  isDownloadOpen.value = true
}

function onFormatChange(format: DownloadFormat) {
  downloadFormat.value = format
  // 切换格式时自动保留已选且当前格式可用的项，或者默认全选当前可用项
  const available = downloadCandidates.value.filter((item) => candidateHasFormat(item, format)).map((item) => item.file.id)
  const currentSelectedAvailable = available.filter((id) => downloadFileIds.value.has(id))
  downloadFileIds.value = new Set(currentSelectedAvailable.length ? currentSelectedAvailable : available)
}

function toggleDownloadFile(id: string) {
  const next = new Set(downloadFileIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  downloadFileIds.value = next
}

function toggleAllDownloadFiles() {
  if (allDownloadSelected.value) {
    downloadFileIds.value = new Set()
  } else {
    downloadFileIds.value = new Set(downloadCandidates.value.filter((item) => candidateHasFormat(item, downloadFormat.value)).map((item) => item.file.id))
  }
}

async function executeDownload() {
  const selected = downloadCandidates.value.filter((item) => downloadFileIds.value.has(item.file.id) && candidateHasFormat(item, downloadFormat.value))
  if (!selected.length) {
    uiStore.toast('请至少选择一个当前格式可用的文件', 'warn')
    return
  }
  isDownloading.value = true
  try {
    const JSZip = (await import('jszip')).default
    const zip = new JSZip()
    const usedNames = new Set<string>()
    let failed = 0
    for (const [index, item] of selected.entries()) {
      const baseName = item.file.name.replace(/\.[^/.]+$/, '')
      let fileName = `${baseName}.${downloadFormat.value}`
      let suffix = 1
      while (usedNames.has(fileName.toLowerCase())) {
        fileName = `${baseName}(${suffix++}).${downloadFormat.value}`
      }
      usedNames.add(fileName.toLowerCase())

      try {
        if (downloadFormat.value === 'pdf') {
          downloadProgress.value = `正在生成 PDF ${index + 1}/${selected.length} · ${item.file.name}`
          if (item.pdfKey) {
            // 原本就是 PDF 格式的附件
            const content = await drawingFileService.read(item.pdfKey)
            zip.file(fileName, content)
          } else {
            // CAD 图纸（DWG / DXF）：读取二进制数据并使用 CAD 查看器渲染为高清 PDF
            const cadSourceKey = item.dwgKey || item.file.storageKey || ''
            if (!cadSourceKey) throw new Error('缺少 CAD 图纸源文件')
            const cadBlob = await drawingFileService.read(cadSourceKey)
            const cadBuffer = await cadBlob.arrayBuffer()
            const pdfBlob = await convertCadToPdfBlob(cadBuffer, item.file.name)
            zip.file(fileName, pdfBlob)
          }
        } else {
          downloadProgress.value = `正在获取 ${index + 1}/${selected.length} · ${item.file.name}`
          const key = downloadFormat.value === 'exb' ? item.exbKey : item.dwgKey
          const content = await drawingFileService.read(key)
          // 直接存放在 zip 根目录下，不套外层文件夹
          zip.file(fileName, content)
        }
      } catch (itemError) {
        console.error(`获取/转换文件失败：${item.file.name}`, itemError)
        failed += 1
      }
    }
    downloadProgress.value = '正在打包 zip...'
    const uint8Array = await zip.generateAsync({ type: 'uint8array' })
    const formatNameLabel = downloadFormat.value === 'exb' ? 'EXB原始格式' : downloadFormat.value === 'dwg' ? 'DWG格式' : 'PDF格式'
    const defaultZipName = `${currentItem.value?.no || '图纸文件'}-${formatNameLabel}.zip`

    // 1. 桌面客户端模式：调起系统原生“另存为”文件选择框
    if (isTauri()) {
      const savedPath = await saveDownloadFile(defaultZipName, uint8Array)
      if (!savedPath) {
        // 用户在文件选择框中点击了取消
        return
      }
      isDownloadOpen.value = false
      const okCount = selected.length - failed
      uiStore.toast(`已成功保存至：${savedPath}（共 ${okCount} 个文件）`, 'ok')
      return
    }

    // 2. 浏览器端模式：优先调起浏览器原生另存为文件选择器
    const blob = new Blob([uint8Array.buffer as ArrayBuffer], { type: 'application/zip' })
    const showSaveFilePicker = (window as unknown as { showSaveFilePicker?: (options: unknown) => Promise<FileSystemFileHandle> }).showSaveFilePicker
    if (typeof showSaveFilePicker === 'function') {
      try {
        const handle = await showSaveFilePicker({
          suggestedName: defaultZipName,
          types: [{
            description: 'ZIP 压缩包 (*.zip)',
            accept: { 'application/zip': ['.zip'] },
          }],
        })
        const writable = await handle.createWritable()
        await writable.write(blob)
        await writable.close()
        isDownloadOpen.value = false
        const okCount = selected.length - failed
        uiStore.toast(`已成功保存所选图纸（共 ${okCount} 个文件）`, 'ok')
        return
      } catch (err: unknown) {
        if (err && typeof err === 'object' && 'name' in err && err.name === 'AbortError') {
          // 用户取消保存
          return
        }
      }
    }

    // 3. 浏览器降级：触发 a 标签下载
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = defaultZipName
    anchor.click()
    URL.revokeObjectURL(url)
    isDownloadOpen.value = false
    const okCount = selected.length - failed
    uiStore.toast(`已打包下载 ${okCount} 个文件${failed ? `，${failed} 个获取失败已跳过` : ''}`, failed ? 'warn' : 'ok')
  } catch (error) {
    console.error('批量下载失败', error)
    uiStore.toast('批量下载失败，请重试', 'warn')
  } finally {
    isDownloading.value = false
    downloadProgress.value = ''
  }
}

function triggerUploadAssembly() {
  assemblyInput.value?.click()
}

function triggerUploadPart() {
  if (!hasAssemblyFile.value && isAssembly.value) {
    uiStore.toast('请先上传总图文件，再进行零件图上传', 'warn')
    return
  }
  partInput.value?.click()
}

async function onAssemblyFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file || !currentItem.value) return

  const newFile: DrawingFile = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    name: file.name,
    size: formatFileSize(file.size),
    role: 'assembly',
    drawingNo: currentItem.value.no,
    version: currentItem.value.version || 'v1.0',
        uploadedBy: authStore.currentUser?.displayName || authStore.currentUser?.account || '未知',
    uploadedAt: formatCurrentTime(),
    previewable: true,
  }

  try {
    await drawingOperationsStore.uploadDrawingFile(currentItem.value.no, newFile, file)
    await drawingStore.refresh()
    uiStore.toast(`总图文件「${file.name}」上传成功`, 'ok')
    openBrowse(newFile)
  } catch (error) {
    console.error('上传总图失败', error)
    uiStore.toast('上传总图失败', 'warn')
  } finally {
    target.value = ''
  }
}

async function onPartFilesChange(event: Event) {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (!files || !files.length || !currentItem.value) return

  let createdCount = 0
  let otherCount = 0
  try {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      if (!file) continue
      const cleanName = file.name.replace(/\.[^/.]+$/, '')
      const rootNo = rootDrawingNo.value
      const extension = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] || ''
      const isCadPart = extension === '.exb' || extension === '.dwg' || extension === '.dxf'
      if (!isCadPart) {
        const newFile: DrawingFile = {
          id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
          name: file.name,
          size: formatFileSize(file.size),
          role: 'other',
          drawingNo: rootNo,
          version: 'v1.0',
          uploadedBy: authStore.currentUser?.displayName || authStore.currentUser?.account || '未知',
          uploadedAt: formatCurrentTime(),
          previewable: true,
        }
        await drawingOperationsStore.uploadOtherFile(rootNo, newFile, file)
        await drawingStore.refresh()
        otherCount += 1
        continue
      }

      const identity = await drawingFileService.identify(file, file.name)
      const parsed = parseDrawingNumber(identity.partNo)
      const material = identity.material || identity.titleBlock?.['材料名称'] || identity.titleBlock?.['材料'] || identity.titleBlock?.['材质'] || '—'
      const isBorrowed = parsed.rootNo !== rootNo
      const isStructuredPart = parsed.no !== rootNo
      const parentNo = isBorrowed ? rootNo : parsed.parentNo ?? rootNo
      const partNo = parsed.no

      const newFile: DrawingFile = {
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        name: file.name,
        size: formatFileSize(file.size),
        role: isStructuredPart ? 'part' : 'other',
        drawingNo: rootNo,
        ...(isStructuredPart ? { partNo } : {}),
        version: 'v1.0',
        uploadedBy: authStore.currentUser?.displayName || authStore.currentUser?.account || '未知',
        uploadedAt: formatCurrentTime(),
        previewable: true,
      }

      const parentExists = Boolean(
        drawingStore.drawings.some((drawing) => drawing.no === parentNo)
        || drawingStore.parts.some((part) => part.no === parentNo),
      )
      const existingPart = partNo
        ? drawingStore.parts.find((part) => part.no === partNo && part.parentNo === rootNo)
        : undefined

      if (!isStructuredPart || !parentExists) {
        await drawingOperationsStore.uploadOtherFile(rootNo, newFile, file)
        await drawingStore.refresh()
        otherCount += 1
      } else if (existingPart) {
        if (material !== '—') existingPart.material = material
        await drawingOperationsStore.uploadDrawingFile(partNo, newFile, file)
        await drawingStore.refresh()
        createdCount += 1
      } else {
        await drawingOperationsStore.createPartWithFile(parentNo, {
          no: partNo,
          name: cleanName,
          parentNo,
          project: '',
           material,
          spec: '',
          weight: 0,
          surfaceTreatment: '',
          partType: '自制件',
          qty: 1,
          status: 'draft',
          ver: 'v1.0',
          hasFile: true,
           files: [newFile],
            ...(isBorrowed ? { borrowFrom: parsed.rootNo ?? parsed.no } : {}),
        }, newFile, file)
        await drawingStore.refresh()
        createdCount += 1
      }
    }
    uiStore.toast(`已整理 ${createdCount} 个规范零件文件${otherCount ? `，${otherCount} 个文件归入其他文件` : ''}`, 'ok')
  } catch (error) {
    console.error('上传零件图失败', error)
    uiStore.toast('上传零件图失败', 'warn')
  } finally {
    target.value = ''
  }
}

// 批量识别校正弹窗状态
const isReidentifyModalOpen = ref(false)
const reidentifyList = ref<Array<{ file: DrawingFile; oldPartNo: string; newPartNo: string; checked: boolean }>>([])
const reidentifyFailures = ref<string[]>([])
const isExecutingReidentify = ref(false)

function isNonPartCadFile(fileName: string): boolean {
  const lower = fileName.toLowerCase()
  return (
    lower.includes('明细表') ||
    lower.includes('外购件') ||
    lower.includes('标准件') ||
    lower.includes('密封件') ||
    lower.includes('汇总表') ||
    lower.includes('目录') ||
    lower.includes('bom')
  )
}

async function reidentifyAllPartFiles() {
  if (isReidentifyingAll.value) return
  const currentRootNo = rootDrawingNo.value
  const cadFiles = allFiles.value.filter((file) => {
    const extension = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] || ''
    // 排除总图、非 CAD 以及明细表/BOM等表格文件
    return (
      file.role !== 'assembly' &&
      Boolean(file.storageKey) &&
      ['.exb', '.dwg', '.dxf'].includes(extension) &&
      !isNonPartCadFile(file.name)
    )
  })
  if (!cadFiles.length) {
    uiStore.toast('当前图纸没有可重新识别的零件 CAD 文件（已自动过滤明细表与表格）', 'warn')
    return
  }

  isReidentifyingAll.value = true
  try {
    const results: Array<{ file: DrawingFile; oldPartNo: string; newPartNo: string; checked: boolean }> = []
    const failures: string[] = []
    for (const file of cadFiles) {
      try {
        const content = await drawingFileService.read(file.storageKey as string)
        const identity = await drawingFileService.identify(content, file.name)
        const identifiedNo = identity.partNo.trim()

        // 过滤：如果识别出的图号与总图号完全相同，说明是附属文件或总图明细，跳过
        if (currentRootNo && identifiedNo === currentRootNo) {
          continue
        }

        if (identifiedNo !== (file.partNo || '')) {
          results.push({ file, oldPartNo: file.partNo || '未关联', newPartNo: identifiedNo, checked: true })
        }
      } catch (error) {
        failures.push(`${file.name}：${error instanceof Error ? error.message : String(error)}`)
      }
    }

    if (!results.length) {
      uiStore.toast(failures.length ? `没有发现图号变化，${failures.length} 个文件识别失败` : '所有零件图号均已与文件名一致', failures.length ? 'warn' : 'ok')
      return
    }

    reidentifyList.value = results
    reidentifyFailures.value = failures
    isReidentifyModalOpen.value = true
  } finally {
    isReidentifyingAll.value = false
  }
}

async function confirmBatchReidentify() {
  const selected = reidentifyList.value.filter((item) => item.checked)
  if (!selected.length) {
    uiStore.toast('请至少勾选一个要校正的文件', 'warn')
    return
  }

  isExecutingReidentify.value = true
  let updatedCount = 0
  const executeFailures: string[] = []
  try {
    for (const item of selected) {
      try {
        await drawingOperationsStore.reidentifyDrawingFile(item.file, item.newPartNo)
        await drawingStore.refresh()
        updatedCount += 1
      } catch (error) {
        executeFailures.push(`${item.file.name}：${error instanceof Error ? error.message : String(error)}`)
      }
    }
    uiStore.toast(
      `已完成 ${updatedCount}/${selected.length} 个文件的图号校正${executeFailures.length ? `，${executeFailures.length} 个失败` : ''}`,
      executeFailures.length ? 'warn' : 'ok',
    )
    isReidentifyModalOpen.value = false
  } finally {
    isExecutingReidentify.value = false
  }
}

function closeReidentifyModal() {
  if (isExecutingReidentify.value) return
  isReidentifyModalOpen.value = false
}
</script>

<template>
  <div class="drawing-preview-view">
    <!-- 隐藏式文件选择框 -->
    <input
      ref="assemblyInput"
      type="file"
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onAssemblyFileChange"
    />
    <input
      ref="partInput"
      type="file"
      multiple
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onPartFilesChange"
    />
    <input
      ref="replaceInput"
      type="file"
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onReplaceFileSelected"
    />

    <!-- 顶部操作栏：规范的上传总图 / 零件图入口 -->
    <div class="preview-actions-header card card-pad">
      <div class="header-info">
        <DemoIcon name="layers" :size="18" />
        <div>
          <h3>图纸文件管理与在线浏览</h3>
          <p>先上传总图建立主框架，后上传关联零件图；点击任意文件「浏览」开启矢量画布控制。</p>
        </div>
      </div>

      <div class="header-buttons">
        <button v-if="canCreateDrawing && isAssembly" class="btn primary" type="button" @click="openCreateDrawing">
          <DemoIcon name="plus" :size="14" />新建图纸
        </button>
        <button class="btn" type="button" @click="router.push({ name: 'drawing-models', params: { drawingId: route.params.drawingId } })"><DemoIcon name="box" :size="14" />3D 图纸</button>
        <button class="btn" type="button" @click="router.push({ name: 'drawing-compare', params: { drawingId: route.params.drawingId } })">图纸对比</button>
        <button class="btn" type="button" title="选择文件与格式（EXB / DWG / PDF），打包为 zip 下载" @click="openDownloadModal">
          <DemoIcon name="download" :size="14" />下载
        </button>
        <button class="btn" type="button" title="从其他工程项目借用零件图及关联文件" @click="openBorrowModal">
          <DemoIcon name="share-2" :size="14" />借用零件
        </button>
        <button class="btn primary" type="button" @click="triggerUploadAssembly">
          <DemoIcon name="upload" :size="14" />上传总图文件
        </button>
        <button
          class="btn"
          :class="{ primary: hasAssemblyFile }"
          type="button"
          :disabled="!hasAssemblyFile && isAssembly"
          :title="!hasAssemblyFile && isAssembly ? '请先上传总图' : '上传零件图'"
          @click="triggerUploadPart"
        >
          <DemoIcon name="files" :size="14" />上传零件图
        </button>
      </div>
    </div>

    <!-- 本地 CAD 协同状态面板（支持多开协同编辑，置于表格上方） -->
    <div v-if="myActiveSessions.length > 0" class="collab-multi-container">
      <div v-for="session in myActiveSessions" :key="session.sessionId" class="card collab-dock-card">
        <div class="dock-left">
          <div class="dock-status-tag">
            <span class="pulse-dot"></span>
            <strong>本地协同编辑中</strong>
          </div>
          <div class="dock-file-info">
            <span class="file-name" :title="session.fileName">{{ session.fileName }}</span>
            <span class="dock-time">
              开始于 {{ session.startedAt }} ·
              <b :class="isHeartbeatFresh(session) ? 'hb-ok' : 'hb-lost'">{{ isHeartbeatFresh(session) ? '心跳正常' : '心跳检测中' }}</b>
              · {{ isHeartbeatFresh(session) ? '自动落盘与版本保护生效中' : '等待心跳确认...' }}
            </span>
          </div>
        </div>
        <div class="dock-actions">
          <button class="btn sm" type="button" title="在外部 CAD 或资源管理器中打开此共享路径" @click="copyEditLink(session.uncPath, '网络工作路径')">
            <DemoIcon name="copy" :size="13" />复制路径
          </button>
          <button class="btn sm" type="button" title="重新唤醒本地 CAXA CAD 程序" @click="relaunchEditor(session)">
            <DemoIcon name="external-link" :size="13" />呼出 CAXA
          </button>
          <button
            class="btn sm primary danger-tone"
            type="button"
            :disabled="closingSessionIds.has(session.sessionId)"
            title="结束当前编辑：等待图纸落盘后生成新版本并释放文件锁"
            @click="stopSession(session)"
          >
            <span v-if="closingSessionIds.has(session.sessionId)" class="local-edit-spinner" aria-hidden="true"></span>
            <DemoIcon v-else name="square" :size="12" />
            {{ closingSessionIds.has(session.sessionId) ? '正在结束，等待图纸落盘...' : '结束编辑' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 结束成功反馈：后端完成版本捕获后短暂展示 -->
    <div v-if="closedSessions.length > 0" class="collab-multi-container">
      <div v-for="closed in closedSessions" :key="closed.sessionId" class="card collab-dock-card closed-ok">
        <div class="dock-left">
          <div class="dock-status-tag success">
            <DemoIcon name="check-circle-2" :size="15" />
            <strong>结束成功</strong>
          </div>
          <div class="dock-file-info">
            <span class="file-name" :title="closed.fileName">{{ closed.fileName }}</span>
            <span class="dock-time">{{ closed.savedAt }} · 图纸已落盘并生成新版本</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 文件总览列表 -->
    <div class="card files-table-card">
      <div class="card-title files-title-row">
        <DemoIcon name="file-text" :size="16" />
        已关联图纸文件清单 ({{ allFiles.length }})
        <span class="hint">支持 DWG / DXF / EXB / PDF / STEP</span>
        <button class="btn sm" type="button" :disabled="isReidentifyingAll" title="按全部零件 CAD 文件名批量校正图号" @click="reidentifyAllPartFiles">
          <DemoIcon name="scan" :size="13" />
          {{ isReidentifyingAll ? '识别中...' : '全部重新识别图号' }}
        </button>
      </div>

      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>文件类型</th>
              <th>文件名</th>
               <th>版本</th>
               <th>上传人</th>
               <th style="width: 560px; text-align: right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="file in allFiles" :key="file.id" :class="{ 'row-editing': isFileEditingByMe(file), 'row-locked': isFileLockedByOther(file) }">
              <td>
                <span class="tag" :class="file.role === 'assembly' ? 'plain' : file.role === 'other' ? 'mute' : 'info'">
                  {{ file.role === 'assembly' ? '项目总图' : file.role === 'other' ? '其他文件' : '零件图' }}
                </span>
              </td>
              <td class="file-name-cell">
                <DemoIcon name="file-check-2" :size="16" />
                <div class="file-title-wrap">
                  <div class="file-title-text">
                    <b>{{ file.name }}</b>
                    <span class="file-owner">{{ file.role === 'assembly' ? `项目总图 · ${file.ownerNo}` : `所属零件 · ${file.ownerNo}${file.ownerName && file.ownerName !== file.ownerNo ? `（${file.ownerName}）` : ''}` }}</span>
                  </div>
                  <span v-if="isFileEditingByMe(file)" class="badge-collab active">
                    <span class="pulse-dot"></span>我正在编辑
                  </span>
                  <span v-else-if="getFileLockInfo(file)" class="badge-collab locked" :title="`由 ${getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount} 占用${getFileLockInfo(file)?.online === false ? '（离线）' : ''}`">
                    <DemoIcon name="lock" :size="11" />{{ getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount }} {{ getFileLockInfo(file)?.online === false ? '离线占用' : '编辑中' }}
                  </span>
                </div>
              </td>
              <td class="num"><span class="ver-badge" :title="file.version">{{ versionDisplayLabel(file.version) }}</span></td>
              <td>{{ file.uploadedBy }}</td>
              <td class="row-actions" style="text-align: right">
                <button class="btn sm primary" type="button" title="在线 CAD 矢量浏览" @click="openBrowse(file)">
                  <DemoIcon name="eye" :size="13" />浏览
                </button>
                <button
                  class="btn sm"
                  type="button"
                  :disabled="Boolean(readonlyFileId)"
                  title="下载临时只读副本到本机用 CAD 打开，关闭后自动销毁，不影响服务器数据"
                  @click="openReadonly(file)"
                >
                  <span v-if="readonlyFileId === file.id" class="local-edit-spinner" aria-hidden="true"></span>
                  <DemoIcon v-else name="eye" :size="13" />本地查看
                </button>
                <button
                  v-if="false && canEditFile(file)"
                  class="btn sm"
                  type="button"
                  title="使用网页 CAD 编辑器打开并编辑文件"
                  @click="openOnlineEditor(file)"
                >
                  <img class="editor-icon" src="/编辑.svg" alt="" aria-hidden="true" />在线编辑
                </button>

                <!-- 本地 CAD 编辑按钮三种状态：编辑中（绿色高亮）、被他人锁定（禁用锁止）、正常空闲；无权限直接隐藏 -->
                <button
                  v-if="isFileEditingByMe(file)"
                  class="btn sm success-btn"
                  type="button"
                  title="当前已在本地 CAD 中打开，点击呼出/重新聚焦"
                  @click="relaunchEditorForFile(file)"
                >
                  <span class="pulse-dot"></span>编辑中
                </button>
                <template v-else-if="canEditFile(file) && !isFileLockedByOther(file)">
                  <button
                    class="btn sm"
                    type="button"
                    :disabled="Boolean(editingFileId)"
                    title="使用本机 CAD 软件（如 CAXA）打开并协同编辑"
                    @click="openEditor(file)"
                  >
                    <span v-if="editingFileId === file.id" class="local-edit-spinner" aria-hidden="true"></span>
                    <img v-else class="editor-icon" src="/编辑.svg" alt="" aria-hidden="true" />{{ editingFileId === file.id ? '启动中...' : '本地编辑' }}
                  </button>
                </template>
                <button
                  v-else-if="isFileLockedByOther(file)"
                  class="btn sm locked-btn"
                  type="button"
                  :disabled="!getFileLockInfo(file)?.canClose || closingSessionIds.has(getFileLockInfo(file)!.id)"
                  :title="getFileLockInfo(file)?.canClose
                    ? `文件正由「${getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount}」占用，可强制释放（将尝试保存其改动并生成版本）`
                    : `文件正由「${getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount}」独占编辑中`"
                  @click="getFileLockInfo(file) && stopSession(getFileLockInfo(file)!)"
                >
                  <span v-if="getFileLockInfo(file)?.canClose && closingSessionIds.has(getFileLockInfo(file)!.id)" class="local-edit-spinner" aria-hidden="true"></span>
                  <DemoIcon v-else name="lock" :size="12" />
                  {{ getFileLockInfo(file)?.canClose ? (getFileLockInfo(file)?.online === false ? '强制释放(离线)' : '强制释放') : (getFileLockInfo(file)?.online === false ? '已被占用(离线)' : '已被占用') }}
                </button>

                <button
                  v-if="canEditFile(file)"
                  class="btn sm"
                  type="button"
                  title="替换当前图纸文件并生成新版本"
                  @click="triggerReplace(file)"
                >
                  <DemoIcon name="refresh-cw" :size="13" />替换
                </button>
                <button class="btn sm history-action" type="button" title="查看该文件所有历史版本树与演进" @click="openHistory(file)">
                  <DemoIcon name="history" :size="13" />历史
                  <span v-if="isHistoryUnread(file)" class="hist-count">{{ file.history?.length }}</span>
                </button>
                <button
                  v-if="canDeleteFiles"
                  class="btn sm danger"
                  type="button"
                  title="删除文件"
                  @click="handleDeleteFile(file)"
                >
                  <DemoIcon name="trash-2" :size="13" />删除
                </button>
              </td>
            </tr>
            <tr v-if="!allFiles.length">
              <td colspan="6">
                <div class="empty">
                  <DemoIcon name="file-up" :size="36" />
                  <div class="t">尚未上传任何图纸文件</div>
                  <p>请点击上方「上传总图文件」开始建立工程档案</p>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 批量重新识别并校正图号弹窗 -->
    <div v-if="isReidentifyModalOpen" class="modal-backdrop">
      <div class="modal card reidentify-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="scan" :size="18" />
            <span>批量校正零件图号</span>
          </div>
          <button class="btn sm close-btn" type="button" :disabled="isExecutingReidentify" @click="closeReidentifyModal">✕</button>
        </div>

        <div class="modal-body reidentify-modal-body">
          <div class="reidentify-hint">
            <DemoIcon name="info" :size="14" />
            <span>系统已按图纸文件名识别图号，并已自动过滤明细表和非零件图文件。请核对并勾选需校正的项：</span>
          </div>

          <div class="reidentify-table-wrap">
            <table class="tbl compact-tbl">
              <thead>
                <tr>
                  <th style="width: 40px; text-align: center">
                    <input
                      type="checkbox"
                      :checked="reidentifyList.length > 0 && reidentifyList.every((i) => i.checked)"
                      @change="reidentifyList.forEach((i) => (i.checked = ($event.target as HTMLInputElement).checked))"
                    />
                  </th>
                  <th>文件名</th>
                  <th>当前关联图号</th>
                  <th>识别图号 (文件名)</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in reidentifyList" :key="item.file.id">
                  <td style="text-align: center">
                    <input v-model="item.checked" type="checkbox" />
                  </td>
                  <td class="file-name-cell">
                    <DemoIcon name="file" :size="14" />
                    <span>{{ item.file.name }}</span>
                  </td>
                  <td class="num mono text-muted">{{ item.oldPartNo }}</td>
                  <td class="num mono bold text-accent">
                    {{ item.newPartNo }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="reidentifyFailures.length" class="reidentify-fail-box">
            <div class="fail-title">
              <DemoIcon name="alert-triangle" :size="13" />
              <span>以下 {{ reidentifyFailures.length }} 个文件未能从文件名识别出规范图号（已忽略）：</span>
            </div>
            <ul>
              <li v-for="(msg, idx) in reidentifyFailures" :key="idx">{{ msg }}</li>
            </ul>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" :disabled="isExecutingReidentify" @click="closeReidentifyModal">取消</button>
          <button class="btn primary" type="button" :disabled="isExecutingReidentify" @click="confirmBatchReidentify">
            <DemoIcon name="check" :size="14" />
            {{ isExecutingReidentify ? '校正中...' : `确认校正 (${reidentifyList.filter((i) => i.checked).length} 项)` }}
          </button>
        </div>
      </div>
    </div>

    <!-- 本机未找到 CAXA：提供可操作的解决途径，而不是一闪而过的提示 -->
    <div v-if="caxaHelpVisible" class="modal-backdrop">
      <div class="modal card caxa-help-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="alert-triangle" :size="18" />
            <span>未找到本机 CAD 程序</span>
          </div>
          <button class="btn sm close-btn" type="button" @click="closeCaxaHelpModal">✕</button>
        </div>

        <div class="modal-body">
          <p class="caxa-help-detail">{{ caxaHelpDetail }}</p>
          <p class="caxa-help-text">请选择一种处理方式；指定一次后系统会自动记住，之后无需重复选择：</p>
          <div class="caxa-help-actions">
            <button class="btn primary" type="button" :disabled="isSavingCaxaPath" @click="pickAndSaveCaxa">
              <DemoIcon name="folder-open" :size="14" />
              {{ isSavingCaxaPath ? '处理中...' : '选择 CAXA 程序（CDRAFT_M.exe）' }}
            </button>
            <button class="btn" type="button" @click="openSystemDefaultApps">
              <DemoIcon name="settings" :size="14" />打开系统「默认应用」设置
            </button>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" @click="closeCaxaHelpModal">稍后处理</button>
          <button class="btn primary" type="button" :disabled="isSavingCaxaPath" @click="pickAndSaveCaxa">选择程序并重试</button>
        </div>
      </div>
    </div>

    <!-- 替换图纸确认与说明弹窗 -->
    <div v-if="isReplacing" class="modal-backdrop">
      <div class="modal card replace-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="refresh-cw" :size="18" />
            <span>替换图纸文件并生成新版本</span>
          </div>
          <button class="btn sm close-btn" type="button" @click="cancelReplace">✕</button>
        </div>

        <div class="modal-body">
          <div class="replace-meta-box">
            <div class="meta-row">
              <span class="lbl">原文件：</span>
              <span class="val mono bold">{{ targetReplaceFile?.name }}</span>
              <span class="tag info">{{ targetReplaceFile ? versionDisplayLabel(targetReplaceFile.version) : 'v1.0' }}</span>
            </div>
            <div class="meta-row">
              <span class="lbl">新文件：</span>
              <span class="val mono bold text-accent">{{ selectedReplaceBlob?.name }}</span>
              <span class="tag ok">({{ formatFileSize(selectedReplaceBlob?.size || 0) }})</span>
            </div>
          </div>

          <div class="field">
            <label class="bold">版本更新说明 / 替换原因</label>
            <input
              v-model="replaceReasonInput"
              type="text"
              class="inp"
              placeholder="例如：修改活塞密封槽倒角与公差，重新出图"
            />
          </div>

          <div class="note info-note">
            <DemoIcon name="shield-check" :size="15" />
            <div>替换将保留原文件所有图纸历史树与下载凭据，版本号自动递进，全程留痕。</div>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" @click="cancelReplace">取消</button>
          <button class="btn primary" type="button" @click="confirmReplace">
            <DemoIcon name="check" :size="14" />确认替换升级
          </button>
        </div>
      </div>
    </div>
    <!-- 批量下载弹窗：选择文件与格式（EXB 原始 / DWG / PDF），zip 打包 -->
    <div v-if="isDownloadOpen" class="modal-backdrop">
      <div class="modal card download-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="download" :size="18" />
            <span>批量下载图纸文件</span>
          </div>
          <button class="btn sm close-btn" type="button" @click="isDownloadOpen = false">✕</button>
        </div>

        <div class="modal-body">
          <div class="download-format-row">
            <span class="lbl bold">下载格式：</span>
            <label class="mode-option" :class="{ active: downloadFormat === 'exb' }">
              <input type="radio" :checked="downloadFormat === 'exb'" @change="onFormatChange('exb')" />
              <span>EXB 原始格式</span>
            </label>
            <label class="mode-option" :class="{ active: downloadFormat === 'dwg' }">
              <input type="radio" :checked="downloadFormat === 'dwg'" @change="onFormatChange('dwg')" />
              <span>DWG 格式</span>
            </label>
            <label class="mode-option" :class="{ active: downloadFormat === 'pdf' }">
              <input type="radio" :checked="downloadFormat === 'pdf'" @change="onFormatChange('pdf')" />
              <span>PDF 格式 (矢量)</span>
            </label>
          </div>

          <div class="download-list-head">
            <label class="download-check-all">
              <input type="checkbox" :checked="allDownloadSelected" @change="toggleAllDownloadFiles" />
              <b>全选</b>
            </label>
            <span class="hint">已选 {{ downloadFileIds.size }} / {{ downloadCandidates.length }} 个文件 · {{ downloadFormat === 'pdf' ? '支持 CAD 转矢量 PDF 及 PDF 附件' : '按所选格式下载' }}</span>
          </div>

          <div class="download-file-list">
            <label
              v-for="item in downloadCandidates"
              :key="item.file.id"
              class="download-file-row"
              :class="{ unavailable: !candidateHasFormat(item, downloadFormat) }"
            >
              <input
                type="checkbox"
                :checked="downloadFileIds.has(item.file.id)"
                :disabled="!candidateHasFormat(item, downloadFormat)"
                @change="toggleDownloadFile(item.file.id)"
              />
              <span class="file-name mono" :title="item.file.name">{{ item.file.name }}</span>
              <span class="dl-size">{{ item.file.size }}</span>
              <span class="tag" :class="candidateHasFormat(item, downloadFormat) ? 'ok' : 'mute'">
                {{ candidateHasFormat(item, downloadFormat) ? (downloadFormat === 'exb' ? 'EXB' : downloadFormat === 'dwg' ? 'DWG' : 'PDF') : '无此格式' }}
              </span>
            </label>
            <div v-if="!downloadCandidates.length" class="empty compact-empty">
              <DemoIcon name="file" :size="28" />
              <div class="t">当前图纸暂无可下载的文件</div>
            </div>
          </div>

          <div class="note info-note">
            <DemoIcon name="info" :size="15" />
            <div>所选文件将打包为一个 zip 压缩包；没有对应格式文件的行会被跳过并标注。</div>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" @click="isDownloadOpen = false">取消</button>
          <button class="btn primary" type="button" :disabled="isDownloading || downloadFileIds.size === 0" @click="executeDownload">
            <span v-if="isDownloading" class="local-edit-spinner" aria-hidden="true"></span>
            <DemoIcon v-else name="download" :size="14" />
            {{ isDownloading ? (downloadProgress || '正在打包...') : `打包下载 (${downloadFileIds.size})` }}
          </button>
        </div>
      </div>
    </div>
    <Teleport to="body">
    <div v-if="isCreatingDrawing" class="modal-backdrop creation-backdrop">
      <div v-if="creatingDrawing" class="modal card drawing-creating-modal" role="alertdialog" aria-modal="true" aria-labelledby="drawing-creating-title" aria-describedby="drawing-creating-detail">
        <div class="drawing-creating-spinner" aria-hidden="true"><span></span></div>
        <div class="drawing-creating-copy">
          <span class="drawing-creating-step">步骤 {{ drawingCreationProgress.step }} / 4</span>
          <h3 id="drawing-creating-title">{{ drawingCreationProgress.title }}</h3>
          <p id="drawing-creating-detail">{{ drawingCreationProgress.detail }}</p>
          <strong>{{ newDrawingNo }} · {{ newDrawingName }}</strong>
        </div>
        <div class="drawing-creating-track" aria-hidden="true"><i :style="{ width: `${drawingCreationProgress.step * 25}%` }"></i></div>
        <p class="drawing-creating-warning"><DemoIcon name="info" :size="15" />创建期间请勿刷新、返回或重复操作</p>
      </div>
      <form v-else class="modal card new-drawing-modal" role="dialog" aria-modal="true" aria-labelledby="new-drawing-title" @submit.prevent="createDrawing">
        <div class="modal-head"><h3 id="new-drawing-title">新建图纸</h3></div>
        <p>使用 CAXA 空白模板创建草稿零件，并在本机 CAXA 中绘制。保存后点击「结束编辑」回写版本。</p>
        <div v-if="drawingCreationError" class="drawing-creation-error" role="alert"><DemoIcon name="alert-triangle" :size="16" />{{ drawingCreationError }}</div>
        <label>名称<input v-model="newDrawingName" class="input" required maxlength="200" autofocus /></label>
        <label>图号<input v-model="newDrawingNo" class="input" required maxlength="200" /></label>
        <div class="modal-foot">
          <button class="btn" type="button" @click="closeCreateDrawing">取消</button>
          <button class="btn primary" type="submit">创建并本地编辑</button>
        </div>
      </form>
    </div>
    </Teleport>
    <!-- 借用其他项目零件弹窗 (支持千级项目/海量零件双模智能选型体系) -->
    <div v-if="isBorrowing" class="modal-backdrop">
      <div class="modal card borrow-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="share-2" :size="18" />
            <span>跨工程项目借用零件与图纸</span>
          </div>
          <div class="modal-mode-tabs">
            <button
              class="mode-tab-btn"
              :class="{ active: borrowSearchMode === 'by-project' }"
              type="button"
              @click="borrowSearchMode = 'by-project'"
            >
              <DemoIcon name="folder" :size="13" />按工程项目选型
            </button>
            <button
              class="mode-tab-btn"
              :class="{ active: borrowSearchMode === 'global-part' }"
              type="button"
              @click="borrowSearchMode = 'global-part'"
            >
              <DemoIcon name="search" :size="13" />全库全局穿透搜索
            </button>
          </div>
          <button class="btn sm close-btn" type="button" @click="cancelBorrowModal">✕</button>
        </div>

        <div class="modal-body borrow-modal-body">
          <!-- 模式 1：按工程项目三栏分级导航与选型（专为几千个项目设计） -->
          <div v-if="borrowSearchMode === 'by-project'" class="borrow-three-grid">
            <!-- 栏 1：项目库快速检索与选择 -->
            <div class="borrow-panel-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="folder" :size="13" />工程项目库 ({{ candidateProjects.length }})</span>
              </div>
              <div class="search-input-wrap compact">
                <DemoIcon name="search" :size="13" />
                <input
                  v-model="projectSearchQuery"
                  type="text"
                  class="inp filter-inp"
                  placeholder="搜索项目名称/图号/厂商..."
                />
              </div>
              <div class="scroll-select-list">
                <div
                  v-for="proj in candidateProjects"
                  :key="proj.no"
                  class="project-item-card"
                  :class="{ active: (selectedSourceProjectNo || candidateProjects[0]?.no) === proj.no }"
                  @click="selectProject(proj.no)"
                >
                  <div class="proj-card-title">{{ proj.name }}</div>
                  <div class="proj-card-meta">
                    <span class="mono">{{ proj.no }}</span>
                    <span class="tag tag-no-dot plain tag-xs">{{ getProjectPartCount(proj.no) }} 个零件</span>
                  </div>
                </div>
                <div v-if="!candidateProjects.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="20" />
                  <span>未找到匹配的项目</span>
                </div>
              </div>
            </div>

            <!-- 栏 2：当前项目下的零件列表 -->
            <div class="borrow-panel-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="file" :size="13" />可选零件清单 ({{ candidateParts.length }})</span>
                <span class="col-badge mono">{{ selectedProjectDetail?.no }}</span>
              </div>
              <div class="search-input-wrap compact">
                <DemoIcon name="filter" :size="13" />
                <input
                  v-model="partSearchQuery"
                  type="text"
                  class="inp filter-inp"
                  placeholder="过滤零件图号/名称/材质..."
                />
              </div>
              <div class="scroll-select-list">
                <div
                  v-for="part in candidateParts"
                  :key="part.no"
                  class="part-candidate-card"
                  :class="{ active: selectedSourcePartNo === part.no }"
                  @click="selectedSourcePartNo = part.no"
                >
                  <div class="part-card-head">
                    <span class="part-name">{{ part.name }}</span>
                    <span class="tag tag-no-dot mono tag-xs">{{ part.no }}</span>
                  </div>
                  <div class="part-card-sub">
                    <span>材质: {{ part.material || '—' }}</span>
                    <span>数量: {{ part.qty || 1 }}</span>
                    <span class="has-file-badge" :class="{ ok: part.files?.length }">
                      {{ part.files?.length ? `${part.files.length} 份图纸` : '无图纸' }}
                    </span>
                  </div>
                </div>
                <div v-if="!candidateParts.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="20" />
                  <span>该项目暂无匹配零件</span>
                </div>
              </div>
            </div>

            <!-- 栏 3：选中零件档案核对与借用理由 -->
            <div class="borrow-panel-col borrow-detail-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
              </div>

              <div v-if="selectedPartDetail" class="borrow-card-detail-content">
                <div class="preview-hero-card">
                  <DemoIcon name="file-check-2" :size="22" />
                  <div>
                    <h4>{{ selectedPartDetail.name }}</h4>
                    <span class="mono text-accent">{{ selectedPartDetail.no }}</span>
                  </div>
                </div>

                <div class="preview-spec-grid">
                  <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ selectedProjectDetail?.name }}</span></div>
                  <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPartDetail.partType }}</span></div>
                  <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPartDetail.material }} {{ selectedPartDetail.spec }}</span></div>
                  <div class="spec-row"><span class="k">单重/数量：</span><span class="v">{{ selectedPartDetail.weight ? `${selectedPartDetail.weight} kg` : '—' }} / {{ selectedPartDetail.qty }} 件</span></div>
                  <div class="spec-row"><span class="k">表面处理：</span><span class="v">{{ selectedPartDetail.surfaceTreatment || '—' }}</span></div>
                  <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPartDetail.files?.[0]?.name || '（无独立图纸文件）' }}</span></div>
                </div>

                <div class="field" style="margin-top: auto;">
                  <label class="bold">借用备注说明 / 选型原因</label>
                  <input
                    v-model="borrowReasonInput"
                    type="text"
                    class="inp"
                    placeholder="例如：复用成熟导向套设计，缩短加工周期"
                  />
                </div>
              </div>

              <div v-else class="empty-preview-prompt">
                <DemoIcon name="mouse-pointer-click" :size="32" />
                <p>请点击中间列表选择需要借用的零件</p>
              </div>
            </div>
          </div>

          <!-- 模式 2：全库全局穿透搜索（直击数万图纸） -->
          <div v-else class="borrow-two-grid">
            <div class="borrow-panel-col">
              <div class="search-input-wrap">
                <DemoIcon name="search" :size="15" />
                <input
                  v-model="partSearchQuery"
                  type="text"
                  class="inp global-search-inp"
                  placeholder="在企业全库中穿透搜索：输入图号如 04、活塞、HT200、或项目名称..."
                  autofocus
                />
              </div>

              <div class="scroll-select-list global-list" style="margin-top: 10px;">
                <div
                  v-for="part in candidateParts"
                  :key="part.no"
                  class="global-part-card"
                  :class="{ active: selectedSourcePartNo === part.no }"
                  @click="selectedSourcePartNo = part.no; selectedSourceProjectNo = part.parentNo"
                >
                  <div class="gp-top">
                    <span class="gp-name">{{ part.name }}</span>
                    <span class="mono gp-no">{{ part.no }}</span>
                    <span class="tag info tag-xs">{{ getPartProjectName(part) }}</span>
                  </div>
                  <div class="gp-btm">
                    <span>材质: {{ part.material || '—' }}</span>
                    <span>数量: {{ part.qty || 1 }}</span>
                    <span>图纸: {{ part.files?.[0]?.name || '无文件' }}</span>
                  </div>
                </div>
                <div v-if="!candidateParts.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="24" />
                  <span>全库中未找到匹配的零件，请尝试更简短的关键词</span>
                </div>
              </div>
            </div>

            <!-- 右侧详情 -->
            <div class="borrow-panel-col borrow-detail-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
              </div>

              <div v-if="selectedPartDetail" class="borrow-card-detail-content">
                <div class="preview-hero-card">
                  <DemoIcon name="file-check-2" :size="22" />
                  <div>
                    <h4>{{ selectedPartDetail.name }}</h4>
                    <span class="mono text-accent">{{ selectedPartDetail.no }}</span>
                  </div>
                </div>

                <div class="preview-spec-grid">
                  <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ getPartProjectName(selectedPartDetail) }}</span></div>
                  <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPartDetail.partType }}</span></div>
                  <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPartDetail.material }} {{ selectedPartDetail.spec }}</span></div>
                  <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPartDetail.files?.[0]?.name || '（无文件）' }}</span></div>
                </div>

                <div class="field" style="margin-top: auto;">
                  <label class="bold">借用备注说明</label>
                  <input
                    v-model="borrowReasonInput"
                    type="text"
                    class="inp"
                    placeholder="输入借用说明..."
                  />
                </div>
              </div>

              <div v-else class="empty-preview-prompt">
                <DemoIcon name="mouse-pointer-click" :size="32" />
                <p>请在搜索结果中点击选定要借用的零件</p>
              </div>
            </div>
          </div>

          <div class="note info-note" style="margin-top: 12px;">
            <DemoIcon name="shield-check" :size="14" />
            <div>系统将自动克隆图纸零件并挂载至当前工程，在借用记录台账中建立双向可追溯凭据，不污染源工程。</div>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" :disabled="isSubmittingBorrow" @click="cancelBorrowModal">取消</button>
          <button
            class="btn primary"
            type="button"
            :disabled="!selectedSourcePartNo || isSubmittingBorrow"
            @click="confirmBorrowPart"
          >
            <DemoIcon name="check" :size="14" />{{ isSubmittingBorrow ? '借入中...' : '确认借入此零件' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.new-drawing-modal { width: min(460px, calc(100vw - 32px)); padding: 24px; display: grid; gap: 18px; }
.new-drawing-modal p { color: var(--text-2); line-height: 1.6; margin: 0; }
.new-drawing-modal label { display: grid; gap: 8px; }
.new-drawing-modal .modal-foot { display: flex; justify-content: flex-end; gap: 10px; }
.creation-backdrop { z-index: 2900; cursor: wait; }
.new-drawing-modal { cursor: default; }
.drawing-creating-modal { width: min(480px, calc(100vw - 32px)); padding: 30px; display: grid; justify-items: center; gap: 16px; text-align: center; cursor: wait; }
.drawing-creating-spinner { width: 54px; height: 54px; padding: 5px; border-radius: 50%; background: conic-gradient(var(--accent), transparent 65%); animation: drawing-creating-spin .85s linear infinite; }
.drawing-creating-spinner span { display: block; width: 100%; height: 100%; border-radius: 50%; background: var(--panel); }
.drawing-creating-copy { display: grid; justify-items: center; gap: 7px; }
.drawing-creating-copy h3, .drawing-creating-copy p { margin: 0; }
.drawing-creating-copy p { color: var(--text-2); line-height: 1.6; }
.drawing-creating-copy strong { color: var(--text-1); font-family: 'JetBrains Mono', monospace; overflow-wrap: anywhere; }
.drawing-creating-step { color: var(--accent); font-size: 12px; font-weight: 700; }
.drawing-creating-track { width: 100%; height: 5px; overflow: hidden; border-radius: 999px; background: var(--panel-2); }
.drawing-creating-track i { display: block; height: 100%; border-radius: inherit; background: var(--accent); transition: width .35s ease; }
.drawing-creating-warning, .drawing-creation-error { display: flex; align-items: center; gap: 8px; }
.drawing-creating-warning { color: var(--text-2); font-size: 12px; margin: 0; }
.drawing-creation-error { padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--danger) 45%, transparent); border-radius: 6px; color: var(--danger); background: color-mix(in srgb, var(--danger) 8%, transparent); }
@keyframes drawing-creating-spin { to { transform: rotate(360deg); } }
.ver-badge {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
  font-weight: 700;
  color: var(--accent);
  background: var(--panel-2);
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid var(--line);
}

.hist-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10.5px;
  font-weight: 700;
  min-width: 16px;
  height: 16px;
  border-radius: 8px;
  background: var(--accent);
  color: #fff;
  padding: 0 4px;
  margin-left: 2px;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.replace-modal {
  width: 520px;
  max-width: 90vw;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 16px 36px rgba(0, 0, 0, 0.4);
  display: flex;
  flex-direction: column;
}

.reidentify-modal {
  width: 680px;
  max-width: 92vw;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.45);
  display: flex;
  flex-direction: column;
  max-height: 85vh;
}

.reidentify-modal-body {
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  min-height: 0;
}

.reidentify-hint {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12.5px;
  color: var(--text-2);
  line-height: 1.5;
  background: var(--panel-2);
  padding: 8px 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.reidentify-table-wrap {
  border: 1px solid var(--line);
  border-radius: 6px;
  overflow: hidden;
  max-height: 320px;
  overflow-y: auto;
}

.compact-tbl th,
.compact-tbl td {
  padding: 8px 10px;
  font-size: 12.5px;
}

.text-muted {
  color: var(--text-3);
}

.reidentify-fail-box {
  background: rgba(234, 179, 8, 0.08);
  border: 1px dashed rgba(234, 179, 8, 0.35);
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  color: var(--text-2);
}

.reidentify-fail-box .fail-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--warn, #eab308);
  margin-bottom: 6px;
}

.reidentify-fail-box ul {
  margin: 0;
  padding-left: 18px;
  color: var(--text-3);
  max-height: 80px;
  overflow-y: auto;
}

.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--line);
}

.modal-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-1);
}

.modal-title svg {
  color: var(--accent);
}

.close-btn {
  border: none;
  background: transparent;
  font-size: 14px;
  color: var(--text-3);
  cursor: pointer;
}

.close-btn:hover {
  color: var(--text-1);
}

.modal-body {
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.replace-meta-box {
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.meta-row .lbl {
  color: var(--text-3);
  min-width: 60px;
}

.meta-row .val {
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 260px;
}

.text-accent {
  color: var(--accent) !important;
}

.info-note {
  margin: 0;
}

.modal-foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 18px;
  border-top: 1px solid var(--line);
  background: var(--panel-2);
  border-bottom-left-radius: inherit;
  border-bottom-right-radius: inherit;
}

.files-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 16px;
}

.files-title-row .hint {
  flex: 1;
  min-width: 180px;
}

.files-title-row .btn {
  flex: 0 0 auto;
  white-space: nowrap;
}

/* 借用零件高阶选型体系弹窗 */
.borrow-modal {
  width: 960px;
  max-width: 96vw;
  height: 640px;
  max-height: 92vh;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.modal-mode-tabs {
  display: flex;
  gap: 6px;
  background: var(--panel-2);
  padding: 3px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.mode-tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--text-3);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.mode-tab-btn:hover {
  color: var(--text-1);
}

.mode-tab-btn.active {
  background: var(--panel);
  color: var(--accent);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
}

.borrow-modal-body {
  flex: 1;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

/* 模式 1：三栏式自适应布局 */
.borrow-three-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 260px 310px 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

/* 模式 2：双栏穿透搜索布局 */
.borrow-two-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

.borrow-panel-col {
  display: flex;
  flex-direction: column;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 12px;
  min-height: 0;
  overflow: hidden;
}

.col-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.col-title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  display: flex;
  align-items: center;
  gap: 6px;
}

.col-title svg {
  color: var(--accent);
}

.col-badge {
  font-size: 11px;
  color: var(--text-3);
  background: var(--panel);
  padding: 1px 5px;
  border-radius: 3px;
  border: 1px solid var(--line);
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.compact {
  margin-top: 0 !important;
  margin-bottom: 8px;
}

.filter-inp {
  font-size: 12px !important;
  height: 30px !important;
  padding-left: 28px !important;
}

.global-search-inp {
  padding-left: 36px !important;
  height: 38px !important;
  font-size: 13px !important;
}

.scroll-select-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-right: 2px;
}

.project-item-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.project-item-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.project-item-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.proj-card-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.proj-card-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.part-candidate-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.part-candidate-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.part-candidate-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.part-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.part-name {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.part-card-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.has-file-badge {
  color: var(--text-3);
}

.has-file-badge.ok {
  color: var(--accent);
}

.global-part-card {
  padding: 10px 12px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.global-part-card:hover {
  background: var(--hover);
  border-color: var(--accent);
}

.global-part-card.active {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.gp-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.gp-name {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.gp-no {
  font-size: 12px;
  color: var(--text-2);
}

.gp-btm {
  display: flex;
  align-items: center;
  gap: 14px;
  font-size: 11.5px;
  color: var(--text-3);
}

.borrow-detail-col {
  background: var(--panel);
  padding: 14px;
}

.borrow-card-detail-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow-y: auto;
}

.preview-hero-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
}

.preview-hero-card svg {
  color: var(--accent);
}

.preview-hero-card h4 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.preview-spec-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--panel-2);
  padding: 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.spec-row {
  display: flex;
  align-items: center;
  font-size: 12.5px;
  gap: 6px;
}

.spec-row .k {
  color: var(--text-3);
  width: 75px;
  flex-shrink: 0;
}

.spec-row .v {
  color: var(--text-1);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
}

.empty-list-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px 10px;
  color: var(--text-3);
  gap: 6px;
  font-size: 12px;
}

.empty-preview-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--text-3);
  gap: 8px;
  font-size: 13px;
}

@media (max-width: 900px) {
  .borrow-three-grid {
    grid-template-columns: 1fr;
  }
  .borrow-two-grid {
    grid-template-columns: 1fr;
  }
}




<style scoped>
.drawing-preview-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}

.drawing-preview-view > * {
  max-width: 100%;
  min-width: 0;
}

.hidden-file-input {
  display: none;
}

.preview-actions-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  flex-wrap: wrap;
}

.header-info {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1 1 auto;
}

.header-info svg {
  color: var(--accent);
}

.header-info h3 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.header-info p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-buttons {
  display: flex;
  flex: 0 1 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  min-width: 0;
}

.files-table-card {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}

.files-table-card .tbl {
  width: 100%;
  min-width: 0;
  table-layout: auto;
}

.files-table-card .tbl th,
.files-table-card .tbl td {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.files-table-card .tbl td.row-actions {
  padding-right: 18px;
  overflow: visible;
  text-overflow: clip;
}

.files-table-card .tbl thead th {
  position: sticky;
  top: 0;
  z-index: 3;
  padding-top: 13px;
  padding-bottom: 11px;
  background: var(--panel-top);
  background-clip: padding-box;
  box-shadow: inset 0 -1px 0 var(--line);
}

@media (max-width: 1280px) {
  .preview-actions-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .header-buttons {
    justify-content: flex-start;
    width: 100%;
  }
}

@media (max-width: 760px) {
  .files-table-card .tbl th:nth-child(2),
  .files-table-card .tbl td:nth-child(2) {
    min-width: 200px;
  }
}

.files-table-card .tbl th:nth-child(1),
.files-table-card .tbl td:nth-child(1) { width: 100px; }
.files-table-card .tbl th:nth-child(2),
.files-table-card .tbl td:nth-child(2) { width: 100%; }
.files-table-card .tbl th:nth-child(3),
.files-table-card .tbl td:nth-child(3) { width: 84px; }
.files-table-card .tbl th:nth-child(4),
.files-table-card .tbl td:nth-child(4) { width: 120px; }
.files-table-card .tbl th:nth-child(5),
.files-table-card .tbl td:nth-child(5) { width: 1%; white-space: nowrap; }

.file-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.file-title-text {
  display: grid;
  min-width: 0;
}

.file-title-text b {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-owner {
  margin-top: 2px;
  color: var(--muted);
  font-size: 12px;
}

.download-modal {
  width: 560px;
}

.download-format-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.download-list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-bottom: 1px solid var(--line);
}

.download-check-all {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  cursor: pointer;
  font-size: 12.5px;
}

.download-file-list {
  display: flex;
  max-height: 320px;
  flex-direction: column;
  overflow-y: auto;
  border: 1px solid var(--line);
  border-radius: 10px;
}

.download-file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--line);
  cursor: pointer;
  font-size: 12.5px;
}

.download-file-row:last-child {
  border-bottom: none;
}

.download-file-row:hover {
  background: var(--panel-2);
}

.download-file-row.unavailable {
  cursor: not-allowed;
  opacity: 0.55;
}

.download-file-row .file-name {
  flex: 1;
  overflow: hidden;
  min-width: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.download-file-row .dl-size {
  flex-shrink: 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.badge-collab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 1px 7px;
  border-radius: 99px;
  font-size: 10px;
  font-weight: 600;
  line-height: 16px;
  white-space: nowrap;
  flex: none;
}

.badge-collab.active {
  background: var(--ok-soft, rgba(34, 197, 94, 0.15));
  color: var(--ok, #16a34a);
  border: 1px solid rgba(34, 197, 94, 0.3);
}

.badge-collab.locked {
  background: var(--warn-soft, rgba(245, 158, 11, 0.15));
  color: var(--warn, #d97706);
  border: 1px solid rgba(245, 158, 11, 0.3);
}

.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ok, #22c55e);
  box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  animation: pulse-ring 1.8s infinite cubic-bezier(0.66, 0, 0, 1);
  flex: none;
}

@keyframes pulse-ring {
  0% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  }
  70% {
    box-shadow: 0 0 0 6px rgba(34, 197, 94, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0);
  }
}

.row-editing {
  background: var(--accent-soft, rgba(59, 130, 246, 0.05)) !important;
}

.row-locked {
  opacity: 0.88;
}

.success-btn {
  background: var(--ok-soft, rgba(34, 197, 94, 0.15)) !important;
  color: var(--ok, #16a34a) !important;
  border-color: rgba(34, 197, 94, 0.35) !important;
  font-weight: 600;
}

.dock-time .hb-ok {
  color: var(--ok, #16a34a);
}

.dock-time .hb-lost {
  color: var(--warn, #f59e0b);
}

.locked-btn {
  opacity: 0.6;
  cursor: not-allowed !important;
}

.collab-multi-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.caxa-help-modal {
  max-width: 540px;
  width: min(540px, calc(100vw - 48px));
}

.caxa-help-detail {
  margin: 0 0 10px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--warn-soft, rgba(234, 179, 8, 0.12));
  color: var(--warn, #b45309);
  font-size: 12.5px;
  line-height: 1.7;
  word-break: break-all;
}

.caxa-help-text {
  margin: 0 0 12px;
  font-size: 13px;
  color: var(--text-2, #4b5563);
}

.caxa-help-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.caxa-help-actions .btn {
  justify-content: center;
}

.collab-dock-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 18px;
  border: 1px solid var(--accent);
  background: linear-gradient(135deg, var(--panel) 0%, var(--panel-2) 100%);
  border-radius: 12px;
  box-shadow: 0 4px 16px -4px rgba(0, 0, 0, 0.1);
}

.dock-left {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.dock-status-tag {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 4px 10px;
  background: var(--ok-soft, rgba(34, 197, 94, 0.12));
  color: var(--ok, #16a34a);
  border-radius: 99px;
  font-size: 11.5px;
  font-weight: 700;
  white-space: nowrap;
  flex: none;
}

.dock-file-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.dock-file-info .file-name {
  color: var(--text-1);
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dock-file-info .dock-time {
  color: var(--text-3);
  font-size: 11px;
}

.dock-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

.danger-tone {
  background: var(--danger, #ef4444) !important;
  color: #fff !important;
  border-color: transparent !important;
}

.dock-fade-enter-active,
.dock-fade-leave-active {
  transition: all 0.25s ease;
}

.dock-fade-enter-from,
.dock-fade-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

.table-pad {
  width: 100%;
  min-width: 0;
  max-height: max(280px, calc(100vh - 420px));
  padding: 0 14px 10px;
  overflow: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--line-strong) transparent;
}

.table-pad::-webkit-scrollbar {
  width: 7px;
  height: 7px;
}

.table-pad::-webkit-scrollbar-thumb {
  background: var(--line-strong);
  border-radius: 8px;
}

.table-pad::-webkit-scrollbar-track {
  background: transparent;
}

.actions-heading {
  width: 10%;
  min-width: 0;
  text-align: right !important;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.file-name-cell svg {
  color: var(--accent);
}

.file-name-cell b {
  display: block;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
}

.row-actions {
  position: relative;
  min-width: 0;
  white-space: nowrap;
  text-align: right;
}

.row-actions .btn {
  display: inline-flex;
  min-width: 0;
  height: 28px;
  padding: 5px 8px;
  gap: 5px;
  font-size: 11px;
  white-space: nowrap;
  margin-left: 4px;
}

.row-actions .btn :deep(svg) {
  width: 14px;
  height: 14px;
}

.row-actions .hist-count {
  position: absolute;
  top: -7px;
  right: -5px;
  min-width: 13px;
  padding: 1px 3px;
  border-radius: 99px;
  background: var(--accent);
  color: var(--accent-ink);
  font-size: 9px;
  line-height: 12px;
  text-align: center;
}

.row-actions .history-action {
  position: relative;
}

.editor-icon {
  width: 14px;
  height: 14px;
  flex: none;
}

.local-edit-spinner {
  width: 13px;
  height: 13px;
  flex: none;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: local-edit-spin 0.75s linear infinite;
}

.dock-status-tag.success {
  color: var(--ok);
}

.dock-status-tag.success :deep(svg),
.dock-status-tag.success strong {
  color: var(--ok);
}

.closed-ok {
  border-color: rgb(52 211 153 / 40%);
  background: rgb(52 211 153 / 6%);
}

@keyframes local-edit-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .local-edit-spinner {
    animation-duration: 1.5s;
  }
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  color: var(--text-3);
  gap: 8px;
}

.empty .t {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-2);
}

.empty p {
  font-size: 12px;
  margin: 0;
}
</style>
