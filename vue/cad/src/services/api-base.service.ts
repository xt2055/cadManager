import { invoke, isTauri } from '@tauri-apps/api/core'

const compiledDefault = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
let currentBaseUrl = compiledDefault

interface ApiConfigResult {
  apiBaseUrl?: string
  path?: string
}

function normalizeApiBaseUrl(input: string): string {
  let value = input.trim()
  if (!value) return ''
  if (!/^[a-z][a-z\d+.-]*:\/\//i.test(value)) value = `http://${value}`

  try {
    const url = new URL(value)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return ''
    const pathname = url.pathname.replace(/\/+$/, '')
    url.pathname = pathname === '' ? '/api' : pathname
    url.search = ''
    url.hash = ''
    return url.toString().replace(/\/$/, '')
  } catch {
    return ''
  }
}

export function getApiBaseUrl(): string {
  return currentBaseUrl
}

/**
 * 桌面客户端首次启动自动生成 api-config.json；之后以该文件中的地址为准。
 * 浏览器开发环境继续使用构建时的 VITE_API_BASE_URL。
 */
export async function initializeApiBaseUrl(): Promise<void> {
  if (!isTauri()) return

  try {
    const config = await invoke<ApiConfigResult>('ensure_api_config', {
      defaultApiBaseUrl: compiledDefault,
    })
    const configured = normalizeApiBaseUrl(config.apiBaseUrl || '')
    if (configured) {
      currentBaseUrl = configured
      console.info('[api] 使用配置文件中的后端地址', configured, config.path || '')
    } else {
      console.warn('[api] 配置文件中的后端地址无效，使用默认地址', compiledDefault, config.path || '')
    }
  } catch (error) {
    console.warn('[api] 读取 api-config.json 失败，使用默认地址', error)
  }
}
