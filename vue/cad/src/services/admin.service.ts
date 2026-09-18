import { getApiBaseUrl } from '@/services/api-base.service'
import { authorizationHeaders } from '@/services/auth/access-token'
import { notifySessionExpired } from '@/services/auth/session-expiry'

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

async function request<T>(path: string, init: RequestInit): Promise<T> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, init)
  if (response.status === 403) throw new Error('需要管理员权限')
  const body: unknown = await response.json().catch(() => ({}))
  const data = unwrap<T>(body)
  if (response.status === 401) notifySessionExpired()
  if (!response.ok) {
    throw new Error(isRecord(body) && typeof body.message === 'string' ? body.message : `请求失败：HTTP ${response.status}`)
  }
  return data
}

export async function fetchSystemLogs(lines = 400, keyword = ''): Promise<SystemLogLine[]> {
  const query = new URLSearchParams({ lines: String(lines) })
  if (keyword) query.set('keyword', keyword)
  return request<SystemLogLine[]>(`/system/logs?${query.toString()}`, { headers: authorizationHeaders() })
}

export async function fetchSystemLogFiles(): Promise<SystemLogFile[]> {
  return request<SystemLogFile[]>('/system/logs/files', { headers: authorizationHeaders() })
}

export function systemLogDownloadUrl(file: string): string {
  return `${getApiBaseUrl()}/system/logs/download?file=${encodeURIComponent(file)}`
}

export async function fetchUpdateList(): Promise<UpdateRecord[]> {
  return request<UpdateRecord[]>('/system/updates', { headers: authorizationHeaders() })
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
  return request<UpdateRecord>('/system/updates', { method: 'POST', headers: authorizationHeaders(), body: form })
}

export async function deleteUpdatePackage(id: string): Promise<void> {
  await request<{ ok: boolean }>(`/system/updates/${id}`, { method: 'DELETE', headers: authorizationHeaders() })
}

export function updatePackageDownloadUrl(record: UpdateRecord): string {
  return record.downloadUrl ? `${getApiBaseUrl()}${record.downloadUrl}` : ''
}
