import { normalizeDataDocument, type DataDocument } from './data.types'
import type { AttachmentMetadata, AttachmentResult, DataProvider } from './data-provider'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export class ApiDataProvider implements DataProvider {
  private readonly baseUrl: string

  constructor(baseUrl = import.meta.env.VITE_API_BASE_URL || '/api') {
    this.baseUrl = baseUrl.replace(/\/$/, '')
  }

  async load(): Promise<DataDocument> {
    const response = await this.request<unknown>('/data/document', { method: 'GET' })
    return normalizeDataDocument(response)
  }

  async save(document: DataDocument): Promise<void> {
    const payload = { ...document, logs: [] }
    await this.request('/data/document', {
      method: 'PUT',
      body: JSON.stringify(payload),
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
    const token = typeof window !== 'undefined' ? window.localStorage.getItem('cad_access_token') : null
    if (token) headers.Authorization = `Bearer ${token}`

    const response = await fetch(`${this.baseUrl}/attachments`, {
      method: 'POST',
      headers,
      body: formData,
      credentials: 'include',
    })
    if (!response.ok) throw new Error(`附件上传失败：HTTP ${response.status}`)

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
    }
  }

  async deleteAttachment(storageKey: string): Promise<void> {
    await this.request(`/attachments/${encodeURIComponent(storageKey)}`, { method: 'DELETE' })
  }

  async readAttachment(storageKey: string): Promise<Blob> {
    const headers: Record<string, string> = { Accept: '*/*' }
    const token = typeof window !== 'undefined' ? window.localStorage.getItem('cad_access_token') : null
    if (token) headers.Authorization = `Bearer ${token}`
    const response = await fetch(`${this.baseUrl}/attachments/${encodeURIComponent(storageKey)}`, {
      method: 'GET',
      headers,
      credentials: 'include',
    })
    if (!response.ok) throw new Error(`附件读取失败：HTTP ${response.status}`)
    return response.blob()
  }

  private async request<T>(path: string, options: { method: 'GET' | 'PUT' | 'DELETE'; body?: string }): Promise<T> {
    const headers: Record<string, string> = {
      Accept: 'application/json',
    }

    if (options.body) {
      headers['Content-Type'] = 'application/json'
    }

    const token = typeof window !== 'undefined' ? window.localStorage.getItem('cad_access_token') : null
    if (token) {
      headers.Authorization = `Bearer ${token}`
    }

    const response = await fetch(`${this.baseUrl}${path}`, {
      method: options.method,
      headers,
      body: options.body,
      credentials: 'include',
    })

    if (!response.ok) {
      throw new Error(`数据接口请求失败：HTTP ${response.status}`)
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
