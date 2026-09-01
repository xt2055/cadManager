export interface ApiDrawingLifecycle {
  id: string
  no: string
  name: string
  status: string
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

async function request<T>(path: string): Promise<T> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    method: 'POST',
    headers: authHeaders(),
    credentials: 'include',
  })
  const body = await response.json().catch(() => null) as { data?: T; message?: string } | T | null
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
