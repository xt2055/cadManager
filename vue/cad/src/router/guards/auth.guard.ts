import type { Router } from 'vue-router'
import { RouteName } from '../route-names'

export function setupAuthGuard(router: Router): void {
  router.beforeEach((to) => {
    const requiresAuth = to.meta.requiresAuth as boolean | undefined
    if (requiresAuth === false) {
      return true
    }
    const token = localStorage.getItem('cad_access_token')
    if (!token) {
      return { name: RouteName.Login, query: { redirect: to.fullPath } }
    }
    return true
  })
}
