import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import type { UserRole } from '@/types/domain.types'
import { createAuthProvider } from '@/services/auth/auth-provider.factory'
import type { AuthUser, LoginRequest, StoredAuthSession } from '@/features/auth/types/auth.types'

const SESSION_STORAGE_KEY = 'cad:auth-session:v1'

const rolePermissions: Record<UserRole, string[]> = {
  admin: ['*'],
  designer: ['dashboard.read', 'drawing.read', 'drawing.create', 'drawing.edit'],
  reviewer: ['dashboard.read', 'drawing.read', 'review.read', 'review.process'],
}

function readStoredSession(): StoredAuthSession | null {
  const localRaw = window.localStorage.getItem(SESSION_STORAGE_KEY)
  const sessionRaw = window.sessionStorage.getItem(SESSION_STORAGE_KEY)
  const raw = localRaw || sessionRaw
  if (!raw) return null

  try {
    const value: unknown = JSON.parse(raw)
    if (!value || typeof value !== 'object') return null
    const session = value as Partial<StoredAuthSession>
    if (typeof session.token !== 'string' || typeof session.userId !== 'string') return null
    return { token: session.token, userId: session.userId }
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', () => {
  let providerPromise: Promise<import('@/features/auth/types/auth.types').AuthProvider> | null = null
  const currentUser = ref<AuthUser | null>(null)
  const token = ref('')
  const initialized = ref(false)
  const isAuthenticated = computed(() => Boolean(currentUser.value && token.value))

  function clearLegacySession() {
    window.localStorage.removeItem('cad_access_token')
    window.localStorage.removeItem('cad_current_user')
    window.sessionStorage.removeItem('cad_access_token')
    window.sessionStorage.removeItem('cad_current_user')
  }

  function clearSession() {
    currentUser.value = null
    token.value = ''
    window.localStorage.removeItem(SESSION_STORAGE_KEY)
    window.sessionStorage.removeItem(SESSION_STORAGE_KEY)
    clearLegacySession()
  }

  async function restoreSession(): Promise<void> {
    if (initialized.value) return
    initialized.value = true
    const session = readStoredSession()
    if (!session) {
      clearLegacySession()
      return
    }

    try {
      const provider = await getProvider()
      const user = await provider.restore(session)
      if (!user) {
        clearSession()
        return
      }
      currentUser.value = user
      token.value = session.token
    } catch (error) {
      console.error('恢复登录会话失败', error)
      clearSession()
    }
  }

  async function login(request: LoginRequest): Promise<AuthUser> {
    const result = await (await getProvider()).login(request)
    currentUser.value = result.user
    token.value = result.token

    const sessionPayload = JSON.stringify({ token: result.token, userId: result.user.id } satisfies StoredAuthSession)
    if (request.rememberMe) {
      window.localStorage.setItem(SESSION_STORAGE_KEY, sessionPayload)
      window.localStorage.setItem('cad_access_token', result.token)
      window.sessionStorage.removeItem(SESSION_STORAGE_KEY)
      window.sessionStorage.removeItem('cad_access_token')
    } else {
      window.sessionStorage.setItem(SESSION_STORAGE_KEY, sessionPayload)
      window.sessionStorage.setItem('cad_access_token', result.token)
      window.localStorage.removeItem(SESSION_STORAGE_KEY)
      window.localStorage.removeItem('cad_access_token')
    }
    initialized.value = true
    return result.user
  }

  function logout() {
    clearSession()
    initialized.value = true
  }

  function resetProvider() {
    providerPromise = null
  }

  function getProvider() {
    if (!providerPromise) providerPromise = createAuthProvider()
    return providerPromise
  }

  function hasRole(role: UserRole): boolean {
    return currentUser.value?.roles.includes(role) ?? false
  }

  function hasAnyRole(roles: UserRole[]): boolean {
    return roles.some((role) => hasRole(role))
  }

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
    resetProvider,
    hasRole,
    hasAnyRole,
    hasPermission,
    defaultPath,
  }
})
