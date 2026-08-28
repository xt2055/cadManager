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
  material?: string
  titleBlock?: Record<string, string>
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

export interface DataProvider {
  load(): Promise<DataDocument>
  save(document: DataDocument): Promise<void>
  uploadAttachment(file: Blob, metadata: AttachmentMetadata): Promise<AttachmentResult>
  deleteAttachment(storageKey: string): Promise<void>
  readAttachment(storageKey: string): Promise<Blob>
  exportBOM?(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob>
  scanDrawingDesigner?(drawingNo: string): Promise<string>
  identifyDrawingFile?(file: Blob, name: string): Promise<DrawingFileIdentity>
  identifyDrawingMaterial?(file: Blob, name: string): Promise<DrawingFileIdentity>
  reidentifyDrawingFile?(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult>
  listUsers?(): Promise<UserAccount[]>
  createUser?(input: UserManagementInput): Promise<UserAccount>
  updateUser?(userId: string, input: UserManagementInput): Promise<UserAccount>
}
