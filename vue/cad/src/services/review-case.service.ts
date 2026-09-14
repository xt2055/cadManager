import { getApiBaseUrl } from '@/services/api-base.service'

export interface ApiReviewCaseNode {
  name: string
  assignedUserId?: string
  assignedName: string
  status: 'pass' | 'pending' | 'rejected'
  opinion: string
  required: boolean
  order: number
  time?: string
}

export interface ApiReviewCase {
  changeSubmissionId?: string
  id: string
  drawingNo: string
  drawingName: string
  flow: string
  status: string
  initiator: string
  startedAt: string
  completedAt?: string
  nodes: ApiReviewCaseNode[]
}

export interface ApiCompletedAction {
  id: string
  reviewCaseId: string
  no: string
  name: string
  node: string
  by: string
  reviewer: string
  time: string
  result: 'pass' | 'rejected'
  opinion: string
  ver: string
}

function apiBaseUrl() {
  return getApiBaseUrl()
}

function authHeaders(): Record<string, string> {
  const token = typeof window !== 'undefined'
    ? window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token')
    : null
  return token ? { Accept: 'application/json', Authorization: `Bearer ${token}` } : { Accept: 'application/json' }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...options,
    headers: { ...authHeaders(), ...(options.headers || {}) },
    credentials: 'include',
  })
  const body = await response.json().catch(() => null) as { data?: T; message?: string } | T | null
  if (!response.ok) {
    const message = body && typeof body === 'object' && 'message' in body ? body.message : undefined
    throw new Error(typeof message === 'string' ? message : `审核接口请求失败：HTTP ${response.status}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data as T
  return body as T
}

export const reviewCaseService = {
  /** 发起审核（存在被驳回案例时自动从驳回节点续审）。 */
  start(drawingNo: string): Promise<ApiReviewCase> {
    return request<ApiReviewCase>('/review-cases', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ drawingNo }),
    })
  },

  list(): Promise<ApiReviewCase[]> {
    return request<ApiReviewCase[]>('/review-cases')
  },

  submit(caseId: string, nodeName: string, action: 'pass' | 'rejected', opinion: string): Promise<ApiReviewCase> {
    return request<ApiReviewCase>(`/review-cases/${encodeURIComponent(caseId)}/submit`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ nodeName, action, opinion }),
    })
  },

  completed(): Promise<ApiCompletedAction[]> {
    return request<ApiCompletedAction[]>('/review-cases/completed')
  },
}
