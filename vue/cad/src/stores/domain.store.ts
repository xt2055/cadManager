import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { dataManager } from '@/services/data-manager'
import type { StoredAttachment } from '@/services/data-manager/data-provider'
import { deleteUploadRecoverySession, loadUploadRecoveryFile, saveUploadRecoveryFile } from '@/services/upload-file-storage'
import { readDocxAuthor } from '@/utils/docx-metadata'
import { parseMaterialFileContent } from '@/utils/material-table-parser'
import { directParentDrawingNo, isSameDrawingFamily } from '@/utils/drawing-number-parser'
import { useAuthStore } from '@/stores/auth.store'
import { createDrawingOperationLog, listDrawingOperationLogs } from '@/services/drawing-operation-log.service'
import { reviewFlowService, signerRoleForNode } from '@/services/review-flow.service'
import { reviewCaseService, type ApiReviewCase } from '@/services/review-case.service'
import { drawingLifecycleService } from '@/services/drawing-lifecycle.service'
import { ensureSmbCredential } from '@/services/tauri/cad-edit.service'
import { formatReadableDateTime } from '@/utils/date-time'
import type {
  ActivityLog,
  ActivityResult,
  ActivityTargetType,
  ActivityType,
  AdminLog,
  BomItem,
  Branch,
  BorrowRecord,
  CompletedReview,
  CraftFile,
  Drawing,
  DrawingAttribute,
  DrawingAttributeField,
  DrawingFile,
  DrawingSigners,
  DrawingVersion,
  MaterialFile,
  MyReview,
  ReviewCase,
  ReviewFlow,
  ReviewNode,
  StructurePart,
  StructurePartEditable,
  UserAccount,
  UserRole,
} from '@/types/domain.types'

export const STATUS = {
  published: { t: '生产中', c: 'ok' },
  reviewing: { t: '审核中', c: 'info' },
  draft: { t: '草稿', c: 'mute' },
  disabled: { t: '已禁用', c: 'danger' },
  archived: { t: '已存档', c: 'warn' },
} as const

interface AttachmentInput {
  id: string
  content: Blob
}

interface DrawingCreateTransactionContext {
  bom?: BomItem[]
  borrows?: BorrowRecord[]
  branches?: Branch[]
}

interface MaterialUploadResult {
  importedCount: number
}

function createId(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function nowLabel(): string {
  const d = new Date()
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}`
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function createVersionStorageKey(sourceKey: string | undefined, fileName: string): string | undefined {
  if (!sourceKey) return undefined
  const normalizedKey = sourceKey.replaceAll('\\', '/')
  const directory = normalizedKey.includes('/') ? normalizedKey.slice(0, normalizedKey.lastIndexOf('/')) : ''
  const extension = fileName.match(/\.[^./]+$/)?.[0] || '.dwg'
  const baseName = fileName.replace(/\.[^./]+$/, '').replace(/[^a-zA-Z0-9._\u4e00-\u9fa5-]+/g, '_') || 'drawing'
  const dateTag = new Date().toISOString().slice(0, 10).replace(/-/g, '')
  const timeTag = new Date().toTimeString().slice(0, 8).replace(/:/g, '')
  const uniqueName = `${baseName}_${dateTag}_${timeTag}${extension}`
  return directory ? `${directory}/history/${baseName}/${uniqueName}` : `history/${baseName}/${uniqueName}`
}

function activityTime(): string {
  return new Date().toISOString()
}

export const useDomainStore = defineStore('domain', () => {
	const authStore = useAuthStore()
  const drawings = ref<Drawing[]>([])
  const attributes = ref<DrawingAttribute[]>([])
  const structure = ref<StructurePart[]>([])
  const versions = ref<DrawingVersion[]>([])
  const branches = ref<Branch[]>([])
  const borrows = ref<BorrowRecord[]>([])
  const bomItems = ref<BomItem[]>([])
  const craftFiles = ref<CraftFile[]>([])
  // 数据库附件快照：项目级文件清单以它为准，避免结构树局部状态遗漏零件文件。
  const storedAttachments = ref<StoredAttachment[]>([])
  const logs = ref<ActivityLog[]>([])
  const reviewCases = ref<ReviewCase[]>([])
  const myReviews = ref<MyReview[]>([])
  const completedReviews = ref<CompletedReview[]>([])
  const users = ref<UserAccount[]>([])
  const flows = ref<ReviewFlow[]>([])
  const adminLogs = ref<AdminLog[]>([])

  const currentDrawing = ref<Drawing | StructurePart | null>(null)
  const selectedStructureIndex = ref(0)
  const treeOpen = ref(true)
  const initialized = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const currentNo = computed(() => currentDrawing.value?.no ?? '')
  const currentReviewCase = computed(() => getReviewCase(currentNo.value))
  const currentReviewNodes = computed(() => currentReviewCase.value?.nodes ?? [])
  const bom = computed(() => bomItems.value.filter((item) => item.drawingNo === currentNo.value))
  const crafts = computed(() => craftFiles.value.filter((file) => file.drawingNo === currentNo.value))
  // 待我审核：从进行中的审核案例实时推导，仅含当前活动节点且责任人为当前登录人的任务。
  const myPendingReviews = computed<MyReview[]>(() => {
    const user = authStore.currentUser
    if (!user) return []
    const result: MyReview[] = []
    for (const reviewCase of reviewCases.value) {
      if (reviewCase.status !== 'reviewing') continue
      const activeNode = reviewCase.nodes
        .filter((node) => node.status === 'pending')
        .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))[0]
      if (!activeNode) continue
      if (activeNode.user !== user.displayName && activeNode.user !== user.account) continue
      const target = drawings.value.find((item) => item.no === reviewCase.drawingNo)
        ?? structure.value.find((item) => item.no === reviewCase.drawingNo)
      result.push({
        reviewCaseId: reviewCase.id,
        no: reviewCase.drawingNo,
        name: target?.name ?? reviewCase.drawingNo,
        node: activeNode.name,
        by: activeNode.user,
        time: reviewCase.startedAt,
      })
    }
    return result
  })
  const reviewCount = computed(() => myPendingReviews.value.length)
  const drawingStats = computed(() => {
    const assemblies = drawings.value.filter((item) => item.kind === '总图').length
    const parts = structure.value.length
    return {
      total: assemblies + parts,
      assemblies,
      parts,
    }
  })

  let initializationPromise: Promise<void> | null = null
  let saveQueue: Promise<void> = Promise.resolve()
  const pendingDrawingUploads = new Map<string, Map<string, { itemId: string; file: DrawingFile | MaterialFile | CraftFile; content: Blob }>>()
  const pendingUploadSessionId = ref<string | null>(null)
  const uploadProgress = ref<Record<string, number>>({})
  let persistedSnapshots = {
    drawings: '', structure: '', attributes: '', versions: '', branches: '', borrows: '', bom: '', crafts: '',
  }

  function moduleSnapshot(value: unknown): string {
    return JSON.stringify(value)
  }

  function refreshPersistedSnapshots(): void {
    persistedSnapshots = {
      drawings: moduleSnapshot(drawings.value),
      structure: moduleSnapshot(structure.value),
      attributes: moduleSnapshot(attributes.value),
      versions: moduleSnapshot(versions.value),
      branches: moduleSnapshot(branches.value),
      borrows: moduleSnapshot(borrows.value),
      bom: moduleSnapshot(bomItems.value),
      crafts: moduleSnapshot(craftFiles.value),
    }
  }

  function recordActivity(input: {
    drawingNo: string
    drawingName?: string
    targetType: ActivityTargetType
    act: ActivityType
    text: string
    result?: ActivityResult
    detail?: Record<string, unknown>
  }): ActivityLog {
    const operator = authStore.currentUser
    const occurredAt = activityTime()
    const target = findDrawingOrPart(input.drawingNo)
    const activity: ActivityLog = {
      id: createId('drawing-log'),
      drawingNo: input.drawingNo,
      drawingName: input.drawingName || target?.name || input.drawingNo,
      targetType: input.targetType,
      ...(operator?.id ? { userId: operator.id } : {}),
      user: operator?.displayName || '未知',
      act: input.act,
      txt: input.text,
      time: occurredAt,
      occurredAt,
      result: input.result ?? 'success',
      ...(input.detail ? { detail: input.detail } : {}),
    }
    logs.value.unshift(activity)
    void createDrawingOperationLog({
      drawingNo: activity.drawingNo,
      drawingName: activity.drawingName,
      targetType: activity.targetType,
      act: activity.act,
      txt: activity.txt,
      result: activity.result,
      detail: activity.detail,
    }).catch(() => undefined)
    return activity
  }

  async function loadRemoteActivityLogs(): Promise<void> {
    try {
      const page = await listDrawingOperationLogs({ page: 1, pageSize: 100 })
      logs.value = page.list
    } catch {
      // 调试模式使用本地 JSON 数据时没有后端日志接口，保留本地记录。
    }
  }

  function persist(): Promise<void> {
    const saveOperation = saveQueue
      .catch(() => undefined)
      .then(async () => {
        const operations: Promise<void>[] = []
        // 图纸不再允许全量回写：旧页面的整个数组会复活已删除资源、覆盖他人修改。
        // 图纸字段必须由 /api/drawings/{id} 的带 revision PATCH 提交。
        if (moduleSnapshot(structure.value) !== persistedSnapshots.structure) operations.push(dataManager.saveStructure(structure.value))
        if (moduleSnapshot(attributes.value) !== persistedSnapshots.attributes) operations.push(dataManager.saveAttributes(attributes.value))
        if (moduleSnapshot(versions.value) !== persistedSnapshots.versions) operations.push(dataManager.saveVersions(versions.value))
        if (moduleSnapshot(branches.value) !== persistedSnapshots.branches) operations.push(dataManager.saveBranches(branches.value))
        if (moduleSnapshot(borrows.value) !== persistedSnapshots.borrows) operations.push(dataManager.saveBorrows(borrows.value))
        if (moduleSnapshot(bomItems.value) !== persistedSnapshots.bom) operations.push(dataManager.saveBom(bomItems.value))
        if (moduleSnapshot(craftFiles.value) !== persistedSnapshots.crafts) operations.push(dataManager.saveCrafts(craftFiles.value))
        await Promise.all(operations)
        refreshPersistedSnapshots()
      })
    saveQueue = saveOperation.catch(() => undefined)

    return saveOperation.catch((saveError: unknown) => {
      error.value = saveError instanceof Error ? saveError.message : String(saveError)
      console.error('保存业务模块失败', saveError)
      throw saveError
    })
  }

  async function recordActivityAndPersist(input: Parameters<typeof recordActivity>[0]): Promise<void> {
    recordActivity(input)
    await persist()
  }

  // 审核案例与已办归档的权威数据在 Go 后端，初始化与每次流转后从这里刷新。
  function mapApiReviewCase(item: ApiReviewCase) {
    return {
      id: item.id,
      drawingNo: item.drawingNo,
      flow: item.flow,
      status: item.status as 'pending' | 'reviewing' | 'published' | 'rejected',
      initiator: item.initiator,
      startedAt: item.startedAt,
      nodes: item.nodes.map((node) => ({
        name: node.name,
        user: node.assignedName,
        assignedUserId: node.assignedUserId,
        status: node.status,
        time: node.time || '—',
        opinion: node.opinion,
        required: node.required,
        order: node.order,
      })),
    }
  }

  async function refreshReviewData(): Promise<void> {
    try {
      const [cases, completed] = await Promise.all([
        reviewCaseService.list(),
        reviewCaseService.completed(),
      ])
      reviewCases.value = cases.map(mapApiReviewCase)
      completedReviews.value = completed.map((item) => ({
        id: item.id,
        reviewCaseId: item.reviewCaseId,
        no: item.no,
        name: item.name,
        node: item.node,
        by: item.by,
        reviewer: item.reviewer,
        time: item.time,
        result: item.result,
        opinion: item.opinion,
        ver: item.ver,
      }))
      // 后端案例状态是权威：同步图纸文档状态，避免驳回/发布后前端仍停留在旧状态。
      // rejected → draft（等发起人重新发起）、published → published、reviewing → reviewing。
      // cases 按 started_at 降序：只取每张图纸最新案例的状态，防止旧驳回案例覆盖进行中状态。
      const statusByDrawing = new Map<string, 'reviewing' | 'draft' | 'published'>()
      for (const item of reviewCases.value) {
        if (statusByDrawing.has(item.drawingNo)) continue
        statusByDrawing.set(item.drawingNo, item.status === 'reviewing' ? 'reviewing' : item.status === 'published' ? 'published' : 'draft')
      }
      let statusChanged = false
      for (const [drawingNo, status] of statusByDrawing) {
        const target = findDrawingOrPart(drawingNo)
        if (target && target.status !== status && target.status !== 'archived') {
          target.status = status
          statusChanged = true
        }
      }
      if (statusChanged) await persist()
    } catch (loadError) {
      // 审核服务不可用时保留文档内数据，避免整个工作台不可用。
      console.warn('加载审核数据失败', loadError)
    }
  }

  function applyStoredAttachments(items: StoredAttachment[]): void {
    storedAttachments.value = items
    if (!items.length) return
    const drawingByNo = new Map(drawings.value.map((drawing) => [drawing.no, drawing]))
    const partByNo = new Map(structure.value.map((part) => [part.no, part]))
    for (const drawing of drawings.value) {
      drawing.files = []
      drawing.otherFiles = []
      drawing.materialFiles = []
      drawing.craftFiles = []
      drawing.hasFile = false
    }
    for (const part of structure.value) {
      part.files = []
      part.otherFiles = []
      part.materialFiles = []
      part.craftFiles = []
      part.hasFile = false
    }
    craftFiles.value = []
    for (const item of items) {
      const owner = item.partNo ? partByNo.get(item.partNo) : drawingByNo.get(item.drawingNo)
      if (!owner) continue
			// 页面显示原始真实文件名；currentName 仅代表后台转换后的当前格式（例如 EXB -> DWG）。
			const name = item.name || item.currentName || '未命名文件'
			const file: DrawingFile = {
				id: item.id,
				name,
				rawName: item.currentName && item.currentName !== name ? item.currentName : undefined,
				size: formatFileSize(item.currentSize ?? item.size),
        role: item.role === 'assembly' || item.role === 'part' ? item.role : 'other',
        drawingNo: item.drawingNo,
        ...(item.partNo ? { partNo: item.partNo } : {}),
        version: item.version,
        uploadedBy: item.uploadedBy || '未知用户',
        uploadedAt: formatReadableDateTime(item.createdAt, '历史记录'),
        storageKey: item.currentStorageKey || item.storageKey,
        rawStorageKey: item.storageKey,
        currentStorageKey: item.currentStorageKey,
				mimeType: item.currentMimeType || item.mimeType,
		previewable: item.previewable,
		revision: item.revision,
      }
      if (item.role === 'material') {
        const material: MaterialFile = {
          id: item.id,
          drawingNo: item.drawingNo,
          name,
          size: formatFileSize(item.size),
          version: item.version,
          uploadedBy: item.uploadedBy || '未知用户',
          uploadedAt: formatReadableDateTime(item.createdAt, '历史记录'),
          storageKey: item.currentStorageKey || item.storageKey,
			mimeType: item.mimeType,
			revision: item.revision,
        }
        owner.materialFiles = [...(owner.materialFiles ?? []).filter((file) => file.id !== material.id), material]
      } else if (item.role === 'craft') {
        const craft: CraftFile = {
          id: item.id,
          drawingNo: item.drawingNo,
          name,
          op: '未分类工艺',
          ver: item.version,
          by: item.uploadedBy || '未知用户',
          date: item.createdAt || '历史记录',
          size: formatFileSize(item.size),
          storageKey: item.currentStorageKey || item.storageKey,
          mimeType: item.mimeType,
          previewable: item.previewable,
			scanned: false,
			revision: item.revision,
        }
        owner.craftFiles = [...(owner.craftFiles ?? []).filter((file) => file.id !== craft.id), craft]
        craftFiles.value.push(craft)
      } else if ((item.role === 'assembly' && 'kind' in owner) || (item.role === 'part' && 'parentNo' in owner)) {
        owner.files = [...(owner.files ?? []).filter((candidate) => candidate.id !== file.id), file]
      } else {
        owner.otherFiles = [...(owner.otherFiles ?? []).filter((candidate) => candidate.id !== file.id), file]
      }
      owner.hasFile = (owner.files ?? []).length > 0
    }
  }

  // 从后端重新加载业务文档：本地编辑结束生成新版本后调用，
  // 保证前端文件列表/版本号以后端数据库为准，不用旧数据覆盖服务端状态。
  async function reloadFromServer(): Promise<void> {
    try {
      const [loadedDrawings, loadedStructure, loadedAttributes, loadedVersions, loadedBranches, loadedBorrows, loadedBom, loadedCrafts, loadedAttachments] = await Promise.all([
        dataManager.loadDrawings(),
        dataManager.loadStructure(),
        dataManager.loadAttributes(),
        dataManager.loadVersions(),
        dataManager.loadBranches(),
        dataManager.loadBorrows(),
        dataManager.loadBom(),
        dataManager.loadCrafts(),
        dataManager.loadAttachments(),
      ])
      drawings.value = loadedDrawings
      structure.value = loadedStructure
      attributes.value = loadedAttributes
      versions.value = loadedVersions
      branches.value = loadedBranches
      borrows.value = loadedBorrows
      bomItems.value = loadedBom
      craftFiles.value = loadedCrafts
      applyStoredAttachments(loadedAttachments)
      currentDrawing.value = currentDrawing.value ? findDrawingOrPart(currentDrawing.value.no) : null
      refreshPersistedSnapshots()
    } catch (loadError) {
      console.warn('刷新业务数据失败', loadError)
    }
  }

  function initialize(): Promise<void> {
    if (initialized.value) return Promise.resolve()
    if (initializationPromise) return initializationPromise

    loading.value = true
    error.value = null
    initializationPromise = (async () => {
      try {
        const [loadedDrawings, loadedStructure, loadedAttributes, loadedVersions, loadedBranches, loadedBorrows, loadedBom, loadedCrafts, loadedAttachments] = await Promise.all([
          dataManager.loadDrawings(),
          dataManager.loadStructure(),
          dataManager.loadAttributes(),
          dataManager.loadVersions(),
          dataManager.loadBranches(),
          dataManager.loadBorrows(),
          dataManager.loadBom(),
          dataManager.loadCrafts(),
          dataManager.loadAttachments(),
        ])
        drawings.value = loadedDrawings
        structure.value = loadedStructure
        attributes.value = loadedAttributes
        versions.value = loadedVersions
        branches.value = loadedBranches
        borrows.value = loadedBorrows
        bomItems.value = loadedBom
        craftFiles.value = loadedCrafts
	        applyStoredAttachments(loadedAttachments)
	        refreshPersistedSnapshots()
	        const lastSessionId = window.localStorage.getItem('cad:last-upload-session')
	        if (lastSessionId) {
	          try {
	            const snapshot = await dataManager.getUploadSession(lastSessionId)
	            if (snapshot.session.status === 'open' || snapshot.session.status === 'failed') {
	              pendingUploadSessionId.value = lastSessionId
	              const entries = new Map<string, { itemId: string; file: DrawingFile | MaterialFile | CraftFile; content: Blob }>()
	              for (const item of snapshot.items) {
	                const content = await loadUploadRecoveryFile(lastSessionId, item.clientRef).catch(() => null)
	                if (!content) continue
	                entries.set(item.clientRef, {
	                  itemId: item.id,
	                  file: {
	                    id: item.clientRef,
	                    name: item.originalName,
	                    size: formatFileSize(item.size),
	                    role: item.role === 'assembly' || item.role === 'part' ? item.role : 'other',
	                    drawingNo: item.drawingNo,
	                    ...(item.partNo ? { partNo: item.partNo } : {}),
	                    version: 'v1.0',
	                    uploadedBy: '恢复上传',
	                    uploadedAt: item.updatedAt,
	                    previewable: true,
	                  } as DrawingFile,

	                  content,

	                })
	              }
	              if (entries.size) pendingDrawingUploads.set(lastSessionId, entries)
	            } else {
	              window.localStorage.removeItem('cad:last-upload-session')
	            }
	          } catch {
	            window.localStorage.removeItem('cad:last-upload-session')
	          }
	        }
        if (authStore.hasRole('admin')) {
          users.value = await dataManager.listUsers()
        }
        await scanUnscannedCraftFiles()
        await loadRemoteActivityLogs()
        await refreshReviewData()
        initialized.value = true
        startReviewPolling()
        // 桌面客户端：自动写入 SMB 访问凭据（失败不影响业务）。
        void ensureSmbCredential().catch((smbError: unknown) => {
          console.warn('SMB 凭据自动配置失败', smbError)
        })
      } catch (loadError: unknown) {
        initialized.value = false
        error.value = loadError instanceof Error ? loadError.message : String(loadError)
        console.error('加载业务数据失败', loadError)
        throw loadError
      } finally {
        loading.value = false
        initializationPromise = null
      }
    })()

    return initializationPromise
  }

  // 审核数据轮询：发起/签署/驳回后，其他登录用户（如审核员）的待办与详情在 30s 内自动更新，
  // 无需手动刷新页面。
  let reviewPollTimer: number | null = null
  function startReviewPolling(): void {
    if (reviewPollTimer !== null) return
    reviewPollTimer = window.setInterval(() => {
      if (!authStore.currentUser) return
      void refreshReviewData()
    }, 30_000)
  }

  async function scanUnscannedCraftFiles(): Promise<void> {
    let changed = false

    for (const file of craftFiles.value) {
      if (file.scanned || !file.name.toLowerCase().endsWith('.docx') || !file.storageKey) continue

      try {
        const content = await dataManager.readAttachment(file.storageKey)
        file.author = await readDocxAuthor(content)
        file.scanned = true

        const target = findDrawingOrPart(file.drawingNo)
        const targetFile = target?.craftFiles?.find((item) => item.id === file.id)
        if (targetFile) {
          targetFile.author = file.author
          targetFile.scanned = true
        }
        changed = true
      } catch (scanError) {
        console.warn(`自动扫描工艺文件失败：${file.name}`, scanError)
      }
    }

    // 自动扫描备料表中的编制人员
    const allMaterialFiles = [
      ...drawings.value.flatMap((d) => d.materialFiles ?? []),
      ...structure.value.flatMap((p) => p.materialFiles ?? []),
    ]
    for (const file of allMaterialFiles) {
      if (file.author || !file.storageKey) continue
      try {
        const content = await dataManager.readAttachment(file.storageKey)
        const parseResult = await parseMaterialFileContent(content, file.name, file.drawingNo, file.id)
        if (parseResult.author) {
          file.author = parseResult.author
          changed = true
        }
      } catch (scanErr) {
        console.warn(`自动扫描备料表编制人员失败：${file.name}`, scanErr)
      }
    }

    // 自动修复历史文件中显示为“刚刚”、历史占位符或缺失的上传时间。
    const formatTimeStr = (t: string) => (!t || t === '刚刚' || t === '历史记录' ? nowLabel() : t)
    for (const d of drawings.value) {
      for (const file of [...(d.files ?? []), ...(d.otherFiles ?? [])]) {
        const uploadedAt = formatTimeStr(file.uploadedAt)
        if (uploadedAt !== file.uploadedAt) {
          file.uploadedAt = uploadedAt
          changed = true
        }
        for (const history of file.history ?? []) {
          const historyUploadedAt = formatTimeStr(history.uploadedAt)
          if (historyUploadedAt !== history.uploadedAt) {
            history.uploadedAt = historyUploadedAt
            changed = true
          }
        }
      }
    }
    for (const part of structure.value) {
      for (const file of [...(part.files ?? []), ...(part.otherFiles ?? [])]) {
        const uploadedAt = formatTimeStr(file.uploadedAt)
        if (uploadedAt !== file.uploadedAt) {
          file.uploadedAt = uploadedAt
          changed = true
        }
        for (const history of file.history ?? []) {
          const historyUploadedAt = formatTimeStr(history.uploadedAt)
          if (historyUploadedAt !== history.uploadedAt) {
            history.uploadedAt = historyUploadedAt
            changed = true
          }
        }
      }
    }
    for (const d of drawings.value) {
      if (d.materialFiles) {
        for (const mf of d.materialFiles) {
          if (mf.uploadedAt === '刚刚') {
            mf.uploadedAt = nowLabel()
            changed = true
          }
        }
      }
      if (d.craftFiles) {
        for (const cf of d.craftFiles) {
          if (cf.date === '刚刚') {
            cf.date = nowLabel()
            changed = true
          }
        }
      }
    }
    for (const p of structure.value) {
      if (p.materialFiles) {
        for (const mf of p.materialFiles) {
          if (mf.uploadedAt === '刚刚') {
            mf.uploadedAt = nowLabel()
            changed = true
          }
        }
      }
      if (p.craftFiles) {
        for (const cf of p.craftFiles) {
          if (cf.date === '刚刚') {
            cf.date = nowLabel()
            changed = true
          }
        }
      }
    }
    for (const cf of craftFiles.value) {
      if (cf.date === '刚刚') {
        cf.date = nowLabel()
        changed = true
      }
    }

    // 从总图或同目录其他 CAD 图纸的标题栏读取设计人
    for (const drawing of drawings.value) {
      if (drawing.designer || drawing.kind !== '总图') continue
      try {
        const designer = await dataManager.scanDrawingDesigner(drawing.no)
        if (designer) {
          drawing.designer = designer
          changed = true
        }
      } catch (scanErr) {
        console.warn(`自动扫描图纸设计人失败：${drawing.no}`, scanErr)
      }
    }

    if (changed) await persist()
  }

  async function refreshDrawingDesigner(drawingNo: string): Promise<void> {
    await initialize()
    let targetNo = drawingNo
    const visited = new Set<string>()
    while (targetNo && !visited.has(targetNo)) {
      visited.add(targetNo)
      const drawing = drawings.value.find((item) => item.no === targetNo)
      if (drawing) {
        const designer = await dataManager.scanDrawingDesigner(drawing.no)
        if (designer && designer !== drawing.designer) {
          drawing.designer = designer
          await persist()
        }
        return
      }
      const part = structure.value.find((item) => item.no === targetNo)
      if (!part) return
      targetNo = part.parentNo
    }
  }

  function findDrawingOrPart(no: string): Drawing | StructurePart | null {
    const drawing = drawings.value.find((item) => item.no === no)
    if (drawing) return drawing
    return structure.value.find((item) => item.no === no) ?? null
  }

  function getReviewCase(drawingNo: string, reviewCaseId?: string): ReviewCase | null {
    if (!drawingNo) return null
    if (reviewCaseId) {
      return reviewCases.value.find((item) => item.id === reviewCaseId && item.drawingNo === drawingNo) ?? null
    }
    // 同一图纸可能存在历史案例（驳回/已完成）：始终返回最新发起的一个，
    // 否则续审后详情页仍显示旧驳回案例（节点状态与待办全部错位）。
    let latest: ReviewCase | null = null
    for (const reviewCase of reviewCases.value) {
      if (reviewCase?.drawingNo !== drawingNo) continue
      if (!latest || (reviewCase.startedAt || '') >= (latest.startedAt || '')) latest = reviewCase
    }
    return latest
  }

  function getTargetFiles(target: Drawing | StructurePart): DrawingFile[] {
    target.files = target.files ?? []
    return target.files
  }

  function getTargetAttachmentFiles(target: Drawing | StructurePart): {
    materialFiles: MaterialFile[]
    craftFiles: CraftFile[]
  } {
    target.materialFiles = target.materialFiles ?? []
    target.craftFiles = target.craftFiles ?? []
    return { materialFiles: target.materialFiles, craftFiles: target.craftFiles }
  }

	async function uploadSessionFile(sessionId: string, itemId: string, content: Blob, name: string, sha256: string): Promise<void> {
		const chunkSize = 8 * 1024 * 1024
		uploadProgress.value = { ...uploadProgress.value, [itemId]: 0 }
		if (!sha256 || content.size < chunkSize) {
			await dataManager.uploadSessionItem(sessionId, itemId, content, name)
			uploadProgress.value = { ...uploadProgress.value, [itemId]: 100 }
			return
		}
		const snapshot = await dataManager.initUploadChunks(sessionId, itemId, {
			totalSize: content.size,
			chunkSize,
			sha256,
		})
		const uploaded = new Set(snapshot.parts.map((part) => part.partNumber))
		const partCount = Math.ceil(content.size / chunkSize)
		uploadProgress.value = { ...uploadProgress.value, [itemId]: Math.round((uploaded.size / partCount) * 100) }
		for (let partNumber = 0; partNumber < partCount; partNumber += 1) {
			if (uploaded.has(partNumber)) continue
			const start = partNumber * chunkSize
			const end = Math.min(content.size, start + chunkSize)
			await dataManager.uploadSessionChunk(sessionId, itemId, partNumber, content.slice(start, end))
			uploadProgress.value = { ...uploadProgress.value, [itemId]: Math.round(((uploaded.size + 1) / partCount) * 100) }
			uploaded.add(partNumber)
		}
		await dataManager.completeUploadChunks(sessionId, itemId)
		uploadProgress.value = { ...uploadProgress.value, [itemId]: 100 }
	}

	async function uploadReplacementSession(
		drawingNo: string,
		file: { id: string; name: string; revision?: number; role: 'assembly' | 'part' | 'material' | 'craft' | 'other'; partNo?: string },
		content: Blob,
	): Promise<Record<string, unknown>> {
		const uploadName = content instanceof File ? content.name : file.name
		let hash: Awaited<ReturnType<typeof dataManager.checkUploadHash>>
		try {
			hash = await dataManager.checkUploadHash(content)
		} catch (error) {
			console.warn(`文件「${uploadName}」哈希预检失败，改为完整上传`, error)
			hash = { exists: false, sha256: '', size: content.size, mimeType: content.type || 'application/octet-stream' }
		}
		const session = await dataManager.createUploadSession({
			kind: 'attachment',
			idempotencyKey: `attachment-replace:${file.id}:${file.revision ?? 1}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
			metadata: { drawingNo, attachmentId: file.id, expectedRevision: file.revision ?? 1 },
		})
		try {
			const item = await dataManager.createUploadSessionItem(session.id, {
				clientRef: file.id,
				attachmentId: file.id,
				drawingNo,
				...(file.partNo ? { partNo: file.partNo } : {}),
				role: file.role,
				originalName: uploadName,
				mimeType: content.type || 'application/octet-stream',
				expectedRevision: file.revision ?? 1,
				sha256: hash.sha256,
				size: content.size,
				...(!isCADFileName(uploadName) && hash.exists && hash.blobId ? { blobId: hash.blobId } : {}),
			})
			if (item.status !== 'ready') await uploadSessionFile(session.id, item.id, content, uploadName, hash.sha256)
			else uploadProgress.value = { ...uploadProgress.value, [item.id]: 100 }
			return await dataManager.commitUploadSession(session.id)
		} catch (error) {
			await dataManager.cancelUploadSession(session.id).catch(() => undefined)
			throw error
		}
	}

	async function uploadNewAttachmentSession(
		drawingNo: string,
		file: { id: string; name: string; role: 'assembly' | 'part' | 'material' | 'craft' | 'other'; partNo?: string },
		content: Blob,
	): Promise<Record<string, unknown>> {
		const uploadName = content instanceof File ? content.name : file.name
		let hash: Awaited<ReturnType<typeof dataManager.checkUploadHash>>
		try {
			hash = await dataManager.checkUploadHash(content)
		} catch (error) {
			console.warn(`文件「${uploadName}」哈希预检失败，改为完整上传`, error)
			hash = { exists: false, sha256: '', size: content.size, mimeType: content.type || 'application/octet-stream' }
		}
		const session = await dataManager.createUploadSession({
			kind: 'attachment',
			idempotencyKey: `attachment-create:${file.id}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
			metadata: { drawingNo, partNo: file.partNo || '', role: file.role },
		})
		try {
			const item = await dataManager.createUploadSessionItem(session.id, {
				clientRef: file.id,
				drawingNo,
				...(file.partNo ? { partNo: file.partNo } : {}),
				role: file.role,
				originalName: uploadName,
				mimeType: content.type || 'application/octet-stream',
				sha256: hash.sha256,
				size: content.size,
				...(!isCADFileName(uploadName) && hash.exists && hash.blobId ? { blobId: hash.blobId } : {}),
			})
			if (item.status !== 'ready') await uploadSessionFile(session.id, item.id, content, uploadName, hash.sha256)
			else uploadProgress.value = { ...uploadProgress.value, [item.id]: 100 }
			return await dataManager.commitUploadSession(session.id)
		} catch (error) {
			await dataManager.cancelUploadSession(session.id).catch(() => undefined)
			throw error
		}
	}

	function isDrawingFile(file: DrawingFile | MaterialFile | CraftFile): file is DrawingFile {
		return 'role' in file
	}

	function isCADFileName(name: string): boolean {
		return ['.exb', '.dwg', '.dxf'].includes(name.slice(name.lastIndexOf('.')).toLowerCase())
	}

  function openDrawing(no: string) {
    if (currentDrawing.value?.no === no) return
    currentDrawing.value = findDrawingOrPart(no)
    selectedStructureIndex.value = 0
    if (currentDrawing.value) {
      recordActivity({
        drawingNo: currentDrawing.value.no,
        drawingName: currentDrawing.value.name,
        targetType: 'drawing',
        act: 'view',
        text: `查看图纸 ${currentDrawing.value.no}`,
      })
      // 纯浏览操作不持久化业务文档：全量保存会让旧页面内存覆盖数据库新数据。
    }
  }

  function clearCurrentDrawing() {
    currentDrawing.value = null
    selectedStructureIndex.value = 0
  }

  // ---------- 图纸属性（平铺属性 + 字段选项） ----------

  const sortedAttributes = computed(() => attributes.value
    .filter((attribute) => attribute.enabled)
    .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, 'zh-CN')))

  function attributeName(id: string): string {
    return attributes.value.find((attribute) => attribute.id === id)?.name ?? ''
  }

  function attributeFieldName(attributeId: string, fieldId?: string): string {
    if (!fieldId) return ''
    return attributes.value.find((attribute) => attribute.id === attributeId)?.fields.find((field) => field.id === fieldId)?.name ?? ''
  }

  function validateAttributeValues(values: Record<string, string> = {}): string[] {
    return sortedAttributes.value.flatMap((attribute) => {
      const value = values[attribute.id] ?? ''
      if (attribute.required && !value) return [`请填写属性「${attribute.name}」`]
      if (value && !attribute.fields.some((field) => field.enabled && field.id === value)) {
        return [`属性「${attribute.name}」的字段选项无效`]
      }
      return []
    })
  }

  async function addAttribute(name: string, required = false): Promise<DrawingAttribute> {
    await initialize()
    const trimmed = name.trim()
    if (!trimmed) throw new Error('属性名称不能为空')
    if (attributes.value.some((attribute) => attribute.name === trimmed)) throw new Error(`属性「${trimmed}」已存在`)
    const attribute: DrawingAttribute = {
      id: createId('attribute'),
      name: trimmed,
      required,
      enabled: true,
      sortOrder: attributes.value.length + 1,
      fields: [],
      createdAt: nowLabel(),
    }
    attributes.value.push(attribute)
    await persist()
    return attribute
  }

  async function updateAttribute(id: string, payload: Pick<DrawingAttribute, 'name' | 'required' | 'enabled'>): Promise<void> {
    await initialize()
    const attribute = attributes.value.find((item) => item.id === id)
    if (!attribute) throw new Error('属性不存在')
    const name = payload.name.trim()
    if (!name) throw new Error('属性名称不能为空')
    if (attributes.value.some((item) => item.id !== id && item.name === name)) throw new Error(`属性「${name}」已存在`)
    Object.assign(attribute, { name, required: payload.required, enabled: payload.enabled })
    await persist()
  }

  async function deleteAttribute(id: string): Promise<void> {
    await initialize()
    if (!attributes.value.some((item) => item.id === id)) throw new Error('属性不存在')
    attributes.value = attributes.value.filter((item) => item.id !== id)
    for (const drawing of drawings.value) {
      if (drawing.attributeValues) delete drawing.attributeValues[id]
    }
    await persist()
  }

  async function addAttributeField(attributeId: string, name: string): Promise<DrawingAttributeField> {
    await initialize()
    const attribute = attributes.value.find((item) => item.id === attributeId)
    if (!attribute) throw new Error('属性不存在')
    const trimmed = name.trim()
    if (!trimmed) throw new Error('字段名称不能为空')
    if (attribute.fields.some((field) => field.name === trimmed)) throw new Error(`字段「${trimmed}」已存在`)
    const field: DrawingAttributeField = {
      id: createId('attribute-field'),
      name: trimmed,
      enabled: true,
      sortOrder: attribute.fields.length + 1,
      createdAt: nowLabel(),
    }
    attribute.fields.push(field)
    await persist()
    return field
  }

  async function updateAttributeField(attributeId: string, fieldId: string, name: string, enabled: boolean): Promise<void> {
    await initialize()
    const attribute = attributes.value.find((item) => item.id === attributeId)
    const field = attribute?.fields.find((item) => item.id === fieldId)
    if (!attribute || !field) throw new Error('字段不存在')
    const trimmed = name.trim()
    if (!trimmed) throw new Error('字段名称不能为空')
    if (attribute.fields.some((item) => item.id !== fieldId && item.name === trimmed)) throw new Error(`字段「${trimmed}」已存在`)
    Object.assign(field, { name: trimmed, enabled })
    await persist()
  }

  async function reorderAttributeFields(attributeId: string, fieldIds: string[]): Promise<void> {
    await initialize()
    const attribute = attributes.value.find((item) => item.id === attributeId)
    if (!attribute) throw new Error('属性不存在')
    const fieldMap = new Map(attribute.fields.map((field) => [field.id, field]))
    const newFields: DrawingAttributeField[] = []
    fieldIds.forEach((id, index) => {
      const field = fieldMap.get(id)
      if (field) {
        field.sortOrder = index + 1
        newFields.push(field)
        fieldMap.delete(id)
      }
    })
    // 补齐遗漏字段
    fieldMap.forEach((field) => {
      field.sortOrder = newFields.length + 1
      newFields.push(field)
    })
    attribute.fields = newFields
    await persist()
  }

  async function setDrawingAttributes(drawingNo: string, values: Record<string, string>): Promise<void> {
    await initialize()
    const drawing = drawings.value.find((item) => item.no === drawingNo)
    if (!drawing) throw new Error(`图纸 ${drawingNo} 不存在`)
    const errors = validateAttributeValues(values)
    if (errors.length) throw new Error(errors[0])
    if (!drawing.id || !drawing.revision) throw new Error('图纸缺少服务端版本信息，请刷新后重试')
    const attributeValues = Object.fromEntries(Object.entries(values).filter(([, value]) => value))
    await dataManager.updateDrawing(drawing.id, {
      expectedRevision: drawing.revision,
      attributeValues,
    })
    // PATCH 成功后重新取服务端快照，防止同一窗口继续保留旧 revision。
    await reloadFromServer()
  }

	  async function addDrawing(
	    drawing: Drawing,
	    structureParts: StructurePart[] = [],
	    attachments: AttachmentInput[] = [],
	    transactionContext: DrawingCreateTransactionContext = {},
	  ): Promise<void> {
    await initialize()
    if (drawings.value.some((item) => item.no === drawing.no)) {
      throw new Error(`总图图号「${drawing.no}」已存在，请确认总图图号；项目号与总图图号是两个不同字段`)
    }

    const attachmentMap = new Map(attachments.map((item) => [item.id, item.content]))
	    const allFiles = [
	      ...(drawing.files ?? []),
	      ...(drawing.otherFiles ?? []),
	      ...(drawing.materialFiles ?? []),
	      ...(drawing.craftFiles ?? []),
	      ...structureParts.flatMap((part) => part.files ?? []),
	      ...structureParts.flatMap((part) => part.otherFiles ?? []),
	      ...structureParts.flatMap((part) => part.materialFiles ?? []),
	      ...structureParts.flatMap((part) => part.craftFiles ?? []),
    ]

    drawing.files = drawing.files ?? []
    drawing.hasFile = drawing.files.length > 0
    const structurePartNos = new Set(structureParts.map((part) => part.no))
    for (const part of structureParts) {
      if (!part.parentNo || (part.parentNo !== drawing.no && !structurePartNos.has(part.parentNo) && !part.parentNo.startsWith(`${drawing.no}-`))) {
        throw new Error(`零件 ${part.no} 未正确关联到总图 ${drawing.no} 或其子级结构`)
      }
      if (structureParts.some((candidate) => candidate !== part && candidate.no === part.no)) {
        throw new Error(`零件编号已重复：${part.no}`)
      }
      part.files = part.files ?? []
      part.hasFile = part.files.length > 0
      part.project = part.project || drawing.project
    }

    const idempotencyKey = `drawing-create:${drawing.no}:${Date.now()}:${Math.random().toString(36).slice(2, 10)}`
    const session = await dataManager.createUploadSession({
      kind: 'drawing-create',
      idempotencyKey,
      metadata: {
        drawing: {
          no: drawing.no,
          name: drawing.name,
          kind: drawing.kind,
          project: drawing.project,
          material: drawing.material,
          vendor: drawing.vendor,
          status: drawing.status,
          ver: drawing.ver,
          borrowFrom: drawing.borrowFrom,
          remark: drawing.remark,
          signers: drawing.signers,
          attributeValues: drawing.attributeValues,
        },
	        parts: structureParts.map((part) => ({
          no: part.no,
          name: part.name,
          parentNo: part.parentNo,
          project: part.project,
          material: part.material,
          spec: part.spec,
          weight: part.weight,
          surfaceTreatment: part.surfaceTreatment,
          partType: part.partType,
          qty: part.qty,
          status: part.status,
          ver: part.ver,
          vendor: part.vendor,
          borrowFrom: part.borrowFrom,
          remark: part.remark,
	          signers: part.signers,
	        })),
	        bom: (transactionContext.bom ?? bomItems.value)
	          .filter((item) => item.drawingNo === drawing.no || structurePartNos.has(item.drawingNo))
	          .map((item) => ({
	            ...item,
	            drawingNo: drawing.no,
	            ...(structurePartNos.has(item.drawingNo) ? { partNo: item.drawingNo } : {}),
	          })),
		borrows: [
			// 新建项目只接受本次提交明确传入的借用关系，绝不能把当前内存中
			// 其他项目的借用记录一并复制进新项目。
			...(transactionContext.borrows ?? [])
				.filter((item) => !item.targetDrawingNo || item.targetDrawingNo === drawing.no)
				.map((item) => ({
					direction: item.dir,
					sourceDrawingNo: item.sourceDrawingNo || item.project,
					sourcePartNo: item.dir === 'in' ? item.partNo : undefined,
					targetPartNo: item.dir === 'in' ? item.partNo : undefined,
					status: item.status === '已归档' ? 'archived' : 'active',
				})),
	          ...structureParts.filter((part) => part.borrowFrom).map((part) => ({
	            direction: 'in',
	            sourceDrawingNo: part.borrowFrom,
	            targetPartNo: part.no,
	            status: 'active',
	          })),
	        ],
	        branches: transactionContext.branches ?? [],
	      },
	    })
	    window.localStorage.setItem('cad:last-upload-session', session.id)
	    const recoveryEntries = new Map<string, { itemId: string; file: DrawingFile | MaterialFile | CraftFile; content: Blob }>()
	    pendingDrawingUploads.set(session.id, recoveryEntries)
	    pendingUploadSessionId.value = session.id

	    const uploadItems = new Map<string, { itemId: string; file: DrawingFile | MaterialFile | CraftFile }>()
	    for (const file of allFiles) {
		        const content = attachmentMap.get(file.id)
		        if (!content) throw new Error(`文件「${file.name}」缺少文件内容（上传会话：${session.id}）`)
	        await saveUploadRecoveryFile(session.id, file.id, content, file.name).catch((error) => {
	          console.warn(`保存文件「${file.name}」的刷新恢复副本失败`, error)
	        })
	        const isDrawing = isDrawingFile(file)
	        const role = isDrawing ? file.role : ('op' in file ? 'craft' : 'material')
	        // 客户端先算 SHA-256 做预检；预检失败时降级为普通上传，不能阻断业务。
	        let hashCheck: Awaited<ReturnType<typeof dataManager.checkUploadHash>>
	        try {
	          hashCheck = await dataManager.checkUploadHash(content)
	        } catch (hashError) {
	          console.warn(`文件「${file.name}」哈希预检失败，改为完整上传`, hashError)
	          hashCheck = { exists: false, sha256: '', size: content.size, mimeType: content.type || 'application/octet-stream' }
	        }
	        const item = await dataManager.createUploadSessionItem(session.id, {
        clientRef: file.id,
        drawingNo: drawing.no,
        ...(isDrawing && file.partNo ? { partNo: file.partNo } : {}),
	          role,
	          originalName: file.name,
	          mimeType: content.type || 'application/octet-stream',
	          sha256: hashCheck.sha256,
	          size: content.size,
	          // CAD 需要服务端生成处理对象，不能直接复用原始内容对象。
	          ...(!isCADFileName(file.name) && hashCheck.exists && hashCheck.blobId ? { blobId: hashCheck.blobId } : {}),
	      })
	      uploadItems.set(file.id, { itemId: item.id, file })
	      recoveryEntries.set(file.id, { itemId: item.id, file, content })
	    }

	    const failedFiles: string[] = []
    const queue = [...uploadItems.values()]
    const worker = async (): Promise<void> => {
      while (queue.length) {
        const entry = queue.shift()
        if (!entry) return
        const content = attachmentMap.get(entry.file.id)
	        if (!content) {
	          failedFiles.push(entry.file.name)
	          continue
	        }
		        const sessionItem = uploadItems.get(entry.file.id)
		        if (sessionItem && (await dataManager.getUploadSession(session.id)).items.find((item) => item.id === sessionItem.itemId)?.status === 'ready') {
		          uploadProgress.value = { ...uploadProgress.value, [sessionItem.itemId]: 100 }
		          continue
		        }
	        try {
					let hash = ''
					try {
						hash = (await dataManager.checkUploadHash(content)).sha256
					} catch (hashError) {
						console.warn(`文件「${entry.file.name}」哈希预检失败，改为完整上传`, hashError)
					}
					await uploadSessionFile(session.id, entry.itemId, content, entry.file.name, hash)
        } catch (uploadError) {
          failedFiles.push(`${entry.file.name}：${uploadError instanceof Error ? uploadError.message : String(uploadError)}`)
        }
      }
    }
    await Promise.all(Array.from({ length: Math.min(4, Math.max(queue.length, 1)) }, () => worker()))
    if (failedFiles.length) {
      throw new Error(`有 ${failedFiles.length} 个文件上传失败，可使用会话 ${session.id} 查询并单独重试：${failedFiles.join('；')}`)
    }

    await dataManager.commitUploadSession(session.id)
	    pendingDrawingUploads.delete(session.id)
	    await deleteUploadRecoverySession(session.id).catch(() => undefined)
    pendingUploadSessionId.value = null
    window.localStorage.removeItem('cad:last-upload-session')
    await reloadFromServer()
    recordActivity({
      drawingNo: drawing.no,
      drawingName: drawing.name,
      targetType: 'drawing',
      act: 'create',
      text: `新建图纸 ${drawing.no}`,
      detail: { partCount: structureParts.length, fileCount: allFiles.length, uploadSessionId: session.id },
    })
    await persist()
  }

	  async function retryFailedDrawingUpload(): Promise<string> {
    const sessionId = pendingUploadSessionId.value
    if (!sessionId) throw new Error('没有可恢复的上传会话')
    const entries = pendingDrawingUploads.get(sessionId)
    if (!entries) throw new Error('当前页面已无法恢复文件内容，请重新选择文件上传')
    const snapshot = await dataManager.getUploadSession(sessionId)
    const failed = snapshot.items.filter((item) => item.status === 'failed')
    if (!failed.length) {
      if (snapshot.items.some((item) => item.status !== 'ready')) throw new Error('仍有文件未准备完成')
	}

	    const errors: string[] = []
    for (const item of failed) {
      const entry = [...entries.values()].find((candidate) => candidate.itemId === item.id)
      if (!entry) {
        errors.push(`${item.originalName}：浏览器中已找不到原始文件`)
        continue
      }
		try {
			await dataManager.retryUploadSessionItem(sessionId, item.id)
			let hash = ''
			try {
				hash = (await dataManager.checkUploadHash(entry.content)).sha256
			} catch {
				// 服务端预检暂时不可用时，仍允许整文件重试；服务端会校验已登记的摘要。
			}
			await uploadSessionFile(sessionId, item.id, entry.content, entry.file.name, hash)
      } catch (retryError) {
        errors.push(`${entry.file.name}：${retryError instanceof Error ? retryError.message : String(retryError)}`)
      }
    }
    if (errors.length) throw new Error(`仍有 ${errors.length} 个文件上传失败：${errors.join('；')}`)
    const updated = await dataManager.getUploadSession(sessionId)
    if (updated.items.some((item) => item.status !== 'ready')) throw new Error('仍有文件未准备完成，请继续重试失败项')
    const result = await dataManager.commitUploadSession(sessionId)
	    pendingDrawingUploads.delete(sessionId)
	    await deleteUploadRecoverySession(sessionId).catch(() => undefined)
    pendingUploadSessionId.value = null
    window.localStorage.removeItem('cad:last-upload-session')
    await reloadFromServer()
    const drawingNo = typeof result.drawingNo === 'string' ? result.drawingNo : ''
    if (!drawingNo) throw new Error('项目提交成功但未返回图号')
	    return drawingNo
	  }

	  async function retryFailedDrawingUploadItem(itemId: string): Promise<void> {
	    const sessionId = pendingUploadSessionId.value
	    if (!sessionId) throw new Error('没有可恢复的上传会话')
	    const entries = pendingDrawingUploads.get(sessionId)
	    if (!entries) throw new Error('当前页面已无法恢复文件内容，请重新选择文件上传')
	    const snapshot = await dataManager.getUploadSession(sessionId)
	    const item = snapshot.items.find((candidate) => candidate.id === itemId)
	    if (!item) throw new Error('上传文件项不存在')
	    if (item.status === 'ready' || item.status === 'committed') return
	    const entry = [...entries.values()].find((candidate) => candidate.itemId === itemId)
	    if (!entry) throw new Error(`浏览器中已找不到原始文件：${item.originalName}`)
	    await dataManager.retryUploadSessionItem(sessionId, itemId)
	    let hash = ''
	    try {
	      hash = (await dataManager.checkUploadHash(entry.content)).sha256
	    } catch {
	      // 哈希预检不可用时继续整文件/分片上传，服务端仍会按会话中登记的摘要校验。
	    }
	    await uploadSessionFile(sessionId, itemId, entry.content, entry.file.name, hash)
	  }

		async function dismissPendingUploadSession(): Promise<void> {
			const sessionId = pendingUploadSessionId.value || window.localStorage.getItem('cad:last-upload-session')
			if (!sessionId) return
			pendingDrawingUploads.delete(sessionId)
			pendingUploadSessionId.value = null
			window.localStorage.removeItem('cad:last-upload-session')
			uploadProgress.value = {}
			await deleteUploadRecoverySession(sessionId).catch(() => undefined)
		}

		async function cancelPendingUploadSession(): Promise<void> {
			const sessionId = pendingUploadSessionId.value
			if (!sessionId) return
			try {
				await dataManager.cancelUploadSession(sessionId)
			} finally {
				await dismissPendingUploadSession()
			}
		}

	  async function forkDrawing(
    sourceNo: string,
    newDrawingNo: string,
    newProjectName: string,
    newVendor?: string,
    newRemark?: string,
	operator = authStore.currentUser?.displayName || '当前用户',
    newProjectNo?: string,
    newAttributeValues?: Record<string, string>,
  ): Promise<void> {
    await initialize()
    const sourceDrawing = drawings.value.find((item) => item.no === sourceNo)
    if (!sourceDrawing) throw new Error(`未找到源图纸：${sourceNo}`)
    if (drawings.value.some((item) => item.no === newDrawingNo)) {
      throw new Error(`新图号已存在：${newDrawingNo}`)
    }

    // 按图号族提取源零件（含子件），排除借用件（借用件属于源项目，不随分叉复制）。
    const sourceParts = structure.value.filter((part) =>
      !part.borrowFrom && (part.no.startsWith(`${sourceNo}-`) || part.parentNo === sourceNo || isSameDrawingFamily(part.no, sourceNo)))
    const partNoMap = new Map<string, string>()

    // 零件号映射：兼容前缀规则（JG-001-01）与族基座规则
    // （总图 JG9055e-50/32-00 的零件为去斜杠变体 JG9055e-5032-01，子件 JG9055e-5032-01-1）。
    const normSource = sourceNo.replace(/[/\\]/g, '')
    const sourceFamily = normSource.slice(0, Math.max(normSource.lastIndexOf('-'), 0))
    const newBase = newDrawingNo.replace(/[/\\]/g, '')
    const newFamily = newBase.slice(0, Math.max(newBase.lastIndexOf('-'), 0))
    const mapPartNo = (partNo: string): string => {
      const normPart = partNo.replace(/[/\\]/g, '')
      if (normPart.startsWith(`${normSource}-`)) return `${newDrawingNo}${normPart.slice(normSource.length)}`
      if (normPart.startsWith(`${sourceFamily}-`) || normPart === sourceFamily) return `${newFamily}${normPart.slice(sourceFamily.length)}`
      return `${newDrawingNo}-${partNo}`
    }

    for (const part of sourceParts) {
      const targetNo = mapPartNo(part.no)
      if (structure.value.some((p) => p.no === targetNo)) {
        throw new Error(`分叉生成的零件图号已存在：${targetNo}`)
      }
      partNoMap.set(part.no, targetNo)
    }

    const cloneFile = <T extends DrawingFile | MaterialFile | CraftFile>(file: T, drawingNo: string, partNo?: string): T => {
      const clone = JSON.parse(JSON.stringify(file)) as T
      clone.id = createId('fork-file')
      clone.drawingNo = drawingNo
      if ('role' in clone) {
        if (partNo) clone.partNo = partNo
        else delete clone.partNo
      }
      delete clone.storageKey
      if ('currentStorageKey' in clone) delete clone.currentStorageKey
      return clone
    }

    const forkedDrawing: Drawing = {
      ...JSON.parse(JSON.stringify(sourceDrawing)),
      no: newDrawingNo,
      name: newProjectName || `${sourceDrawing.name} (分叉)`,
      project: newProjectNo || sourceDrawing.project,
      vendor: newVendor ?? sourceDrawing.vendor,
      remark: newRemark ?? (sourceDrawing.remark ? `${sourceDrawing.remark} (分叉自 ${sourceNo})` : `分叉自 ${sourceNo}`),
      status: 'draft',
      ver: 'v1.0',
				forkedFrom: sourceNo,
				createdBy: operator,
      createdAt: nowLabel(),
      updatedBy: operator,
      updated: nowLabel(),
      borrow: 0,
      signers: { ...(sourceDrawing.signers ?? {}) },
      ...(newAttributeValues ? { attributeValues: { ...newAttributeValues } } : {}),
    }

    forkedDrawing.files = (sourceDrawing.files ?? []).map((file) => cloneFile(file, newDrawingNo))
    forkedDrawing.otherFiles = (sourceDrawing.otherFiles ?? []).map((file) => cloneFile(file, newDrawingNo))
    forkedDrawing.materialFiles = (sourceDrawing.materialFiles ?? []).map((file) => cloneFile(file, newDrawingNo))
    forkedDrawing.craftFiles = (sourceDrawing.craftFiles ?? []).map((file) => cloneFile(file, newDrawingNo))

    const forkedParts: StructurePart[] = sourceParts.map((part) => {
      const newPartNo = partNoMap.get(part.no) || `${newDrawingNo}-01`
      const newParentNo = part.parentNo === sourceNo ? newDrawingNo : (partNoMap.get(part.parentNo) || newDrawingNo)
      return {
        ...JSON.parse(JSON.stringify(part)),
        no: newPartNo,
        parentNo: newParentNo,
        project: newProjectNo || part.project,
        status: 'draft',
        ver: 'v1.0',
        forkedFrom: part.no,
        createdBy: operator,
        createdAt: nowLabel(),
        updatedBy: operator,
        updated: nowLabel(),
        signers: { ...(part.signers ?? {}) },
      }
    })

    forkedParts.forEach((part) => {
      const sourcePart = sourceParts.find((item) => item.no === part.forkedFrom)
      if (!sourcePart) return
      part.files = (sourcePart.files ?? []).map((file) => cloneFile(file, newDrawingNo, part.no))
      part.otherFiles = (sourcePart.otherFiles ?? []).map((file) => cloneFile(file, newDrawingNo, part.no))
      part.materialFiles = (sourcePart.materialFiles ?? []).map((file) => cloneFile(file, newDrawingNo, part.no))
      part.craftFiles = (sourcePart.craftFiles ?? []).map((file) => cloneFile(file, newDrawingNo, part.no))
    })

    // 分支记录与活动日志：随建档持久化一起落库。
    const branchRecord: Branch = {
      name: `${newDrawingNo} (${newProjectName || forkedDrawing.name})`,
      from: sourceNo,
      by: operator,
      date: nowLabel(),
      status: '使用中',
      desc: `从 ${sourceNo} 分叉生成全新项目工程`,
    }

	    // 克隆文件清单：源文件内容将逐个复制为新项目的附件。
    const fileCopies = [
      ...(sourceDrawing.files ?? []).map((source, index) => ({ source, target: forkedDrawing.files?.[index] })),
      ...(sourceDrawing.otherFiles ?? []).map((source, index) => ({ source, target: forkedDrawing.otherFiles?.[index] })),
      ...(sourceDrawing.materialFiles ?? []).map((source, index) => ({ source, target: forkedDrawing.materialFiles?.[index] })),
      ...(sourceDrawing.craftFiles ?? []).map((source, index) => ({ source, target: forkedDrawing.craftFiles?.[index] })),
      ...sourceParts.flatMap((sourcePart) => {
        const targetPart = forkedParts.find((item) => item.forkedFrom === sourcePart.no)
        if (!targetPart) return []
        return [
          ...(sourcePart.files ?? []).map((source, index) => ({ source, target: targetPart.files?.[index] })),
          ...(sourcePart.otherFiles ?? []).map((source, index) => ({ source, target: targetPart.otherFiles?.[index] })),
          ...(sourcePart.materialFiles ?? []).map((source, index) => ({ source, target: targetPart.materialFiles?.[index] })),
          ...(sourcePart.craftFiles ?? []).map((source, index) => ({ source, target: targetPart.craftFiles?.[index] })),
        ]
      }),
    ]

	    // 先读源文件内容，再交给 drawing-create 会话；项目元数据、结构和附件
	    // 将由同一个后端事务提交，避免旧的“先建档、再逐文件复制”残留半项目。
	    const forkAttachments: AttachmentInput[] = []
	    for (const copy of fileCopies) {
		      const sourceKey = ('currentStorageKey' in copy.source ? copy.source.currentStorageKey : undefined) || copy.source.storageKey
		      if (!copy.target || !sourceKey) continue
		      const content = await dataManager.readAttachment(sourceKey)
		      forkAttachments.push({ id: copy.target.id, content })
		    }
		    await addDrawing(forkedDrawing, forkedParts, forkAttachments, {
		      branches: [branchRecord],
		      bom: bomItems.value
		        .filter((item) => item.drawingNo === sourceNo || sourceParts.some((part) => part.no === item.drawingNo))
		        .map((item) => ({ ...item, drawingNo: newDrawingNo })),
		    })
	    branches.value.unshift(branchRecord)
	    recordActivity({
	      drawingNo: newDrawingNo,
	      drawingName: forkedDrawing.name,
	      targetType: 'branch',
	      act: 'branch',
	      text: `从图纸 <b>${sourceNo}</b> 分叉创建了新项目 <b>${newDrawingNo}</b>`,
	      detail: { sourceDrawingNo: sourceNo, newDrawingNo, partCount: forkedParts.length },
	    })
	    await persist()
  }

  async function createPartWithFile(
    parentNo: string,
    part: StructurePart,
    file: DrawingFile,
    content: Blob,
  ): Promise<void> {
    await initialize()
    const parent = findDrawingOrPart(parentNo)
    if (!parent) throw new Error(`未找到所属总图：${parentNo}`)
    if (structure.value.some((item) => item.no === part.no)) throw new Error(`零件编号已存在：${part.no}`)
    if (part.parentNo !== parentNo) throw new Error(`零件 ${part.no} 的所属总图不正确`)

    let storageKey: string | undefined
    try {
	      const commitResult = await uploadNewAttachmentSession(parentNo, file, content)
	      storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      file.storageKey = storageKey
       part.project = part.project || parent.project
      part.files = [file]
      part.hasFile = true
      structure.value.push(part)
      await persist()
      recordActivity({
        drawingNo: part.no,
        drawingName: part.name,
        targetType: 'part',
        act: 'create',
        text: `创建零件图 ${part.no}`,
        detail: { parentNo: parentNo, fileName: file.name },
      })
      await persist()
    } catch (saveError) {
      const insertedIndex = structure.value.findIndex((item) => item.no === part.no)
      if (insertedIndex >= 0) structure.value.splice(insertedIndex, 1)
      if (storageKey) await dataManager.deleteAttachment(storageKey).catch(() => undefined)
      throw saveError
    }
  }

  async function uploadDrawingFile(drawingNo: string, file: DrawingFile, content?: Blob): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    if ('parentNo' in target && file.role !== 'part') throw new Error('零件对象只能上传零件图文件')
    if (!('parentNo' in target) && file.role !== 'assembly') throw new Error('总图对象只能上传总图文件，请先创建零件关联')

    const files = getTargetFiles(target)
    if (files.some((item) => item.id === file.id)) throw new Error(`文件已存在：${file.name}`)
    const originalFiles = [...files]
    const originalHasFile = target.hasFile
    const originalPartNo = file.partNo
    let storageKey: string | undefined
    try {
	      const commitResult = await uploadNewAttachmentSession(drawingNo, file, content as Blob)
	      storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      file.storageKey = storageKey
      // 只有新上传 CAD 时读取一次标题栏材料，并随业务文档持久化。
      if ('parentNo' in target && content) {
        try {
          const identity = await dataManager.identifyDrawingMaterial(content, file.name)
          const material = identity.material
            || identity.titleBlock?.['材料名称']
            || identity.titleBlock?.['材料']
            || identity.titleBlock?.['材质']
          if (material) target.material = material
        } catch (scanError) {
          console.warn(`上传图纸后读取零件材料失败：${target.no}`, scanError)
        }
      }
      if ('parentNo' in target && !file.partNo) file.partNo = target.no
      files.push(file)
      target.hasFile = true
      if ('updated' in target) target.updated = nowLabel()
      await persist()
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'upload',
        text: `上传图纸文件 <b>${file.name}</b> 到 <b>${target.no}</b>`,
        detail: { fileId: file.id, fileName: file.name, version: file.version, role: file.role },
      })
      await persist()
    } catch (saveError) {
      target.files = originalFiles
      target.hasFile = originalHasFile
      if (originalPartNo) file.partNo = originalPartNo
      else delete file.partNo
      if (storageKey) await dataManager.deleteAttachment(storageKey).catch(() => undefined)
      throw saveError
    }
  }

  async function reidentifyDrawingFile(file: DrawingFile, newPartNo: string): Promise<void> {
    await initialize()
    const extension = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] || ''
    if (file.role === 'assembly' || !['.exb', '.dwg', '.dxf'].includes(extension) || !file.storageKey) {
      throw new Error('只有已关联存储位置的零件图支持重新识别')
    }
    const result = await dataManager.reidentifyDrawingFile(file.storageKey, newPartNo)
    const oldPart = structure.value.find((part) => part.no === result.oldPartNo)
    const targetPart = structure.value.find((part) => part.no === result.partNo)

    for (const drawing of drawings.value) {
      drawing.files = (drawing.files ?? []).filter((item) => item.id !== file.id)
      drawing.otherFiles = (drawing.otherFiles ?? []).filter((item) => item.id !== file.id)
    }
    for (const part of structure.value) {
      part.files = (part.files ?? []).filter((item) => item.id !== file.id)
      part.otherFiles = (part.otherFiles ?? []).filter((item) => item.id !== file.id)
      part.hasFile = part.files.length > 0
    }

    if (oldPart && targetPart && oldPart !== targetPart) {
      targetPart.files = [...(targetPart.files ?? []), { ...file, partNo: result.partNo }]
      targetPart.hasFile = true
    } else if (oldPart) {
      const oldPartNo = oldPart.no
      oldPart.no = result.partNo
      oldPart.parentNo = directParentDrawingNo(result.partNo) || oldPart.parentNo
      oldPart.files = (oldPart.files ?? []).map((item) => ({ ...item, partNo: result.partNo }))
      for (const child of structure.value) {
        if (child.parentNo === oldPartNo) child.parentNo = result.partNo
      }
      oldPart.files = [...(oldPart.files ?? []), { ...file, role: 'part', partNo: result.partNo }]
      oldPart.hasFile = true
    } else if (targetPart) {
      targetPart.files = [...(targetPart.files ?? []), { ...file, role: 'part', partNo: result.partNo }]
      targetPart.hasFile = true
    } else {
      const parentNo = directParentDrawingNo(result.partNo) || result.drawingNo
      const drawing = drawings.value.find((item) => item.no === result.drawingNo)
      structure.value.push({
        no: result.partNo,
        name: file.name.replace(/\.[^/.]+$/, ''),
        parentNo,
        project: drawing?.project || result.drawingNo,
        material: '—',
        spec: '',
        weight: 0,
        surfaceTreatment: '',
        partType: '自制件',
        qty: 1,
        status: 'draft',
        ver: file.version || 'v1.0',
        hasFile: true,
        files: [{ ...file, role: 'part', partNo: result.partNo }],
        otherFiles: [],
        materialFiles: [],
        craftFiles: [],
      })
    }
    file.partNo = result.partNo
    await persist()
  }

  // 借用其他项目的零件到当前项目（包含其图纸文件、属性，并建立双向借用追溯记录）
  async function borrowPartToProject(
    targetProjectNo: string,
    sourcePartNo: string,
    borrowReason?: string,
  ): Promise<StructurePart> {
    await initialize()
    const targetDrawing = findDrawingOrPart(targetProjectNo)
    if (!targetDrawing) throw new Error(`未找到目标项目：${targetProjectNo}`)
    const sourcePart = structure.value.find((p) => p.no === sourcePartNo)
    if (!sourcePart) throw new Error(`未找到源零件：${sourcePartNo}`)

    // 检查是否已经在目标项目中存在
    if (structure.value.some((p) => p.no === sourcePartNo && p.parentNo === targetProjectNo)) {
      throw new Error(`零件「${sourcePart.name} (${sourcePartNo})」已在当前项目中，无需重复借用`)
    }

    const operatorName = authStore.currentUser?.displayName || '当前用户'
    const today = new Date().toISOString().slice(0, 10)
    const findSourceDrawingNo = (partNo: string): string => {
      let currentNo = partNo
      const visited = new Set<string>()
      while (currentNo && !visited.has(currentNo)) {
        visited.add(currentNo)
        if (drawings.value.some((drawing) => drawing.no === currentNo)) return currentNo
        const parent = structure.value.find((part) => part.no === currentNo)
        if (!parent) break
        currentNo = parent.parentNo
      }
      return sourcePart.parentNo || ''
    }
    const sourceDrawingNo = findSourceDrawingNo(sourcePart.no)

    // 克隆源零件数据到目标项目
    const clonedFiles = (sourcePart.files ?? []).map((f) => ({
      ...f,
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      drawingNo: targetProjectNo,
      partNo: sourcePart.no,
    }))

    const borrowedPart: StructurePart = {
      ...sourcePart,
      parentNo: targetProjectNo,
      project: ('project' in targetDrawing ? targetDrawing.project : targetDrawing.name) || targetProjectNo,
      borrowFrom: sourcePart.parentNo || sourcePart.borrowFrom || '其他项目',
      files: clonedFiles,
      otherFiles: (sourcePart.otherFiles ?? []).map((f) => ({
        ...f,
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        drawingNo: targetProjectNo,
      })),
      hasFile: clonedFiles.length > 0,
      status: 'published',
    }

    structure.value.push(borrowedPart)

    // 记录借入与借出两条流转台账
    const sourceProjectName = drawings.value.find((d) => d.no === sourcePart.parentNo)?.name || sourcePart.parentNo
    const targetProjectName = targetDrawing.name || targetProjectNo

    const inRecord: BorrowRecord = {
      dir: 'in',
      project: `${sourceProjectName} (${sourcePart.parentNo})`,
      part: `${sourcePart.no} ${sourcePart.name}`,
      partNo: sourcePart.no,
      partName: sourcePart.name,
      sourceDrawingNo,
      targetDrawingNo: targetProjectNo,
      user: operatorName,
      date: today,
      status: '使用中',
    }

    const outRecord: BorrowRecord = {
      dir: 'out',
      project: `${targetProjectName} (${targetProjectNo})`,
      part: `${sourcePart.no} ${sourcePart.name}`,
      partNo: sourcePart.no,
      partName: sourcePart.name,
      sourceDrawingNo,
      targetDrawingNo: targetProjectNo,
      user: operatorName,
      date: today,
      status: '使用中',
    }

    borrows.value.unshift(inRecord, outRecord)

    // 增加源图与目标图的借用计数
    if ('borrow' in targetDrawing && typeof targetDrawing.borrow === 'number') {
      targetDrawing.borrow += 1
    }
    const sourceDrawing = drawings.value.find((d) => d.no === sourcePart.parentNo)
    if (sourceDrawing && typeof sourceDrawing.borrow === 'number') {
      sourceDrawing.borrow += 1
    }

    await persist()

    recordActivity({
      drawingNo: targetProjectNo,
      drawingName: targetDrawing.name,
      targetType: 'drawing',
      act: 'create',
      text: `借用了项目 <b>${sourceProjectName}</b> 的零件 <b>${sourcePart.name}</b> (${sourcePart.no})`,
      detail: { sourcePartNo, sourceParentNo: sourcePart.parentNo, reason: borrowReason || '' },
    })

    await persist()
    return borrowedPart
  }
  async function replaceDrawingFile(
    drawingNo: string,
    fileId: string,
    newFileInfo: {
      name: string
      size: string
      replaceReason?: string
    },
    content?: Blob,
  ): Promise<DrawingFile> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)

    // 查找目标文件（优先在 files，其次在 otherFiles）
    const isMainFiles = (target.files ?? []).some((f) => f.id === fileId)
    const fileList = isMainFiles ? (target.files ?? []) : (target.otherFiles ?? [])
    const currentFile = fileList.find((f) => f.id === fileId)
    if (!currentFile) throw new Error(`未找到待替换文件：${fileId}`)

    // 智能版本递进：v1.0 -> v1.1, v1.9 -> v2.0
    const parseVer = (vStr: string) => {
      const m = vStr.match(/v?(\d+)\.(\d+)/i)
      if (!m || !m[1] || !m[2]) return { major: 1, minor: 1 }
      const maj = parseInt(m[1], 10)
      const min = parseInt(m[2], 10)
      return { major: maj, minor: min + 1 }
    }
    const nextVerObj = parseVer(currentFile.version || 'v1.0')
    const nextVersion = `v${nextVerObj.major}.${nextVerObj.minor}`

    const operatorName = authStore.currentUser?.displayName || '当前用户'
    const replaceTime = nowLabel()
    const newStorageKey = createVersionStorageKey(currentFile.storageKey, newFileInfo.name)

    // 1. 将当前版本完整记录进历史归档数组
    const oldHistoryItem: import('@/types/domain.types').DrawingFileHistoryItem = {
      id: currentFile.id,
      name: currentFile.name,
      size: currentFile.size,
      version: currentFile.version,
      uploadedBy: currentFile.uploadedBy,
      uploadedAt: currentFile.uploadedAt,
      replacedBy: operatorName,
      replacedAt: replaceTime,
      replaceReason: newFileInfo.replaceReason || '版本替换更新',
      storageKey: currentFile.storageKey,
      mimeType: currentFile.mimeType,
      previewable: currentFile.previewable,
    }

    const previousHistory = currentFile.history ? [...currentFile.history] : []
    const updatedHistory = [...previousHistory, oldHistoryItem]

    // 2. 构造替换后的新 DrawingFile
    const updatedFile: DrawingFile = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      name: newFileInfo.name,
      size: newFileInfo.size,
      role: currentFile.role,
      drawingNo: currentFile.drawingNo,
      ...(currentFile.partNo ? { partNo: currentFile.partNo } : {}),
      version: nextVersion,
      uploadedBy: operatorName,
      uploadedAt: replaceTime,
      previewable: true,
      replaceReason: newFileInfo.replaceReason || '',
      replacedBy: operatorName,
      replacedAt: replaceTime,
      history: updatedHistory,
	      ...(newStorageKey ? { storageKey: newStorageKey } : {}),
	    }
	    if (!content) throw new Error(`文件「${newFileInfo.name}」缺少真实内容，无法替换`)

	    try {
	      const commitResult = await uploadReplacementSession(drawingNo, { ...currentFile, role: currentFile.role }, content)
	      updatedFile.storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      updatedFile.currentStorageKey = updatedFile.storageKey
	      if (typeof commitResult.version === 'string') updatedFile.version = commitResult.version

      // 只有替换产生新版本时重新读取标题栏材料，避免点击详情时重复请求。
      if ('parentNo' in target && content) {
        try {
          const identity = await dataManager.identifyDrawingMaterial(content, updatedFile.name)
          const material = identity.material
            || identity.titleBlock?.['材料名称']
            || identity.titleBlock?.['材料']
            || identity.titleBlock?.['材质']
          if (material) target.material = material
        } catch (scanError) {
          console.warn(`替换版本后读取零件材料失败：${target.no}`, scanError)
        }
      }

      // 更新在目标数组中的对象引用
      const idx = fileList.findIndex((f) => f.id === fileId)
      if (idx >= 0) {
        fileList[idx] = updatedFile
      }
      if ('updated' in target) target.updated = replaceTime
      await persist()

      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'edit',
        text: `替换了图纸文件 <b>${currentFile.name}</b> (${currentFile.version}) 为 <b>${updatedFile.name}</b> (${updatedFile.version})`,
        detail: {
          previousFileId: currentFile.id,
          newFileId: updatedFile.id,
          version: nextVersion,
          reason: newFileInfo.replaceReason || '',
        },
      })
      await persist()
      return updatedFile
    } catch (saveError) {
	      throw saveError
    }
  }

  async function uploadOtherFile(ownerNo: string, file: DrawingFile, content?: Blob): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(ownerNo)
    if (!target) throw new Error(`未找到其他文件所属对象：${ownerNo}`)
    const otherFiles = target.otherFiles ?? []
    if (otherFiles.some((item) => item.id === file.id)) throw new Error(`文件已存在：${file.name}`)

    const originalFiles = [...otherFiles]
    let storageKey: string | undefined
    try {
      file.role = 'other'
      file.drawingNo = ownerNo
      delete file.partNo
	      const commitResult = await uploadNewAttachmentSession(ownerNo, file, content as Blob)
	      storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      file.storageKey = storageKey
      target.otherFiles = [...otherFiles, file]
      await persist()
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'upload',
        text: `上传其他文件 <b>${file.name}</b> 到 <b>${target.no}</b>`,
        detail: { fileId: file.id, fileName: file.name, version: file.version, role: 'other' },
      })
      await persist()
    } catch (saveError) {
      target.otherFiles = originalFiles
      if (storageKey) await dataManager.deleteAttachment(storageKey).catch(() => undefined)
      throw saveError
    }
  }

  async function deleteOtherFile(ownerNo: string, fileId: string): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(ownerNo)
    if (!target) throw new Error(`未找到其他文件所属对象：${ownerNo}`)
    const otherFiles = target.otherFiles ?? []
    const file = otherFiles.find((item) => item.id === fileId)
    if (!file) throw new Error(`未找到其他文件：${fileId}`)
    target.otherFiles = otherFiles.filter((item) => item.id !== fileId)
    try {
      await persist()
      if (file.storageKey) await dataManager.deleteAttachment(file.storageKey)
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'delete',
        text: `删除其他文件 <b>${file.name}</b>`,
        detail: { fileId: file.id, fileName: file.name, role: 'other' },
      })
      await persist()
    } catch (deleteError) {
      target.otherFiles = otherFiles
      await persist().catch(() => undefined)
      throw deleteError
    }
  }

  async function deleteDrawingFile(drawingNo: string, fileId: string): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    const files = getTargetFiles(target)
    const file = files.find((item) => item.id === fileId)
    if (!file) throw new Error(`未找到图纸文件：${fileId}`)
    const originalFiles = [...files]
    const originalHasFile = target.hasFile
    target.files = files.filter((item) => item.id !== fileId)
    target.hasFile = target.files.length > 0
    try {
      await persist()
      if (file.storageKey) await dataManager.deleteAttachment(file.storageKey)
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'delete',
        text: `删除图纸文件 <b>${file.name}</b>`,
        detail: { fileId: file.id, fileName: file.name, role: file.role },
      })
      await persist()
    } catch (deleteError) {
      target.files = originalFiles
      target.hasFile = originalHasFile
      await persist().catch(() => undefined)
      throw deleteError
    }
  }

  async function uploadMaterialFile(
    drawingNo: string,
    file: MaterialFile,
    content?: Blob,
  ): Promise<MaterialUploadResult> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    file.drawingNo = drawingNo
    const attachments = getTargetAttachmentFiles(target)
    const originalMaterialFiles = [...attachments.materialFiles]
    const originalBom = [...bomItems.value]
    let storageKey: string | undefined
    try {
	      const commitResult = await uploadNewAttachmentSession(drawingNo, { ...file, role: 'material' }, content as Blob)
	      storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      file.storageKey = storageKey
      attachments.materialFiles.unshift(file)

      let importedCount = 0
      if (content) {
        const parseResult = await parseMaterialFileContent(content, file.name, drawingNo, file.id)
        if (parseResult.author) {
          file.author = parseResult.author
        }
        if (parseResult.items.length) {
          bomItems.value = [
            ...bomItems.value.filter((item) => item.drawingNo !== drawingNo),
            ...parseResult.items,
          ]
          importedCount = parseResult.items.length
        }
      }

      await persist()
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'upload',
        text: `上传备料表 <b>${file.name}</b>`,
        detail: { fileId: file.id, fileName: file.name, version: file.version, importedCount },
      })
      await persist()
      return { importedCount }
    } catch (saveError) {
      target.materialFiles = originalMaterialFiles
      bomItems.value = originalBom
      if (storageKey) await dataManager.deleteAttachment(storageKey).catch(() => undefined)
      throw saveError
    }
  }

  async function replaceMaterialFile(drawingNo: string, fileId: string, content: File): Promise<MaterialUploadResult> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    const attachments = getTargetAttachmentFiles(target)
    const currentFile = attachments.materialFiles.find((item) => item.id === fileId)
    if (!currentFile) throw new Error(`未找到备料表文件：${fileId}`)

    const originalFiles = [...attachments.materialFiles]
    const originalBom = [...bomItems.value]
    const updatedFile: MaterialFile = {
      ...currentFile,
      name: content.name,
      size: formatFileSize(content.size),
      uploadedBy: '张工',
      uploadedAt: nowLabel(),
      storageKey: undefined,
      mimeType: content.type || undefined,
    }

    try {
	      const commitResult = await uploadReplacementSession(drawingNo, { ...currentFile, role: 'material' }, content)
	      updatedFile.storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      if (typeof commitResult.version === 'string') updatedFile.version = commitResult.version
      const index = attachments.materialFiles.findIndex((item) => item.id === fileId)
      if (index >= 0) attachments.materialFiles[index] = updatedFile

      const parseResult = await parseMaterialFileContent(content, updatedFile.name, drawingNo, updatedFile.id)
      if (parseResult.author) {
        updatedFile.author = parseResult.author
      }
      if (parseResult.items.length) {
        bomItems.value = [
          ...bomItems.value.filter((item) => item.drawingNo !== drawingNo),
          ...parseResult.items,
        ]
      }
      await persist()
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'edit',
        text: `替换备料表 <b>${currentFile.name}</b> 为 <b>${updatedFile.name}</b>`,
        detail: { fileId, fileName: updatedFile.name, importedCount: parseResult.items.length },
      })
      await persist()
      return { importedCount: parseResult.items.length }
    } catch (replaceError) {
      target.materialFiles = originalFiles
      bomItems.value = originalBom
      await persist().catch(() => undefined)
      throw replaceError
    }
  }

  async function deleteMaterialFile(drawingNo: string, fileId: string): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    const attachments = getTargetAttachmentFiles(target)
    const file = attachments.materialFiles.find((item) => item.id === fileId)
    if (!file) throw new Error(`未找到备料表文件：${fileId}`)
    const originalMaterialFiles = [...attachments.materialFiles]
    const originalBom = [...bomItems.value]
    target.materialFiles = attachments.materialFiles.filter((item) => item.id !== fileId)
    bomItems.value = bomItems.value.filter((item) => item.sourceFileId !== fileId)
    try {
      await persist()
      if (file.storageKey) await dataManager.deleteAttachment(file.storageKey)
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'delete',
        text: `删除备料表 <b>${file.name}</b>`,
        detail: { fileId: file.id, fileName: file.name },
      })
      await persist()
    } catch (deleteError) {
      target.materialFiles = originalMaterialFiles
      bomItems.value = originalBom
      await persist().catch(() => undefined)
      throw deleteError
    }
  }

  async function parseMaterialFile(drawingNo: string, fileId: string): Promise<MaterialUploadResult> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    const file = getTargetAttachmentFiles(target).materialFiles.find((item) => item.id === fileId)
    if (!file) throw new Error(`未找到备料表文件：${fileId}`)
    if (!file.storageKey) throw new Error(`备料表「${file.name}」没有可读取的附件内容`)

    const content = await dataManager.readAttachment(file.storageKey)
    const parseResult = await parseMaterialFileContent(content, file.name, drawingNo, file.id)
    if (parseResult.author) {
      file.author = parseResult.author
    }
    bomItems.value = [
      ...bomItems.value.filter((item) => item.drawingNo !== drawingNo),
      ...parseResult.items,
    ]
    await persist()
    recordActivity({
      drawingNo: target.no,
      drawingName: target.name,
      targetType: 'file',
      act: 'parse',
      text: `解析备料表 <b>${file.name}</b>`,
      detail: { fileId: file.id, fileName: file.name, importedCount: parseResult.items.length },
    })
    await persist()
    return { importedCount: parseResult.items.length }
  }

  async function saveDrawingBom(drawingNo: string, items: BomItem[]): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    const originalBom = [...bomItems.value]
    bomItems.value = [
      ...bomItems.value.filter((item) => item.drawingNo !== drawingNo),
      ...items.map((item, index) => ({
        ...item,
        no: index + 1,
        drawingNo,
      })),
    ]
    try {
      await persist()
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'edit',
        text: `更新备料表数据（共 <b>${items.length}</b> 项）`,
        detail: { count: items.length },
      })
      await persist()
    } catch (saveError) {
      bomItems.value = originalBom
      await persist().catch(() => undefined)
      throw saveError
    }
  }

  async function uploadCraftFile(drawingNo: string, file: CraftFile, content?: Blob): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    file.drawingNo = drawingNo
    const attachments = getTargetAttachmentFiles(target)
    const originalCraftFiles = [...attachments.craftFiles]
    const originalGlobalCraftFiles = [...craftFiles.value]
    let storageKey: string | undefined
    try {
      if (content && file.name.toLowerCase().endsWith('.docx')) {
        try {
          file.author = await readDocxAuthor(content)
          file.scanned = true
        } catch (scanError) {
          file.scanned = false
          console.warn(`读取工艺文件编制人员失败：${file.name}`, scanError)
        }
      }
	      const commitResult = await uploadNewAttachmentSession(drawingNo, { ...file, role: 'craft' }, content as Blob)
	      storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      file.storageKey = storageKey
      attachments.craftFiles.unshift(file)
      craftFiles.value = [file, ...craftFiles.value.filter((item) => item.id !== file.id)]
      await persist()
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'upload',
        text: `上传工艺文件 <b>${file.name}</b>`,
        detail: { fileId: file.id, fileName: file.name, version: file.ver, operation: file.op },
      })
      await persist()
    } catch (saveError) {
      target.craftFiles = originalCraftFiles
      craftFiles.value = originalGlobalCraftFiles
      if (storageKey) await dataManager.deleteAttachment(storageKey).catch(() => undefined)
      throw saveError
    }
  }

  async function replaceCraftFile(drawingNo: string, fileId: string, content: File): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    const attachments = getTargetAttachmentFiles(target)
    const currentFile = attachments.craftFiles.find((item) => item.id === fileId)
      ?? craftFiles.value.find((item) => item.id === fileId && item.drawingNo === drawingNo)
    if (!currentFile) throw new Error(`未找到工艺文件：${fileId}`)

    const originalFiles = [...attachments.craftFiles]
    const originalGlobalFiles = [...craftFiles.value]
    const updatedFile: CraftFile = {
      ...currentFile,
      name: content.name,
      size: formatFileSize(content.size),
      by: '张工',
      date: nowLabel(),
      storageKey: undefined,
      mimeType: content.type || undefined,
      author: undefined,
      scanned: false,
    }

    try {
      if (content.name.toLowerCase().endsWith('.docx')) {
        try {
          updatedFile.author = await readDocxAuthor(content)
          updatedFile.scanned = true
        } catch (scanError) {
          console.warn(`替换工艺文件后读取编制人员失败：${content.name}`, scanError)
        }
      }
	      const commitResult = await uploadReplacementSession(drawingNo, { ...currentFile, role: 'craft' }, content)
	      updatedFile.storageKey = typeof commitResult.currentStorageKey === 'string'
	        ? commitResult.currentStorageKey
	        : typeof commitResult.storageKey === 'string' ? commitResult.storageKey : undefined
	      if (typeof commitResult.version === 'string') updatedFile.ver = commitResult.version
      const targetIndex = attachments.craftFiles.findIndex((item) => item.id === fileId)
      if (targetIndex >= 0) attachments.craftFiles[targetIndex] = updatedFile
      const globalIndex = craftFiles.value.findIndex((item) => item.id === fileId && item.drawingNo === drawingNo)
      if (globalIndex >= 0) craftFiles.value[globalIndex] = updatedFile
      await persist()
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'edit',
        text: `替换工艺文件 <b>${currentFile.name}</b> 为 <b>${updatedFile.name}</b>`,
        detail: { fileId, fileName: updatedFile.name, author: updatedFile.author },
      })
      await persist()
    } catch (replaceError) {
      target.craftFiles = originalFiles
      craftFiles.value = originalGlobalFiles
      await persist().catch(() => undefined)
      throw replaceError
    }
  }

  async function deleteCraftFile(drawingNo: string, fileId: string): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸或零件：${drawingNo}`)
    const attachments = getTargetAttachmentFiles(target)
    const file = attachments.craftFiles.find((item) => item.id === fileId)
      ?? craftFiles.value.find((item) => item.id === fileId && item.drawingNo === drawingNo)
    if (!file) throw new Error(`未找到工艺文件：${fileId}`)
    const originalCraftFiles = [...attachments.craftFiles]
    const originalGlobalCraftFiles = [...craftFiles.value]
    target.craftFiles = attachments.craftFiles.filter((item) => item.id !== fileId)
    craftFiles.value = craftFiles.value.filter((item) => item.id !== fileId)
    try {
      await persist()
      if (file.storageKey) await dataManager.deleteAttachment(file.storageKey)
      recordActivity({
        drawingNo: target.no,
        drawingName: target.name,
        targetType: 'file',
        act: 'delete',
        text: `删除工艺文件 <b>${file.name}</b>`,
        detail: { fileId: file.id, fileName: file.name },
      })
      await persist()
    } catch (deleteError) {
      target.craftFiles = originalCraftFiles
      craftFiles.value = originalGlobalCraftFiles
      await persist().catch(() => undefined)
      throw deleteError
    }
  }

  async function downloadAttachment(file: DrawingFile | MaterialFile | CraftFile): Promise<void> {
    if (!file.storageKey) throw new Error(`文件「${file.name}」没有可用的存储键`)
    const content = await dataManager.readAttachment(file.storageKey)
    const url = URL.createObjectURL(content)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = file.name
    anchor.click()
    URL.revokeObjectURL(url)
    recordActivity({
      drawingNo: ('partNo' in file && file.partNo) ? file.partNo : file.drawingNo,
      targetType: 'file',
      act: 'download',
      text: `下载文件 <b>${file.name}</b>`,
      detail: { fileId: file.id, fileName: file.name },
    })
    await persist()
  }

  // 存档 / 解除存档：生产 ⇄ 存档。权威校验在后端（创建者或管理员存档，管理员解档）。
  async function setDrawingArchived(drawingNo: string, archived: boolean): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸：${drawingNo}`)
    const item = archived
      ? await drawingLifecycleService.archive(drawingNo)
      : await drawingLifecycleService.unarchive(drawingNo)
    target.status = (item.status as Drawing['status']) ?? (archived ? 'archived' : 'published')
    recordActivity({
      drawingNo,
      drawingName: target.name,
      targetType: 'drawing',
      act: 'edit',
      text: archived ? `将图纸 <b>${drawingNo}</b> 存档归档` : `解除图纸 <b>${drawingNo}</b> 的存档`,
    })
    await persist()
  }

  // 节点责任人是否为当前登录人（优先按后端分配的用户 ID 匹配，姓名/账号兜底）。
  function isNodeAssignee(user: string | undefined, assignedUserId?: string): boolean {
    const current = authStore.currentUser
    if (!current) return false
    if (assignedUserId) return assignedUserId === current.id
    if (!user) return false
    return user === current.displayName || user === current.account
  }

  async function startReview(drawingNo: string, _initiator?: string): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到待审核对象：${drawingNo}`)

    // 审核案例权威数据在 Go 后端（总图与零件统一处理），由后端强制顺序与责任人。
    await reviewCaseService.start(drawingNo)
    const refreshed = await reviewCaseService.list().catch(() => [])
    const created = refreshed.find((item) => item.drawingNo === drawingNo && item.status === 'reviewing')
    if (created) {
      // 用后端节点责任人同步图纸签署栏展示。
      const signerMap: Partial<DrawingSigners> = {}
      for (const node of created.nodes) {
        const role = signerRoleForNode(node.name, '')
        if (role) signerMap[role as keyof DrawingSigners] = node.assignedName
      }
      target.signers = { ...(target.signers ?? {}), ...signerMap }
    }
    target.status = 'reviewing'
    await persist()
    await refreshReviewData()
  }

  async function submitNodeReview(
    drawingNo: string,
    nodeName: string,
    action: 'pass' | 'rejected',
    opinion: string,
    reviewer = '当前审核人',
    reviewCaseId?: string,
  ): Promise<void> {
    await initialize()
    const reviewCase = getReviewCase(drawingNo, reviewCaseId)
    if (!reviewCase) throw new Error(`未找到图纸 ${drawingNo} 的审核流程`)
    const node = reviewCase.nodes.find((item) => item.name === nodeName)
    if (!node) throw new Error(`未找到审核节点：${nodeName}`)
    if (node.status === 'pass' && action === 'pass') throw new Error(`审核节点已完成：${nodeName}`)
    if (node.status === 'pending') {
      // 顺序守卫：只允许处理顺序最靠前的待处理节点。
      const activeNode = reviewCase.nodes
        .filter((item) => item.status === 'pending')
        .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))[0]
      if (!activeNode || activeNode.name !== nodeName) {
        throw new Error(
          activeNode
            ? `审核须按顺序流转：请先处理当前节点「${activeNode.name}」`
            : '审核流程当前没有可处理的节点',
        )
      }
      // 责任人守卫：仅节点责任人可签署，防止代签。
      if (!isNodeAssignee(node.user, node.assignedUserId)) {
        throw new Error(`节点「${nodeName}」由「${node.user || '待定'}」负责，当前登录人无权签署`)
      }
    }

    // 签署权威校验与落库在 Go 后端：顺序、责任人、状态推进全部由后端事务保证。
    await reviewCaseService.submit(reviewCase.id, nodeName, action, opinion)

    const target = findDrawingOrPart(drawingNo)
    if (target) {
      recordActivity({
        drawingNo,
        drawingName: target.name,
        targetType: 'review',
        act: 'check',
        text: `审核图纸 <b>${drawingNo}</b>（节点：${nodeName}）：${action === 'pass' ? '通过' : '驳回'}`,
        detail: { reviewCaseId: reviewCase.id, nodeName, result: action },
      })
      await persist()
    }
    await refreshReviewData()
  }

  async function setReviewNodeStatus(name: string, status: 'pass' | 'pending' | 'rejected', opinion = ''): Promise<void> {
    await initialize()
    const reviewCase = currentReviewCase.value
    const node = reviewCase?.nodes.find((item) => item.name === name)
    if (!node || !reviewCase) return
    node.status = status
    node.time = status === 'pass' ? nowLabel() : '—'
    node.opinion = opinion
    await persist()
  }

  async function updateStructurePart(partNo: string, payload: StructurePartEditable): Promise<void> {
    await initialize()
    const part = structure.value.find((item) => item.no === partNo)
    if (!part) throw new Error(`未找到零件图：${partNo}`)
    if (!findDrawingOrPart(part.parentNo)) {
      throw new Error(`零件 ${partNo} 未关联有效父级图纸`)
    }

    const name = payload.name.trim()
    const nextPartNo = payload.partNo?.trim() || part.no
    const material = payload.material.trim()
    const spec = payload.spec.trim()
    const surfaceTreatment = payload.surfaceTreatment.trim()
    const vendor = payload.vendor?.trim() || undefined
    const remark = payload.remark?.trim() || undefined
    if (!name) throw new Error('请输入零件名称')
    if (!nextPartNo) throw new Error('请输入零件图号')
    if (nextPartNo !== part.no && structure.value.some((item) => item !== part && item.no === nextPartNo)) {
      throw new Error(`零件编号已存在：${nextPartNo}`)
    }
    if (!material) throw new Error('请输入材料牌号')
    if (!Number.isFinite(payload.qty) || payload.qty <= 0) throw new Error('装配数量必须大于 0')
    if (!Number.isFinite(payload.weight) || payload.weight < 0) throw new Error('理论重量不能小于 0')
    if (!part.id || !part.revision) throw new Error('零件缺少服务端版本信息，请刷新后重试')

    await dataManager.updatePart(part.id, {
      expectedRevision: part.revision,
      ...(nextPartNo !== part.no ? { no: nextPartNo } : {}),
      name,
      material,
      spec,
      weight: payload.weight,
      surfaceTreatment,
      partType: payload.partType,
      qty: payload.qty,
      ...(vendor ? { vendor } : {}),
      ...(remark ? { remark } : {}),
    })
    await reloadFromServer()
    recordActivity({
      drawingNo: nextPartNo,
      drawingName: name,
      targetType: 'part',
      act: 'edit',
      text: `更新零件图 <b>${nextPartNo}</b> 属性：材料 ${material} · 规格 ${spec || '未填写'} · 数量 ×${payload.qty}`,
      detail: { changedFields: payload, ...(nextPartNo !== partNo ? { oldPartNo: partNo, newPartNo: nextPartNo } : {}) },
    })
  }

  async function refreshUsers(): Promise<void> {
    if (!authStore.hasRole('admin')) return
    try {
      users.value = await dataManager.listUsers()
    } catch (error) {
      console.warn('刷新账号列表失败', error)
    }
  }

  async function createUser(
    account: string,
    displayName: string,
    password: string,
    roles: UserRole[],
  ): Promise<void> {
    await initialize()
    const normalizedAccount = account.trim().toLowerCase()
    const normalizedName = displayName.trim()
    const normalizedPassword = password.trim()
    const normalizedRoles = [...new Set(roles)]
    if (!normalizedAccount) throw new Error('请输入登录账号')
    if (!normalizedName) throw new Error('请输入用户姓名')
    if (!normalizedPassword) throw new Error('请输入初始密码')
    if (!normalizedRoles.length) throw new Error('请至少选择一个角色')
    if (users.value.some((user) => user.account.toLowerCase() === normalizedAccount)) {
      throw new Error(`账号已存在：${normalizedAccount}`)
    }

    const user = await dataManager.createUser({
      account: normalizedAccount,
      displayName: normalizedName,
      password: normalizedPassword,
      roles: normalizedRoles,
    })
    users.value.unshift(user)
  }

  async function resetUserPassword(userId: string, password: string): Promise<void> {
    await initialize()
    const user = users.value.find((item) => item.id === userId)
    if (!user) throw new Error('未找到目标账号')
    const normalizedPassword = password.trim()
    if (!normalizedPassword) throw new Error('请输入新密码')
    const updated = await dataManager.updateUser(userId, {
      account: user.account,
      displayName: user.displayName,
      password: normalizedPassword,
      roles: user.roles,
      status: user.status,
    })
    Object.assign(user, updated)
  }

  async function toggleUser(userId: string): Promise<void> {
    await initialize()
    const user = users.value.find((item) => item.id === userId)
    if (!user) throw new Error('未找到目标账号')
    if (user.status === 'active' && user.roles.includes('admin')) {
      const activeAdminCount = users.value.filter((item) => item.status === 'active' && item.roles.includes('admin')).length
      if (activeAdminCount <= 1) throw new Error('不能禁用最后一个管理员账号')
    }

    const updated = await dataManager.updateUser(userId, {
      account: user.account,
      displayName: user.displayName,
      roles: user.roles,
      status: user.status === 'active' ? 'disabled' : 'active',
    })
    Object.assign(user, updated)
  }

  async function toggleFlow(index: number): Promise<void> {
    await initialize()
    const flow = flows.value[index]
    if (flow) {
      flow.on = !flow.on
      await persist()
    }
  }


  return {
    drawings,
    attributes,
    sortedAttributes,
    structure,
    versions,
    branches,
    borrows,
    bom,
    crafts,
    storedAttachments,
    logs,
    reviewCases,
    currentReviewNodes,
    myReviews,
    completedReviews,
    users,
    flows,
    adminLogs,
    myPendingReviews,
    currentDrawing,
    currentReviewCase,
    selectedStructureIndex,
    treeOpen,
    initialized,
    loading,
    error,
    reviewCount,
    drawingStats,
    initialize,
    pendingUploadSessionId,
    uploadProgress,
    reloadFromServer,
    refreshDrawingDesigner,
    recordActivity,
    recordActivityAndPersist,
    openDrawing,
    clearCurrentDrawing,
    getReviewCase,
    addDrawing,
			retryFailedDrawingUpload,
			retryFailedDrawingUploadItem,
		dismissPendingUploadSession,
			cancelPendingUploadSession,
    attributeName,
    attributeFieldName,
    validateAttributeValues,
    addAttribute,
    updateAttribute,
    deleteAttribute,
    addAttributeField,
    updateAttributeField,
    reorderAttributeFields,
    setDrawingAttributes,
    forkDrawing,
    createPartWithFile,
    borrowPartToProject,
    uploadDrawingFile,
    reidentifyDrawingFile,
    replaceDrawingFile,
    uploadOtherFile,
    deleteOtherFile,
    deleteDrawingFile,
    uploadMaterialFile,
    replaceMaterialFile,
    deleteMaterialFile,
    parseMaterialFile,
    saveDrawingBom,
    uploadCraftFile,
    replaceCraftFile,
    deleteCraftFile,
    downloadAttachment,
    startReview,
    submitNodeReview,
    setDrawingArchived,
    setReviewNodeStatus,
    updateStructurePart,
    refreshUsers,
    createUser,
    resetUserPassword,
    toggleUser,
    toggleFlow,
  }
})

// 兼容迁移期的旧命名，业务页面逐步切换到正式名称。
export const useCadDemoStore = useDomainStore
export const useDemoStore = useDomainStore
