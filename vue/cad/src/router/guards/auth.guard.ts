import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth.store'
import { RouteName } from '../route-names'

export function setupAuthGuard(router: Router): void {
  router.beforeEach(async (to) => {
    const authStore = useAuthStore()
    await authStore.restoreSession()
    const requiresAuth = to.meta.requiresAuth as boolean | undefined
    if (requiresAuth === false) {
      if (authStore.isAuthenticated && to.name === RouteName.Login) {
        return authStore.defaultPath()
      }
      return true
    }
    if (!authStore.isAuthenticated) {
      return { name: RouteName.Login, query: { redirect: to.fullPath } }
    }
    return true
  })
}
