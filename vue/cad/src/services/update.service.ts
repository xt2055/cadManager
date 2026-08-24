import { appConfig } from '@/app/app.config'

export interface UpdateCheckResult {
  currentVersion: string
  latestVersion: string
  updateAvailable: boolean
  notes: string
  publishedAt?: string
  downloadUrl?: string
  mandatory: boolean
  platform?: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export async function checkForUpdates(): Promise<UpdateCheckResult> {
  const baseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
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
  }
}
