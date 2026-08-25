import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { dataManager } from '@/services/data-manager'
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
  return '刚刚'
}

function activityTime(): string {
  return new Date().toISOString()
}

function formatNumber(value: string | undefined): number {
  if (!value) return 0
  const normalized = value.replace(/,/g, '').trim()
  const parsed = Number(normalized)
  return Number.isFinite(parsed) ? parsed : 0
}

function parseCsvLine(line: string): string[] {
  const values: string[] = []
  let value = ''
  let quoted = false

  for (let index = 0; index < line.length; index += 1) {
    const character = line[index]
    if (character === '"') {
      if (quoted && line[index + 1] === '"') {
        value += '"'
        index += 1
      } else {
        quoted = !quoted
      }
    } else if (character === ',' && !quoted) {
      values.push(value.trim())
      value = ''
    } else {
      value += character
    }
  }

  values.push(value.trim())
  return values
}

async function parseCsvBom(content: Blob, drawingNo: string, sourceFileId: string): Promise<BomItem[]> {
  const text = await content.text()
  const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean)
  if (lines.length < 2) return []

  const headers = parseCsvLine(lines[0] ?? '').map((header) => header.toLowerCase().replace(/\s+/g, ''))
  const findColumn = (names: string[]) => headers.findIndex((header) => names.some((name) => header.includes(name)))
  const noIndex = findColumn(['序号', 'no', '编号'])
  const idIndex = findColumn(['图号', '标准号', '零件号', 'id'])
  const nameIndex = findColumn(['名称', '物料名称', 'name'])
  const specIndex = findColumn(['规格', '材质', 'spec'])
  const qtyIndex = findColumn(['数量', 'qty', 'count'])
  const weightIndex = findColumn(['单重', '重量', 'weight'])
  const remarkIndex = findColumn(['备注', 'remark', '说明'])

  return lines.slice(1).map((line, index) => {
    const columns = parseCsvLine(line)
    const get = (columnIndex: number) => columnIndex >= 0 ? columns[columnIndex] : undefined
    return {
      no: formatNumber(get(noIndex)) || index + 1,
      id: get(idIndex) || `${drawingNo}-BOM-${String(index + 1).padStart(3, '0')}`,
      drawingNo,
      sourceFileId,
      name: get(nameIndex) || '未命名物料',
      spec: get(specIndex) || '—',
      qty: formatNumber(get(qtyIndex)),
      weight: formatNumber(get(weightIndex)),
      remark: get(remarkIndex) || '',
    }
  })
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
      if (file.name.toLowerCase().endsWith('.csv') && content) {
        const importedItems = await parseCsvBom(content, drawingNo, file.id)
        if (importedItems.length) {
          bomItems.value = [
            ...bomItems.value.filter((item) => item.drawingNo !== drawingNo),
            ...importedItems,
          ]
          importedCount = importedItems.length
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
    recordActivity,
    recordActivityAndPersist,
    openDrawing,
    clearCurrentDrawing,
    getReviewCase,
    addDrawing,
    forkDrawing,
    createPartWithFile,
    uploadDrawingFile,
    uploadOtherFile,
    deleteOtherFile,
    deleteDrawingFile,
    uploadMaterialFile,
    deleteMaterialFile,
    uploadCraftFile,
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
