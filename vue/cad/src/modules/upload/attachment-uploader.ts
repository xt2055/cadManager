import type { UploadHashCheckResult } from '@/types/application.types'
import type { UploadGateway } from './upload-gateway'

export type AttachmentRole = 'assembly' | 'part' | 'material' | 'craft' | 'other'

export interface AttachmentTarget {
  id: string
  name: string
  revision?: number
  role: AttachmentRole
	partNo?: string
	createPart?: Record<string, unknown>
}

export class AttachmentUploader {
  public constructor(private readonly gateway: UploadGateway) {}

  async replace(drawingNo: string, target: AttachmentTarget, content: Blob): Promise<Record<string, unknown>> {
    const name = getBlobName(content) || target.name
    const hash = await this.safeHash(content)
    const revision = target.revision ?? 1
    const session = await this.gateway.createSession({
      kind: 'attachment',
      idempotencyKey: `attachment-replace:${target.id}:${revision}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
      metadata: { drawingNo, attachmentId: target.id, expectedRevision: revision },
    })
    try {
      const item = await this.gateway.createItem(session.id, {
        clientRef: target.id,
        attachmentId: target.id,
        drawingNo,
        ...(target.partNo ? { partNo: target.partNo } : {}),
        role: target.role,
        originalName: name,
        mimeType: content.type || 'application/octet-stream',
        expectedRevision: revision,
        sha256: hash.sha256,
        size: content.size,
        ...(!isCAD(name) && hash.exists && hash.blobId ? { blobId: hash.blobId } : {}),
      })
      if (item.status !== 'ready') await this.gateway.uploadFile(session.id, item.id, content, { name, sha256: hash.sha256 })
      return this.gateway.commitSession(session.id)
    } catch (error) {
      await this.gateway.cancelSession(session.id).catch(() => undefined)
      throw error
    }
  }

  async create(drawingNo: string, target: AttachmentTarget, content: Blob): Promise<Record<string, unknown>> {
    const name = getBlobName(content) || target.name
    const hash = await this.safeHash(content)
    const session = await this.gateway.createSession({
      kind: 'attachment',
      idempotencyKey: `attachment-create:${target.id}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`,
	      metadata: {
	        drawingNo,
	        partNo: target.partNo || '',
	        role: target.role,
	        ...(target.createPart ? { createPart: target.createPart } : {}),
	      },
    })
    try {
      const item = await this.gateway.createItem(session.id, {
        clientRef: target.id,
        drawingNo,
        ...(target.partNo ? { partNo: target.partNo } : {}),
        role: target.role,
        originalName: name,
        mimeType: content.type || 'application/octet-stream',
        sha256: hash.sha256,
        size: content.size,
        ...(!isCAD(name) && hash.exists && hash.blobId ? { blobId: hash.blobId } : {}),
      })
      if (item.status !== 'ready') await this.gateway.uploadFile(session.id, item.id, content, { name, sha256: hash.sha256 })
      return this.gateway.commitSession(session.id)
    } catch (error) {
      await this.gateway.cancelSession(session.id).catch(() => undefined)
      throw error
    }
  }

  private async safeHash(file: Blob): Promise<UploadHashCheckResult> {
    try {
      return await this.gateway.hashCheck(file)
    } catch {
      return { exists: false, sha256: '', size: file.size, mimeType: file.type || 'application/octet-stream' }
    }
  }
}

function isCAD(name: string): boolean {
  const extension = name.slice(name.lastIndexOf('.')).toLowerCase()
  return extension === '.exb' || extension === '.dwg' || extension === '.dxf'
}

function getBlobName(content: Blob): string | undefined {
  return typeof File !== 'undefined' && content instanceof File ? content.name : undefined
}
