import type { AuthUser } from '@/features/auth/types/auth.types'
import { dataManager } from '@/services/data-manager'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export async function fetchReviewerCandidates(role = 'reviewer'): Promise<Array<{ id: string; name: string; account: string }>> {
  const baseUrl = getApiBaseUrl()
  try {
    const token = typeof window !== 'undefined' ? window.localStorage.getItem('cad_access_token') : null
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (token) headers.Authorization = `Bearer ${token}`
    const response = await fetch(`${baseUrl}/users/reviewers?role=${encodeURIComponent(role)}`, {
      method: 'GET',
      headers,
      credentials: 'include',
    })
    if (response.ok) {
      const body: unknown = await response.json()
      const data = isRecord(body) && 'data' in body ? body.data : body
      if (Array.isArray(data)) {
        return data
          .filter((item): item is AuthUser => isRecord(item) && typeof item.displayName === 'string')
          .map((item) => ({ id: item.id, name: item.displayName, account: item.account }))
      }
    }
  } catch (error) {
    console.warn('请求后端审核人员失败，回退到本地文档用户', error)
  }

  try {
    const users = await dataManager.listUsers()
    return users
      .filter((user) => user.status === 'active' && (!role || user.roles.includes(role as any)))
      .map((user) => ({ id: user.id, name: user.displayName, account: user.account }))
  } catch {
    return []
  }
}
import { getApiBaseUrl } from '@/services/api-base.service'
