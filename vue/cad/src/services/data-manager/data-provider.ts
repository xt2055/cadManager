import type { DataDocument } from './data.types'
import type { UserAccount, UserRole, UserStatus } from '@/types/domain.types'

export interface AttachmentMetadata {
  name: string
  mimeType?: string
  storageKey?: string
  drawingNo?: string
  partNo?: string
  role?: 'assembly' | 'part' | 'material' | 'craft' | 'other'
  version?: string
  previewable?: boolean
}

export interface AttachmentResult {
  storageKey: string
  size: number
  mimeType: string
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
  load(): Promise<DataDocument>
  save(document: DataDocument): Promise<void>
  uploadAttachment(file: Blob, metadata: AttachmentMetadata): Promise<AttachmentResult>
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
