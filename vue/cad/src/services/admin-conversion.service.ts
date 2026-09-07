import { getApiBaseUrl } from '@/services/api-base.service'

export interface ConversionJobItem {
  id: string
  uploadItemId: string
  attachmentId?: string | null
  drawingId?: string | null
  drawingNo?: string | null
  drawingName?: string | null
  sourceName: string
  sourceSize: number
  sourceMimeType: string
  status: 'pending' | 'processing' | 'retry' | 'backoff' | 'failed' | string
  attempts: number
  nextAttemptAt: string
  leaseUntil?: string | null
  lastError?: string | null
  createdAt: string
  updatedAt: string
}

export interface ConversionStats {
  total: number
  pending: number
  processing: number
  retry: number
  backoff: number
  failed: number
}

export interface ConversionListResponse {
  items: ConversionJobItem[]
  total: number
  page: number
  pageSize: number
  stats: ConversionStats
  caxaStatus: string
}

export interface ConversionLogResponse {
  lines: string[]
  path: string
  modTime?: string | null
  total: number
}

function authHeaders(): Record<string, string> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  const token = typeof window !== 'undefined'
    ? window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token')
    : null
  if (token) headers.Authorization = `Bearer ${token}`
  return headers
}

function unwrap<T>(body: unknown): T {
  if (typeof body === 'object' && body !== null && 'code' in body && 'data' in body) {
    const r = body as { code: number; message?: string; data: T }
    if (r.code !== 0 && r.code !== 200) {
      throw new Error(r.message || '请求失败')
    }
    return r.data
  }
  return body as T
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, {
    ...init,
    headers: { ...authHeaders(), ...init?.headers },
  })
  if (response.status === 403) throw new Error('需要管理员权限')
  const body = await response.json().catch(() => ({}))
  if (!response.ok) {
    const msg = (body as { message?: string })?.message || `请求失败: HTTP ${response.status}`
    throw new Error(msg)
  }
  return unwrap<T>(body)
}

export async function fetchConversionJobs(params: {
  page?: number
  pageSize?: number
  status?: string
  keyword?: string
}): Promise<ConversionListResponse> {
  const query = new URLSearchParams()
  if (params.page) query.set('page', String(params.page))
  if (params.pageSize) query.set('pageSize', String(params.pageSize))
  if (params.status && params.status !== 'all') query.set('status', params.status)
  if (params.keyword) query.set('keyword', params.keyword.trim())

  return request<ConversionListResponse>(`/admin/cad-conversions?${query.toString()}`)
}

export async function retryConversionJob(id: string): Promise<{ message: string }> {
  return request<{ message: string }>(`/admin/cad-conversions/${id}/retry`, {
    method: 'POST',
  })
}

export async function retryAllFailedConversions(): Promise<{ retriedCount: number; message: string }> {
  return request<{ retriedCount: number; message: string }>('/admin/cad-conversions/all-failed/retry', {
    method: 'POST',
  })
}

export async function fetchConversionLogs(limit = 300): Promise<ConversionLogResponse> {
  return request<ConversionLogResponse>(`/admin/cad-conversion-logs?limit=${limit}`)
}
