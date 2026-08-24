import type { DataDocument } from './data.types'

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
}

export interface DataProvider {
  load(): Promise<DataDocument>
  save(document: DataDocument): Promise<void>
  uploadAttachment(file: Blob, metadata: AttachmentMetadata): Promise<AttachmentResult>
  deleteAttachment(storageKey: string): Promise<void>
  readAttachment(storageKey: string): Promise<Blob>
}
