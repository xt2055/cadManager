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

export interface DataProvider {
  loadDrawings(): Promise<Drawing[]>
  saveDrawings(items: Drawing[]): Promise<void>
  loadStructure(): Promise<StructurePart[]>
  saveStructure(items: StructurePart[]): Promise<void>
  loadAttributes(): Promise<DrawingAttribute[]>
  saveAttributes(items: DrawingAttribute[]): Promise<void>
  loadVersions(): Promise<DrawingVersion[]>
  saveVersions(items: DrawingVersion[]): Promise<void>
  loadBranches(): Promise<Branch[]>
  saveBranches(items: Branch[]): Promise<void>
  loadBorrows(): Promise<BorrowRecord[]>
  saveBorrows(items: BorrowRecord[]): Promise<void>
  loadBom(): Promise<BomItem[]>
  saveBom(items: BomItem[]): Promise<void>
  loadCrafts(): Promise<CraftFile[]>
  saveCrafts(items: CraftFile[]): Promise<void>
  loadAttachments(): Promise<StoredAttachment[]>
  createUploadSession(input: CreateUploadSessionInput): Promise<UploadSession>
	createUploadSessionItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem>
	checkUploadHash(file: Blob): Promise<UploadHashCheckResult>
	initUploadChunks(sessionId: string, itemId: string, manifest: UploadChunkManifest): Promise<UploadChunkSnapshot>
	listUploadChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot>
	uploadSessionChunk(sessionId: string, itemId: string, partNumber: number, chunk: Blob): Promise<UploadChunkInfo>
	completeUploadChunks(sessionId: string, itemId: string): Promise<UploadSessionItem>
  uploadSessionItem(sessionId: string, itemId: string, file: Blob, name?: string): Promise<UploadSessionItem>
  retryUploadSessionItem(sessionId: string, itemId: string): Promise<UploadSessionItem>
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
