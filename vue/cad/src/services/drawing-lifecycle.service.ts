import { getApiBaseUrl } from '@/services/api-base.service'
import { authorizationHeaders } from '@/services/auth/access-token'
import { notifySessionExpired } from '@/services/auth/session-expiry'

export interface ApiDrawingLifecycle {
  id: string
  no: string
  name: string
  status: string
}

function apiBaseUrl() {
  return getApiBaseUrl()
}

async function request<T>(path: string): Promise<T> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    method: 'POST',
    headers: authorizationHeaders(),
    credentials: 'include',
  })
  const body = await response.json().catch(() => null) as { data?: T; message?: string } | T | null
  if (response.status === 401) notifySessionExpired()
  if (!response.ok) {
    const message = body && typeof body === 'object' && 'message' in body ? body.message : undefined
    throw new Error(typeof message === 'string' ? message : `图纸状态更新失败：HTTP ${response.status}`)
  }
  if (body && typeof body === 'object' && 'data' in body) return body.data as T
  return body as T
}

export const drawingLifecycleService = {
  /** 存档：仅「生产中」图纸，创建者或管理员可操作。 */
  archive(drawingNo: string): Promise<ApiDrawingLifecycle> {
    return request(`/drawings/${encodeURIComponent(drawingNo)}/archive`)
  },

  /** 解除存档：仅管理员，恢复为「生产中」。 */
  unarchive(drawingNo: string): Promise<ApiDrawingLifecycle> {
    return request(`/drawings/${encodeURIComponent(drawingNo)}/unarchive`)
  },
}
