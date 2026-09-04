import type { DrawingAttribute, DrawingAttributeField } from '@/types/domain.types'
import type { AttributeCommandGateway, AttributeCommandInput, AttributeFieldCommandInput } from '@/modules/attribute'
import { getApiBaseUrl } from './api-base.service'

export class AttributeCommandUnsupportedError extends Error {
  public constructor(status: number) {
    super(`属性原子接口暂不可用：HTTP ${status}`)
    this.name = 'AttributeCommandUnsupportedError'
  }
}

function authHeaders(): Record<string, string> {
  const token = typeof window === 'undefined'
    ? ''
    : window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token') || ''
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function request<T>(path: string, method: 'POST' | 'PATCH' | 'DELETE', body?: unknown): Promise<T> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, {
    method,
    headers: { Accept: 'application/json', ...(body === undefined ? {} : { 'Content-Type': 'application/json' }), ...authHeaders() },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    credentials: 'include',
  })
  const payload: unknown = response.status === 204 ? undefined : await response.json().catch(() => undefined)
  if (!response.ok) {
    if (response.status === 404 || response.status === 405) throw new AttributeCommandUnsupportedError(response.status)
    const message = typeof payload === 'object' && payload !== null && 'message' in payload && typeof payload.message === 'string'
      ? payload.message
      : `属性接口请求失败：HTTP ${response.status}`
    throw new Error(message)
  }
  if (typeof payload === 'object' && payload !== null && 'data' in payload) return payload.data as T
  return payload as T
}

function attribute(value: unknown, fallback: AttributeCommandInput): DrawingAttribute {
  const item = typeof value === 'object' && value !== null ? value as Record<string, unknown> : {}
  return {
    id: typeof item.id === 'string' ? item.id : `attribute-${Date.now()}`,
    name: typeof item.name === 'string' ? item.name : fallback.name,
    required: typeof item.required === 'boolean' ? item.required : fallback.required,
    enabled: typeof item.enabled === 'boolean' ? item.enabled : fallback.enabled,
    sortOrder: typeof item.sortOrder === 'number' ? item.sortOrder : fallback.sortOrder,
    fields: Array.isArray(item.fields) ? item.fields as DrawingAttributeField[] : [],
  }
}

function field(value: unknown, fallback: AttributeFieldCommandInput): DrawingAttributeField {
  const item = typeof value === 'object' && value !== null ? value as Record<string, unknown> : {}
  return {
    id: typeof item.id === 'string' ? item.id : `attribute-field-${Date.now()}`,
    name: typeof item.name === 'string' ? item.name : fallback.name,
    enabled: typeof item.enabled === 'boolean' ? item.enabled : fallback.enabled,
    sortOrder: typeof item.sortOrder === 'number' ? item.sortOrder : fallback.sortOrder,
    ...(typeof item.createdAt === 'string' ? { createdAt: item.createdAt } : {}),
  }
}

export const attributeCommandApiGateway: AttributeCommandGateway = {
  async create(input) {
    return attribute(await request('/drawing-attributes', 'POST', input), input)
  },
  async update(id, input) {
    return attribute(await request(`/drawing-attributes/${encodeURIComponent(id)}`, 'PATCH', input), input)
  },
  async remove(id) {
    await request(`/drawing-attributes/${encodeURIComponent(id)}`, 'DELETE')
  },
  async addField(attributeId, input) {
    return field(await request(`/drawing-attributes/${encodeURIComponent(attributeId)}/fields`, 'POST', input), input)
  },
  async updateField(attributeId, fieldId, input) {
    return field(await request(`/drawing-attributes/${encodeURIComponent(attributeId)}/fields/${encodeURIComponent(fieldId)}`, 'PATCH', input), input)
  },
  async removeField(attributeId, fieldId) {
    await request(`/drawing-attributes/${encodeURIComponent(attributeId)}/fields/${encodeURIComponent(fieldId)}`, 'DELETE')
  },
}
