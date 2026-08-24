import type { Router } from 'vue-router'
import { setupAuthGuard } from './auth.guard'
import { setupPermissionGuard } from './permission.guard'
import { setupTitleGuard } from './title.guard'

export function setupGuards(router: Router): void {
  setupAuthGuard(router)
  setupPermissionGuard(router)
  setupTitleGuard(router)
}
