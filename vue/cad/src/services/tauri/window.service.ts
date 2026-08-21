import { isTauri } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import type { Window as TauriWindow } from '@tauri-apps/api/window'

function getAppWindow(): TauriWindow | null {
  if (!isTauri()) {
    return null
  }

  return getCurrentWindow()
}

function requireAppWindow(): TauriWindow {
  const appWindow = getAppWindow()
  if (!appWindow) {
    throw new Error('当前不是 Tauri 窗口，无法执行窗口控制操作')
  }
  return appWindow
}

export const windowService = {
  async minimize(): Promise<void> {
    await requireAppWindow().minimize()
  },

  async toggleMaximize(): Promise<boolean> {
    const appWindow = requireAppWindow()
    await appWindow.toggleMaximize()
    return appWindow.isMaximized()
  },

  async hide(): Promise<void> {
    await requireAppWindow().hide()
  },

  async close(): Promise<void> {
    await requireAppWindow().close()
  },

  async ensureNormal(): Promise<void> {
    const appWindow = requireAppWindow()
    if (await appWindow.isMaximized()) {
      await appWindow.unmaximize()
    }
  },

  async isMaximized(): Promise<boolean> {
    return requireAppWindow().isMaximized()
  },

  async isFullscreen(): Promise<boolean> {
    return requireAppWindow().isFullscreen()
  },

  async startDragging(): Promise<void> {
    await requireAppWindow().startDragging()
  },
}
