import type {
  BomItem,
  Branch,
  BorrowRecord,
  CraftFile,
  Drawing,
  DrawingAttribute,
  DrawingAttributeField,
  DrawingFile,
  DrawingFileHistoryItem,
  MaterialFile,
  StructurePart,
  UserAccount,
  UserRole,
  UserStatus,
} from '@/types/domain.types'

function record(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null ? value as Record<string, unknown> : {}
}

function array<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : []
}

function text(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function number(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function boolean(value: unknown, fallback = false): boolean {
  return typeof value === 'boolean' ? value : fallback
}

export function readSeedModule<T>(seed: unknown, key: string, fallback: T): T {
  const source = record(seed)
  return Array.isArray(source[key]) ? source[key] as T : fallback
}

function id(prefix: string, value: string): string {
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return `${prefix}-${(hash >>> 0).toString(36)}`
}

function normalizeRole(value: unknown): UserRole[] {
  const roles = array<unknown>(value).filter((item): item is UserRole => item === 'admin' || item === 'designer' || item === 'reviewer')
  return roles.length ? [...new Set(roles)] : ['designer']
}

export function normalizeUsers(value: unknown): UserAccount[] {
  return array<unknown>(value).map((raw, index) => {
    const source = record(raw)
    const account = text(source.account, text(source.acc, `user-${index + 1}`)).trim().toLowerCase()
    const status: UserStatus = source.status === 'disabled' || source.status === '已禁用' ? 'disabled' : 'active'
    return {
      id: text(source.id, id('user', account)),
      account,
      displayName: text(source.displayName, text(source.name, account)),
      password: text(source.password),
      roles: normalizeRole(source.roles),
      status,
      createdAt: text(source.createdAt, '历史记录'),
      lastLoginAt: text(source.lastLoginAt) || null,
    }
  }).filter((item) => Boolean(item.account))
}

function normalizeHistory(value: unknown, fallback: DrawingFile): DrawingFileHistoryItem[] {
  return array<unknown>(value).map((raw, index) => {
    const source = record(raw)
    return {
      id: text(source.id, id('file-history', `${fallback.id}|${index}`)),
      name: text(source.name, fallback.name),
      size: text(source.size, fallback.size),
      version: text(source.version, 'v1.0'),
      uploadedBy: text(source.uploadedBy, '未知用户'),
      uploadedAt: text(source.uploadedAt, '历史记录'),
      ...(text(source.storageKey) ? { storageKey: text(source.storageKey) } : {}),
      ...(text(source.mimeType) ? { mimeType: text(source.mimeType) } : {}),
      ...(text(source.replacedBy) ? { replacedBy: text(source.replacedBy) } : {}),
      ...(text(source.replacedAt) ? { replacedAt: text(source.replacedAt) } : {}),
      ...(text(source.replaceReason) ? { replaceReason: text(source.replaceReason) } : {}),
      previewable: boolean(source.previewable, true),
    }
  })
}

export function normalizeDrawingFile(value: unknown, ownerNo: string, role: DrawingFile['role'] = 'other', partNo = ''): DrawingFile {
  const source = record(value)
  const name = text(source.name, '未命名文件')
  const resolvedPartNo = text(source.partNo, partNo)
  const resolvedRole: DrawingFile['role'] = source.role === 'assembly' && !resolvedPartNo
    ? 'assembly'
    : source.role === 'part' || resolvedPartNo
      ? 'part'
      : role
  const item: DrawingFile = {
    id: text(source.id, id('file', `${ownerNo}|${resolvedPartNo}|${name}`)),
    name,
    size: text(source.size, '—'),
    role: resolvedRole,
    drawingNo: text(source.drawingNo, ownerNo),
    version: text(source.version, 'v1.0'),
    uploadedBy: text(source.uploadedBy, '未知用户'),
    uploadedAt: text(source.uploadedAt, '历史记录'),
    previewable: boolean(source.previewable, true),
    ...(text(source.rawName) ? { rawName: text(source.rawName) } : {}),
    ...(resolvedPartNo ? { partNo: resolvedPartNo } : {}),
    ...(text(source.storageKey) ? { storageKey: text(source.storageKey) } : {}),
    ...(text(source.rawStorageKey) ? { rawStorageKey: text(source.rawStorageKey) } : {}),
    ...(text(source.currentStorageKey) ? { currentStorageKey: text(source.currentStorageKey) } : {}),
    ...(text(source.mimeType) ? { mimeType: text(source.mimeType) } : {}),
    ...(text(source.replaceReason) ? { replaceReason: text(source.replaceReason) } : {}),
    ...(text(source.replacedBy) ? { replacedBy: text(source.replacedBy) } : {}),
    ...(text(source.replacedAt) ? { replacedAt: text(source.replacedAt) } : {}),
  }
  const history = normalizeHistory(source.history, item)
  return history.length ? { ...item, history } : item
}

export function normalizeMaterialFile(value: unknown, drawingNo: string): MaterialFile {
  const source = record(value)
  const name = text(source.name, '未命名备料表')
  return {
    id: text(source.id, id('material', `${drawingNo}|${name}`)),
    drawingNo: text(source.drawingNo, drawingNo),
    name,
    size: text(source.size, '—'),
    version: text(source.version, 'v1.0'),
    uploadedBy: text(source.uploadedBy, '未知用户'),
    uploadedAt: text(source.uploadedAt, '历史记录'),
    ...(text(source.storageKey) ? { storageKey: text(source.storageKey) } : {}),
    ...(text(source.mimeType) ? { mimeType: text(source.mimeType) } : {}),
    ...(text(source.author) ? { author: text(source.author) } : {}),
  }
}

export function normalizeCraftFile(value: unknown, drawingNo: string): CraftFile {
  const source = record(value)
  const name = text(source.name, '未命名工艺文件')
  return {
    id: text(source.id, id('craft', `${drawingNo}|${name}`)),
    drawingNo: text(source.drawingNo, drawingNo),
    name,
    op: text(source.op, '未分类工艺'),
    ver: text(source.ver, 'v1.0'),
    by: text(source.by, '未知用户'),
    date: text(source.date, '历史记录'),
    size: text(source.size, '—'),
    previewable: boolean(source.previewable, true),
    scanned: boolean(source.scanned),
    ...(text(source.storageKey) ? { storageKey: text(source.storageKey) } : {}),
    ...(text(source.mimeType) ? { mimeType: text(source.mimeType) } : {}),
    ...(text(source.author) ? { author: text(source.author) } : {}),
  }
}

export function normalizeAttributes(value: unknown): DrawingAttribute[] {
  return array<unknown>(value).map((raw, index) => {
    const source = record(raw)
    const fields = array<unknown>(source.fields).map((fieldRaw, fieldIndex): DrawingAttributeField => {
      const field = record(fieldRaw)
      return {
        id: text(field.id, id('attribute-field', `${index}|${fieldIndex}|${text(field.name)}`)),
        name: text(field.name, `字段 ${fieldIndex + 1}`),
        enabled: boolean(field.enabled, true),
        sortOrder: number(field.sortOrder, fieldIndex + 1),
        ...(text(field.createdAt) ? { createdAt: text(field.createdAt) } : {}),
      }
    }).filter((field) => Boolean(field.name.trim()))
    return {
      id: text(source.id, id('attribute', `${index}|${text(source.name)}`)),
      name: text(source.name, `图纸属性 ${index + 1}`),
      required: boolean(source.required),
      enabled: boolean(source.enabled, true),
      sortOrder: number(source.sortOrder, index + 1),
      fields,
      ...(text(source.createdAt) ? { createdAt: text(source.createdAt) } : {}),
    }
  }).filter((item) => Boolean(item.name.trim()))
}

export function normalizeDrawings(value: unknown): Drawing[] {
  return array<unknown>(value).map((raw) => {
    const source = record(raw)
    const no = text(source.no, id('drawing', text(source.name, '未命名图纸')))
    const files = array<unknown>(source.files).map((file) => normalizeDrawingFile(file, no, 'assembly'))
    const otherFiles = array<unknown>(source.otherFiles).map((file) => normalizeDrawingFile(file, no, 'other'))
	return {
	      ...(text(source.id) ? { id: text(source.id) } : {}),
	      ...(number(source.revision) > 0 ? { revision: number(source.revision) } : {}),
	      no,
      name: text(source.name, no),
      kind: source.kind === '零件图' ? '零件图' : '总图',
      project: text(source.project, text(source.name, no)),
      material: text(source.material, '—'),
      vendor: text(source.vendor, '内部项目部'),
      status: source.status === 'published' || source.status === 'reviewing' || source.status === 'disabled' || source.status === 'archived' ? source.status : 'draft',
      ver: text(source.ver, 'v1.0'),
      updated: text(source.updated, '历史记录'),
      by: text(source.by, '未知用户'),
      borrow: number(source.borrow),
      hasFile: files.length > 0 || boolean(source.hasFile),
      ...(text(source.borrowFrom) ? { borrowFrom: text(source.borrowFrom) } : {}),
      ...(typeof source.signers === 'object' && source.signers !== null ? { signers: source.signers as Drawing['signers'] } : {}),
      ...(typeof source.attributeValues === 'object' && source.attributeValues !== null ? { attributeValues: source.attributeValues as Record<string, string> } : {}),
      ...(text(source.remark) ? { remark: text(source.remark) } : {}),
      files,
      otherFiles,
      materialFiles: array<unknown>(source.materialFiles).map((file) => normalizeMaterialFile(file, no)),
      craftFiles: array<unknown>(source.craftFiles).map((file) => normalizeCraftFile(file, no)),
    }
  })
}

export function normalizeStructure(value: unknown): StructurePart[] {
  return array<unknown>(value).map((raw) => {
    const source = record(raw)
    const no = text(source.no, id('part', text(source.name, '未命名零件')))
    const files = array<unknown>(source.files).map((file) => normalizeDrawingFile(file, text(source.parentNo), 'part', no))
	return {
	      ...(text(source.id) ? { id: text(source.id) } : {}),
	      ...(number(source.revision) > 0 ? { revision: number(source.revision) } : {}),
	      no,
      name: text(source.name, no),
      parentNo: text(source.parentNo),
      ...(text(source.project) ? { project: text(source.project) } : {}),
      material: text(source.material, '—'),
      spec: text(source.spec),
      weight: number(source.weight),
      surfaceTreatment: text(source.surfaceTreatment),
      partType: source.partType === '外协件' || source.partType === '标准件' || source.partType === '外购件' ? source.partType : '自制件',
      qty: number(source.qty, 1),
      status: source.status === 'published' || source.status === 'reviewing' || source.status === 'disabled' || source.status === 'archived' ? source.status : 'draft',
      ver: text(source.ver, 'v1.0'),
      hasFile: files.length > 0 || boolean(source.hasFile),
      ...(text(source.borrowFrom) ? { borrowFrom: text(source.borrowFrom) } : {}),
      ...(text(source.vendor) ? { vendor: text(source.vendor) } : {}),
      ...(typeof source.signers === 'object' && source.signers !== null ? { signers: source.signers as StructurePart['signers'] } : {}),
      files,
      otherFiles: array<unknown>(source.otherFiles).map((file) => normalizeDrawingFile(file, text(source.parentNo), 'other', no)),
      materialFiles: array<unknown>(source.materialFiles).map((file) => normalizeMaterialFile(file, no)),
      craftFiles: array<unknown>(source.craftFiles).map((file) => normalizeCraftFile(file, no)),
      ...(text(source.remark) ? { remark: text(source.remark) } : {}),
    }
  })
}

export function normalizeBom(value: unknown): BomItem[] {
  return array<unknown>(value).map((raw, index) => {
    const source = record(raw)
    return {
      no: number(source.no, index + 1),
      id: text(source.id, id('bom', `${index}|${text(source.name)}`)),
      drawingNo: text(source.drawingNo),
      ...(text(source.sourceFileId, text(source.sourceAttachmentVersion)) ? { sourceFileId: text(source.sourceFileId, text(source.sourceAttachmentVersion)) } : {}),
      name: text(source.name, '未命名物料'),
      spec: text(source.spec, '—'),
      qty: number(source.qty, number(source.quantity)),
      weight: number(source.weight),
      remark: text(source.remark),
    }
  })
}

export function normalizeBranches(value: unknown): Branch[] {
  return array<unknown>(value).map((raw) => {
    const source = record(raw)
    return {
      name: text(source.name),
      from: text(source.from, text(source.sourceDrawingNo)),
      by: text(source.by, '未知用户'),
      date: text(source.date, '历史记录'),
      status: source.status === '已禁用' ? '已禁用' : '使用中',
      desc: text(source.desc, text(source.description)),
    }
  })
}

export function normalizeBorrows(value: unknown): BorrowRecord[] {
  return array<unknown>(value).map((raw) => {
    const source = record(raw)
    return {
      dir: source.dir === 'out' ? 'out' : 'in',
      project: text(source.project),
      part: text(source.part),
      ...(text(source.partNo) ? { partNo: text(source.partNo) } : {}),
      ...(text(source.partName) ? { partName: text(source.partName) } : {}),
      ...(text(source.sourceDrawingNo) ? { sourceDrawingNo: text(source.sourceDrawingNo) } : {}),
      ...(text(source.targetDrawingNo) ? { targetDrawingNo: text(source.targetDrawingNo) } : {}),
      user: text(source.user, '未知用户'),
      date: text(source.date, '历史记录'),
      status: source.status === '已归档' || source.status === 'archived' ? '已归档' : '使用中',
    }
  })
}
