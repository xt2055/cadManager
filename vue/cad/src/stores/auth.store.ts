import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import type { UserRole } from '@/types/domain.types'
import { createAuthProvider } from '@/services/auth/auth-provider.factory'
import type { AuthUser, LoginRequest, StoredAuthSession } from '@/features/auth/types/auth.types'
import {
  ACCESS_TOKEN_MIRROR_KEY,
  SESSION_STORAGE_KEY,
  readStoredSession,
  setAccessToken,
  syncAccessTokenMirror,
  type TokenStorageKind,
} from '@/services/auth/access-token'

/**
 * 角色权限表。
 * 建档权（drawing.create）只在计划员与管理员手里：创建图纸的职责已从设计人员
 * 移交给计划员，设计人员通过「任务管理台」被指派为负责人后编制图纸，
 * 其权限来自图纸负责人身份（isDrawingDecider），不再来自角色。
 */
const rolePermissions: Record<UserRole, string[]> = {
  admin: ['*'],
  planner: ['dashboard.read', 'drawing.read', 'drawing.create', 'drawing.edit', 'task.read', 'task.assign'],
  designer: ['dashboard.read', 'drawing.read', 'drawing.edit', 'task.read'],
  reviewer: ['dashboard.read', 'drawing.read', 'review.read', 'review.process', 'task.read'],
}

/** 会话记录解析统一在 services/auth/access-token，这里只补上「存在哪个存储」这一信息。 */
function readStoredSessionEntry(): { session: StoredAuthSession; storage: TokenStorageKind } | null {
  const stored = readStoredSession()
  if (!stored) return null
  return { session: { token: stored.token, userId: stored.userId }, storage: stored.storage }
}

export const useAuthStore = defineStore('auth', () => {
  let providerPromise: Promise<import('@/features/auth/types/auth.types').AuthProvider> | null = null
  const currentUser = ref<AuthUser | null>(null)
  const token = ref('')
  const initialized = ref(false)
  const isAuthenticated = computed(() => Boolean(currentUser.value && token.value))

  function clearLegacySession() {
    window.localStorage.removeItem(ACCESS_TOKEN_MIRROR_KEY)
    window.localStorage.removeItem('cad_current_user')
    window.sessionStorage.removeItem(ACCESS_TOKEN_MIRROR_KEY)
    window.sessionStorage.removeItem('cad_current_user')
  }

  function clearSession() {
    currentUser.value = null
    token.value = ''
    // 内存令牌只增不减会让退出后的后台轮询继续带旧令牌打接口，一律同源清空。
    setAccessToken('')
    window.localStorage.removeItem(SESSION_STORAGE_KEY)
    window.sessionStorage.removeItem(SESSION_STORAGE_KEY)
    clearLegacySession()
  }

  async function restoreSession(): Promise<void> {
    if (initialized.value) return
    initialized.value = true
    const entry = readStoredSessionEntry()
    if (!entry) {
      clearLegacySession()
      return
    }

    try {
      const provider = await getProvider()
      const user = await provider.restore(entry.session)
      if (!user) {
        clearSession()
        return
      }
      currentUser.value = user
      token.value = entry.session.token
      // 恢复出登录态后立刻同步内存令牌与镜像键：只认存储镜像的调用点过去会在
      // 「内存有令牌、镜像被清掉」的窗口里发出无鉴权请求并拿到 401。
      setAccessToken(entry.session.token)
      syncAccessTokenMirror(entry.session.token, entry.storage)
    } catch (error) {
      console.error('恢复登录会话失败', error)
      clearSession()
    }
  }

  async function login(request: LoginRequest): Promise<AuthUser> {
    const result = await (await getProvider()).login(request)
    currentUser.value = result.user
    token.value = result.token
    setAccessToken(result.token)

    const sessionPayload = JSON.stringify({ token: result.token, userId: result.user.id } satisfies StoredAuthSession)
    if (request.rememberMe) {
      window.localStorage.setItem(SESSION_STORAGE_KEY, sessionPayload)
      syncAccessTokenMirror(result.token, 'local')
      window.sessionStorage.removeItem(SESSION_STORAGE_KEY)
      window.sessionStorage.removeItem(ACCESS_TOKEN_MIRROR_KEY)
    } else {
      window.sessionStorage.setItem(SESSION_STORAGE_KEY, sessionPayload)
      syncAccessTokenMirror(result.token, 'session')
      window.localStorage.removeItem(SESSION_STORAGE_KEY)
      window.localStorage.removeItem(ACCESS_TOKEN_MIRROR_KEY)
    }
    initialized.value = true
    return result.user
  }

  function logout() {
    clearSession()
    initialized.value = true
  }

  function getProvider() {
    if (!providerPromise) providerPromise = Promise.resolve(createAuthProvider())
    return providerPromise
  }

  function hasRole(role: UserRole): boolean {
    return currentUser.value?.roles.includes(role) ?? false
  }

  function hasAnyRole(roles: UserRole[]): boolean {
    return roles.some((role) => hasRole(role))
  }

  /** 建档权：只有计划员与管理员可以创建图纸（创建新图纸 / 上传老图纸 / 从老图纸分叉）。 */
  const canCreateDrawing = computed(() => hasPermission('drawing.create'))

  /** 任务管理台：只有计划员与管理员可以指派、改派负责人。 */
  const canAssignTasks = computed(() => hasPermission('task.assign'))

  function hasPermission(permission: string): boolean {
    if (!currentUser.value) return false
    return currentUser.value.roles.some((role) => {
      const permissions = rolePermissions[role]
      return permissions.includes('*') || permissions.includes(permission)
    })
  }

  function defaultPath(): string {
    if (hasRole('admin')) return '/admin/accounts'
    if (hasRole('reviewer')) return '/reviews/pending'
    return '/dashboard'
  }

  return {
    currentUser,
    token,
    initialized,
    isAuthenticated,
    restoreSession,
    login,
    logout,
    hasRole,
    hasAnyRole,
    hasPermission,
    canCreateDrawing,
    canAssignTasks,
    defaultPath,
  }
})
