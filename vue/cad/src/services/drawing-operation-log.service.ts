import type { ActivityLog, ActivityResult, ActivityTargetType, ActivityType } from '@/types/domain.types'
import { authorizationHeaders } from '@/services/auth/access-token'
import { notifySessionExpired } from '@/services/auth/session-expiry'
import { getApiBaseUrl } from '@/services/api-base.service'

interface OperationLogInput {
  drawingNo: string
  drawingName?: string
  targetType: ActivityTargetType
  act: ActivityType
  txt: string
  result?: ActivityResult
  detail?: Record<string, unknown>
}

export interface OperationLogPage {
  list: ActivityLog[]
  total: number
  page: number
  pageSize: number
}

export interface OperationLogOption {
  value: string
  label: string
}

export interface OperationLogOptionPage {
  list: OperationLogOption[]
}

export interface DrawingOperationLogQuery {
  page?: number
  pageSize?: number
  action?: string
  drawingNo?: string
  actor?: string
  keyword?: string
}

export interface AdminOperationLogQuery {
  page?: number
  pageSize?: number
  action?: ActivityType
  drawingNo?: string
  targetType?: ActivityTargetType
  actorId?: string
  result?: ActivityResult
  keyword?: string
  from?: string
  to?: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function unwrap<T>(body: unknown): T {
  if (isRecord(body) && typeof body.code === 'number' && 'data' in body) {
    if (body.code !== 0 && body.code !== 200) throw new Error(typeof body.message === 'string' ? body.message : '图纸操作日志接口返回失败')
    return body.data as T
  }
  return body as T
}

export async function createDrawingOperationLog(input: OperationLogInput): Promise<ActivityLog> {
  const response = await fetch(`${getApiBaseUrl()}/drawing-operation-logs`, {
    method: 'POST',
    headers: authorizationHeaders({ 'Content-Type': 'application/json' }),
    credentials: 'include',
    body: JSON.stringify(input),
  })
  if (response.status === 401) notifySessionExpired()
  if (!response.ok) throw new Error(`图纸操作日志保存失败：HTTP ${response.status}`)
  return unwrap<ActivityLog>(await response.json())
}

export async function listDrawingOperationLogs(options: DrawingOperationLogQuery = {}): Promise<OperationLogPage> {
  const query = new URLSearchParams({
    page: String(options.page ?? 1),
    page_size: String(options.pageSize ?? 20),
  })
  if (options.action) query.set('action', options.action)
  if (options.drawingNo) query.set('drawing_no', options.drawingNo)
  if (options.actor) query.set('actor', options.actor)
  if (options.keyword) query.set('keyword', options.keyword)
  const response = await fetch(`${getApiBaseUrl()}/drawing-operation-logs?${query}`, {
    method: 'GET',
    headers: authorizationHeaders(),
    credentials: 'include',
  })
  if (response.status === 401) notifySessionExpired()
  if (!response.ok) throw new Error(`图纸操作日志读取失败：HTTP ${response.status}`)
  return unwrap<OperationLogPage>(await response.json())
}

export async function listOperationLogOptions(
  adminOnly: boolean,
  kind: 'drawing' | 'actor',
  keyword = '',
): Promise<OperationLogOptionPage> {
  const query = new URLSearchParams({ kind, limit: '50' })
  if (keyword.trim()) query.set('keyword', keyword.trim())
  const path = adminOnly ? '/admin/audit-logs/options' : '/drawing-operation-logs/options'
  const response = await fetch(`${getApiBaseUrl()}${path}?${query}`, {
    method: 'GET',
    headers: authorizationHeaders(),
    credentials: 'include',
  })
  if (response.status === 401) notifySessionExpired()
  if (!response.ok) throw new Error(`操作日志候选读取失败：HTTP ${response.status}`)
  return unwrap<OperationLogOptionPage>(await response.json())
}

export async function listAdminOperationLogs(options: AdminOperationLogQuery = {}): Promise<OperationLogPage> {
  const query = new URLSearchParams({
    page: String(options.page ?? 1),
    page_size: String(options.pageSize ?? 20),
  })
  if (options.action) query.set('action', options.action)
  if (options.drawingNo) query.set('drawing_no', options.drawingNo)
  if (options.targetType) query.set('target_type', options.targetType)
  if (options.actorId) query.set('actor_id', options.actorId)
  if (options.result) query.set('result', options.result)
  if (options.keyword) query.set('keyword', options.keyword)
  if (options.from) query.set('from', options.from)
  if (options.to) query.set('to', options.to)
  const response = await fetch(`${getApiBaseUrl()}/admin/audit-logs?${query}`, {
    method: 'GET',
    headers: authorizationHeaders(),
    credentials: 'include',
  })
  if (response.status === 401) notifySessionExpired()
  if (!response.ok) throw new Error(`管理员操作日志读取失败：HTTP ${response.status}`)
  return unwrap<OperationLogPage>(await response.json())
}
