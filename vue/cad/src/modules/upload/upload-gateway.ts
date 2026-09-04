import type {
  CreateUploadSessionInput,
  CreateUploadSessionItemInput,
  UploadChunkInfo,
  UploadChunkManifest,
  UploadChunkSnapshot,
  UploadHashCheckResult,
  UploadSession,
  UploadSessionItem,
  UploadSessionSnapshot,
} from '@/services/data-manager/data-provider'

export interface UploadGateway {
  createSession(input: CreateUploadSessionInput): Promise<UploadSession>
  createItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem>
  hashCheck(file: Blob): Promise<UploadHashCheckResult>
  getSession(sessionId: string): Promise<UploadSessionSnapshot>
  listChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot>
  uploadFile(
    sessionId: string,
    itemId: string,
    file: Blob,
    options?: { name?: string; sha256?: string; chunkSize?: number; onProgress?: (percent: number) => void },
  ): Promise<UploadSessionItem>
  retryItem(sessionId: string, itemId: string): Promise<UploadSessionItem>
  retryConversion(sessionId: string, itemId: string): Promise<UploadSessionItem>
  commitSession(sessionId: string): Promise<Record<string, unknown>>
  cancelSession(sessionId: string): Promise<void>
}

export interface UploadGatewayClient {
  createUploadSession(input: CreateUploadSessionInput): Promise<UploadSession>
  createUploadSessionItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem>
  checkUploadHash(file: Blob): Promise<UploadHashCheckResult>
  listUploadChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot>
  initUploadChunks(sessionId: string, itemId: string, manifest: UploadChunkManifest): Promise<UploadChunkSnapshot>
  uploadSessionChunk(sessionId: string, itemId: string, partNumber: number, chunk: Blob): Promise<UploadChunkInfo>
  completeUploadChunks(sessionId: string, itemId: string): Promise<UploadSessionItem>
  uploadSessionItem(sessionId: string, itemId: string, file: Blob, name?: string): Promise<UploadSessionItem>
  retryUploadSessionItem(sessionId: string, itemId: string): Promise<UploadSessionItem>
  retryUploadSessionConversion(sessionId: string, itemId: string): Promise<UploadSessionItem>
  getUploadSession(sessionId: string): Promise<UploadSessionSnapshot>
  commitUploadSession(sessionId: string): Promise<Record<string, unknown>>
  cancelUploadSession(sessionId: string): Promise<void>
}

export class ApiUploadGateway implements UploadGateway {
  public constructor(private readonly client: UploadGatewayClient) {}

  createSession(input: CreateUploadSessionInput): Promise<UploadSession> {
    return this.client.createUploadSession(input)
  }

  createItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem> {
    return this.client.createUploadSessionItem(sessionId, input)
  }

  hashCheck(file: Blob): Promise<UploadHashCheckResult> {
    return this.client.checkUploadHash(file)
  }

  getSession(sessionId: string): Promise<UploadSessionSnapshot> {
    return this.client.getUploadSession(sessionId)
  }

  listChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot> {
    return this.client.listUploadChunks(sessionId, itemId)
  }

  async uploadFile(
    sessionId: string,
    itemId: string,
    file: Blob,
    options: { name?: string; sha256?: string; chunkSize?: number; onProgress?: (percent: number) => void } = {},
  ): Promise<UploadSessionItem> {
    const chunkSize = options.chunkSize ?? 8 * 1024 * 1024
    if (file.size < chunkSize) {
      const item = await this.client.uploadSessionItem(sessionId, itemId, file, options.name)
      options.onProgress?.(100)
      return item
    }

    const hash = options.sha256 ? { sha256: options.sha256 } : await this.client.checkUploadHash(file)
    const manifest: UploadChunkManifest = { totalSize: file.size, chunkSize, sha256: hash.sha256 }
    const snapshot = await this.client.initUploadChunks(sessionId, itemId, manifest)
    const uploaded = new Set(snapshot.parts.map((part) => part.partNumber))
    const partCount = Math.ceil(file.size / chunkSize)
    options.onProgress?.(Math.round((uploaded.size / partCount) * 100))
    for (let partNumber = 0; partNumber < partCount; partNumber += 1) {
      if (uploaded.has(partNumber)) continue
      const start = partNumber * chunkSize
      await this.client.uploadSessionChunk(sessionId, itemId, partNumber, file.slice(start, Math.min(file.size, start + chunkSize)))
      uploaded.add(partNumber)
      options.onProgress?.(Math.round((uploaded.size / partCount) * 100))
    }
    return this.client.completeUploadChunks(sessionId, itemId)
  }

  retryItem(sessionId: string, itemId: string): Promise<UploadSessionItem> {
    return this.client.retryUploadSessionItem(sessionId, itemId)
  }

  retryConversion(sessionId: string, itemId: string): Promise<UploadSessionItem> {
    return this.client.retryUploadSessionConversion(sessionId, itemId)
  }

  commitSession(sessionId: string): Promise<Record<string, unknown>> {
    return this.client.commitUploadSession(sessionId)
  }

  cancelSession(sessionId: string): Promise<void> {
    return this.client.cancelUploadSession(sessionId)
  }
}
