import type { Router } from 'vue-router'
import { usePermission } from '@/composables/usePermission'
import { RouteName } from '../route-names'

export function setupPermissionGuard(router: Router): void {
  router.beforeEach((to) => {
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
