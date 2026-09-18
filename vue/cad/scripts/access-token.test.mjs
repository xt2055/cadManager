import assert from 'node:assert/strict'
import test from 'node:test'

import {
  ACCESS_TOKEN_MIRROR_KEY,
  SESSION_STORAGE_KEY,
  authorizationHeaders,
  readAccessToken,
  readStoredAccessToken,
  readStoredSession,
  setAccessToken,
  syncAccessTokenMirror,
} from '../src/services/auth/access-token.ts'

/**
 * 令牌读取的回归测试。
 * 背景：真实故障是预览页的 GET /api/change-requests 401 —— 内存里登录态还有效
 * （心跳、系统状态都正常），但存储里的令牌镜像键被清掉，而服务层只认镜像键。
 * 这里锁定「只要还有会话记录就一定能拿到令牌」这一条。
 */
function fakeStorage(seed = {}) {
  const map = new Map(Object.entries(seed))
  return {
    getItem: (key) => (map.has(key) ? map.get(key) : null),
    setItem: (key, value) => { map.set(key, String(value)) },
    removeItem: (key) => { map.delete(key) },
    dump: () => Object.fromEntries(map),
  }
}

function useStorage({ local = {}, session = {} } = {}) {
  const localStorage = fakeStorage(local)
  const sessionStorage = fakeStorage(session)
  globalThis.window = { localStorage, sessionStorage }
  return { localStorage, sessionStorage }
}

function resetToken() {
  setAccessToken('')
}

const sessionRecord = (token, userId = 'user-1') => JSON.stringify({ token, userId })

test('会话记录是令牌的唯一事实来源，镜像键只在缺失时兜底', () => {
  resetToken()
  useStorage({ local: { [ACCESS_TOKEN_MIRROR_KEY]: 'stale-mirror', [SESSION_STORAGE_KEY]: sessionRecord('fresh-session') } })
  assert.equal(readAccessToken(), 'fresh-session')
})

test('镜像键被清掉时仍能从会话记录取到令牌（本次 401 的回归点）', () => {
  resetToken()
  useStorage({ local: { [SESSION_STORAGE_KEY]: sessionRecord('only-session') } })
  assert.equal(readAccessToken(), 'only-session')
  assert.equal(authorizationHeaders().Authorization, 'Bearer only-session')
})

test('会话存在会话存储里时同样能被读到', () => {
  resetToken()
  useStorage({ session: { [SESSION_STORAGE_KEY]: sessionRecord('session-only') } })
  assert.deepEqual(readStoredAccessToken(), { token: 'session-only', storage: 'session' })
  assert.equal(readStoredSession()?.storage, 'session')
})

test('只写过镜像键的旧登录态继续可用', () => {
  resetToken()
  useStorage({ local: { [ACCESS_TOKEN_MIRROR_KEY]: 'legacy-token' } })
  assert.equal(readAccessToken(), 'legacy-token')
})

test('会话记录损坏时回退到镜像键', () => {
  resetToken()
  useStorage({ local: { [SESSION_STORAGE_KEY]: '{不是 JSON', [ACCESS_TOKEN_MIRROR_KEY]: 'mirror-token' } })
  assert.equal(readAccessToken(), 'mirror-token')

  resetToken()
  useStorage({ local: { [SESSION_STORAGE_KEY]: JSON.stringify({ token: 'a' }) , [ACCESS_TOKEN_MIRROR_KEY]: 'mirror-token' } })
  assert.equal(readAccessToken(), 'mirror-token')
})

test('内存令牌优先：退出登录清空内存后不再沿用存储里的旧令牌', () => {
  resetToken()
  useStorage({ local: { [SESSION_STORAGE_KEY]: sessionRecord('stored') } })
  setAccessToken('memory')
  assert.equal(readAccessToken(), 'memory')
  setAccessToken('')
  assert.equal(readAccessToken(), 'stored')
})

test('没有登录态时不带 Authorization，而不是带一个空的 Bearer', () => {
  resetToken()
  useStorage()
  assert.deepEqual(authorizationHeaders(), { Accept: 'application/json' })
  assert.deepEqual(readStoredAccessToken(), { token: '', storage: null })
})

test('authorizationHeaders 叠加附加头且不覆盖 Authorization', () => {
  resetToken()
  useStorage({ local: { [SESSION_STORAGE_KEY]: sessionRecord('t') } })
  assert.deepEqual(authorizationHeaders({ 'Content-Type': 'application/json' }), {
    Accept: 'application/json',
    'Content-Type': 'application/json',
    Authorization: 'Bearer t',
  })
})

test('syncAccessTokenMirror 把镜像键写回会话所在的存储，空令牌不写', () => {
  resetToken()
  const { sessionStorage } = useStorage({ session: { [SESSION_STORAGE_KEY]: sessionRecord('t') } })
  syncAccessTokenMirror('t', 'session')
  assert.equal(sessionStorage.dump()[ACCESS_TOKEN_MIRROR_KEY], 't')

  syncAccessTokenMirror('', 'session')
  assert.equal(sessionStorage.dump()[ACCESS_TOKEN_MIRROR_KEY], 't')
})

test('没有 window（Worker / Node 环境）时按未登录处理，不抛异常', () => {
  resetToken()
  delete globalThis.window
  assert.equal(readAccessToken(), '')
  assert.equal(readStoredSession(), null)
  assert.deepEqual(authorizationHeaders(), { Accept: 'application/json' })
})
