import type {
  DrawingFileIdentity,
  DrawingFileIdentifyOptions,
  ReidentifyDrawingFileResult,
} from '@/types/application.types'

export interface DrawingFileGateway {
  readAttachment(storageKey: string): Promise<Blob>
  exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob>
  identifyDrawingFile(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity>
  identifyDrawingMaterial(file: Blob, name: string): Promise<DrawingFileIdentity>
  reidentifyDrawingFile(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult>
  scanDrawingDesigner(drawingNo: string): Promise<string>
  deleteAttachment(storageKey: string): Promise<void>
}

/** 图纸文件应用服务：识别、读取和导出能力通过 Gateway 注入，页面不接触底层 HTTP 客户端。 */
export class DrawingFileService {
  public constructor(private readonly gateway: DrawingFileGateway) {}

  read(storageKey: string): Promise<Blob> { return this.gateway.readAttachment(storageKey) }
  exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob> { return this.gateway.exportBOM(drawingNo, storageKey, items) }
  identify(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity> {
    return this.gateway.identifyDrawingFile(file, name, options)
  }
  identifyMaterial(file: Blob, name: string): Promise<DrawingFileIdentity> { return this.gateway.identifyDrawingMaterial(file, name) }
  reidentify(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult> {
    return this.gateway.reidentifyDrawingFile(storageKey, partNo)
  }
  scanDesigner(drawingNo: string): Promise<string> { return this.gateway.scanDrawingDesigner(drawingNo) }
  delete(storageKey: string): Promise<void> { return this.gateway.deleteAttachment(storageKey) }
}

export type { DrawingFileIdentity, DrawingFileIdentifyOptions, ReidentifyDrawingFileResult }
