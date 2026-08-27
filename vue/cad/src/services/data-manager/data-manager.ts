import { ApiDataProvider } from './api-data-provider'
import { createEmptyDataDocument, normalizeDataDocument, type DataDocument } from './data.types'
import type { DataProvider } from './data-provider'
import type { AttachmentMetadata, AttachmentResult } from './data-provider'
import { JsonDataProvider } from './json-data-provider'
import { readDebugMode } from '@/services/runtime-config.service'

export interface DataManager {
  load(): Promise<DataDocument>
  save(document: DataDocument): Promise<void>
  uploadAttachment(file: Blob, metadata: AttachmentMetadata): Promise<AttachmentResult>
  deleteAttachment(storageKey: string): Promise<void>
  readAttachment(storageKey: string): Promise<Blob>
  exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob>
  scanDrawingDesigner(drawingNo: string): Promise<string>
  resetProvider(): void
}

export async function createDataProvider(): Promise<DataProvider> {
  const configuredProvider = import.meta.env.VITE_DATA_PROVIDER
  if (configuredProvider === 'json') return new JsonDataProvider()
  if (configuredProvider === 'api') return new ApiDataProvider()
  return (await readDebugMode()) ? new JsonDataProvider() : new ApiDataProvider()
}

export class DefaultDataManager implements DataManager {
  private provider: DataProvider | null
  private providerPromise: Promise<DataProvider> | null

  constructor(provider?: DataProvider) {
    this.provider = provider ?? null
    this.providerPromise = provider ? Promise.resolve(provider) : null
  }

  private async getProvider(): Promise<DataProvider> {
    if (this.provider) return this.provider
    if (!this.providerPromise) {
      this.providerPromise = createDataProvider()
    }
    this.provider = await this.providerPromise
    return this.provider
  }

  async load(): Promise<DataDocument> {
    return normalizeDataDocument(await (await this.getProvider()).load())
  }

  async save(document: DataDocument): Promise<void> {
    await (await this.getProvider()).save(normalizeDataDocument(document))
  }

  uploadAttachment(file: Blob, metadata: AttachmentMetadata): Promise<AttachmentResult> {
    return this.getProvider().then((provider) => provider.uploadAttachment(file, metadata))
  }

  deleteAttachment(storageKey: string): Promise<void> {
    return this.getProvider().then((provider) => provider.deleteAttachment(storageKey))
  }

  readAttachment(storageKey: string): Promise<Blob> {
    return this.getProvider().then((provider) => provider.readAttachment(storageKey))
  }

  async exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob> {
    const provider = await this.getProvider()
    if (provider.exportBOM) {
      return provider.exportBOM(drawingNo, storageKey, items)
    }
    throw new Error('当前存储模式不支持服务端 BOM 导出')
  }

  async scanDrawingDesigner(drawingNo: string): Promise<string> {
    const provider = await this.getProvider()
    return provider.scanDrawingDesigner ? provider.scanDrawingDesigner(drawingNo) : ''
  }

  resetProvider(): void {
    this.provider = null
    this.providerPromise = null
  }
}

export const dataManager: DataManager = new DefaultDataManager()

export function emptyDataDocument(): DataDocument {
  return createEmptyDataDocument()
}
