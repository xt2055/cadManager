import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { i18n } from '@mlightcad/cad-viewer'
import { AcApDocManager } from '@mlightcad/cad-simple-viewer'
import App from './App.vue'
import { router } from './router'
import { getTimeBasedMode } from './stores/theme.store'
import '@mlightcad/cad-viewer/style.css'
import './styles/index.css'

document.documentElement.dataset.skin = 'classic'
document.documentElement.dataset.theme = getTimeBasedMode()

if (typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window) {
  document.documentElement.dataset.tauri = 'true'
}

// 完整版 MlCadViewer 暂未暴露 webworkerFileUrls，统一注入根路径资源，避免深层路由拼接出错误地址。
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

const app = createApp(App)

router.onError((error) => {
  console.error('路由导航失败', error)
})

app.use(createPinia())
app.use(i18n)
app.use(router)
app.mount('#app')
