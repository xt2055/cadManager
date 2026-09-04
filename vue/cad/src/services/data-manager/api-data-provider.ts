import type { BomItem, Branch, BorrowRecord, CraftFile, Drawing, DrawingAttribute, DrawingVersion, StructurePart } from '@/types/domain.types'
import { normalizeAttributes, normalizeBom, normalizeBranches, normalizeBorrows, normalizeCraftFile, normalizeDrawings, normalizeStructure, normalizeVersions } from './data.types'
import type { DataProvider, DrawingFileIdentity, DrawingFileIdentifyOptions, EditSessionControlResult, EditSessionOpenResult, ActiveEditSessionInfo, FileVersionInfo, ReidentifyDrawingFileResult, StoredAttachment, CreateUploadSessionInput, CreateUploadSessionItemInput, UploadSession, UploadSessionItem, UploadSessionSnapshot, UploadHashCheckResult, UploadChunkManifest, UploadChunkSnapshot, UploadChunkInfo } from './data-provider'
import type { UserAccount } from '@/types/domain.types'
import type { UserManagementInput } from './data-provider'
import { getApiBaseUrl } from '@/services/api-base.service'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function getAccessToken(): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token') || ''
}

function unwrapResponseData(value: unknown): unknown {
  let current = value
  for (let depth = 0; depth < 3; depth += 1) {
    if (!isRecord(current) || !('data' in current)) return current
    current = current.data
  }
  return current
}

export class ApiDataProvider implements DataProvider {
  private readonly baseUrl: string
  /** 最近一次读取/保存确认的文档更新时间，用于乐观并发校验（防止旧窗口覆盖新数据）。 */
  private documentStamp = ''

  constructor(baseUrl = getApiBaseUrl()) {
    this.baseUrl = baseUrl.replace(/\/$/, '')
  }

  private async loadModule<T>(name: string): Promise<T> {
    return this.request<T>(`/data/${name}`, { method: 'GET' })
  }

  private async saveModule<T>(name: string, items: T[]): Promise<void> {
    await this.request(`/data/${name}`, { method: 'PUT', body: JSON.stringify(items) })
  }

  async loadDrawings(): Promise<Drawing[]> { return normalizeDrawings(await this.loadModule<unknown>('drawings')) }
  saveDrawings(items: Drawing[]): Promise<void> { return this.saveModule('drawings', items) }
  async loadStructure(): Promise<StructurePart[]> { return normalizeStructure(await this.loadModule<unknown>('structure')) }
  saveStructure(items: StructurePart[]): Promise<void> { return this.saveModule('structure', items) }
  async loadAttributes(): Promise<DrawingAttribute[]> { return normalizeAttributes(await this.loadModule<unknown>('attributes')) }
  saveAttributes(items: DrawingAttribute[]): Promise<void> { return this.saveModule('attributes', items) }
  async loadVersions(): Promise<DrawingVersion[]> { return normalizeVersions(await this.loadModule<unknown>('versions').catch(() => [])) }
  saveVersions(_items: DrawingVersion[]): Promise<void> { return Promise.resolve() }
  async loadBranches(): Promise<Branch[]> { return normalizeBranches(await this.loadModule<unknown>('branches')) }
  saveBranches(items: Branch[]): Promise<void> { return this.saveModule('branches', items) }
  async loadBorrows(): Promise<BorrowRecord[]> { return normalizeBorrows(await this.loadModule<unknown>('borrows')) }
  saveBorrows(items: BorrowRecord[]): Promise<void> { return this.saveModule('borrows', items) }
  async loadBom(): Promise<BomItem[]> { return normalizeBom(await this.loadModule<unknown>('bom')) }
  saveBom(items: BomItem[]): Promise<void> { return this.saveModule('bom', items) }
  async loadCrafts(): Promise<CraftFile[]> {
    const items = await this.loadModule<unknown>('crafts').catch(() => [])
    return Array.isArray(items) ? items.map((item) => normalizeCraftFile(item, '')) : []
  }
  saveCrafts(_items: CraftFile[]): Promise<void> { return Promise.resolve() }
  loadAttachments(): Promise<StoredAttachment[]> { return this.loadModule<StoredAttachment[]>('attachments') }

  async deleteAttachment(storageKey: string): Promise<void> {
    await this.request(`/attachments/${encodeURIComponent(storageKey)}`, { method: 'DELETE' })
  }

  async createUploadSession(input: CreateUploadSessionInput): Promise<UploadSession> {
    return this.request<UploadSession>('/upload-sessions', {
      method: 'POST',
      body: JSON.stringify(input),
    })
  }

	async createUploadSessionItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem> {
    return this.request<UploadSessionItem>(`/upload-sessions/${encodeURIComponent(sessionId)}/items`, {
      method: 'POST',
      body: JSON.stringify(input),
    })
	}

	async checkUploadHash(file: Blob): Promise<UploadHashCheckResult> {
		if (!globalThis.crypto?.subtle) throw new Error('当前浏览器不支持 SHA-256 计算')
		const digest = await globalThis.crypto.subtle.digest('SHA-256', await file.arrayBuffer())
		const sha256 = Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, '0')).join('')
		const result = await this.request<UploadHashCheckResult>('/upload-sessions/hash-check', {
			method: 'POST',
			body: JSON.stringify({ sha256, size: file.size, mimeType: file.type || 'application/octet-stream' }),
		})
		return { ...result, sha256, size: file.size }
	}

	initUploadChunks(sessionId: string, itemId: string, manifest: UploadChunkManifest): Promise<UploadChunkSnapshot> {
		return this.request<UploadChunkSnapshot>(`/upload-sessions/${encodeURIComponent(sessionId)}/items/${encodeURIComponent(itemId)}/chunks/init`, {
			method: 'POST', body: JSON.stringify(manifest),
		})
	}

	listUploadChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot> {
		return this.request<UploadChunkSnapshot>(`/upload-sessions/${encodeURIComponent(sessionId)}/items/${encodeURIComponent(itemId)}/chunks`, { method: 'GET' })
	}

	async uploadSessionChunk(sessionId: string, itemId: string, partNumber: number, chunk: Blob): Promise<UploadChunkInfo> {
			const headers: Record<string, string> = { 'Content-Type': 'application/octet-stream', Accept: 'application/json' }
			if (!globalThis.crypto?.subtle) throw new Error('当前浏览器不支持 SHA-256 计算')
			const digest = await globalThis.crypto.subtle.digest('SHA-256', await chunk.arrayBuffer())
			headers['X-Chunk-SHA256'] = Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, '0')).join('')
		const token = getAccessToken()
		if (token) headers.Authorization = `Bearer ${token}`
		const response = await fetch(`${this.baseUrl}/upload-sessions/${encodeURIComponent(sessionId)}/items/${encodeURIComponent(itemId)}/chunks/${partNumber}`, {
			method: 'PUT', headers, body: chunk, credentials: 'include',
		})
		const body: unknown = await response.json().catch(() => undefined)
		if (!response.ok) {
			const message = isRecord(body) && typeof body.message === 'string' ? body.message : `HTTP ${response.status}`
			throw new Error(`分片上传失败：${message}`)
		}
		const payload = unwrapResponseData(body)
		if (!isRecord(payload) || typeof payload.partNumber !== 'number') throw new Error('分片接口返回格式无效')
		return payload as unknown as UploadChunkInfo
	}

	completeUploadChunks(sessionId: string, itemId: string): Promise<UploadSessionItem> {
		return this.request<UploadSessionItem>(`/upload-sessions/${encodeURIComponent(sessionId)}/items/${encodeURIComponent(itemId)}/chunks/complete`, { method: 'POST' })
	}

  async uploadSessionItem(sessionId: string, itemId: string, file: Blob, name?: string): Promise<UploadSessionItem> {
    const formData = new FormData()
    formData.append('file', file, name)
    const headers: Record<string, string> = { Accept: 'application/json' }
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`
    const response = await fetch(`${this.baseUrl}/upload-sessions/${encodeURIComponent(sessionId)}/items/${encodeURIComponent(itemId)}`, {
      method: 'POST',
      headers,
      body: formData,
      credentials: 'include',
    })
    const body: unknown = await response.json().catch(() => undefined)
    if (!response.ok) {
      const message = isRecord(body) && typeof body.message === 'string' ? body.message : `HTTP ${response.status}`
      throw new Error(`文件上传失败：${message}`)
    }
    const payload = unwrapResponseData(body)
    if (!isRecord(payload) || typeof payload.id !== 'string') throw new Error('上传文件项返回格式无效')
    return payload as unknown as UploadSessionItem
  }

  retryUploadSessionItem(sessionId: string, itemId: string): Promise<UploadSessionItem> {
    return this.request<UploadSessionItem>(`/upload-sessions/${encodeURIComponent(sessionId)}/items/${encodeURIComponent(itemId)}?action=retry`, { method: 'POST' })
  }

  getUploadSession(sessionId: string): Promise<UploadSessionSnapshot> {
    return this.request<UploadSessionSnapshot>(`/upload-sessions/${encodeURIComponent(sessionId)}`, { method: 'GET' })
  }

  commitUploadSession(sessionId: string): Promise<Record<string, unknown>> {
    return this.request<Record<string, unknown>>(`/upload-sessions/${encodeURIComponent(sessionId)}/commit`, { method: 'POST' })
  }

  async cancelUploadSession(sessionId: string): Promise<void> {
    await this.request(`/upload-sessions/${encodeURIComponent(sessionId)}`, { method: 'DELETE' })
  }

  async readAttachment(storageKey: string): Promise<Blob> {
    const headers: Record<string, string> = { Accept: '*/*' }
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`
    const response = await fetch(`${this.baseUrl}/attachments/${encodeURIComponent(storageKey)}`, {
      method: 'GET',
      headers,
      credentials: 'include',
    })
    if (!response.ok) throw new Error(`附件读取失败：HTTP ${response.status}`)
    return response.blob()
  }

  async exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet, */*',
    }
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`

    const response = await fetch(`${this.baseUrl}/bom/export`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ drawingNo, storageKey, items }),
      credentials: 'include',
    })
    if (!response.ok) throw new Error(`导出备料表失败：HTTP ${response.status}`)
    return response.blob()
  }

  async scanDrawingDesigner(drawingNo: string): Promise<string> {
    const response = await fetch(`${this.baseUrl}/exb/designer?drawingNo=${encodeURIComponent(drawingNo)}`, {
      method: 'POST',
      headers: {
        Accept: 'application/json',
        ...(getAccessToken()
          ? { Authorization: `Bearer ${getAccessToken()}` }
          : {}),
      },
      credentials: 'include',
    })
    if (!response.ok) throw new Error(`扫描图纸设计人失败：HTTP ${response.status}`)
    const body: unknown = await response.json()
    const payload = unwrapResponseData(body)
    if (!isRecord(payload)) return ''
    return typeof payload.designer === 'string' ? payload.designer : ''
  }

  async identifyDrawingFile(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity> {
    return this.identifyDrawingFields('/exb/identify', file, name, !options?.titleBlockOnly, options)
  }

  async identifyDrawingMaterial(file: Blob, name: string): Promise<DrawingFileIdentity> {
    return this.identifyDrawingFields('/exb/material', file, name, false)
  }

  private async identifyDrawingFields(path: string, file: Blob, name: string, requirePartNo: boolean, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity> {
    const formData = new FormData()
    formData.append('file', file, name)
    if (options?.titleBlockOnly) formData.append('titleBlockOnly', 'true')
    const headers: Record<string, string> = { Accept: 'application/json' }
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: 'POST',
      headers,
      body: formData,
      credentials: 'include',
    })
    const body: unknown = await response.json().catch(() => undefined)
    if (!response.ok) {
      const detail = isRecord(body) && typeof body.message === 'string'
        ? body.message
        : `HTTP ${response.status}`
      throw new Error(`读取「${name}」图纸图号失败：${detail}`)
    }
    const payload = unwrapResponseData(body)
    if (!isRecord(payload) || (requirePartNo && (typeof payload.partNo !== 'string' || !payload.partNo.trim()))) {
      throw new Error(`读取「${name}」图纸时未返回有效内部图号`)
    }
    return {
      partNo: typeof payload.partNo === 'string' ? payload.partNo.trim() : '',
      partNoSource: payload.partNoSource === 'titleBlock' || payload.partNoSource === 'filename' || payload.partNoSource === 'none'
        ? payload.partNoSource
        : undefined,
      material: typeof payload.material === 'string' ? payload.material.trim() : undefined,
      titleBlock: isRecord(payload.titleBlock) ? Object.fromEntries(
        Object.entries(payload.titleBlock).filter((entry): entry is [string, string] => typeof entry[1] === 'string'),
      ) : undefined,
    }
  }

  async reidentifyDrawingFile(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult> {
    const body = await this.request<unknown>('/exb/reidentify', {
      method: 'POST',
      body: JSON.stringify({ storageKey, partNo }),
    })
    const payload = isRecord(body) && typeof body.storageKey === 'string' ? body : body
    if (!isRecord(payload) || typeof payload.storageKey !== 'string' || typeof payload.partNo !== 'string') {
      throw new Error('历史图纸校正接口未返回有效结果')
    }
    return {
      storageKey: payload.storageKey,
      drawingNo: typeof payload.drawingNo === 'string' ? payload.drawingNo : '',
      oldPartNo: typeof payload.oldPartNo === 'string' ? payload.oldPartNo : '',
      partNo: payload.partNo,
    }
  }

  async listUsers(): Promise<UserAccount[]> {
    const body = await this.request<unknown>('/users', { method: 'GET' })
    const payload = isRecord(body) && 'data' in body ? body.data : body
    if (!Array.isArray(payload)) throw new Error('账号接口返回格式无效')
    return payload as UserAccount[]
  }

  async createUser(input: UserManagementInput): Promise<UserAccount> {
    const body = await this.request<unknown>('/users', { method: 'POST', body: JSON.stringify(input) })
    return this.readUserResponse(body)
  }

  async updateUser(userId: string, input: UserManagementInput): Promise<UserAccount> {
    const body = await this.request<unknown>(`/users/${encodeURIComponent(userId)}`, { method: 'PUT', body: JSON.stringify(input) })
    return this.readUserResponse(body)
  }

  async listEditSessions(drawingNo?: string): Promise<ActiveEditSessionInfo[]> {
    const query = drawingNo ? `?drawingNo=${encodeURIComponent(drawingNo)}` : ''
    const body = await this.request<unknown>(`/edit-sessions${query}`, { method: 'GET' })
    const payload = isRecord(body) && 'data' in body ? body.data : body
    if (!Array.isArray(payload)) return []
    return payload as ActiveEditSessionInfo[]
  }

  async openEditSession(storageKey: string): Promise<EditSessionOpenResult> {
    const body = await this.request<unknown>('/edit-sessions/open', {
      method: 'POST',
      body: JSON.stringify({ storageKey }),
    })
    const payload = isRecord(body) && 'data' in body ? body.data : body
    if (!isRecord(payload) || typeof payload.sessionId !== 'string' || typeof payload.openUrl !== 'string' || typeof payload.uncPath !== 'string' || typeof payload.smbRoot !== 'string' || typeof payload.expiresAt !== 'string') {
      throw new Error('编辑会话接口返回格式无效')
    }
    return {
      sessionId: payload.sessionId,
      openUrl: payload.openUrl,
      uncPath: payload.uncPath,
      smbRoot: payload.smbRoot,
      expiresAt: payload.expiresAt,
    }
  }

  async heartbeatEditSession(sessionId: string): Promise<EditSessionControlResult> {
    await this.request(`/edit-sessions/${encodeURIComponent(sessionId)}/heartbeat`, { method: 'POST' })
    return { sessionId }
  }

  async closeEditSession(sessionId: string): Promise<EditSessionControlResult> {
    const body = await this.request<unknown>(`/edit-sessions/${encodeURIComponent(sessionId)}/close`, { method: 'POST' })
    const payload = isRecord(body) && 'data' in body ? body.data : body
    if (!isRecord(payload) || typeof payload.sessionId !== 'string') {
      return { sessionId }
    }
    return {
      sessionId: payload.sessionId,
      changed: typeof payload.changed === 'boolean' ? payload.changed : undefined,
      version: typeof payload.version === 'string' ? payload.version : undefined,
      currentStorageKey: typeof payload.currentStorageKey === 'string' ? payload.currentStorageKey : undefined,
      currentName: typeof payload.currentName === 'string' ? payload.currentName : undefined,
    }
  }

  async listFileVersions(storageKey: string): Promise<FileVersionInfo[]> {
    const body = await this.request<unknown>(`/file-versions?storageKey=${encodeURIComponent(storageKey)}`, { method: 'GET' })
    const payload = isRecord(body) && 'data' in body ? body.data : body
    if (!Array.isArray(payload)) return []
    return payload as FileVersionInfo[]
  }

  async downloadFileVersion(versionId: string): Promise<Blob> {
    const token = getAccessToken()
    const response = await fetch(`${this.baseUrl}/file-versions/${encodeURIComponent(versionId)}/content`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      credentials: 'include',
    })
    if (!response.ok) throw new Error(`下载版本文件失败：HTTP ${response.status}`)
    return await response.blob()
  }

  async restoreFileVersion(versionId: string): Promise<void> {
    await this.request(`/file-versions/${encodeURIComponent(versionId)}/restore`, { method: 'POST' })
  }

  private readUserResponse(body: unknown): UserAccount {
    const payload = isRecord(body) && 'data' in body ? body.data : body
    if (!isRecord(payload) || typeof payload.id !== 'string') throw new Error('账号接口返回格式无效')
    return payload as unknown as UserAccount
  }

  private async request<T>(path: string, options: { method: 'GET' | 'POST' | 'PUT' | 'DELETE'; body?: string; headers?: Record<string, string>; onResponse?: (response: Response) => void }): Promise<T> {
    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...(options.headers || {}),
    }

    if (options.body) {
      headers['Content-Type'] = 'application/json'
    }

    const token = getAccessToken()
    if (token) {
      headers.Authorization = `Bearer ${token}`
    }

    const response = await fetch(`${this.baseUrl}${path}`, {
      method: options.method,
      headers,
      body: options.body,
      credentials: 'include',
    })

    options.onResponse?.(response)

    if (!response.ok) {
      let message = `数据接口请求失败：HTTP ${response.status}`
      try {
        const errorBody: unknown = await response.json()
        if (isRecord(errorBody) && typeof errorBody.message === 'string' && errorBody.message.trim()) {
          message = `数据接口请求失败：${errorBody.message}`
        }
      } catch {
        // 非 JSON 错误响应使用默认 HTTP 错误信息。
      }
      throw new Error(message)
    }

    if (response.status === 204) {
      return undefined as T
    }

    const body: unknown = await response.json()
    if (isRecord(body) && typeof body.code === 'number' && 'data' in body) {
      if (body.code !== 0 && body.code !== 200) {
        throw new Error(typeof body.message === 'string' ? body.message : '数据接口返回失败')
      }
      return body.data as T
    }

    return body as T
  }
}
