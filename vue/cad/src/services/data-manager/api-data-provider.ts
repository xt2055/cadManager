import { normalizeDataDocument, type DataDocument } from './data.types'
import type { AttachmentMetadata, AttachmentResult, DataProvider, DrawingFileIdentity, DrawingFileIdentifyOptions, EditSessionControlResult, EditSessionOpenResult, ActiveEditSessionInfo, FileVersionInfo, ReidentifyDrawingFileResult } from './data-provider'
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

  async load(): Promise<DataDocument> {
    const response = await this.request<unknown>('/data/document', {
      method: 'GET',
      onResponse: (resolved) => {
        if (!resolved.ok) return
        const stamp = resolved.headers.get('X-Document-UpdatedAt')
        if (stamp) this.documentStamp = stamp
      },
    })
    return normalizeDataDocument(response)
  }

  async save(document: DataDocument): Promise<void> {
    const { users: _users, ...businessDocument } = document
    const payload = { ...businessDocument, logs: [] }
    await this.request('/data/document', {
      method: 'PUT',
      body: JSON.stringify(payload),
      headers: this.documentStamp ? { 'X-Expected-UpdatedAt': this.documentStamp } : {},
      onResponse: (resolved) => {
        if (!resolved.ok) return
        const stamp = resolved.headers.get('X-Document-UpdatedAt')
        if (stamp) this.documentStamp = stamp
      },
    })
  }

  async uploadAttachment(file: Blob, metadata: AttachmentMetadata): Promise<AttachmentResult> {
    const formData = new FormData()
    formData.append('file', file, metadata.name)
    if (metadata.storageKey) formData.append('storageKey', metadata.storageKey)
    if (metadata.drawingNo) formData.append('drawingNo', metadata.drawingNo)
    if (metadata.partNo) formData.append('partNo', metadata.partNo)
    if (metadata.role) formData.append('role', metadata.role)
    if (metadata.version) formData.append('version', metadata.version)
    if (metadata.previewable !== undefined) formData.append('previewable', String(metadata.previewable))

    const headers: Record<string, string> = { Accept: 'application/json' }
    const token = getAccessToken()
    if (token) headers.Authorization = `Bearer ${token}`

    const response = await fetch(`${this.baseUrl}/attachments`, {
      method: 'POST',
      headers,
      body: formData,
      credentials: 'include',
    })
    if (!response.ok) {
      let message = `附件上传失败：HTTP ${response.status}`
      try {
        const errorBody: unknown = await response.json()
        if (isRecord(errorBody) && typeof errorBody.message === 'string' && errorBody.message.trim()) {
          message = `附件上传失败：${errorBody.message}`
        }
      } catch {
        // 非 JSON 错误响应使用默认 HTTP 错误信息。
      }
      throw new Error(message)
    }

    const body: unknown = await response.json()
    if (isRecord(body) && typeof body.code === 'number' && body.code !== 0 && body.code !== 200) {
      throw new Error(typeof body.message === 'string' ? body.message : '附件上传接口返回失败')
    }
    const payload = isRecord(body) && typeof body.code === 'number' && 'data' in body ? body.data : body
    if (!isRecord(payload) || typeof payload.storageKey !== 'string') {
      throw new Error('附件上传接口未返回有效 storageKey')
    }

    return {
      storageKey: payload.storageKey,
      size: typeof payload.size === 'number' ? payload.size : file.size,
      mimeType: typeof payload.mimeType === 'string' ? payload.mimeType : file.type || 'application/octet-stream',
      createdAt: typeof payload.createdAt === 'string' ? payload.createdAt : undefined,
    }
  }

  async deleteAttachment(storageKey: string): Promise<void> {
    await this.request(`/attachments/${encodeURIComponent(storageKey)}`, { method: 'DELETE' })
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
    await this.request(`/edit-sessions/${encodeURIComponent(sessionId)}/close`, { method: 'POST' })
    return { sessionId }
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
