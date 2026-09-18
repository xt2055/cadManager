/**
 * 访问令牌的唯一读取入口。
 *
 * 登录态同时存在三处：内存登录态（auth.store）、会话记录 `cad:auth-session:v1`、
 * 历史遗留的镜像键 `cad_access_token`。服务层过去只认镜像键，于是会出现
 * 「本窗口已登录、心跳与系统状态都正常，但普通接口 401」的状态：
 * 跨窗口登录/退出、旧版本写入或存储被清都会让镜像键变空，请求不带 Authorization
 * 发出去，业务层又把 401 吞成空数据，最终表现为预览页空白、变更工单读不到。
 *
 * 因此服务层统一从这里取令牌，优先级：
 *   内存令牌（由 auth.store 登录/恢复/退出时写入） → 会话记录 → 镜像键；
 * 两个存储之间本地存储优先，与会话恢复的既有优先级保持一致。
 */
export const SESSION_STORAGE_KEY = 'cad:auth-session:v1'
export const ACCESS_TOKEN_MIRROR_KEY = 'cad_access_token'

export type TokenStorageKind = 'local' | 'session'

const TOKEN_STORAGE_ORDER: readonly TokenStorageKind[] = ['local', 'session']

let memoryToken = ''

/** 由 auth.store 在登录、恢复会话与退出登录时调用，保证服务层与内存登录态同源。 */
export function setAccessToken(token: string | null | undefined): void {
  memoryToken = typeof token === 'string' ? token.trim() : ''
}

export interface StoredAccessSession {
  token: string
  userId: string
  storage: TokenStorageKind
}

/** 读取某个存储；隐私模式或禁用存储时浏览器会抛错，这里一律按「没有存储」处理。 */
export function storageOf(kind: TokenStorageKind): Storage | null {
  if (typeof window === 'undefined') return null
  try {
    return kind === 'local' ? window.localStorage : window.sessionStorage
  } catch {
    return null
  }
}

function safeGetItem(storage: Storage | null, key: string): string {
  if (!storage) return ''
  try {
    return storage.getItem(key) || ''
  } catch {
    return ''
  }
}

/** 会话记录（token + userId）的解析只有这一处，避免各调用点各写一份。 */
export function readStoredSession(): StoredAccessSession | null {
  for (const kind of TOKEN_STORAGE_ORDER) {
    const raw = safeGetItem(storageOf(kind), SESSION_STORAGE_KEY)
    if (!raw) continue
    try {
      const value: unknown = JSON.parse(raw)
      if (typeof value !== 'object' || value === null) continue
      const record = value as { token?: unknown; userId?: unknown }
      if (typeof record.token !== 'string' || typeof record.userId !== 'string') continue
      const token = record.token.trim()
      if (!token) continue
      return { token, userId: record.userId, storage: kind }
    } catch {
      continue
    }
  }
  return null
}

function readMirrorToken(): { token: string; storage: TokenStorageKind } | null {
  for (const kind of TOKEN_STORAGE_ORDER) {
    const token = safeGetItem(storageOf(kind), ACCESS_TOKEN_MIRROR_KEY).trim()
    if (token) return { token, storage: kind }
  }
  return null
}

/** 存储侧令牌：会话记录优先，镜像键兜底（兼容只写过镜像键的旧登录态）。 */
export function readStoredAccessToken(): { token: string; storage: TokenStorageKind | null } {
  const session = readStoredSession()
  if (session) return { token: session.token, storage: session.storage }
  const mirror = readMirrorToken()
  return mirror ?? { token: '', storage: null }
}

/** 服务层取令牌的唯一入口：内存登录态优先，其次才是存储。 */
export function readAccessToken(): string {
  return memoryToken || readStoredAccessToken().token
}

/** 带鉴权的请求头。extra 用于叠加 Content-Type 等附加头。 */
export function authorizationHeaders(extra: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = { Accept: 'application/json', ...extra }
  const token = readAccessToken()
  if (token) headers.Authorization = `Bearer ${token}`
  return headers
}

/**
 * 把令牌镜像写回会话所在的存储。
 * 用于恢复会话后自愈：内存与会话记录都还有令牌、只有镜像键被清掉的状态，
 * 会让尚未改造的调用点继续 401，写回镜像即可消除。
 */
export function syncAccessTokenMirror(token: string, storage: TokenStorageKind = 'local'): void {
  const trimmed = token.trim()
  if (!trimmed) return
  try {
    storageOf(storage)?.setItem(ACCESS_TOKEN_MIRROR_KEY, trimmed)
  } catch {
    // 存储不可写时忽略：鉴权仍走内存令牌与会话记录。
  }
}
