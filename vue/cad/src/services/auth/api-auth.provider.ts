import type { AuthProvider, AuthResult, AuthUser, LoginRequest, StoredAuthSession } from '@/features/auth/types/auth.types'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export class ApiAuthProvider implements AuthProvider {
  private readonly baseUrl: string

  constructor(baseUrl = import.meta.env.VITE_API_BASE_URL || '/api') {
    this.baseUrl = baseUrl.replace(/\/$/, '')
  }

  async login(request: LoginRequest): Promise<AuthResult> {
    const body = await this.request<unknown>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(request),
    })
    const payload = this.unwrap(body)
    if (!isRecord(payload) || !isRecord(payload.user) || typeof payload.token !== 'string') {
      throw new Error('登录接口返回格式无效')
    }
    return { token: payload.token, user: payload.user as unknown as AuthUser }
  }

  async restore(session: StoredAuthSession): Promise<AuthUser | null> {
    try {
      const body = await this.request<unknown>('/auth/me', { method: 'GET' }, session.token)
      const payload = this.unwrap(body)
      if (!isRecord(payload)) return null
      return (isRecord(payload.user) ? payload.user : payload) as unknown as AuthUser
    } catch {
      return null
    }
  }

  private unwrap(body: unknown): unknown {
    if (isRecord(body) && typeof body.code === 'number' && 'data' in body) {
      if (body.code !== 0 && body.code !== 200) {
        throw new Error(typeof body.message === 'string' ? body.message : '认证接口返回失败')
      }
      return body.data
    }
    return body
  }

  private async request<T>(path: string, options: { method: 'GET' | 'POST'; body?: string }, token?: string): Promise<T> {
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (options.body) headers['Content-Type'] = 'application/json'
    if (token) headers.Authorization = `Bearer ${token}`
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: options.method,
      headers,
      body: options.body,
      credentials: 'include',
    })
    if (!response.ok) throw new Error(`认证接口请求失败：HTTP ${response.status}`)
    const contentType = response.headers.get('content-type') || ''
    if (!contentType.includes('application/json')) {
      throw new Error('认证接口未返回 JSON，请检查后端地址或调试模式配置')
    }
    return response.json() as Promise<T>
  }
}
