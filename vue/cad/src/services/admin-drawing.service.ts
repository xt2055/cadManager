import { getApiBaseUrl } from '@/services/api-base.service'

export interface AdminDrawingSummary {
  id: string
  no: string
  name: string
  kind: string
  project: string
  material: string
  vendor: string
  status: string
  version: string
  createdBy: string
  createdAt: string
  updatedBy: string
  updatedAt: string
  partCount: number
  attachmentCount: number
  sessionCount: number
}

export interface AdminPartSummary {
  id: string
  drawingId: string
  drawingNo: string
  no: string
  name: string
  parentNo: string
  project: string
  material: string
  status: string
  version: string
  createdBy: string
  createdAt: string
  updatedBy: string
  updatedAt: string
  attachmentCount: number
  sessionCount: number
}

export interface AdminAttachment {
  id: string
  originalName: string
  currentName: string
  role: string
  storageKey: string
  currentStorageKey: string
  version: string
  size: number
  mimeType: string
  uploadedBy: string
  createdAt: string
}

export interface AdminFileVersion {
  id: string
  attachmentId: string
  storageKey: string
  version: string
  versionKind: string
  size: number
  mimeType: string
  createdBy: string
  createdAt: string
}

export interface AdminDrawingDetail {
  drawing: AdminDrawingSummary
  parts: AdminPartSummary[]
  attachments: AdminAttachment[]
  versions: AdminFileVersion[]
  sessions: Array<Record<string, unknown>>
}

export interface AdminDrawingPage {
  list: Array<AdminDrawingSummary | AdminPartSummary>
  total: number
  page: number
  pageSize: number
}

function authHeaders(): Record<string, string> {
  const token = typeof window !== 'undefined'
    ? window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token')
    : null
  return token ? { Accept: 'application/json', Authorization: `Bearer ${token}` } : { Accept: 'application/json' }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, {
    ...init,
    headers: { ...authHeaders(), ...(init.headers || {}) },
    credentials: 'include',
  })
  const body = await response.json().catch(() => null) as { data?: T; message?: string } | T | null
  if (!response.ok) {
    const message = body && typeof body === 'object' && body !== null && 'message' in body ? body.message : undefined
    throw new Error(typeof message === 'string' ? message : `后台图纸请求失败：HTTP ${response.status}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data as T
  return body as T
}

export async function listAdminDrawings(options: { page?: number; pageSize?: number; keyword?: string; status?: string; kind?: 'drawing' | 'part' } = {}): Promise<AdminDrawingPage> {
  const query = new URLSearchParams({ page: String(options.page ?? 1), page_size: String(options.pageSize ?? 20) })
  if (options.keyword) query.set('keyword', options.keyword)
  if (options.status) query.set('status', options.status)
  if (options.kind) query.set('kind', options.kind)
  return request<AdminDrawingPage>(`/admin/drawings?${query}`)
}

export function getAdminDrawing(id: string): Promise<AdminDrawingDetail> {
  return request<AdminDrawingDetail>(`/admin/drawings/${encodeURIComponent(id)}`)
}

export function setAdminDrawingStatus(id: string, status: 'disabled' | 'draft'): Promise<{ id: string; status: string }> {
  return request<{ id: string; status: string }>(`/admin/drawings/${encodeURIComponent(id)}/${status === 'disabled' ? 'disable' : 'enable'}`, { method: 'POST' })
}

export function deleteAdminDrawing(id: string): Promise<{ no: string }> {
  return request<{ no: string }>(`/admin/drawings/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function setAdminPartStatus(id: string, status: 'disabled' | 'draft'): Promise<{ id: string; status: string }> {
  return request<{ id: string; status: string }>(`/admin/parts/${encodeURIComponent(id)}/${status === 'disabled' ? 'disable' : 'enable'}`, { method: 'POST' })
}

export function deleteAdminAttachment(id: string): Promise<{ id: string; name: string }> {
  return request<{ id: string; name: string }>(`/admin/attachments/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function reconvertAdminAttachment(id: string): Promise<{ id: string; name: string }> {
  return request<{ id: string; name: string }>(`/admin/attachments/${encodeURIComponent(id)}/reconvert`, { method: 'POST' })
}

export function closeAdminEditSession(id: string): Promise<Record<string, unknown>> {
  return request<Record<string, unknown>>(`/admin/edit-sessions/${encodeURIComponent(id)}/close`, { method: 'POST' })
}

export function downloadAdminVersion(id: string): Promise<Blob> {
  return fetch(`${getApiBaseUrl()}/file-versions/${encodeURIComponent(id)}/content`, {
    headers: authHeaders(),
    credentials: 'include',
  }).then(async (response) => {
    if (!response.ok) throw new Error(`版本下载失败：HTTP ${response.status}`)
    return response.blob()
  })
}

export function restoreAdminVersion(id: string): Promise<Record<string, unknown>> {
  return request<Record<string, unknown>>(`/file-versions/${encodeURIComponent(id)}/restore`, { method: 'POST' })
}
