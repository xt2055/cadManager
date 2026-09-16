import { isTauri } from '@tauri-apps/api/core'
import { currentMonitor, getCurrentWindow, LogicalSize } from '@tauri-apps/api/window'
import type { Window as TauriWindow } from '@tauri-apps/api/window'

interface AdaptiveWindowSize {
  width: number
  height: number
  minWidth: number
  minHeight: number
}

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

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, Math.floor(value)))
}

async function logicalWorkArea(): Promise<{ width: number; height: number }> {
  if (!isTauri()) {
    return {
      width: typeof window === 'undefined' ? 1440 : window.innerWidth,
      height: typeof window === 'undefined' ? 900 : window.innerHeight,
    }
  }

  const monitor = await currentMonitor()
  if (monitor) {
    const workArea = monitor.workArea.size.toLogical(monitor.scaleFactor)
    return { width: workArea.width, height: workArea.height }
  }

  const appWindow = requireAppWindow()
  const scaleFactor = await appWindow.scaleFactor()
  const size = (await appWindow.innerSize()).toLogical(scaleFactor)
  return { width: size.width, height: size.height }
}

async function adaptiveSize(kind: 'login' | 'workspace'): Promise<AdaptiveWindowSize> {
  const workArea = await logicalWorkArea()
  if (kind === 'login') {
    const availableWidth = Math.max(300, workArea.width - 32)
    const availableHeight = Math.max(440, workArea.height - 32)
    const width = clamp(Math.min(440, availableWidth), 300, 440)
    const height = clamp(Math.min(600, availableHeight), 440, 600)
    return {
      width,
      height,
      minWidth: Math.min(360, width),
      minHeight: Math.min(520, height),
    }
  }

  const availableWidth = Math.max(360, workArea.width - 32)
  const availableHeight = Math.max(440, workArea.height - 32)
  const width = clamp(Math.min(1440, availableWidth), 360, 1440)
  const height = clamp(Math.min(900, availableHeight), 440, 900)
  return {
    width,
    height,
    minWidth: Math.min(640, width),
    minHeight: Math.min(480, height),
  }
}

export const windowService = {
  async guardClose(canClose: () => Promise<boolean>): Promise<() => void> {
    const appWindow = getAppWindow()
    if (!appWindow) return () => undefined
    let approved = false
    return appWindow.onCloseRequested(async event => {
      if (approved) return
      event.preventDefault()
      if (await canClose()) {
        approved = true
        try { await appWindow.close() }
        catch (error) { approved = false; console.error('关闭窗口失败', error) }
      }
    })
  },
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

  async setSize(width: number, height: number): Promise<void> {
    const appWindow = getAppWindow()
    if (!appWindow) return
    await appWindow.setSize(new LogicalSize(width, height))
  },

  async setSizeConstraints(minWidth: number, minHeight: number): Promise<void> {
    const appWindow = getAppWindow()
    if (!appWindow) return
    await appWindow.setSizeConstraints({ minWidth, minHeight })
  },

  async setLoginWindowSize(): Promise<void> {
    const size = await adaptiveSize('login')
    await this.setSizeConstraints(size.minWidth, size.minHeight)
    await this.setResizable(false)
    await this.setSize(size.width, size.height)
    await this.center()
  },

  async setWorkspaceWindowSize(): Promise<void> {
    const size = await adaptiveSize('workspace')
    await this.setSizeConstraints(size.minWidth, size.minHeight)
    await this.setResizable(true)
    await this.setSize(size.width, size.height)
    await this.center()
  },

  async setResizable(value: boolean): Promise<void> {
    const appWindow = getAppWindow()
    if (!appWindow) return
    await appWindow.setResizable(value)
  },

  async center(): Promise<void> {
    const appWindow = getAppWindow()
    if (!appWindow) return
    await appWindow.center()
  },
}
