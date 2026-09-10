import { getApiBaseUrl } from './api-base.service'
import { partIndexQueryString } from '@/features/part-index/part-index.helpers'

export { partIndexQueryString } from '@/features/part-index/part-index.helpers'
import type { TitlePayload } from './title-block-workflow'

export type PartIndexStatus = 'pending' | 'failed' | 'needs_confirmation' | 'recognized' | 'edited' | 'confirmed' | 'recheck'

export interface PartIndexFields {
  drawingNo: string
  partName: string
  material: string
  designer: string
  checker: string
  approver: string
  drawingDateRaw: string
  scale: string
  sheetSize: string
  process: string
  standard: string
  company: string
}

export interface PartIndexProject {
  drawingId: string
  projectCode: string
  projectName: string
  drawingNo: string
  relationTypes: string[]
}

export interface PartIndexItem {
  attachmentId: string
  versionId: string
  fileName: string
  partId: string | null
  registeredPartNo: string
  drawingNo: string
  partName: string
  material: string
  designer: string
  drawingDateRaw: string
  drawingDate: string | null
  status: PartIndexStatus
  extractionStatus: 'pending' | 'extracted' | 'failed'
  canWrite: boolean
  projectCount: number
  projects: PartIndexProject[]
  versionCreatedAt: string
}

export interface PartIndexDetail extends PartIndexItem {
  revision: number
  snapshotRevision: number
  sourceSnapshotRevision: number
  selectedSpaceId: string | null
  selectionMode: 'auto' | 'manual'
  selectedSpaceMissing: boolean
  hasManualFields: boolean
  fields: PartIndexFields
  autoFields: PartIndexFields
  extractionError: string
  confirmedBy: string | null
  confirmedAt: string | null
  confirmedSnapshotRevision: number | null
  editedBy: string | null
  editedAt: string | null
  createdAt: string | null
  updatedAt: string | null
  sourcePayload: TitlePayload | null
  defaultProjectDrawingNo: string
}

export interface PartIndexPage {
  list: PartIndexItem[]
  total: number
  page: number
  pageSize: number
}

export interface PartIndexOption {
  value: string
  label: string
}

export interface PartIndexOptionPage {
  list: PartIndexOption[]
  hasMore: boolean
}

export interface PartIndexFilters {
  page: number
  pageSize: number
  keyword?: string
  projectId?: string
  material?: string
  designer?: string
  dateFrom?: string
  dateTo?: string
  status?: PartIndexStatus | ''
}

export interface PartIndexEditInput {
  versionId: string
  expectedRevision: number
  expectedSnapshotRevision: number
  action: 'save' | 'confirm' | 'reset'
  selectedSpaceId?: string | null
  fields?: PartIndexFields
}

export interface PartIndexBackfillInput {
  afterAttachmentId: string | null
  limit: number
}

export interface PartIndexBackfillError {
  attachmentId: string
  message: string
}

export interface PartIndexBackfillResult {
  scanned: number
  created: number
  updated: number
  unchanged: number
  skipped: number
  failed: number
  nextCursor: string | null
  hasMore: boolean
  errors: PartIndexBackfillError[]
}

function headers(): HeadersInit {
  const token = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
  return { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }
}

export class PartIndexRequestError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'PartIndexRequestError'
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, {
    ...init,
    headers: { ...headers(), ...init.headers },
    credentials: 'include',
    signal: init.signal ?? AbortSignal.timeout(30000),
  })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new PartIndexRequestError(body.message || `零件索引请求失败：HTTP ${response.status}`, response.status)
  return body.data ?? body
}

function queryString(values: Record<string, string | number | undefined>): string {
  const query = new URLSearchParams()
  Object.entries(values).forEach(([key, value]) => {
    if ((typeof value === 'string' || typeof value === 'number') && value !== '') query.set(key, String(value))
  })
  const text = query.toString()
  return text ? `?${text}` : ''
}

export const partIndexService = {
  list(filters: PartIndexFilters, signal?: AbortSignal): Promise<PartIndexPage> {
    return request(`/part-indexes${partIndexQueryString(filters)}`, { signal })
  },
  detail(attachmentId: string): Promise<PartIndexDetail> {
    return request(`/part-indexes/${encodeURIComponent(attachmentId)}`)
  },
  options(kind: 'project' | 'material' | 'designer', keyword = '', limit = 100): Promise<PartIndexOptionPage> {
    return request(`/part-indexes/options${queryString({ kind, keyword, limit })}`)
  },
  edit(attachmentId: string, input: PartIndexEditInput): Promise<PartIndexDetail> {
    return request(`/part-indexes/${encodeURIComponent(attachmentId)}`, {
      method: 'PATCH',
      body: JSON.stringify(input),
    })
  },
  rebuild(attachmentId: string, versionId: string, expectedSnapshotRevision: number): Promise<{ result: string, detail: PartIndexDetail }> {
    return request(`/part-indexes/${encodeURIComponent(attachmentId)}/rebuild`, {
      method: 'POST',
      body: JSON.stringify({ versionId, expectedSnapshotRevision }),
    })
  },
  backfill(input: PartIndexBackfillInput): Promise<PartIndexBackfillResult> {
    return request('/admin/part-indexes/backfill', {
      method: 'POST',
      body: JSON.stringify(input),
    })
  },
}
