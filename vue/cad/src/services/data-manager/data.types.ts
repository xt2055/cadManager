import type {
  ActivityType,
  ActivityLog,
  AdminLog,
  BomItem,
  Branch,
  BorrowRecord,
  CompletedReview,
  CraftFile,
  Drawing,
  DrawingFile,
  DrawingVersion,
  HiddenObject,
  MaterialFile,
  MyReview,
  ReviewCase,
  ReviewFlow,
  ReviewNode,
  StructurePart,
  PartManufacturingType,
  UserAccount,
  UserRole,
  UserStatus,
} from '@/types/domain.types'
import { directParentDrawingNo, parseAgainstRoots, parseDrawingFileName, parseStandaloneDrawingFileName } from '@/utils/drawing-number-parser'

export interface DataDocument {
  version: 2
  drawings: Drawing[]
  structure: StructurePart[]
  versions: DrawingVersion[]
  branches: Branch[]
  borrows: BorrowRecord[]
  bom: BomItem[]
  crafts: CraftFile[]
  logs: ActivityLog[]
  reviewCases: ReviewCase[]
  myReviews: MyReview[]
  completedReviews: CompletedReview[]
  users: UserAccount[]
  flows: ReviewFlow[]
  hiddenList: HiddenObject[]
  adminLogs: AdminLog[]
}

export function createEmptyDataDocument(): DataDocument {
  return {
    version: 2,
    drawings: [],
    structure: [],
    versions: [],
    branches: [],
    borrows: [],
    bom: [],
    crafts: [],
    logs: [],
    reviewCases: [],
    myReviews: [],
    completedReviews: [],
    users: [],
    flows: [],
    hiddenList: [],
    adminLogs: [],
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function readArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : []
}

function asString(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function asNumber(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function asBoolean(value: unknown, fallback = false): boolean {
  return typeof value === 'boolean' ? value : fallback
}

function asPartManufacturingType(value: unknown): PartManufacturingType {
  return value === '外协件' || value === '标准件' || value === '外购件' ? value : '自制件'
}

function normalizeUserRoles(value: unknown, legacyRole: unknown): UserRole[] {
  const roles = Array.isArray(value)
    ? value.filter((role): role is UserRole => role === 'admin' || role === 'designer' || role === 'reviewer')
    : []
  if (roles.length) return [...new Set(roles)]

  const roleText = asString(legacyRole)
  const inferred: UserRole[] = []
  if (roleText.includes('管理员')) inferred.push('admin')
  if (roleText.includes('审核')) inferred.push('reviewer')
  if (roleText.includes('设计') || roleText.includes('普通')) inferred.push('designer')
  return inferred.length ? inferred : ['designer']
}

function normalizeUsers(value: unknown): UserAccount[] {
  return readArray<unknown>(value).map((item, index) => {
    const source = isRecord(item) ? item : {}
    const account = asString(source.account, asString(source.acc, `user-${index + 1}`))
    const lastLoginAt = asString(source.lastLoginAt)
    const status: UserStatus = source.status === 'active' || source.status === 'disabled'
      ? source.status
      : source.status === '已禁用'
        ? 'disabled'
        : 'active'
    return {
      id: asString(source.id, stableId('user', account)),
      account,
      displayName: asString(source.displayName, asString(source.name, account)),
      password: asString(source.password),
      roles: normalizeUserRoles(source.roles, source.role),
      status,
      createdAt: asString(source.createdAt, '历史记录'),
      lastLoginAt: lastLoginAt || null,
    }
  }).filter((user) => Boolean(user.account))
}

function stableId(prefix: string, value: string): string {
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return `${prefix}-${(hash >>> 0).toString(36)}`
}

function normalizeActivityLogs(value: unknown): ActivityLog[] {
  return readArray<unknown>(value).map((item, index) => {
    const source = isRecord(item) ? item : {}
    const occurredAt = asString(source.occurredAt, asString(source.time, '历史记录'))
    const act: ActivityType = source.act === 'view' || source.act === 'create' || source.act === 'edit'
      || source.act === 'branch' || source.act === 'upload' || source.act === 'download'
      || source.act === 'delete' || source.act === 'check' || source.act === 'parse'
      ? source.act
      : 'edit'
    const text = asString(source.txt, '记录了一次图纸操作')
    const drawingNo = asString(source.drawingNo, text.match(/<b>([^<]+)<\/b>/)?.[1] ?? '')
    return {
      id: asString(source.id, stableId('drawing-log', `${drawingNo}|${occurredAt}|${index}`)),
      drawingNo,
      drawingName: asString(source.drawingName, drawingNo || '未指定图纸'),
      targetType: source.targetType === 'part' || source.targetType === 'file' || source.targetType === 'review' || source.targetType === 'branch'
        ? source.targetType
        : 'drawing',
      ...(asString(source.userId) ? { userId: asString(source.userId) } : {}),
      user: asString(source.user, '未知用户'),
      act,
      txt: text,
      time: occurredAt,
      occurredAt,
      result: source.result === 'failed' ? 'failed' : 'success',
      ...(isRecord(source.detail) ? { detail: source.detail } : {}),
    }
  })
}

function normalizeDrawingFile(
  value: unknown,
  ownerNo: string,
  roleFallback: 'assembly' | 'part' | 'other',
  partNoFallback = '',
): DrawingFile {
  const source = isRecord(value) ? value : {}
  const name = asString(source.name, '未命名文件')
  const partNo = asString(source.partNo, partNoFallback)
  const role = source.role === 'other'
    ? 'other'
    : source.role === 'part' || partNo
      ? 'part'
      : roleFallback
  const drawingNo = asString(source.drawingNo, ownerNo)
  const id = asString(source.id, stableId('file', `${drawingNo}|${partNo}|${name}|${asString(source.version, 'v1.0')}`))

  const rawHistory = readArray<unknown>(source.history)
  const history = rawHistory.map((item, index) => {
    const rec = isRecord(item) ? item : {}
    return {
      id: asString(rec.id, stableId('file-hist', `${id}|${index}`)),
      name: asString(rec.name, name),
      size: asString(rec.size, '—'),
      version: asString(rec.version, `v1.${index}`),
      uploadedBy: asString(rec.uploadedBy, '未知用户'),
      uploadedAt: asString(rec.uploadedAt, '历史记录'),
      ...(asString(rec.replacedBy) ? { replacedBy: asString(rec.replacedBy) } : {}),
      ...(asString(rec.replacedAt) ? { replacedAt: asString(rec.replacedAt) } : {}),
      ...(asString(rec.replaceReason) ? { replaceReason: asString(rec.replaceReason) } : {}),
      ...(asString(rec.storageKey) ? { storageKey: asString(rec.storageKey) } : {}),
      ...(asString(rec.mimeType) ? { mimeType: asString(rec.mimeType) } : {}),
      previewable: asBoolean(rec.previewable, true),
    }
  })

  return {
    id,
    name,
    size: asString(source.size, '—'),
    role,
    drawingNo,
    ...(partNo ? { partNo } : {}),
    version: asString(source.version, 'v1.0'),
    uploadedBy: asString(source.uploadedBy, '未知用户'),
    uploadedAt: asString(source.uploadedAt, '历史记录'),
    ...(asString(source.storageKey) ? { storageKey: asString(source.storageKey) } : {}),
    ...(asString(source.mimeType) ? { mimeType: asString(source.mimeType) } : {}),
    previewable: asBoolean(source.previewable, true),
    ...(asString(source.replaceReason) ? { replaceReason: asString(source.replaceReason) } : {}),
    ...(asString(source.replacedBy) ? { replacedBy: asString(source.replacedBy) } : {}),
    ...(asString(source.replacedAt) ? { replacedAt: asString(source.replacedAt) } : {}),
    ...(history.length > 0 ? { history } : {}),
  }
}

function normalizeMaterialFile(value: unknown, drawingNo: string): MaterialFile {
  const source = isRecord(value) ? value : {}
  const name = asString(source.name, '未命名备料表')
  const id = asString(source.id, stableId('material', `${drawingNo}|${name}|${asString(source.version, 'v1.0')}`))

  return {
    id,
    drawingNo: asString(source.drawingNo, drawingNo),
    name,
    size: asString(source.size, '—'),
    version: asString(source.version, 'v1.0'),
    uploadedBy: asString(source.uploadedBy, '未知用户'),
    uploadedAt: asString(source.uploadedAt, '历史记录'),
    ...(asString(source.storageKey) ? { storageKey: asString(source.storageKey) } : {}),
    ...(asString(source.mimeType) ? { mimeType: asString(source.mimeType) } : {}),
    ...(asString(source.author) ? { author: asString(source.author) } : {}),
  }
}

function normalizeCraftFile(value: unknown, drawingNo: string): CraftFile {
  const source = isRecord(value) ? value : {}
  const name = asString(source.name, '未命名工艺文件')
  const id = asString(source.id, stableId('craft', `${drawingNo}|${name}|${asString(source.ver, 'v1.0')}`))

  return {
    id,
    drawingNo: asString(source.drawingNo, drawingNo),
    name,
    op: asString(source.op, '未分类工艺'),
    ver: asString(source.ver, 'v1.0'),
    by: asString(source.by, '未知用户'),
    date: asString(source.date, '历史记录'),
    size: asString(source.size, '—'),
    ...(asString(source.storageKey) ? { storageKey: asString(source.storageKey) } : {}),
    ...(asString(source.mimeType) ? { mimeType: asString(source.mimeType) } : {}),
    previewable: asBoolean(source.previewable, true),
    ...(asString(source.author) ? { author: asString(source.author) } : {}),
    scanned: asBoolean(source.scanned, false),
  }
}

function normalizeReviewNode(value: unknown, index: number): ReviewNode {
  const source = isRecord(value) ? value : {}
  return {
    name: asString(source.name, `审核节点 ${index + 1}`),
    user: asString(source.user, '待定'),
    status: source.status === 'pass' || source.status === 'rejected' ? source.status : 'pending',
    time: asString(source.time, '—'),
    opinion: asString(source.opinion),
    required: source.required !== false,
    order: asNumber(source.order, index + 1),
  }
}

function inferParentNo(partNo: string, drawings: Drawing[]): string {
  const assemblies = drawings.filter((drawing) => drawing.kind === '总图')
  const matching = assemblies
    .sort((left, right) => right.no.length - left.no.length)
    .find((drawing) => partNo.startsWith(`${drawing.no}-`))
  if (!matching) return ''
  return directParentDrawingNo(partNo) ?? matching.no
}

function normalizeDrawings(value: unknown): Drawing[] {
  return readArray<unknown>(value).map((item) => {
    const source = isRecord(item) ? item : {}
    const no = asString(source.no, stableId('drawing', asString(source.name, '未命名图纸')))
    const files = readArray<unknown>(source.files).map((file) => normalizeDrawingFile(file, no, 'assembly'))
    return {
      no,
      name: asString(source.name, no),
      kind: source.kind === '零件图' ? '零件图' : '总图',
      project: asString(source.project, asString(source.name, no)),
      material: asString(source.material, '—'),
      vendor: asString(source.vendor, '内部项目部'),
      status: source.status === 'published' || source.status === 'reviewing' || source.status === 'hidden' || source.status === 'disabled' ? source.status : 'draft',
      ver: asString(source.ver, 'v1.0'),
      updated: asString(source.updated, '历史记录'),
      by: asString(source.by, '未知用户'),
      borrow: asNumber(source.borrow),
      hasFile: files.length > 0 || asBoolean(source.hasFile),
      ...(asString(source.borrowFrom) ? { borrowFrom: asString(source.borrowFrom) } : {}),
      ...(isRecord(source.signers) ? { signers: source.signers as Drawing['signers'] } : {}),
      ...(asString(source.remark) ? { remark: asString(source.remark) } : {}),
      ...(asString(source.designer) ? { designer: asString(source.designer) } : {}),
      files,
      otherFiles: readArray<unknown>(source.otherFiles).map((file) => normalizeDrawingFile(file, no, 'other')),
      materialFiles: readArray<unknown>(source.materialFiles).map((file) => normalizeMaterialFile(file, no)),
      craftFiles: readArray<unknown>(source.craftFiles).map((file) => normalizeCraftFile(file, no)),
    }
  })
}

function classifyDrawingPartFile(file: DrawingFile, drawingNo: string): DrawingFile | null {
  const currentProjectParsed = parseDrawingFileName(file.name, drawingNo)
  const standaloneParsed = parseStandaloneDrawingFileName(file.name)
  const parsed = currentProjectParsed.isStandard ? currentProjectParsed : standaloneParsed
  const isBorrowed = standaloneParsed.isStandard && standaloneParsed.rootNo !== drawingNo
  if (!parsed.isStandard || (!isBorrowed && parsed.level < 1)) return null
  return {
    ...file,
    role: 'part',
    drawingNo,
    partNo: parsed.no,
  }
}

function normalizeStructure(value: unknown, drawings: Drawing[]): StructurePart[] {
  const rootNos = drawings.filter((drawing) => drawing.kind === '总图').map((drawing) => drawing.no)
  return readArray<unknown>(value).map((item) => {
    const source = isRecord(item) ? item : {}
    const sourceNo = asString(source.no, stableId('part', asString(source.name, '未命名零件')))
    const sourceBorrowFrom = asString(source.borrowFrom)
    const sourceParentNo = asString(source.parentNo)
    const sourceFiles = [...readArray<unknown>(source.files), ...readArray<unknown>(source.otherFiles)]
      .map((file) => normalizeDrawingFile(file, sourceParentNo || sourceNo, 'part', sourceNo))
      .filter((file, index, files) => files.findIndex((candidate) => candidate.id === file.id) === index)
    const parsedFiles = sourceFiles.map((file) => ({
      file,
      parsed: (() => {
        const parsedAgainstRoots = parseAgainstRoots(file.name, rootNos)
        return parsedAgainstRoots.isStandard ? parsedAgainstRoots : parseStandaloneDrawingFileName(file.name)
      })(),
    }))
    const validFile = parsedFiles.find(({ parsed }) => parsed.isStandard && (parsed.level > 0 || Boolean(sourceBorrowFrom)))
    const parsedAgainstRoots = parseAgainstRoots(sourceNo, rootNos)
    const parsedSourceNo = parsedAgainstRoots.isStandard ? parsedAgainstRoots : parseStandaloneDrawingFileName(sourceNo)
    // 已保存的结构编号和父级关系优先于文件名推导，避免重新加载时把用户数据改挂到别的节点。
    const no = sourceNo || validFile?.parsed.no || (parsedSourceNo.isStandard ? parsedSourceNo.no : sourceNo)
    const parentRootNo = [...rootNos]
      .sort((left, right) => right.length - left.length)
      .find((rootNo) => sourceParentNo === rootNo || sourceParentNo.startsWith(`${rootNo}-`))
    const inferredBorrowFrom = !sourceBorrowFrom && validFile?.parsed.rootNo && parentRootNo && validFile.parsed.rootNo !== parentRootNo
      ? validFile.parsed.rootNo
      : ''
    const borrowFrom = sourceBorrowFrom || inferredBorrowFrom
    const parentNo = sourceParentNo || (borrowFrom
      ? validFile?.parsed.parentNo || (parsedSourceNo.isStandard ? parsedSourceNo.parentNo : '') || ''
      : validFile?.parsed.parentNo || (parsedSourceNo.isStandard ? parsedSourceNo.parentNo : '') || '')
    const files = parsedFiles
      .filter(({ parsed }) => parsed.isStandard && (parsed.level > 0 || Boolean(borrowFrom)))
      .map(({ file, parsed }) => ({
        ...file,
        role: 'part' as const,
        drawingNo: borrowFrom ? (file.drawingNo || parentNo) : parsed.rootNo ?? parentNo,
        partNo: parsed.no,
      }))
    const otherFiles = parsedFiles
      .filter(({ parsed }) => !parsed.isStandard || (parsed.level <= 0 && !borrowFrom))
      .map(({ file }) => ({ ...file, role: 'other' as const, drawingNo: parentNo || file.drawingNo, partNo: undefined }))
    const sourceName = asString(source.name)
    const name = sourceName || (validFile?.parsed.name && validFile.parsed.name !== validFile.parsed.no
      ? validFile.parsed.name
      : no)
    return {
      no,
      name,
      parentNo,
      project: asString(source.project, drawings.find((drawing) => drawing.no === parentNo)?.project),
      material: asString(source.material, '—'),
      spec: asString(source.spec),
      weight: asNumber(source.weight),
      surfaceTreatment: asString(source.surfaceTreatment),
      partType: asPartManufacturingType(source.partType),
      qty: asNumber(source.qty, 1),
      status: source.status === 'published' || source.status === 'reviewing' || source.status === 'hidden' || source.status === 'disabled' ? source.status : 'draft',
      ver: asString(source.ver, 'v1.0'),
      hasFile: files.length > 0 || asBoolean(source.hasFile),
      ...(borrowFrom ? { borrowFrom } : {}),
      ...(asString(source.vendor) ? { vendor: asString(source.vendor) } : {}),
      ...(isRecord(source.signers) ? { signers: source.signers as StructurePart['signers'] } : {}),
      files,
      otherFiles,
      materialFiles: readArray<unknown>(source.materialFiles).map((file) => normalizeMaterialFile(file, no)),
      craftFiles: readArray<unknown>(source.craftFiles).map((file) => normalizeCraftFile(file, no)),
      ...(asString(source.remark) ? { remark: asString(source.remark) } : {}),
    }
  })
}

function normalizeBom(value: unknown, fallbackDrawingNo: string): BomItem[] {
  return readArray<unknown>(value).map((item, index) => {
    const source = isRecord(item) ? item : {}
    return {
      no: asNumber(source.no, index + 1),
      id: asString(source.id, stableId('bom', `${fallbackDrawingNo}|${index}|${asString(source.name)}`)),
      drawingNo: asString(source.drawingNo, fallbackDrawingNo),
      ...(asString(source.sourceFileId) ? { sourceFileId: asString(source.sourceFileId) } : {}),
      name: asString(source.name, '未命名物料'),
      spec: asString(source.spec, '—'),
      qty: asNumber(source.qty, 0),
      weight: asNumber(source.weight, 0),
      remark: asString(source.remark),
    }
  })
}

function reviewCaseStatus(nodes: ReviewNode[]): ReviewCase['status'] {
  if (nodes.some((node) => node.status === 'rejected')) return 'rejected'
  if (nodes.length > 0 && nodes.every((node) => node.required === false || node.status === 'pass')) return 'published'
  return 'reviewing'
}

function normalizeReviewCases(source: Record<string, unknown>, drawings: Drawing[]): ReviewCase[] {
  const reviewCases = readArray<unknown>(source.reviewCases)
  if (reviewCases.length) {
    return reviewCases.map((item, index) => {
      const record = isRecord(item) ? item : {}
      const drawingNo = asString(record.drawingNo)
      const nodes = readArray<unknown>(record.nodes).map((node, nodeIndex) => normalizeReviewNode(node, nodeIndex))
      return {
        id: asString(record.id, stableId('review-case', `${drawingNo}|${index}`)),
        drawingNo,
        flow: asString(record.flow, '企业标准图纸审核流程'),
        status: record.status === 'pending' || record.status === 'reviewing' || record.status === 'published' || record.status === 'rejected' ? record.status : reviewCaseStatus(nodes),
        initiator: asString(record.initiator, '未知用户'),
        startedAt: asString(record.startedAt, '历史记录'),
        nodes,
      }
    }).filter((reviewCase) => Boolean(reviewCase.drawingNo))
  }

  const legacyNodes = readArray<unknown>(source.reviewNodes).map((node, index) => normalizeReviewNode(node, index))
  const legacyReviews = readArray<unknown>(source.myReviews)
  const targets = legacyReviews
    .map((item) => isRecord(item) ? asString(item.no) : '')
    .filter(Boolean)
  const targetNos = targets.length ? [...new Set(targets)] : (legacyNodes.length && drawings[0] ? [drawings[0].no] : [])

  return targetNos.map((drawingNo, index) => ({
    id: stableId('review-case', `${drawingNo}|legacy|${index}`),
    drawingNo,
    flow: '企业标准图纸审核流程',
    status: reviewCaseStatus(legacyNodes),
    initiator: '未知用户',
    startedAt: '历史记录',
    nodes: legacyNodes.map((node) => ({ ...node })),
  }))
}

export function normalizeDataDocument(value: unknown): DataDocument {
  const source = isRecord(value) ? value : {}
  const drawings = normalizeDrawings(source.drawings)
  const structure = normalizeStructure(source.structure, drawings)
  const fallbackDrawingNo = drawings.length === 1 ? drawings[0]?.no ?? '' : ''
  const normalizedDrawings = drawings.map((drawing) => {
    const assemblyFiles: DrawingFile[] = []
    const migratedPartFiles: DrawingFile[] = []
    const otherFiles: DrawingFile[] = []

    const sourceFiles = [...(drawing.files ?? []), ...(drawing.otherFiles ?? [])]
      .filter((file, index, files) => files.findIndex((candidate) => candidate.id === file.id) === index)
    for (const file of sourceFiles) {
      const parsedPartFile = classifyDrawingPartFile(file, drawing.no)
      if (parsedPartFile) {
        migratedPartFiles.push(parsedPartFile)
      } else if (file.role === 'assembly' && !file.partNo) {
        assemblyFiles.push({ ...file, role: 'assembly', drawingNo: drawing.no })
      } else {
        otherFiles.push({ ...file, role: 'other', drawingNo: drawing.no, partNo: undefined })
      }
    }

    return {
      ...drawing,
      files: assemblyFiles,
      otherFiles,
      hasFile: assemblyFiles.length > 0,
      materialFiles: drawing.materialFiles ?? [],
      craftFiles: drawing.craftFiles ?? [],
      _migratedPartFiles: migratedPartFiles,
    }
  })
  const migratedPartFiles = normalizedDrawings.flatMap((drawing) => (
    '_migratedPartFiles' in drawing ? drawing._migratedPartFiles as DrawingFile[] : []
  ))
  const migratedFilesByPartNo = new Map<string, DrawingFile[]>()
  for (const file of migratedPartFiles) {
    if (!file.partNo) continue
    const files = migratedFilesByPartNo.get(file.partNo) ?? []
    files.push(file)
    migratedFilesByPartNo.set(file.partNo, files)
  }
  const migratedStructure = structure.map((part) => {
    const migratedFilesForPart = migratedFilesByPartNo.get(part.no) ?? []
    const files = [...(part.files ?? []), ...migratedFilesForPart]
      .filter((file, index, sourceFiles) => sourceFiles.findIndex((candidate) => candidate.id === file.id) === index)
    return {
      ...part,
      files,
      otherFiles: part.otherFiles ?? [],
      hasFile: files.length > 0,
    }
  })
  const recoveredStructure = [...migratedStructure]
  for (const file of migratedPartFiles) {
    if (!file.partNo || recoveredStructure.some((part) => part.no === file.partNo)) continue
    const rootDrawing = normalizedDrawings
      .filter((drawing) => drawing.no && file.partNo?.startsWith(`${drawing.no}-`))
      .sort((left, right) => right.no.length - left.no.length)[0]
    const borrowFrom = rootDrawing && file.drawingNo !== rootDrawing.no ? file.drawingNo : undefined
    recoveredStructure.push({
      no: file.partNo,
      name: file.name.replace(/\.[^/.]+$/, ''),
      parentNo: directParentDrawingNo(file.partNo) ?? rootDrawing?.no ?? file.drawingNo,
      project: rootDrawing?.project,
      material: '—',
      spec: '',
      weight: 0,
      surfaceTreatment: '',
      partType: '自制件',
      qty: 1,
      status: 'draft',
      ver: file.version,
      hasFile: true,
      ...(borrowFrom ? { borrowFrom } : {}),
      files: [file],
      otherFiles: [],
      materialFiles: [],
      craftFiles: [],
      remark: '由图纸文件名自动建立，请补充零件属性。',
    })
  }
  const cleanDrawings = normalizedDrawings.map(({ _migratedPartFiles: _ignored, ...drawing }) => drawing)
  const craftFiles = [
    ...readArray<unknown>(source.crafts).map((item) => normalizeCraftFile(item, isRecord(item) ? asString(item.drawingNo, fallbackDrawingNo) : fallbackDrawingNo)),
    ...cleanDrawings.flatMap((drawing) => drawing.craftFiles ?? []),
    ...migratedStructure.flatMap((part) => part.craftFiles ?? []),
  ].filter((file, index, files) => files.findIndex((candidate) => candidate.id === file.id) === index)
  const reviewCases = normalizeReviewCases(source, cleanDrawings)
  const caseByDrawing = new Map(reviewCases.map((reviewCase) => [reviewCase.drawingNo, reviewCase]))
  const myReviews = readArray<unknown>(source.myReviews).map((item, index) => {
    const record = isRecord(item) ? item : {}
    const no = asString(record.no)
    const reviewCase = caseByDrawing.get(no)
    return {
      reviewCaseId: asString(record.reviewCaseId, reviewCase?.id ?? stableId('review-case', `${no}|legacy|${index}`)),
      no,
      name: asString(record.name, cleanDrawings.find((drawing) => drawing.no === no)?.name ?? no),
      node: asString(record.node, '专业审核'),
      by: asString(record.by, '未知用户'),
      time: asString(record.time, '历史记录'),
    }
  }).filter((review) => Boolean(review.no))
  const completedReviews = readArray<unknown>(source.completedReviews).map((item, index) => {
    const record = isRecord(item) ? item : {}
    const no = asString(record.no)
    return {
      id: asString(record.id, stableId('completed-review', `${no}|${asString(record.node)}|${index}`)),
      reviewCaseId: asString(record.reviewCaseId, caseByDrawing.get(no)?.id ?? ''),
      no,
      name: asString(record.name, cleanDrawings.find((drawing) => drawing.no === no)?.name ?? no),
      node: asString(record.node, '专业审核'),
      by: asString(record.by, '未知用户'),
      reviewer: asString(record.reviewer, '未知用户'),
      time: asString(record.time, '历史记录'),
      result: record.result === 'rejected' ? ('rejected' as const) : ('pass' as const),
      opinion: asString(record.opinion),
      ver: asString(record.ver, 'v1.0'),
    }
  })

  if (source.version !== 2) {
    console.warn('业务数据已从旧版本迁移到 v2；无法自动归属的历史附件会保留在原始记录中。')
  }

  return {
    version: 2,
    drawings: cleanDrawings,
    structure: recoveredStructure,
    versions: readArray<DrawingVersion>(source.versions),
    branches: readArray<Branch>(source.branches),
    borrows: readArray<unknown>(source.borrows).map((item) => {
      const record = isRecord(item) ? item : {}
      return {
        dir: record.dir === 'out' ? 'out' as const : 'in' as const,
        project: asString(record.project),
        part: asString(record.part),
        ...(asString(record.partNo) ? { partNo: asString(record.partNo) } : {}),
        ...(asString(record.partName) ? { partName: asString(record.partName) } : {}),
        ...(asString(record.sourceDrawingNo) ? { sourceDrawingNo: asString(record.sourceDrawingNo) } : {}),
        ...(asString(record.targetDrawingNo) ? { targetDrawingNo: asString(record.targetDrawingNo) } : {}),
        user: asString(record.user, '未知用户'),
        date: asString(record.date, '历史记录'),
        ...(asString(record.sync) ? { sync: asString(record.sync) } : {}),
        status: record.status === '已归档' ? '已归档' as const : '使用中' as const,
      }
    }),
    bom: normalizeBom(source.bom, fallbackDrawingNo),
    crafts: craftFiles,
    logs: normalizeActivityLogs(source.logs),
    reviewCases,
    myReviews,
    completedReviews,
    users: normalizeUsers(source.users),
    flows: readArray<ReviewFlow>(source.flows),
    hiddenList: readArray<HiddenObject>(source.hiddenList),
    adminLogs: readArray<AdminLog>(source.adminLogs),
  }
}
