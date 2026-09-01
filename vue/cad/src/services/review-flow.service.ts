export type ReviewAccountRole = 'admin' | 'designer' | 'reviewer'

// 节点显示名 → 图纸签署角色。历史数据或自定义流程可能未回填 signerRole，按节点名兜底。
export const NODE_NAME_TO_SIGNER_ROLE: Record<string, string> = {
  设计自检: '设计',
  校对复核: '校对',
  专业审核: '审核',
  工艺会签: '工艺',
  标准化审查: '标准化',
  主管批准: '批准',
}

export function signerRoleForNode(name: string, signerRole?: string): string {
  const trimmedRole = signerRole?.trim()
  if (trimmedRole) return trimmedRole
  return NODE_NAME_TO_SIGNER_ROLE[name?.trim() ?? ''] ?? ''
}

export interface ReviewFlowNodeDto {
  id?: string
  name: string
  signerRole?: string
  candidateRole: ReviewAccountRole
  assignedUserId?: string
  assignedName?: string
  required: boolean
  order: number
}

export interface ReviewFlowDto {
  id: string
  name: string
  description: string
  enabled: boolean
  createdBy?: string
  createdAt?: string
  updatedAt?: string
  nodes: ReviewFlowNodeDto[]
}

function apiBaseUrl() {
  return (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
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
    throw new Error(typeof message === 'string' ? message : `审核流程请求失败：HTTP ${response.status}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data as T
  return body as T
}

export const reviewFlowService = {
  list(): Promise<ReviewFlowDto[]> {
    return request<ReviewFlowDto[]>('/review-flows')
  },

  get(id: string): Promise<ReviewFlowDto> {
    return request<ReviewFlowDto>(`/review-flows/${encodeURIComponent(id)}`)
  },

  create(input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto> {
    return request<ReviewFlowDto>('/review-flows', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    })
  },

  update(id: string, input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto> {
    return request<ReviewFlowDto>(`/review-flows/${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    })
  },

  toggle(id: string, enabled: boolean): Promise<ReviewFlowDto> {
    return request<ReviewFlowDto>(`/review-flows/${encodeURIComponent(id)}?action=toggle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    })
  },
}
