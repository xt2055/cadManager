import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { getTimeBasedMode } from './stores/theme.store'
import './styles/index.css'

document.documentElement.dataset.skin = 'classic'
document.documentElement.dataset.theme = getTimeBasedMode()

if (typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window) {
  document.documentElement.dataset.tauri = 'true'
}

const app = createApp(App)

router.onError((error) => {
  console.error('路由导航失败', error)
})

app.use(createPinia())
app.use(router)
app.mount('#app')
