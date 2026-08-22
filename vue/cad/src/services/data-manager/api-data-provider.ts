import { normalizeDataDocument, type DataDocument } from './data.types'
import type { DataProvider } from './data-provider'

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
    await this.request('/data/document', {
      method: 'PUT',
      body: JSON.stringify(document),
    })
  }

  private async request<T>(path: string, options: { method: 'GET' | 'PUT'; body?: string }): Promise<T> {
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
