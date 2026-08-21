import type { Router } from 'vue-router'
import { setupTitleGuard } from './title.guard'

export function setupGuards(router: Router): void {
  setupTitleGuard(router)
}
