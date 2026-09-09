export type DrawingStatus = 'published' | 'reviewing' | 'draft' | 'disabled' | 'archived'

export type ActivityType = 'view' | 'create' | 'edit' | 'branch' | 'upload' | 'download' | 'delete' | 'check' | 'parse'

export type ActivityTargetType = 'drawing' | 'part' | 'file' | 'review' | 'branch'

export type ActivityResult = 'success' | 'failed'

export const ACTIVITY_LABELS: Record<ActivityType, string> = {
  view: '查看图纸',
  create: '新建图纸',
  edit: '修改图纸',
  branch: '创建分支',
  upload: '上传文件',
  download: '下载文件',
  delete: '删除文件',
  check: '审核操作',
  parse: '解析 EXB',
}

export interface DrawingSigners {
  设计: string
  校对: string
  审核: string
  工艺: string
  标准化: string
  批准: string
}

export interface DrawingFileHistoryItem {
  id: string
  name: string
  size: string
  version: string
  uploadedBy: string
  uploadedAt: string
  replacedBy?: string
  replacedAt?: string
  replaceReason?: string
  storageKey?: string
  mimeType?: string
	previewable: boolean
	}

export interface DrawingFile {
  id: string
  name: string
  rawName?: string
  size: string
  role: 'assembly' | 'part' | 'other'
  drawingNo: string
  partNo?: string
  version: string
  uploadedBy: string
  uploadedAt: string
  storageKey?: string
  rawStorageKey?: string
  currentStorageKey?: string
  mimeType?: string
	previewable: boolean
	revision?: number
  replaceReason?: string
  replacedBy?: string
  replacedAt?: string
  history?: DrawingFileHistoryItem[]
}

export interface MaterialFile {
  id: string
  drawingNo: string
  name: string
  size: string
  version: string
  uploadedBy: string
  uploadedAt: string
  storageKey?: string
  currentVersionId?: string
  mimeType?: string
	author?: string
	revision?: number
}

export interface CraftFile {
  id: string
  drawingNo: string
  name: string
  op: string
  ver: string
  by: string
  date: string
  size: string
  storageKey?: string
  mimeType?: string
  previewable: boolean
  author?: string
	 scanned: boolean
	revision?: number
}

export interface DrawingAttributeField {
  id: string
  name: string
  enabled: boolean
  sortOrder: number
  createdAt?: string
}

export interface DrawingAttribute {
  id: string
  name: string
  required: boolean
  enabled: boolean
  sortOrder: number
  fields: DrawingAttributeField[]
  createdAt?: string
}

export interface Drawing {
	/** 服务端资源 ID；旧的调试数据可不提供。 */
	id?: string
	/** 服务端乐观锁版本；修改时必须原样带回。 */
	revision?: number
	no: string
  name: string
  kind: '总图' | '零件图'
  project: string
  material: string
  vendor: string
  status: DrawingStatus
  ver: string
  updated: string
  by: string
  createdBy?: string
  createdAt?: string
  updatedBy?: string
  updatedAt?: string
  designer?: string
  borrow: number
  hasFile: boolean
  borrowFrom?: string
  forkedFrom?: string
  attributeValues?: Record<string, string>
  signers?: Partial<DrawingSigners>
  remark?: string
  files?: DrawingFile[]
  otherFiles?: DrawingFile[]
  materialFiles?: MaterialFile[]
  craftFiles?: CraftFile[]
}

export interface ActivityLog {
  id: string
  drawingNo: string
  drawingName: string
  targetType: ActivityTargetType
  userId?: string
  user: string
  userAccount?: string
  act: ActivityType
  txt: string
  time: string
  occurredAt: string
  result: ActivityResult
  ipAddress?: string
  userAgent?: string
  detail?: Record<string, unknown>
}

export interface StructurePart {
	id?: string
	drawingId?: string
	relationId?: string
	relationRevision?: number
	relationType?: 'owned' | 'borrowed' | string
	revision?: number
	no: string
  name: string
  parentNo: string
  project?: string
  material: string
  spec: string
  weight: number
  surfaceTreatment: string
  partType: PartManufacturingType
  qty: number
  status: DrawingStatus
  ver: string
  hasFile: boolean
  borrowFrom?: string
  forkedFrom?: string
  createdBy?: string
  createdAt?: string
  updatedBy?: string
  updatedAt?: string
  vendor?: string
  signers?: Partial<DrawingSigners>
  files?: DrawingFile[]
  otherFiles?: DrawingFile[]
  materialFiles?: MaterialFile[]
  craftFiles?: CraftFile[]
  remark?: string
}

export type PartManufacturingType = '自制件' | '外协件' | '标准件' | '外购件'

export type StructurePartEditable = Pick<
  StructurePart,
  'name' | 'material' | 'spec' | 'weight' | 'surfaceTreatment' | 'partType' | 'qty' | 'vendor' | 'remark'
> & {
  partNo?: string
}

export type SignerAssignments = Partial<Record<keyof DrawingSigners, string>>

export interface DrawingVersion {
  v: string
  by: string
  date: string
  note: string
  cur?: boolean
}

export interface Branch {
  name: string
  from: string
  by: string
  date: string
  status: '使用中' | '已禁用'
  desc: string
}

export interface BorrowRecord {
  dir: 'in' | 'out'
  project: string
  part: string
  partNo?: string
  partName?: string
  sourceDrawingNo?: string
  targetDrawingNo?: string
  user: string
  date: string
  sync?: string
  status: '使用中' | '已归档'
}

export interface BomItem {
  no: number
  id: string
  drawingNo: string
  sourceFileId?: string
  name: string
  spec: string
  qty: number
  weight: number
  remark: string
}

export interface ReviewNode {
  name: string
  user: string
  status: 'pass' | 'pending' | 'rejected'
  time: string
  opinion: string
  required?: boolean
  order?: number
  assignedUserId?: string
}

export type ReviewCaseStatus = 'pending' | 'reviewing' | 'published' | 'rejected'

export interface ReviewCase {
  id: string
  drawingNo: string
  flow: string
  status: ReviewCaseStatus
  initiator: string
  startedAt: string
  nodes: ReviewNode[]
}

export interface ReviewData {
  flow: string
  nodes: ReviewNode[]
}

export interface MyReview {
  reviewCaseId: string
  no: string
  name: string
  node: string
  by: string
  time: string
}

export interface CompletedReview {
  id: string
  reviewCaseId: string
  no: string
  name: string
  node: string
  by: string
  reviewer: string
  time: string
  result: 'pass' | 'rejected'
  opinion: string
  ver: string
}

export type UserRole = 'admin' | 'designer' | 'reviewer'

export type UserStatus = 'active' | 'disabled'

export interface UserAccount {
  id: string
  account: string
  displayName: string
  password: string
  roles: UserRole[]
  status: UserStatus
  createdAt: string
  lastLoginAt: string | null
}

// 兼容旧的管理员页面命名，认证数据统一使用 UserAccount。
export type AdminUser = UserAccount

export interface ReviewFlow {
  id?: string
  name: string
  nodes: number
  on: boolean
  desc: string
  createdBy?: string
  createdAt?: string
  nodeList?: Array<{ name: string; user: string; required: boolean; candidateRole?: UserRole; signerRole?: string; order?: number }>
}

export interface AdminLog {
  time: string
  user: string
  act: string
  obj: string
}
