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
