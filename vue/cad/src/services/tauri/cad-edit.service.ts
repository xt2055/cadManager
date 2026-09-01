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

interface SmbAccessInfo {
  host: string
  share: string
  uncRoot: string
  username: string
  password: string
}

/**
 * 自动写入 SMB 访问凭据：从后端拉取访问账号，写入 Windows 凭据管理器（cmdkey），
 * 之后本机访问 \\host\share（资源管理器/CAXA 打开文件）不再弹凭据框。
 * 失败仅告警，不阻塞业务。
 */
export async function ensureSmbCredential(): Promise<void> {
  if (!isTauri()) return
  const accessToken = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
  if (!accessToken) return
  const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
  const response = await fetch(`${apiBaseUrl}/system/smb-access`, {
    headers: { Accept: 'application/json', Authorization: `Bearer ${accessToken}` },
    credentials: 'include',
  })
  if (!response.ok) return
  const body = (await response.json().catch(() => null)) as { data?: SmbAccessInfo } | null
  const info = body?.data
  if (!info?.host || !info?.username || !info?.password) return
  await invoke('ensure_smb_credential', {
    host: info.host,
    username: info.username,
    password: info.password,
  })
}
