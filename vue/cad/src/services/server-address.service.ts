// 服务器地址管理（仅用于客户端更新检查）：内网服务器 IP 可能变化，
// 用户可输入服务器地址用于寻找更新；业务请求不受影响，仍走编译期配置。

const STORAGE_KEY = 'cad:server-base:v1'
const DEFAULT_BASE_URL = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

/** 规范化用户输入：支持 192.168.1.50 / host:8080 / http://host:8080 / http://host:8080/api */
export function normalizeServerInput(input: string): string {
  let text = input.trim()
  if (!text) return ''
  if (!/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(text)) text = `http://${text}`
  let url: URL
  try {
    url = new URL(text)
  } catch {
    return ''
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return ''
  const path = url.pathname.replace(/\/+$/, '')
  url.pathname = path === '' || path === '/' ? '/api' : path
  url.search = ''
  url.hash = ''
  return url.toString().replace(/\/$/, '')
}

/** 更新检查使用的基地址（记住的服务器地址优先，其次编译期默认值） */
export function getUpdateBaseUrl(): string {
  if (typeof window === 'undefined') return DEFAULT_BASE_URL
  const saved = window.localStorage.getItem(STORAGE_KEY)
  return saved && saved.trim() ? saved.trim() : DEFAULT_BASE_URL
}

/** 是否已记住自定义更新服务器地址 */
export function hasCustomUpdateServer(): boolean {
  if (typeof window === 'undefined') return false
  return Boolean(window.localStorage.getItem(STORAGE_KEY)?.trim())
}

/** 当前记住的地址（无则空串） */
export function getSavedUpdateServer(): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem(STORAGE_KEY)?.trim() || ''
}

/** 记住更新服务器地址（入参应为 normalizeServerInput 的结果） */
export function saveUpdateServer(normalized: string): void {
  if (!normalized) return
  window.localStorage.setItem(STORAGE_KEY, normalized)
}

/** 清除记忆，恢复编译期默认 */
export function clearUpdateServer(): void {
  window.localStorage.removeItem(STORAGE_KEY)
}

export interface ProbeResult {
  reachable: boolean
  version?: string
  database?: string
  message?: string
}

interface HealthPayload {
  status?: string
  database?: string
  service?: { version?: string; status?: string }
}

/** 探测服务器可达性（GET /health，无需登录） */
export async function probeServer(baseUrl: string, timeoutMs = 5000): Promise<ProbeResult> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  try {
    const response = await fetch(`${baseUrl.replace(/\/$/, '')}/health`, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      credentials: 'omit',
      signal: controller.signal,
    })
    if (!response.ok) return { reachable: false, message: `HTTP ${response.status}` }
    const body: unknown = await response.json()
    const payload = (isRecord(body) && 'data' in body ? body.data : body) as HealthPayload
    return {
      reachable: true,
      version: payload?.service?.version,
      database: payload?.database,
    }
  } catch (error) {
    return {
      reachable: false,
      message: error instanceof DOMException && error.name === 'AbortError' ? '连接超时' : '无法连接',
    }
  } finally {
    clearTimeout(timer)
  }
}
