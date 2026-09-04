import { invoke } from '@tauri-apps/api/core'

import { normalizeAttributes, normalizeBom, normalizeBranches, normalizeBorrows, normalizeCraftFile, normalizeDrawings, normalizeStructure, normalizeVersions, readSeedModule } from './data.types'
import type { BomItem, Branch, BorrowRecord, CraftFile, Drawing, DrawingAttribute, DrawingVersion, StructurePart } from '@/types/domain.types'
import { deleteBrowserAttachment, readBrowserAttachment } from './attachment-storage'
import type { DataProvider, UserManagementInput, StoredAttachment, CreateUploadSessionInput, CreateUploadSessionItemInput, UploadSession, UploadSessionItem, UploadSessionSnapshot, UploadHashCheckResult, UploadChunkManifest, UploadChunkSnapshot, UploadChunkInfo } from './data-provider'
import type { UserAccount } from '@/types/domain.types'
import seedDocument from './data.seed.json'

const MODULE_STORAGE_PREFIX = 'cad:data-module:v1:'

function isTauriRuntime(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

export class JsonDataProvider implements DataProvider {
  private readModule<T>(name: string, fallback: T): T {
    const raw = window.localStorage.getItem(`${MODULE_STORAGE_PREFIX}${name}`)
    if (!raw) return fallback
    try {
      return JSON.parse(raw) as T
    } catch (error) {
      throw new Error(`本地模块数据解析失败：${error instanceof Error ? error.message : String(error)}`)
    }
  }

  private writeModule<T>(name: string, value: T): void {
    window.localStorage.setItem(`${MODULE_STORAGE_PREFIX}${name}`, JSON.stringify(value, null, 2))
  }

	loadDrawings(): Promise<Drawing[]> { return Promise.resolve(normalizeDrawings(this.readModule('drawings', readSeedModule(seedDocument, 'drawings', [])))) }
  loadStructure(): Promise<StructurePart[]> { return Promise.resolve(normalizeStructure(this.readModule('structure', readSeedModule(seedDocument, 'structure', [])))) }
  saveStructure(items: StructurePart[]): Promise<void> { this.writeModule('structure', items); return Promise.resolve() }
  loadAttributes(): Promise<DrawingAttribute[]> { return Promise.resolve(normalizeAttributes(this.readModule('attributes', readSeedModule(seedDocument, 'attributes', [])))) }
  saveAttributes(items: DrawingAttribute[]): Promise<void> { this.writeModule('attributes', items); return Promise.resolve() }
  loadVersions(): Promise<DrawingVersion[]> { return Promise.resolve(normalizeVersions(this.readModule('versions', [] as DrawingVersion[]))) }
  saveVersions(items: DrawingVersion[]): Promise<void> { this.writeModule('versions', items); return Promise.resolve() }
  loadBranches(): Promise<Branch[]> { return Promise.resolve(normalizeBranches(this.readModule('branches', readSeedModule(seedDocument, 'branches', [])))) }
  saveBranches(items: Branch[]): Promise<void> { this.writeModule('branches', items); return Promise.resolve() }
  loadBorrows(): Promise<BorrowRecord[]> { return Promise.resolve(normalizeBorrows(this.readModule('borrows', readSeedModule(seedDocument, 'borrows', [])))) }
  saveBorrows(items: BorrowRecord[]): Promise<void> { this.writeModule('borrows', items); return Promise.resolve() }
  loadBom(): Promise<BomItem[]> { return Promise.resolve(normalizeBom(this.readModule('bom', readSeedModule(seedDocument, 'bom', [])))) }
  saveBom(items: BomItem[]): Promise<void> { this.writeModule('bom', items); return Promise.resolve() }
  loadCrafts(): Promise<CraftFile[]> { return Promise.resolve(this.readModule('crafts', [] as CraftFile[])) }
  saveCrafts(items: CraftFile[]): Promise<void> { this.writeModule('crafts', items); return Promise.resolve() }
  loadAttachments(): Promise<StoredAttachment[]> { return Promise.resolve(this.readModule('attachments', [] as StoredAttachment[])) }

  async createUploadSession(input: CreateUploadSessionInput): Promise<UploadSession> {
    const now = new Date().toISOString()
    const session: UploadSession = {
      id: `local-upload-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      kind: input.kind,
      idempotencyKey: input.idempotencyKey,
      status: 'open',
      metadata: input.metadata || {},
      createdAt: now,
      lastActivityAt: now,
      expiresAt: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
      absoluteExpiresAt: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
    }
    this.writeModule(`upload-session:${session.id}`, { session, items: [] })
    return session
  }

	async createUploadSessionItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem> {
		throw new Error('本地 JSON 模式暂不支持上传会话，请切换到服务端存储模式')
	}

	async checkUploadHash(file: Blob): Promise<UploadHashCheckResult> {
		if (!globalThis.crypto?.subtle) throw new Error('当前浏览器不支持 SHA-256 计算')
		const digest = await globalThis.crypto.subtle.digest('SHA-256', await file.arrayBuffer())
		const sha256 = Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, '0')).join('')
		return { exists: false, sha256, size: file.size, mimeType: file.type || 'application/octet-stream' }
	}

	async initUploadChunks(_sessionId: string, _itemId: string, _manifest: UploadChunkManifest): Promise<UploadChunkSnapshot> { throw new Error('本地 JSON 模式暂不支持分片上传') }
	async listUploadChunks(_sessionId: string, _itemId: string): Promise<UploadChunkSnapshot> { throw new Error('本地 JSON 模式暂不支持分片上传') }
	async uploadSessionChunk(_sessionId: string, _itemId: string, _partNumber: number, _chunk: Blob): Promise<UploadChunkInfo> { throw new Error('本地 JSON 模式暂不支持分片上传') }
	async completeUploadChunks(_sessionId: string, _itemId: string): Promise<UploadSessionItem> { throw new Error('本地 JSON 模式暂不支持分片上传') }

  async uploadSessionItem(_sessionId: string, _itemId: string, _file: Blob, _name?: string): Promise<UploadSessionItem> {
    throw new Error('本地 JSON 模式暂不支持上传会话，请切换到服务端存储模式')
  }

  async retryUploadSessionItem(_sessionId: string, _itemId: string): Promise<UploadSessionItem> {
    throw new Error('本地 JSON 模式暂不支持上传会话，请切换到服务端存储模式')
  }

  async getUploadSession(sessionId: string): Promise<UploadSessionSnapshot> {
    return this.readModule(`upload-session:${sessionId}`, { session: {} as UploadSession, items: [] })
  }

  async commitUploadSession(_sessionId: string): Promise<Record<string, unknown>> {
    throw new Error('本地 JSON 模式暂不支持上传会话，请切换到服务端存储模式')
  }

  async cancelUploadSession(sessionId: string): Promise<void> {
    window.localStorage.removeItem(`${MODULE_STORAGE_PREFIX}upload-session:${sessionId}`)
  }

  async identifyDrawingFile(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持从图纸内容读取图号，请切换到服务端存储模式')
  }

  async identifyDrawingMaterial(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持从图纸内容读取材料，请切换到服务端存储模式')
  }

  async reidentifyDrawingFile(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持校正历史图纸图号，请切换到服务端存储模式')
  }

  async deleteAttachment(storageKey: string): Promise<void> {
    if (isTauriRuntime()) {
      await invoke('delete_attachment', { storageKey })
      return
    }
    await deleteBrowserAttachment(storageKey)
  }

  async readAttachment(storageKey: string): Promise<Blob> {
    if (isTauriRuntime()) {
      const bytes = await invoke<number[]>('read_attachment', { storageKey })
      return new Blob([new Uint8Array(bytes)], { type: 'application/octet-stream' })
    }
    return readBrowserAttachment(storageKey)
  }

  async exportBOM(drawingNo: string, _storageKey: string, items: unknown[]): Promise<Blob> {
    const XLSX = await import('xlsx')
    const workbook = XLSX.utils.book_new()
    const rows = (items as Array<Record<string, unknown>>).map((item, index) => ({
      序号: item.no ?? index + 1,
      '图号/标准号': item.id ?? '',
      名称: item.name ?? '',
      '规格/材质': item.spec ?? '',
      数量: item.qty ?? 1,
      '单重(kg)': item.weight ?? 0,
      备注: item.remark ?? '',
    }))
    const worksheet = XLSX.utils.json_to_sheet(rows)
    XLSX.utils.book_append_sheet(workbook, worksheet, '备料明细')
    const buffer = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' })
    return new Blob([buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  }

  async scanDrawingDesigner(_drawingNo: string): Promise<string> {
    return ''
  }

  async listUsers(): Promise<UserAccount[]> {
    return this.readModule('users', readSeedModule(seedDocument, 'users', [] as UserAccount[]))
  }

  async createUser(input: UserManagementInput): Promise<UserAccount> {
    const users = await this.listUsers()
    const account = input.account.trim().toLowerCase()
    if (users.some((user) => user.account.toLowerCase() === account)) throw new Error('登录账号已存在')
    const user: UserAccount = {
      id: `user-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      account,
      displayName: input.displayName.trim(),
      password: input.password?.trim() || '',
      roles: [...new Set(input.roles)],
      status: 'active',
      createdAt: new Date().toISOString(),
      lastLoginAt: null,
    }
    users.unshift(user)
    this.writeModule('users', users)
    return user
  }

  async updateUser(userId: string, input: UserManagementInput): Promise<UserAccount> {
    const users = await this.listUsers()
    const user = users.find((item) => item.id === userId)
    if (!user) throw new Error('账号不存在')
    user.account = input.account.trim().toLowerCase()
    user.displayName = input.displayName.trim()
    user.roles = [...new Set(input.roles)]
    user.status = input.status || 'active'
    if (input.password?.trim()) user.password = input.password.trim()
    this.writeModule('users', users)
    return user
  }

  async openEditSession(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持 SMB CAD 编辑，请切换到服务端存储模式')
  }
}
