export interface SystemLogLine {
  time: string
  level: string
  message: string
}

export interface SystemLogFile {
  name: string
  size: number
  modTime: string
}

export interface UpdateRecord {
  id: string
  version: string
  platform: string
  notes: string
  publishedAt?: string
  downloadUrl?: string
  mandatory: boolean
  enabled: boolean
  fileName?: string
  sizeBytes?: number
  createdAt?: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function unwrap<T>(body: unknown): T {
  if (isRecord(body) && typeof body.code === 'number' && 'data' in body) {
    if (body.code !== 0 && body.code !== 200) throw new Error(typeof body.message === 'string' ? body.message : '管理接口返回失败')
    return body.data as T
  }
  return body as T
}

function authHeaders(extra: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = { Accept: 'application/json', ...extra }
  const token = typeof window !== 'undefined'
    ? window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token')
    : null
  if (token) headers.Authorization = `Bearer ${token}`
  return headers
}

const baseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')

async function request<T>(path: string, init: RequestInit): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, init)
  if (response.status === 403) throw new Error('需要管理员权限')
  const body: unknown = await response.json().catch(() => ({}))
  const data = unwrap<T>(body)
  if (!response.ok) {
    throw new Error(isRecord(body) && typeof body.message === 'string' ? body.message : `请求失败：HTTP ${response.status}`)
  }
  return data
}

export async function fetchSystemLogs(lines = 400, keyword = ''): Promise<SystemLogLine[]> {
  const query = new URLSearchParams({ lines: String(lines) })
  if (keyword) query.set('keyword', keyword)
  return request<SystemLogLine[]>(`/system/logs?${query.toString()}`, { headers: authHeaders() })
}

export async function fetchSystemLogFiles(): Promise<SystemLogFile[]> {
  return request<SystemLogFile[]>('/system/logs/files', { headers: authHeaders() })
}

export function systemLogDownloadUrl(file: string): string {
  return `${baseUrl}/system/logs/download?file=${encodeURIComponent(file)}`
}

export async function fetchUpdateList(): Promise<UpdateRecord[]> {
  return request<UpdateRecord[]>('/system/updates', { headers: authHeaders() })
}

export async function uploadUpdatePackage(input: {
  file: File
  version: string
  notes: string
  mandatory: boolean
  platform?: string
}): Promise<UpdateRecord> {
  const form = new FormData()
  form.append('file', input.file)
  form.append('version', input.version)
  form.append('notes', input.notes)
  form.append('mandatory', input.mandatory ? 'true' : 'false')
  if (input.platform) form.append('platform', input.platform)
  return request<UpdateRecord>('/system/updates', { method: 'POST', headers: authHeaders(), body: form })
}

export async function deleteUpdatePackage(id: string): Promise<void> {
  await request<{ ok: boolean }>(`/system/updates/${id}`, { method: 'DELETE', headers: authHeaders() })
}

export function updatePackageDownloadUrl(record: UpdateRecord): string {
  return record.downloadUrl ? `${baseUrl}${record.downloadUrl}` : ''
}
