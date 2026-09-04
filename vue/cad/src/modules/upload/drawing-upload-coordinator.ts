import type { DrawingFile, MaterialFile, CraftFile } from '@/types/domain.types'
import type { UploadHashCheckResult, UploadSession, UploadSessionItem } from '@/services/data-manager/data-provider'
import type { UploadGateway } from './upload-gateway'
import type { UploadRecoveryStore } from './upload-recovery-store'

export type DrawingUploadFile = DrawingFile | MaterialFile | CraftFile

export interface PendingDrawingUploadEntry {
  itemId: string
  file: DrawingUploadFile
  content: Blob
}

export interface DrawingUploadInput {
  file: DrawingUploadFile
  content?: Blob
}

export interface DrawingUploadPlan {
  drawingNo: string
  metadata: Record<string, unknown>
  files: DrawingUploadInput[]
  onProgress?: (itemId: string, percent: number) => void
  onSessionCreated?: (session: UploadSession, entries: Map<string, PendingDrawingUploadEntry>) => void
}

export interface DrawingUploadSessionResult {
  session: UploadSession
  entries: Map<string, PendingDrawingUploadEntry>
}

/**
 * drawing-create 的文件编排器：会话、预检、分片/普通上传、刷新恢复和失败重试
 * 都在这里处理；Store 只负责组装领域 metadata 和刷新 Read Model。
 */
export class DrawingUploadCoordinator {
  public constructor(
    private readonly gateway: UploadGateway,
    private readonly recovery: UploadRecoveryStore,
  ) {}

  async create(plan: DrawingUploadPlan): Promise<DrawingUploadSessionResult> {
    const session = await this.gateway.createSession({
      kind: 'drawing-create',
      idempotencyKey: `drawing-create:${plan.drawingNo}:${Date.now()}:${Math.random().toString(36).slice(2, 10)}`,
      metadata: plan.metadata,
    })
    const entries = new Map<string, PendingDrawingUploadEntry>()
    plan.onSessionCreated?.(session, entries)

    const uploadItems = new Map<string, { itemId: string; file: DrawingUploadFile }>()
    for (const input of plan.files) {
      if (!input.content) throw new Error(`文件「${input.file.name}」缺少文件内容（上传会话：${session.id}）`)
      await this.recovery.save(session.id, input.file.id, input.content, input.file.name).catch((error) => {
        console.warn(`保存文件「${input.file.name}」的刷新恢复副本失败`, error)
      })
      const hashCheck = await this.safeHash(input.content, input.file.name)
      const item = await this.gateway.createItem(session.id, {
        clientRef: input.file.id,
        drawingNo: plan.drawingNo,
        ...('role' in input.file && input.file.partNo ? { partNo: input.file.partNo } : {}),
        role: this.roleOf(input.file),
        originalName: input.file.name,
        mimeType: input.content.type || 'application/octet-stream',
        sha256: hashCheck.sha256,
        size: input.content.size,
        ...(!this.isCAD(input.file.name) && hashCheck.exists && hashCheck.blobId ? { blobId: hashCheck.blobId } : {}),
      })
      uploadItems.set(input.file.id, { itemId: item.id, file: input.file })
      entries.set(input.file.id, { itemId: item.id, file: input.file, content: input.content })
    }

    const failedFiles: string[] = []
    const queue = [...uploadItems.values()]
    const worker = async (): Promise<void> => {
      while (queue.length) {
        const entry = queue.shift()
        if (!entry) return
        const pending = entries.get(entry.file.id)
        if (!pending) {
          failedFiles.push(entry.file.name)
          continue
        }
        const snapshot = await this.gateway.getSession(session.id)
        if (snapshot.items.find((item) => item.id === entry.itemId)?.status === 'ready') {
          plan.onProgress?.(entry.itemId, 100)
          continue
        }
        try {
          const hash = await this.safeUploadHash(pending.content, entry.file.name)
          await this.uploadItem(session.id, entry.itemId, pending.content, entry.file.name, hash, plan.onProgress)
        } catch (error) {
          failedFiles.push(`${entry.file.name}：${error instanceof Error ? error.message : String(error)}`)
        }
      }
    }
    await Promise.all(Array.from({ length: Math.min(4, Math.max(queue.length, 1)) }, () => worker()))
    if (failedFiles.length) {
      throw new Error(`有 ${failedFiles.length} 个文件上传失败，可使用会话 ${session.id} 查询并单独重试：${failedFiles.join('；')}`)
    }
    return { session, entries }
  }

  async retryFailed(
    sessionId: string,
    entries: Map<string, PendingDrawingUploadEntry>,
    onProgress?: (itemId: string, percent: number) => void,
  ): Promise<Record<string, unknown>> {
    const snapshot = await this.gateway.getSession(sessionId)
    const failed = snapshot.items.filter((item) => item.status === 'failed')
    if (!failed.length && snapshot.items.some((item) => item.status !== 'ready')) throw new Error('仍有文件未准备完成')

    const errors: string[] = []
	    for (const item of failed) {
	      if (item.failureStage === 'conversion') {
	        try {
	          await this.gateway.retryConversion(sessionId, item.id)
	          onProgress?.(item.id, 100)
	        } catch (error) {
	          errors.push(`${item.originalName}：${error instanceof Error ? error.message : String(error)}`)
	        }
	        continue
	      }
	      const entry = [...entries.values()].find((candidate) => candidate.itemId === item.id)
      if (!entry) {
        errors.push(`${item.originalName}：浏览器中已找不到原始文件`)
        continue
      }
      try {
        await this.retryItem(sessionId, item, entry, onProgress)
      } catch (error) {
        errors.push(`${entry.file.name}：${error instanceof Error ? error.message : String(error)}`)
      }
    }
    if (errors.length) throw new Error(`仍有 ${errors.length} 个文件上传失败：${errors.join('；')}`)
    const updated = await this.gateway.getSession(sessionId)
    if (updated.items.some((item) => item.status !== 'ready')) throw new Error('仍有文件未准备完成，请继续重试失败项')
    return this.gateway.commitSession(sessionId)
  }

  async retryOne(
    sessionId: string,
    itemId: string,
    entries: Map<string, PendingDrawingUploadEntry>,
    onProgress?: (itemId: string, percent: number) => void,
  ): Promise<void> {
    const snapshot = await this.gateway.getSession(sessionId)
    const item = snapshot.items.find((candidate) => candidate.id === itemId)
    if (!item) throw new Error('上传文件项不存在')
	    if (item.status === 'ready' || item.status === 'committed') return
	    if (item.failureStage === 'conversion') {
	      await this.gateway.retryConversion(sessionId, itemId)
	      onProgress?.(itemId, 100)
	      return
	    }
	    const entry = [...entries.values()].find((candidate) => candidate.itemId === itemId)
    if (!entry) throw new Error(`浏览器中已找不到原始文件：${item.originalName}`)
    await this.retryItem(sessionId, item, entry, onProgress)
  }

  private async retryItem(
    sessionId: string,
    item: UploadSessionItem,
    entry: PendingDrawingUploadEntry,
    onProgress?: (itemId: string, percent: number) => void,
  ): Promise<void> {
    await this.gateway.retryItem(sessionId, item.id)
    const hash = await this.safeUploadHash(entry.content, entry.file.name)
    await this.uploadItem(sessionId, item.id, entry.content, entry.file.name, hash, onProgress)
  }

  private async uploadItem(
    sessionId: string,
    itemId: string,
    content: Blob,
    name: string,
    sha256: string,
    onProgress?: (itemId: string, percent: number) => void,
  ): Promise<void> {
    onProgress?.(itemId, 0)
    await this.gateway.uploadFile(sessionId, itemId, content, {
      name,
      sha256,
      onProgress: (percent) => onProgress?.(itemId, percent),
    })
  }

  private async safeHash(content: Blob, name: string): Promise<UploadHashCheckResult> {
    try {
      return await this.gateway.hashCheck(content)
    } catch (error) {
      console.warn(`文件「${name}」哈希预检失败，改为完整上传`, error)
      return { exists: false, sha256: '', size: content.size, mimeType: content.type || 'application/octet-stream' }
    }
  }

  private async safeUploadHash(content: Blob, name: string): Promise<string> {
    return (await this.safeHash(content, name)).sha256
  }

  private roleOf(file: DrawingUploadFile): UploadSessionItem['role'] {
    return 'role' in file ? file.role : ('op' in file ? 'craft' : 'material')
  }

  private isCAD(name: string): boolean {
    return ['.exb', '.dwg', '.dxf'].includes(name.slice(name.lastIndexOf('.')).toLowerCase())
  }
}
