import { invoke, isTauri } from '@tauri-apps/api/core'
import { getApiBaseUrl } from '@/services/api-base.service'
import { readAccessToken } from '@/services/auth/access-token'

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
  const accessToken = readAccessToken()
  const apiBaseUrl = getApiBaseUrl()
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
  const accessToken = readAccessToken()
  const apiBaseUrl = getApiBaseUrl()
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
  const accessToken = readAccessToken()
  if (!accessToken) return
  const apiBaseUrl = getApiBaseUrl()
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

/** Rust 端约定：本机找不到 CAXA 且文件关联失败时，错误消息以此为前缀。 */
export const CAXA_NOT_FOUND_PREFIX = 'CAXA_NOT_FOUND:'

/** 弹出原生文件选择框，让用户手动指定 CAXA 程序（CDRAFT_M.exe）；取消返回空串。 */
export async function pickCaxaExecutable(): Promise<string> {
  return invoke<string>('pick_caxa_executable')
}

/** 保存用户手动指定的 CAXA 路径（持久化到 caxa-path.json，后续打开优先使用）。 */
export async function saveLocalCaxaPath(path: string): Promise<void> {
  await invoke('save_local_caxa_path', { path })
}

/** 读取已保存的手动 CAXA 路径（未设置时返回空串）。 */
export async function getLocalCaxaPath(): Promise<string> {
  return invoke<string>('get_local_caxa_path')
}

/** 打开 Windows「默认应用」设置页，便于用户为图纸扩展名配置打开方式。 */
export async function openDefaultAppsSettings(): Promise<void> {
  await invoke('open_default_apps_settings')
}

/** 弹出原生“另存为”对话框并直接保存二进制数据，返回保存路径；用户取消则返回 null。 */
export async function saveDownloadFile(defaultName: string, bytes: Uint8Array): Promise<string | null> {
  if (!isTauri()) return null
  return invoke<string | null>('save_download_file', {
    defaultName,
    bytes: Array.from(bytes),
  })
}
