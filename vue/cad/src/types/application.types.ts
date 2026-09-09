import type { BomItem, UserRole, UserStatus } from '@/types/domain.types'

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

export interface UploadSessionSnapshot { session: UploadSession; items: UploadSessionItem[] }
export interface UploadHashCheckResult { exists: boolean; sha256: string; size: number; blobId?: string; storageKey?: string; mimeType?: string }
export interface UploadChunkManifest { totalSize: number; chunkSize: number; sha256: string }
export interface UploadChunkInfo { partNumber: number; offset: number; size: number; sha256: string }
export interface UploadChunkSnapshot { manifest: UploadChunkManifest; parts: UploadChunkInfo[] }
export interface CreateUploadSessionInput { kind: UploadSession['kind']; idempotencyKey: string; metadata?: Record<string, unknown> }
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
  currentVersionId?: string
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
  author?: string
  createdAt?: string
}

export interface DrawingBomSnapshot { revision: number; items: BomItem[] }
export interface ReplaceDrawingBomInput { expectedRevision: number; items: BomItem[] }
export interface BorrowPartInput { sourcePartId: string; qty: number; borrowReason?: string; remark?: string }
export interface DrawingBorrowResult { id: string; drawingId: string; partId: string; qty: number; revision: number; status: string }
export interface DrawingFileIdentity { partNo: string; partNoSource?: 'titleBlock' | 'filename' | 'none'; material?: string; titleBlock?: Record<string, string> }
export interface DrawingFileIdentifyOptions { titleBlockOnly?: boolean }
export interface ReidentifyDrawingFileResult { storageKey: string; drawingNo: string; oldPartNo: string; partNo: string }
export interface UserManagementInput { account: string; displayName: string; password?: string; roles: UserRole[]; status?: UserStatus }

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
  expectedRelationRevision?: number
  relationId?: string
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

export interface EditSessionOpenResult { sessionId: string; openUrl: string; uncPath: string; smbRoot: string; expiresAt: string }
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
  changed?: boolean
  version?: string
  currentStorageKey?: string
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
