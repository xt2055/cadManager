import { appConfig } from '@/app/app.config'
import { getUpdateBaseUrl } from '@/services/server-address.service'

export interface UpdateCheckResult {
  currentVersion: string
  latestVersion: string
  updateAvailable: boolean
  notes: string
  publishedAt?: string
  downloadUrl?: string
  mandatory: boolean
  platform?: string
  /** 本次检查使用的服务器基地址（用于解析相对下载地址） */
  sourceBaseUrl: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export async function checkForUpdates(serverBaseUrl?: string): Promise<UpdateCheckResult> {
  const baseUrl = (serverBaseUrl?.trim() || getUpdateBaseUrl()).replace(/\/$/, '')
  const query = new URLSearchParams({
    current_version: appConfig.version,
    platform: 'windows-x86_64',
  })
  const response = await fetch(`${baseUrl}/updates/latest?${query.toString()}`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    credentials: 'omit',
  })
  if (!response.ok) {
    throw new Error(`更新服务请求失败：HTTP ${response.status}`)
  }

  const body: unknown = await response.json()
  const payload = isRecord(body) && typeof body.code === 'number' && 'data' in body ? body.data : body
  if (isRecord(body) && typeof body.code === 'number' && body.code !== 0 && body.code !== 200) {
    throw new Error(typeof body.message === 'string' ? body.message : '更新服务返回失败')
  }
  if (!isRecord(payload) || typeof payload.latestVersion !== 'string' || typeof payload.updateAvailable !== 'boolean') {
    throw new Error('更新服务返回格式无效')
  }

  return {
    currentVersion: typeof payload.currentVersion === 'string' ? payload.currentVersion : appConfig.version,
    latestVersion: payload.latestVersion,
    updateAvailable: payload.updateAvailable,
    notes: typeof payload.notes === 'string' ? payload.notes : '暂无更新说明',
    ...(typeof payload.publishedAt === 'string' ? { publishedAt: payload.publishedAt } : {}),
    ...(typeof payload.downloadUrl === 'string' ? { downloadUrl: payload.downloadUrl } : {}),
    mandatory: payload.mandatory === true,
    ...(typeof payload.platform === 'string' ? { platform: payload.platform } : {}),
    sourceBaseUrl: baseUrl,
  }
}

/** 解析更新包下载地址：完整 URL 直接用，相对路径拼本次检查的服务器地址 */
export function resolveUpdateDownloadUrl(result: UpdateCheckResult): string {
  if (!result.downloadUrl) return ''
  if (/^https?:\/\//i.test(result.downloadUrl)) return result.downloadUrl
  return `${result.sourceBaseUrl}${result.downloadUrl}`
}
