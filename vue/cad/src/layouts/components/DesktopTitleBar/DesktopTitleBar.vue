<script setup lang="ts">
import { onMounted } from 'vue'
import NotificationButton from './NotificationButton.vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useThemeStore } from '@/stores/theme.store'
import { useWindowStore } from '@/stores/window.store'
import { windowService } from '@/services/tauri/window.service'
import { useUiStore } from '@/stores/ui.store'

defineOptions({
  name: 'DesktopTitleBar',
})

const themeStore = useThemeStore()
const windowStore = useWindowStore()
const uiStore = useUiStore()

async function runWindowAction(action: () => Promise<void>, message: string) {
  try {
    await action()
  } catch (error) {
    console.error(message, error)
    uiStore.toast(`${message}：当前不是 Tauri 原生窗口或窗口权限未开启`, 'warn')
  }
}

async function minimize() {
  await runWindowAction(() => windowService.minimize(), '最小化窗口失败')
}

async function toggleMaximize() {
  try {
    windowStore.setMaximized(await windowService.toggleMaximize())
  } catch (error) {
    console.error('切换窗口最大化失败', error)
    uiStore.toast('切换窗口最大化失败：当前不是 Tauri 原生窗口或窗口权限未开启', 'warn')
  }
}

async function hideWindow() {
  await runWindowAction(() => windowService.hide(), '隐藏窗口失败')
}

async function closeWindow() {
  await runWindowAction(() => windowService.close(), '关闭窗口失败')
}

onMounted(async () => {
  try {
    await windowService.ensureNormal()
    windowStore.setMaximized(false)
  } catch {
    // 浏览器预览模式没有 Tauri 窗口，保留默认状态即可。
  }
})
</script>

<template>
  <header id="titlebar" data-tauri-drag-region>
    <div class="tb-logo">
      <svg class="mark" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <rect x="2.5" y="2.5" width="19" height="19" rx="5" stroke="var(--accent)" stroke-width="2" />
        <path d="M7 16 L12 7 L17 16" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
        <circle cx="12" cy="12" r="1.6" fill="var(--accent-2)" />
      </svg>
      图枢 <span class="sub">CAD·PDM</span>
    </div>

    <div class="tb-right">
      <button
        id="themeBtn"
        class="icon-btn"
        type="button"
        :title="themeStore.mode === 'dark' ? '切换为浅色模式' : '切换为深色模式'"
        @click="themeStore.toggleMode()"
      >
        <DemoIcon :name="themeStore.mode === 'dark' ? 'sun' : 'moon'" :size="16" />
      </button>

      <NotificationButton />

      <div class="win-btns">
        <button class="win-btn" type="button" title="最小化" @click="minimize">
          <DemoIcon name="minus" :size="15" />
        </button>
        <button class="win-btn" type="button" title="隐藏窗口" @click="hideWindow">
          <DemoIcon name="eye-off" :size="15" />
        </button>
        <button class="win-btn" type="button" title="最大化 / 还原" @click="toggleMaximize">
          <DemoIcon :name="windowStore.state.maximized ? 'copy' : 'square'" :size="15" />
        </button>
        <button class="win-btn close" type="button" title="关闭" @click="closeWindow">
          <DemoIcon name="x" :size="15" />
        </button>
      </div>
    </div>
  </header>
</template>
