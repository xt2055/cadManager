import type {
  DrawingFileIdentity,
  DrawingFileIdentifyOptions,
  ReidentifyDrawingFileResult,
} from '@/types/application.types'

export interface DrawingFileGateway {
  setPrimaryModel(storageKey: string, attachmentId: string, expectedRevision: number): Promise<void>
  readAttachment(storageKey: string): Promise<Blob>
  exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob>
  identifyDrawingFile(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity>
  identifyDrawingMaterial(file: Blob, name: string): Promise<DrawingFileIdentity>
	reidentifyDrawingFile(storageKey: string, partNo: string, attachmentId?: string): Promise<ReidentifyDrawingFileResult>
  scanDrawingDesigner(drawingNo: string): Promise<string>
	deleteAttachment(storageKey: string, attachmentId?: string): Promise<void>
  updateAttachmentAuthor(storageKey: string, attachmentId: string, author: string): Promise<void>
}

/** 图纸文件应用服务：识别、读取和导出能力通过 Gateway 注入，页面不接触底层 HTTP 客户端。 */
export class DrawingFileService {
  public constructor(private readonly gateway: DrawingFileGateway) {}

  read(storageKey: string): Promise<Blob> { return this.gateway.readAttachment(storageKey) }
  setPrimaryModel(storageKey: string, attachmentId: string, expectedRevision: number): Promise<void> {
    return this.gateway.setPrimaryModel(storageKey, attachmentId, expectedRevision)
  }
  exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob> { return this.gateway.exportBOM(drawingNo, storageKey, items) }
  identify(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity> {
    return this.gateway.identifyDrawingFile(file, name, options)
  }
  identifyMaterial(file: Blob, name: string): Promise<DrawingFileIdentity> { return this.gateway.identifyDrawingMaterial(file, name) }
	reidentify(storageKey: string, partNo: string, attachmentId?: string): Promise<ReidentifyDrawingFileResult> {
		return this.gateway.reidentifyDrawingFile(storageKey, partNo, attachmentId)
  }
  scanDesigner(drawingNo: string): Promise<string> { return this.gateway.scanDrawingDesigner(drawingNo) }
	delete(storageKey: string, attachmentId?: string): Promise<void> { return this.gateway.deleteAttachment(storageKey, attachmentId) }
  updateAuthor(storageKey: string, attachmentId: string, author: string): Promise<void> {
    return this.gateway.updateAttachmentAuthor(storageKey, attachmentId, author)
  }
}

export type { DrawingFileIdentity, DrawingFileIdentifyOptions, ReidentifyDrawingFileResult }
