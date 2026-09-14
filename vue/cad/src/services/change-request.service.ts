import { getApiBaseUrl } from '@/services/api-base.service'

export type ChangeStatus =
  | 'pending_approval'
  | 'executing'
  | 'pending_verify'
  | 'completed'
  | 'rejected'
  | 'cancelled'

export interface ChangeDiff {
  kind: string
  field: string
  oldValue: string
  newValue: string
}

export interface ChangeAction {
  id: string
  action: string
  opinion: string
  actorName: string
  createdAt: string
}

// ChangeSubmission 一次提交形成的不可变快照轮次；currentRound 为当前待验收轮次的差异来源。
export interface ChangeSubmission {
	legacyHistory: boolean
  id: string
  round: number
  actorName?: string
  actualChanges: string
  proposedAttributes: ProposedAttributes
  status: string
  createdAt: string
  diffs?: ChangeDiff[]
}

export interface ProposedAttributes {
  name?: string
  material?: string
  vendor?: string
  version?: string
}

export interface ChangeTarget {
  attachmentId: string
  name: string
  fileCategory: 'auto' | 'drawing2d' | 'model3d' | 'other'
  drawingNo: string
  partNo?: string
}

export interface ChangeRequest {
  id: string
  requestNo: string
  drawingId: string
  drawingNo: string
  title: string
  reason: string
  scope: string
  status: ChangeStatus
  requireVerify: boolean
  applicantId: string
  applicantName: string
  executorId: string
  executorName: string
  approverId?: string
  approverName?: string
  verifierId?: string
  verifierName?: string
  directAdminApproval: boolean
  approvedAt?: string
  submittedAt?: string
  completedAt?: string
  createdAt: string
  actualChanges: string
  proposedAttributes: ProposedAttributes
  currentSubmissionId?: string
  diffs?: ChangeDiff[]
  actions?: ChangeAction[]
  submissions?: ChangeSubmission[]
  targets?: ChangeTarget[]
}

export interface ChangeUserOption {
  id: string
  displayName: string
  roles?: string[]
  status?: string
}

export interface CreateChangeInput {
  drawingId: string
  reason: string
  scope: string
  title?: string
  executorId?: string
  requireVerify?: boolean
  autoApprove?: boolean
  approveOpinion?: string
  waiveReason?: string
  attachmentIds: string[]
}

function apiBaseUrl(): string {
  return getApiBaseUrl()
}

function authHeaders(): Record<string, string> {
  const token = typeof window !== 'undefined'
    ? window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token')
    : null
  return token ? { Accept: 'application/json', Authorization: `Bearer ${token}` } : { Accept: 'application/json' }
}

async function request<T>(method: string, path: string, payload?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: authHeaders(), credentials: 'include' }
  if (payload !== undefined) {
    init.headers = { ...authHeaders(), 'Content-Type': 'application/json' }
    init.body = JSON.stringify(payload)
  }
  const response = await fetch(`${apiBaseUrl()}${path}`, init)
  const body = await response.json().catch(() => null) as { data?: T; message?: string } | T | null
  if (!response.ok) {
    const message = body && typeof body === 'object' && 'message' in body ? body.message : undefined
    throw new Error(typeof message === 'string' ? message : `变更工单请求失败：HTTP ${response.status}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data as T
  return body as T
}

export const changeRequestService = {
  list(params: { drawingId?: string; status?: string; open?: boolean } = {}): Promise<ChangeRequest[]> {
    const query = new URLSearchParams()
    if (params.drawingId) query.set('drawing_id', params.drawingId)
    if (params.status) query.set('status', params.status)
    if (params.open) query.set('open', '1')
    const suffix = query.toString() ? `?${query.toString()}` : ''
    return request('GET', `/change-requests${suffix}`)
  },
  listByDrawing(drawingId: string): Promise<ChangeRequest[]> {
    return request('GET', `/change-requests?drawing_id=${encodeURIComponent(drawingId)}`)
  },
  get(id: string): Promise<ChangeRequest> {
    return request('GET', `/change-requests/${encodeURIComponent(id)}`)
  },
  create(input: CreateChangeInput): Promise<ChangeRequest> {
    return request('POST', '/change-requests', input)
  },
  approve(id: string, opinion: string, requireVerify: boolean, waiveReason: string, executorId?: string): Promise<ChangeRequest> {
    return request('POST', `/change-requests/${encodeURIComponent(id)}/approve`, { opinion, requireVerify: true, waiveReason: '', executorId })
  },
  reject(id: string, opinion: string): Promise<ChangeRequest> {
    return request('POST', `/change-requests/${encodeURIComponent(id)}/reject`, { opinion })
  },
  submit(id: string, actualChanges: string, proposed: ProposedAttributes): Promise<ChangeRequest> {
    return request('POST', `/change-requests/${encodeURIComponent(id)}/submit`, { actualChanges, proposedAttributes: proposed })
  },
  verify(id: string, opinion: string, submissionId?: string): Promise<ChangeRequest> {
    return request('POST', `/change-requests/${encodeURIComponent(id)}/verify`, { opinion, submissionId })
  },
  returnForEdit(id: string, opinion: string): Promise<ChangeRequest> {
    return request('POST', `/change-requests/${encodeURIComponent(id)}/return`, { opinion })
  },
  cancel(id: string, opinion: string): Promise<ChangeRequest> {
    return request('POST', `/change-requests/${encodeURIComponent(id)}/cancel`, { opinion })
  },
  listUsers(): Promise<ChangeUserOption[]> {
    return request('GET', '/users')
  },
}

export const CHANGE_STATUS_LABELS: Record<ChangeStatus, string> = {
  pending_approval: '待审批',
  executing: '变更执行中',
  pending_verify: '完整审核中',
  completed: '已完成',
  rejected: '已驳回',
  cancelled: '已终止',
}
