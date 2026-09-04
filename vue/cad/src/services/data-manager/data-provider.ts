import type { BomItem, Branch, BorrowRecord, CraftFile, Drawing, DrawingAttribute, DrawingVersion, StructurePart } from '@/types/domain.types'
import type { UserAccount, UserRole, UserStatus } from '@/types/domain.types'

export interface UploadSession {
  id: string
  kind: 'attachment' | 'drawing-create'
  idempotencyKey: string
  status: 'open' | 'committing' | 'committed' | 'cancelled' | 'expired' | 'failed' | string
  metadata: Record<string, unknown>
  createdAt: string
  lastActivityAt: string
  expiresAt: string
  absoluteExpiresAt: string
  errorMessage?: string
}

export interface UploadSessionItem {
  id: string
  clientRef: string
  attachmentId?: string
  drawingNo: string
  partNo: string
  role: 'assembly' | 'part' | 'material' | 'craft' | 'other'
  originalName: string
  mimeType: string
  expectedRevision?: number
  status: 'pending' | 'uploading' | 'ready' | 'failed' | 'committed' | string
  failureStage?: 'hash' | 'upload' | 'validation' | 'conversion' | 'commit' | string
  size: number
  sha256?: string
  attempts: number
  errorMessage?: string
  updatedAt: string
}

export interface UploadSessionSnapshot {
  session: UploadSession
  items: UploadSessionItem[]
}

export interface UploadHashCheckResult {
  exists: boolean
  sha256: string
  size: number
  blobId?: string
  storageKey?: string
  mimeType?: string
}

export interface UploadChunkManifest {
  totalSize: number
  chunkSize: number
  sha256: string
}

export interface UploadChunkInfo {
  partNumber: number
  offset: number
  size: number
  sha256: string
}

export interface UploadChunkSnapshot {
  manifest: UploadChunkManifest
  parts: UploadChunkInfo[]
}

export interface CreateUploadSessionInput {
  kind: UploadSession['kind']
  idempotencyKey: string
  metadata?: Record<string, unknown>
}

export interface CreateUploadSessionItemInput {
  clientRef: string
  attachmentId?: string
  drawingNo: string
  partNo?: string
  role: UploadSessionItem['role']
  originalName: string
  mimeType?: string
	expectedRevision?: number
	sha256?: string
	size?: number
	blobId?: string
}

export interface StoredAttachment {
	  id: string
	  name: string
	  storageKey: string
	  size: number
	  mimeType: string
  currentName?: string
  currentStorageKey?: string
  currentMimeType?: string
	currentSize?: number
	revision?: number
  drawingNo: string
  partNo?: string
  role: 'assembly' | 'part' | 'material' | 'craft' | 'other'
  version: string
  previewable: boolean
  uploadedBy?: string
	createdAt?: string
}

export interface DrawingBomSnapshot {
	revision: number
	items: BomItem[]
}

export interface ReplaceDrawingBomInput {
	expectedRevision: number
	items: BomItem[]
}

export interface BorrowPartInput {
	sourcePartId: string
	qty: number
	borrowReason?: string
	remark?: string
}

export interface DrawingBorrowResult {
	id: string
	drawingId: string
	partId: string
	qty: number
	revision: number
	status: string
}

export interface DrawingFileIdentity {
  partNo: string
  partNoSource?: 'titleBlock' | 'filename' | 'none'
  material?: string
  titleBlock?: Record<string, string>
}

export interface DrawingFileIdentifyOptions {
  titleBlockOnly?: boolean
}

export interface ReidentifyDrawingFileResult {
  storageKey: string
  drawingNo: string
  oldPartNo: string
  partNo: string
}

export interface UserManagementInput {
  account: string
  displayName: string
  password?: string
  roles: UserRole[]
  status?: UserStatus
}

/** 单资源更新的乐观锁请求。仅提交实际变更字段，revision 必须来自最近一次服务端读取。 */
export interface UpdateDrawingInput {
  expectedRevision: number
  name?: string
  kind?: '总图' | '零件图'
  project?: string
  material?: string
  vendor?: string
  status?: string
  ver?: string
  borrowFrom?: string
  remark?: string
  signers?: Record<string, string>
  attributeValues?: Record<string, string>
}

export interface UpdatePartInput {
  expectedRevision: number
  no?: string
  name?: string
  material?: string
  spec?: string
  weight?: number
  surfaceTreatment?: string
  partType?: string
  qty?: number
  status?: string
  ver?: string
  vendor?: string
  borrowFrom?: string
  remark?: string
}

export interface EditSessionOpenResult {
  sessionId: string
  openUrl: string
  uncPath: string
  smbRoot: string
  expiresAt: string
}

export interface ActiveEditSessionInfo {
  id: string
  attachmentId: string
  storageKey: string
  fileName: string
  drawingNo: string
  partNo?: string
  userId: string
  userName: string
  userAccount: string
  uncPath: string
  status: string
  startedAt: string
  lastSeenAt: string
  isCurrent: boolean
  canClose: boolean
  online?: boolean
}

export interface FileVersionInfo {
  id: string
  attachmentId: string
  storageKey: string
  version: string
  versionKind: 'initial' | 'working' | 'release' | string
  size: number
  mimeType?: string
  sha256?: string
  createdByName?: string
  createdAt: string
  isCurrentRelease?: boolean
}

export interface EditSessionControlResult {
  sessionId: string
  /** 结束编辑是否产生新版本（false = 无改动直接关闭） */
  changed?: boolean
  /** 新版本号（如 v1.0-w001） */
  version?: string
  /** 新的当前版本文件存储键 */
  currentStorageKey?: string
  /** 新的当前版本文件名 */
  currentName?: string
}

export interface CreatePartInput {
  no: string
  name: string
  parentNo: string
  material: string
  spec: string
  weight: number
  surfaceTreatment: string
  manufacturingType: string
  quantity: number
  status: string
  version: string
  project: string
  vendor?: string
  borrowFrom?: string
  remark?: string
  signers?: Record<string, string>
}

export interface DataProvider {
	loadDrawings(): Promise<Drawing[]>
	loadStructure(): Promise<StructurePart[]>
	saveStructure(items: StructurePart[]): Promise<void>
	createPart?(drawingId: string, input: CreatePartInput): Promise<StructurePart>
  loadAttributes(): Promise<DrawingAttribute[]>
  saveAttributes(items: DrawingAttribute[]): Promise<void>
  loadVersions(): Promise<DrawingVersion[]>
  saveVersions(items: DrawingVersion[]): Promise<void>
	loadBranches(): Promise<Branch[]>
	loadBorrows(): Promise<BorrowRecord[]>
	loadBom(): Promise<BomItem[]>
	saveBom(items: BomItem[]): Promise<void>
	loadDrawingBom?(drawingId: string): Promise<DrawingBomSnapshot>
	replaceDrawingBom?(drawingId: string, input: ReplaceDrawingBomInput): Promise<DrawingBomSnapshot>
	borrowPart?(drawingId: string, input: BorrowPartInput): Promise<DrawingBorrowResult>
  loadCrafts(): Promise<CraftFile[]>
  saveCrafts(items: CraftFile[]): Promise<void>
	loadAttachments(): Promise<StoredAttachment[]>
	updateDrawing?(drawingId: string, input: UpdateDrawingInput): Promise<Drawing>
	updatePart?(partId: string, input: UpdatePartInput): Promise<StructurePart>
  createUploadSession(input: CreateUploadSessionInput): Promise<UploadSession>
	createUploadSessionItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem>
	checkUploadHash(file: Blob): Promise<UploadHashCheckResult>
	initUploadChunks(sessionId: string, itemId: string, manifest: UploadChunkManifest): Promise<UploadChunkSnapshot>
	listUploadChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot>
	uploadSessionChunk(sessionId: string, itemId: string, partNumber: number, chunk: Blob): Promise<UploadChunkInfo>
	completeUploadChunks(sessionId: string, itemId: string): Promise<UploadSessionItem>
  uploadSessionItem(sessionId: string, itemId: string, file: Blob, name?: string): Promise<UploadSessionItem>
  retryUploadSessionItem(sessionId: string, itemId: string): Promise<UploadSessionItem>
  retryUploadSessionConversion(sessionId: string, itemId: string): Promise<UploadSessionItem>
  getUploadSession(sessionId: string): Promise<UploadSessionSnapshot>
  commitUploadSession(sessionId: string): Promise<Record<string, unknown>>
  cancelUploadSession(sessionId: string): Promise<void>
  deleteAttachment(storageKey: string): Promise<void>
  readAttachment(storageKey: string): Promise<Blob>
  exportBOM?(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob>
  scanDrawingDesigner?(drawingNo: string): Promise<string>
  identifyDrawingFile?(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity>
  identifyDrawingMaterial?(file: Blob, name: string): Promise<DrawingFileIdentity>
  reidentifyDrawingFile?(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult>
  listUsers?(): Promise<UserAccount[]>
  createUser?(input: UserManagementInput): Promise<UserAccount>
  updateUser?(userId: string, input: UserManagementInput): Promise<UserAccount>
  listEditSessions?(drawingNo?: string): Promise<ActiveEditSessionInfo[]>
  openEditSession?(storageKey: string): Promise<EditSessionOpenResult>
  heartbeatEditSession?(sessionId: string): Promise<EditSessionControlResult>
  closeEditSession?(sessionId: string): Promise<EditSessionControlResult>
  listFileVersions?(storageKey: string): Promise<FileVersionInfo[]>
  downloadFileVersion?(versionId: string): Promise<Blob>
  restoreFileVersion?(versionId: string): Promise<void>
}
