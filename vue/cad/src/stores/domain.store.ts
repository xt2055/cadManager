import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { dataManager } from '@/services/data-manager'
import { readDocxAuthor } from '@/utils/docx-metadata'
import { parseMaterialFileContent } from '@/utils/material-table-parser'
import { useAuthStore } from '@/stores/auth.store'
import { createDrawingOperationLog, listDrawingOperationLogs } from '@/services/drawing-operation-log.service'
import type { DataDocument } from '@/services/data-manager'
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
  DrawingFile,
  DrawingSigners,
  DrawingVersion,
  HiddenObject,
  MaterialFile,
  MyReview,
  ReviewCase,
  ReviewFlow,
  ReviewNode,
  StructurePart,
  StructurePartEditable,
  SignerAssignments,
  UserAccount,
  UserRole,
} from '@/types/domain.types'

export const STATUS = {
  published: { t: '已发布', c: 'ok' },
  reviewing: { t: '审核中', c: 'info' },
  draft: { t: '草稿', c: 'mute' },
  hidden: { t: '已隐藏', c: 'danger' },
  disabled: { t: '已禁用', c: 'danger' },
} as const

const REQUIRED_SIGNER_ROLES = ['设计', '校对', '审核', '工艺', '批准'] as const

interface AttachmentInput {
  id: string
  content: Blob
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

function activityTime(): string {
  return new Date().toISOString()
}

export const useDomainStore = defineStore('domain', () => {
	const authStore = useAuthStore()
  const drawings = ref<Drawing[]>([])
  const structure = ref<StructurePart[]>([])
  const versions = ref<DrawingVersion[]>([])
  const branches = ref<Branch[]>([])
  const borrows = ref<BorrowRecord[]>([])
  const bomItems = ref<BomItem[]>([])
  const craftFiles = ref<CraftFile[]>([])
  const logs = ref<ActivityLog[]>([])
  const reviewCases = ref<ReviewCase[]>([])
  const myReviews = ref<MyReview[]>([])
  const completedReviews = ref<CompletedReview[]>([])
  const users = ref<UserAccount[]>([])
  const flows = ref<ReviewFlow[]>([])
  const hiddenList = ref<HiddenObject[]>([])
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
  const reviewCount = computed(() => myReviews.value.length)
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
      user: operator?.displayName || '当前用户',
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

  function toDocument(): DataDocument {
    return {
      version: 2,
      drawings: drawings.value,
      structure: structure.value,
      versions: versions.value,
      branches: branches.value,
      borrows: borrows.value,
      bom: bomItems.value,
      crafts: craftFiles.value,
      logs: logs.value,
      reviewCases: reviewCases.value,
      myReviews: myReviews.value,
      completedReviews: completedReviews.value,
      users: users.value,
      flows: flows.value,
      hiddenList: hiddenList.value,
      adminLogs: adminLogs.value,
    }
  }

  function persist(): Promise<void> {
    const saveOperation = saveQueue
      .catch(() => undefined)
      .then(() => dataManager.save(toDocument()))
    saveQueue = saveOperation.catch(() => undefined)

    return saveOperation.catch((saveError: unknown) => {
      error.value = saveError instanceof Error ? saveError.message : String(saveError)
      console.error('保存业务数据失败', saveError)
      throw saveError
    })
  }

  async function recordActivityAndPersist(input: Parameters<typeof recordActivity>[0]): Promise<void> {
    recordActivity(input)
    await persist()
  }

  function initialize(): Promise<void> {
    if (initialized.value) return Promise.resolve()
    if (initializationPromise) return initializationPromise

    loading.value = true
    error.value = null
    initializationPromise = (async () => {
      try {
        const document = await dataManager.load()
        drawings.value = document.drawings
        structure.value = document.structure
        versions.value = document.versions
        branches.value = document.branches
        borrows.value = document.borrows
        bomItems.value = document.bom
        craftFiles.value = document.crafts
        logs.value = document.logs
        reviewCases.value = document.reviewCases
        myReviews.value = document.myReviews
        completedReviews.value = document.completedReviews
        users.value = document.users
        flows.value = document.flows
        hiddenList.value = document.hiddenList
        adminLogs.value = document.adminLogs
        await scanUnscannedCraftFiles()
        await persist()
        await loadRemoteActivityLogs()
        initialized.value = true
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

    // 自动修复历史文件中显示为“刚刚”的上传时间
    const formatTimeStr = (t: string) => (t === '刚刚' ? nowLabel() : t)
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
    for (let index = reviewCases.value.length - 1; index >= 0; index -= 1) {
      const reviewCase = reviewCases.value[index]
      if (reviewCase?.drawingNo === drawingNo) return reviewCase
    }
    return null
  }

  function getSigners(target: Drawing | StructurePart): Partial<DrawingSigners> {
    return target.signers ?? {}
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

  async function saveAttachmentContent(file: DrawingFile | MaterialFile | CraftFile, content: Blob | undefined): Promise<string | undefined> {
    if (!content) {
      if (file.storageKey) return file.storageKey
      throw new Error(`文件「${file.name}」缺少真实内容，无法保存`)
    }
    const result = await dataManager.uploadAttachment(content, {
      name: file.name,
      mimeType: content.type,
      storageKey: file.storageKey,
      drawingNo: file.drawingNo,
      ...(isDrawingFile(file)
        ? { partNo: file.partNo, role: file.role, version: file.version, previewable: file.previewable }
        : 'op' in file
          ? { role: 'craft' as const, version: file.ver, previewable: file.previewable }
          : { role: 'material' as const, version: file.version, previewable: true }),
    })
    file.storageKey = result.storageKey
    file.mimeType = result.mimeType
    return result.storageKey
  }

  function isDrawingFile(file: DrawingFile | MaterialFile | CraftFile): file is DrawingFile {
    return 'role' in file
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
      void persist()
    }
  }

  function clearCurrentDrawing() {
    currentDrawing.value = null
    selectedStructureIndex.value = 0
  }

  async function addDrawing(
    drawing: Drawing,
    structureParts: StructurePart[] = [],
    attachments: AttachmentInput[] = [],
  ): Promise<void> {
    await initialize()
    if (drawings.value.some((item) => item.no === drawing.no)) {
      throw new Error(`图纸编号已存在：${drawing.no}`)
    }

    const attachmentMap = new Map(attachments.map((item) => [item.id, item.content]))
    const uploadedKeys: string[] = []
    const allFiles = [
      ...(drawing.files ?? []),
      ...(drawing.otherFiles ?? []),
      ...structureParts.flatMap((part) => part.files ?? []),
      ...structureParts.flatMap((part) => part.otherFiles ?? []),
    ]

    const originalDrawings = [...drawings.value]
    const originalStructure = [...structure.value]
    let activityLog: ActivityLog | null = null
    try {
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

      drawings.value.unshift(drawing)
      structure.value.push(...structureParts)
      await persist()

      for (const file of allFiles) {
        const storageKey = await saveAttachmentContent(file, attachmentMap.get(file.id))
        if (storageKey) uploadedKeys.push(storageKey)
      }
      await persist()
      activityLog = recordActivity({
        drawingNo: drawing.no,
        drawingName: drawing.name,
        targetType: 'drawing',
        act: 'create',
        text: `新建图纸 ${drawing.no}`,
        detail: { partCount: structureParts.length, fileCount: allFiles.length },
      })
      await persist()
    } catch (saveError) {
      drawings.value = originalDrawings
      structure.value = originalStructure
      const failedActivityId = activityLog?.id
      if (failedActivityId) logs.value = logs.value.filter((item) => item.id !== failedActivityId)
      await persist().catch(() => undefined)
      await Promise.all(uploadedKeys.map((storageKey) => dataManager.deleteAttachment(storageKey).catch(() => undefined)))
      throw saveError
    }
  }

  async function forkDrawing(
    sourceNo: string,
    newDrawingNo: string,
    newProjectName: string,
    newVendor?: string,
    newRemark?: string,
	operator = authStore.currentUser?.displayName || '当前用户',
  ): Promise<void> {
    await initialize()
    const sourceDrawing = drawings.value.find((item) => item.no === sourceNo)
    if (!sourceDrawing) throw new Error(`未找到源图纸：${sourceNo}`)
    if (drawings.value.some((item) => item.no === newDrawingNo)) {
      throw new Error(`新图号已存在：${newDrawingNo}`)
    }

    const sourceParts = structure.value.filter((part) => part.no.startsWith(`${sourceNo}-`) || part.parentNo === sourceNo)
    const partNoMap = new Map<string, string>()
    const uploadedKeys: string[] = []

    for (const part of sourceParts) {
      const suffix = part.no.startsWith(`${sourceNo}-`) ? part.no.slice(sourceNo.length) : `-${part.no}`
      const targetNo = `${newDrawingNo}${suffix}`
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
      return clone
    }

    const forkedDrawing: Drawing = {
      ...JSON.parse(JSON.stringify(sourceDrawing)),
      no: newDrawingNo,
      name: newProjectName || `${sourceDrawing.name} (分叉)`,
      project: newProjectName || sourceDrawing.project,
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
        project: newProjectName || part.project,
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

    const branchRecord: Branch = {
      name: `${newDrawingNo} (${newProjectName || forkedDrawing.name})`,
      from: sourceNo,
      by: operator,
      date: nowLabel(),
      status: '使用中',
      desc: `从 ${sourceNo} 分叉生成全新项目工程`,
    }

    const activityLog = recordActivity({
      drawingNo: newDrawingNo,
      drawingName: forkedDrawing.name,
      targetType: 'branch',
      act: 'branch',
      text: `从图纸 <b>${sourceNo}</b> 分叉创建了新项目 <b>${newDrawingNo}</b>`,
      detail: { sourceDrawingNo: sourceNo, newDrawingNo, partCount: forkedParts.length },
    })

    drawings.value.unshift(forkedDrawing)
    structure.value.push(...forkedParts)
    branches.value.unshift(branchRecord)

    try {
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

      for (const copy of fileCopies) {
        if (!copy.target || !copy.source.storageKey) continue
        const content = await dataManager.readAttachment(copy.source.storageKey)
        const storageKey = await saveAttachmentContent(copy.target, content)
        if (storageKey) uploadedKeys.push(storageKey)
      }
      await persist()
    } catch (saveError) {
      const idx = drawings.value.findIndex((item) => item.no === newDrawingNo)
      if (idx >= 0) drawings.value.splice(idx, 1)
      const partNos = new Set(forkedParts.map((p) => p.no))
      structure.value = structure.value.filter((p) => !partNos.has(p.no))
      branches.value = branches.value.filter((b) => b !== branchRecord)
      logs.value = logs.value.filter((item) => item.id !== activityLog.id)
      await persist().catch(() => undefined)
      await Promise.all(uploadedKeys.map((key) => dataManager.deleteAttachment(key).catch(() => undefined)))
      throw saveError
    }
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
      storageKey = await saveAttachmentContent(file, content)
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
      storageKey = await saveAttachmentContent(file, content)
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
      user: operatorName,
      date: today,
      status: '使用中',
    }

    const outRecord: BorrowRecord = {
      dir: 'out',
      project: `${targetProjectName} (${targetProjectNo})`,
      part: `${sourcePart.no} ${sourcePart.name}`,
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
    }

    let storageKey: string | undefined
    try {
      storageKey = await saveAttachmentContent(updatedFile, content)

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
      if (storageKey) await dataManager.deleteAttachment(storageKey).catch(() => undefined)
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
      storageKey = await saveAttachmentContent(file, content)
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
      storageKey = await saveAttachmentContent(file, content)
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
    const oldStorageKey = currentFile.storageKey
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
      await saveAttachmentContent(updatedFile, content)
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
      if (oldStorageKey) await dataManager.deleteAttachment(oldStorageKey)
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
      storageKey = await saveAttachmentContent(file, content)
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
    const oldStorageKey = currentFile.storageKey
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
      await saveAttachmentContent(updatedFile, content)
      const targetIndex = attachments.craftFiles.findIndex((item) => item.id === fileId)
      if (targetIndex >= 0) attachments.craftFiles[targetIndex] = updatedFile
      const globalIndex = craftFiles.value.findIndex((item) => item.id === fileId && item.drawingNo === drawingNo)
      if (globalIndex >= 0) craftFiles.value[globalIndex] = updatedFile
      await persist()
      if (oldStorageKey) await dataManager.deleteAttachment(oldStorageKey)
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

  async function startReview(drawingNo: string, initiator?: string): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到待审核对象：${drawingNo}`)
    const signers = getSigners(target)
    const missingRoles = REQUIRED_SIGNER_ROLES.filter((role) => {
      const user = signers[role]
      return !user || user === '待定'
    })
    if (missingRoles.length) {
      throw new Error(`无法发起审核，缺少签署人员：${missingRoles.join('、')}`)
    }

    const reviewCaseId = createId('review-case')
    const designUser = signers['设计'] ?? initiator ?? '当前用户'
    const nodes: ReviewNode[] = [
      { name: '设计自检', user: designUser, status: 'pass', time: nowLabel(), opinion: '设计完成并自检通过，发起审核流转。', required: true, order: 1 },
      { name: '校对复核', user: signers['校对'] ?? '待定', status: 'pending', time: '—', opinion: '', required: true, order: 2 },
      { name: '专业审核', user: signers['审核'] ?? '待定', status: 'pending', time: '—', opinion: '', required: true, order: 3 },
      { name: '工艺会签', user: signers['工艺'] ?? '待定', status: 'pending', time: '—', opinion: '', required: true, order: 4 },
      { name: '标准化审查', user: signers['标准化'] ?? '待定', status: 'pending', time: '—', opinion: '', required: false, order: 5 },
      { name: '主管批准', user: signers['批准'] ?? '待定', status: 'pending', time: '—', opinion: '', required: true, order: 6 },
    ]
    const reviewCase: ReviewCase = {
      id: reviewCaseId,
      drawingNo,
      flow: '企业标准图纸审核流程',
      status: 'reviewing',
      initiator: designUser,
      startedAt: nowLabel(),
      nodes,
    }

    reviewCases.value.push(reviewCase)
    target.status = 'reviewing'
    myReviews.value = myReviews.value.filter((item) => item.no !== drawingNo)
    nodes.filter((node) => node.status === 'pending').forEach((node) => {
      myReviews.value.push({
        reviewCaseId,
        no: drawingNo,
        name: target.name,
        node: node.name,
        by: designUser,
        time: nowLabel(),
      })
    })
    recordActivity({
      drawingNo,
      drawingName: target.name,
      targetType: 'review',
      act: 'check',
      text: `为图纸 <b>${drawingNo}</b> 发起了图纸审核流程`,
      detail: { reviewCaseId },
    })
    await persist()
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

    node.status = action
    node.time = nowLabel()
    node.opinion = opinion || (action === 'pass' ? '同意通过。' : '审核驳回，请按意见修正后重新提交。')
    myReviews.value = myReviews.value.filter((item) => !(item.reviewCaseId === reviewCase.id && item.node === nodeName))

    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到待审核对象：${drawingNo}`)
    const completedReview: CompletedReview = {
      id: createId('completed-review'),
      reviewCaseId: reviewCase.id,
      no: drawingNo,
      name: target.name,
      node: nodeName,
      by: reviewCase.initiator,
      reviewer,
      time: nowLabel(),
      result: action,
      opinion: node.opinion,
      ver: target.ver,
    }
    completedReviews.value.unshift(completedReview)

    if (action === 'rejected') {
      reviewCase.status = 'rejected'
      target.status = 'draft'
      myReviews.value = myReviews.value.filter((item) => item.reviewCaseId !== reviewCase.id)
    } else {
      const requiredPending = reviewCase.nodes.some((item) => item.required !== false && item.status !== 'pass')
      if (!requiredPending) {
        reviewCase.status = 'published'
        target.status = 'published'
        myReviews.value = myReviews.value.filter((item) => item.reviewCaseId !== reviewCase.id)
      }
    }

    recordActivity({
      drawingNo,
      drawingName: target.name,
      targetType: 'review',
      act: 'check',
      text: `审核图纸 <b>${drawingNo}</b>（节点：${nodeName}）：${action === 'pass' ? '通过' : '驳回'}`,
      detail: { reviewCaseId: reviewCase.id, nodeName, result: action, opinion: node.opinion },
    })
    await persist()
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
    const material = payload.material.trim()
    const spec = payload.spec.trim()
    const surfaceTreatment = payload.surfaceTreatment.trim()
    const vendor = payload.vendor?.trim() || undefined
    const remark = payload.remark?.trim() || undefined
    if (!name) throw new Error('请输入零件名称')
    if (!material) throw new Error('请输入材料牌号')
    if (!Number.isFinite(payload.qty) || payload.qty <= 0) throw new Error('装配数量必须大于 0')
    if (!Number.isFinite(payload.weight) || payload.weight < 0) throw new Error('理论重量不能小于 0')

    const original = { ...part }
    Object.assign(part, {
      name,
      material,
      spec,
      weight: payload.weight,
      surfaceTreatment,
      partType: payload.partType,
      qty: payload.qty,
      ...(vendor ? { vendor } : { vendor: undefined }),
      ...(remark ? { remark } : { remark: undefined }),
    })

    const activityLog = recordActivity({
      drawingNo: partNo,
      drawingName: part.name,
      targetType: 'part',
      act: 'edit',
      text: `更新零件图 <b>${partNo}</b> 属性：材料 ${material} · 规格 ${spec || '未填写'} · 数量 ×${payload.qty}`,
      detail: { changedFields: payload },
    })

    try {
      await persist()
    } catch (saveError) {
      Object.assign(part, original)
      logs.value = logs.value.filter((item) => item.id !== activityLog.id)
      throw saveError
    }
  }

  async function updateDrawingSigners(drawingNo: string, assignments: SignerAssignments): Promise<void> {
    await initialize()
    const target = findDrawingOrPart(drawingNo)
    if (!target) throw new Error(`未找到图纸：${drawingNo}`)

    const originalSigners = target.signers
    const signers = Object.fromEntries(
      Object.entries(assignments).map(([role, user]) => [role, user?.trim() || '待定']),
    ) as SignerAssignments
    target.signers = signers
    try {
      await persist()
      recordActivity({
        drawingNo,
        drawingName: target.name,
        targetType: 'drawing',
        act: 'edit',
        text: `修改图纸 <b>${drawingNo}</b> 的签署人员`,
        detail: { before: originalSigners ?? {}, after: signers },
      })
      await persist()
    } catch (saveError) {
      target.signers = originalSigners
      throw saveError
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

    const user: UserAccount = {
      id: createId('user'),
      account: normalizedAccount,
      displayName: normalizedName,
      password: normalizedPassword,
      roles: normalizedRoles,
      status: 'active',
      createdAt: new Date().toISOString(),
      lastLoginAt: null,
    }
    users.value.unshift(user)
    try {
      await persist()
    } catch (saveError) {
      users.value = users.value.filter((item) => item.id !== user.id)
      throw saveError
    }
  }

  async function resetUserPassword(userId: string, password: string): Promise<void> {
    await initialize()
    const user = users.value.find((item) => item.id === userId)
    if (!user) throw new Error('未找到目标账号')
    const normalizedPassword = password.trim()
    if (!normalizedPassword) throw new Error('请输入新密码')
    const originalPassword = user.password
    user.password = normalizedPassword
    try {
      await persist()
    } catch (saveError) {
      user.password = originalPassword
      throw saveError
    }
  }

  async function toggleUser(userId: string): Promise<void> {
    await initialize()
    const user = users.value.find((item) => item.id === userId)
    if (!user) throw new Error('未找到目标账号')
    if (user.status === 'active' && user.roles.includes('admin')) {
      const activeAdminCount = users.value.filter((item) => item.status === 'active' && item.roles.includes('admin')).length
      if (activeAdminCount <= 1) throw new Error('不能禁用最后一个管理员账号')
    }

    const originalStatus = user.status
    user.status = user.status === 'active' ? 'disabled' : 'active'
    try {
      await persist()
    } catch (saveError) {
      user.status = originalStatus
      throw saveError
    }
  }

  async function toggleFlow(index: number): Promise<void> {
    await initialize()
    const flow = flows.value[index]
    if (flow) {
      flow.on = !flow.on
      await persist()
    }
  }

  async function restoreHidden(index: number): Promise<void> {
    await initialize()
    hiddenList.value.splice(index, 1)
    await persist()
  }

  return {
    drawings,
    structure,
    versions,
    branches,
    borrows,
    bom,
    crafts,
    logs,
    reviewCases,
    currentReviewNodes,
    myReviews,
    completedReviews,
    users,
    flows,
    hiddenList,
    adminLogs,
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
    refreshDrawingDesigner,
    recordActivity,
    recordActivityAndPersist,
    openDrawing,
    clearCurrentDrawing,
    getReviewCase,
    addDrawing,
    forkDrawing,
    createPartWithFile,
    borrowPartToProject,
    uploadDrawingFile,
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
    setReviewNodeStatus,
    updateStructurePart,
    updateDrawingSigners,
    createUser,
    resetUserPassword,
    toggleUser,
    toggleFlow,
    restoreHidden,
  }
})

// 兼容迁移期的旧命名，业务页面逐步切换到正式名称。
export const useCadDemoStore = useDomainStore
export const useDemoStore = useDomainStore
