import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { isTauri, invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'
import { getCurrent, onOpenUrl } from '@tauri-apps/plugin-deep-link'
import { i18n } from '@mlightcad/cad-viewer'
import { AcApDocManager } from '@mlightcad/cad-simple-viewer'
import { installCadLineRenderCallback } from './services/cad-line-render-callback'
import { getApiBaseUrl, initializeApiBaseUrl } from './services/api-base.service'
import App from './App.vue'
import { router } from './router'
import { RouteName } from './router/route-names'
import { readAccessToken } from './services/auth/access-token'
import { onSessionExpired } from './services/auth/session-expiry'
import { useAuthStore } from './stores/auth.store'
import { getTimeBasedMode } from './stores/theme.store'
import { useUiStore } from './stores/ui.store'
import '@mlightcad/cad-viewer/style.css'
import './styles/index.css'

// Windows 打开自定义协议时，deep-link 插件和 single-instance 插件可能
// 同时转发同一个 URL。票据只能消费一次，因此必须在客户端先做短时去重。
const handledCadTickets = new Map<string, number>()

function cadTicketFromUrl(url: string): string {
  return new URL(url).searchParams.get('ticket') || url
}

async function openDeepLink(url: string): Promise<void> {
  if (!url.startsWith('cadguanliq://open')) return
  const ticket = cadTicketFromUrl(url)
  const now = Date.now()
  const previous = handledCadTickets.get(ticket)
  if (previous && now - previous < 120_000) return
  handledCadTickets.set(ticket, now)
  const accessToken = readAccessToken()
  const apiBaseUrl = getApiBaseUrl()
  if (!accessToken || !apiBaseUrl) return
  try {
    await invoke('open_cad_edit_session', { apiBaseUrl, accessToken, openUrl: url })
  } catch (error) {
    console.error('处理 CAD 打开链接失败', error)
  }
}

/**
 * 401 的统一出口：清掉本地登录态、提示原因并带着回跳地址回到登录页。
 * 过去 401 被业务层吞成空数据，用户只看到「预览打不开」；这里保证失败可见、可继续。
 */
function handleSessionExpired(): void {
  const authStore = useAuthStore()
  if (!authStore.isAuthenticated) return
  const current = router.currentRoute.value
  const redirect = current.name && current.name !== RouteName.Login ? current.fullPath : ''
  authStore.logout()
  useUiStore().toast('登录已失效，请重新登录', 'warn')
  void router.replace({ name: RouteName.Login, query: redirect ? { redirect } : {} })
}

document.documentElement.dataset.skin = 'classic'
document.documentElement.dataset.theme = getTimeBasedMode()

if (typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window) {
  document.documentElement.dataset.tauri = 'true'
}

// 完整版 MlCadViewer 暂未暴露 webworkerFileUrls，统一注入根路径资源，避免深层路由拼接出错误地址。
installCadLineRenderCallback()
const originalCreateInstance = AcApDocManager.createInstance.bind(AcApDocManager)
const workerUrls = {
  dwgParser: new URL('/assets/libredwg-parser-worker.js', window.location.origin).href,
  mtextRender: new URL('/assets/mtext-renderer-worker.js', window.location.origin).href,
}
AcApDocManager.createInstance = (options = {}) => originalCreateInstance({
  ...options,
  webworkerFileUrls: {
    ...workerUrls,
    ...options.webworkerFileUrls,
  },
})

async function bootstrap(): Promise<void> {
  await initializeApiBaseUrl()

  if (isTauri()) {
    void onOpenUrl((urls) => Promise.all(urls.map(openDeepLink)))
    void getCurrent().then((urls) => Promise.all((urls || []).map(openDeepLink))).catch((error) => {
      console.error('读取 CAD 打开链接失败', error)
    })
    void listen<string[]>('cad-deep-link', (event) => Promise.all(event.payload.map(openDeepLink)))
  }

  const app = createApp(App)
  router.onError((error) => {
    console.error('路由导航失败', error)
  })
  app.use(createPinia())
  // 会话失效处理必须在 Pinia 装好之后注册：处理器内部要用 useAuthStore/useUiStore。
  onSessionExpired(handleSessionExpired)
  app.use(i18n)
  app.use(router)
  app.mount('#app')
}

void bootstrap()
