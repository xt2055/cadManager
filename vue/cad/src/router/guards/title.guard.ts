import type { Router } from 'vue-router'

export function setupTitleGuard(router: Router): void {
  router.afterEach((to) => {
    const title = to.meta.title as string | undefined
    document.title = title ? `${title} · 图枢` : '图枢 · CAD 图纸管理系统'
  })
}
