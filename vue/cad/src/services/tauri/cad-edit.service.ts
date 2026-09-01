import { invoke, isTauri } from '@tauri-apps/api/core'

export interface NativeEditOpenPayload {
  sessionId: string
  openUrl: string
  expiresAt: string
}

export async function openCadEditSession(payload: NativeEditOpenPayload): Promise<void> {
  if (!isTauri()) {
    await navigator.clipboard.writeText(payload.openUrl)
    throw new Error('当前不是桌面客户端，已复制打开链接，请在图枢客户端中使用')
  }
  const accessToken = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
  const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
  await invoke('open_cad_edit_session', {
    apiBaseUrl,
    accessToken,
    openUrl: payload.openUrl,
  })
}

export interface ReadonlyOpenPayload {
  storageKey: string
}

/** 本地只读查看：下载临时副本到本机，CAXA 打开，退出后自动销毁，不回传不产生版本。 */
export async function openCadReadonly(payload: ReadonlyOpenPayload): Promise<void> {
  if (!isTauri()) {
    throw new Error('本地查看需要在图枢桌面客户端中使用')
  }
  const accessToken = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
  const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
  await invoke('open_cad_readonly', {
    apiBaseUrl,
    accessToken,
    storageKey: payload.storageKey,
  })
}
