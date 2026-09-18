import { getApiBaseUrl } from '@/services/api-base.service'
import { authorizationHeaders } from '@/services/auth/access-token'
import { notifySessionExpired } from '@/services/auth/session-expiry'
import type { DrawingStatus } from '@/types/domain.types'

/** 任务进度由图纸生命周期推导，前端不做二次计算，只负责展示。 */
export interface DrawingTaskProgress {
  percent: number
  stage: string
  detail: string
  done: boolean
}

export interface DrawingTaskDrawing {
  id: string
  no: string
  name: string
  project: string
  status: DrawingStatus
  version: string
  fileCount: number
  reviewNode?: string
  reviewDone: number
  reviewTotal: number
  createdAt?: string
  updatedAt?: string
}

export interface DrawingTaskAssignment {
  taskId: string
  assigneeId: string
  assignee: string
  assignedByName?: string
  note: string
  dueDate?: string
  assignedAt?: string
}

export interface DrawingTaskRow {
  drawing: DrawingTaskDrawing
  assignment?: DrawingTaskAssignment
  progress: DrawingTaskProgress
}

export interface DrawingTaskSummary {
  total: number
  assigned: number
  unassigned: number
  active: number
  done: number
  overdue: number
}

export interface DrawingTaskPage {
  list: DrawingTaskRow[]
  total: number
  page: number
  pageSize: number
  summary: DrawingTaskSummary
}

export interface DrawingTaskCandidate {
  userId: string
  account: string
  name: string
  roles: string[]
  activeTasks: number
}

export interface DrawingTaskHistoryEntry {
  taskId: string
  assigneeId: string
  assignee: string
  status: 'active' | 'replaced' | 'cancelled'
  note: string
  dueDate?: string
  assignedByName?: string
  assignedAt?: string
  endedByName?: string
  endedAt?: string
  endReason?: string
}

export interface DrawingTaskQuery {
  scope: 'mine' | 'board'
  page?: number
  pageSize?: number
  keyword?: string
  status?: string
  assigned?: 'assigned' | 'unassigned'
}

function apiBaseUrl(): string {
  return getApiBaseUrl()
}

async function request<T>(method: string, path: string, payload?: unknown): Promise<T> {
  const init: RequestInit = {
    method,
    // 令牌统一入口：内存登录态优先，避免只认存储镜像导致的无鉴权请求（401）。
    headers: payload === undefined ? authorizationHeaders() : authorizationHeaders({ 'Content-Type': 'application/json' }),
    credentials: 'include',
  }
  if (payload !== undefined) init.body = JSON.stringify(payload)
  const response = await fetch(`${apiBaseUrl()}${path}`, init)
  const body = await response.json().catch(() => null) as { data?: T; message?: string } | T | null
  if (response.status === 401) notifySessionExpired()
  if (!response.ok) {
    const message = body && typeof body === 'object' && 'message' in body ? body.message : undefined
    // 后端把「为什么不行 + 去哪里解决」写在 message 里，前端原样透出，不再替换成笼统文案。
    throw new Error(typeof message === 'string' && message ? message : `任务请求失败：HTTP ${response.status}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data as T
  return body as T
}

export const drawingTaskService = {
  list(query: DrawingTaskQuery): Promise<DrawingTaskPage> {
    const params = new URLSearchParams({ scope: query.scope })
    if (query.page) params.set('page', String(query.page))
    if (query.pageSize) params.set('page_size', String(query.pageSize))
    if (query.keyword) params.set('keyword', query.keyword)
    if (query.status) params.set('status', query.status)
    if (query.assigned) params.set('assigned', query.assigned)
    return request('GET', `/drawing-tasks?${params.toString()}`)
  },
  candidates(): Promise<DrawingTaskCandidate[]> {
    return request('GET', '/drawing-tasks/candidates')
  },
  history(drawingId: string): Promise<DrawingTaskHistoryEntry[]> {
    return request('GET', `/drawing-tasks/${encodeURIComponent(drawingId)}/history`)
  },
  assign(input: { drawingId: string; assigneeId: string; note?: string; dueDate?: string }): Promise<DrawingTaskRow> {
    return request('POST', '/drawing-tasks', input)
  },
  /** 换人即改派；assigneeId 与当前负责人相同时只更新任务说明与截止日期。 */
  update(taskId: string, input: { assigneeId?: string; note?: string; dueDate?: string; reason?: string }): Promise<DrawingTaskRow> {
    return request('PUT', `/drawing-tasks/${encodeURIComponent(taskId)}`, input)
  },
  cancel(taskId: string, reason: string): Promise<void> {
    const suffix = reason ? `?reason=${encodeURIComponent(reason)}` : ''
    return request('DELETE', `/drawing-tasks/${encodeURIComponent(taskId)}${suffix}`)
  },
}
