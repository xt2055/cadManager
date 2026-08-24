import type { Router } from 'vue-router'
import { usePermission } from '@/composables/usePermission'
import { useAuthStore } from '@/stores/auth.store'
import type { UserRole } from '@/types/domain.types'
import { RouteName } from '../route-names'

export function setupPermissionGuard(router: Router): void {
  router.beforeEach((to) => {
    const authStore = useAuthStore()
    const roles = to.meta.roles as UserRole[] | undefined
    if (roles?.length && !authStore.hasAnyRole(roles)) {
      return { name: RouteName.Dashboard }
    }
    const required = to.meta.permission as string | undefined
    if (!required) {
      return true
    }
    const { hasPermission } = usePermission()
    if (hasPermission(required)) {
      return true
    }
    return { name: RouteName.Dashboard }
  })
}
