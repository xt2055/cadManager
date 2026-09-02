import { getApiBaseUrl } from '@/services/api-base.service'

// CAD 字体库基地址：优先使用 Tauri 内嵌资源（完全离线可用），
// 探测失败时回退到后端静态伺服地址。
// 注意：@mlightcad/cad-simple-viewer 期望 baseUrl 指向 cad-data 根目录
// （内部会自行拼接 fonts/），不要传入 fonts 目录本身。
const EMBEDDED_FONTS_ROOT = '/cad-data/'

let resolvedBase: string | null = null
let resolving: Promise<string> | null = null

export function cadFontsBaseUrl(): string {
  return resolvedBase ?? serverCadDataBaseUrl()
}

function serverCadDataBaseUrl(): string {
  const api = getApiBaseUrl()
  const root = api.replace(/\/api$/, '')
  return `${root}/cad-data/`
}

async function probeEmbedded(): Promise<boolean> {
  try {
    const resp = await fetch(EMBEDDED_FONTS_ROOT + 'fonts/fonts.json', { cache: 'no-store' })
    if (!resp.ok) return false
    const text = await resp.text()
    const trimmed = text.trimStart()
    // fonts.json 实际为 JSON 数组（[{file, name: [...]}]），对象/数组均可接受
    return (trimmed.startsWith('{') || trimmed.startsWith('[')) && trimmed.includes('"')
  } catch {
    return false
  }
}

export async function resolveCadFontsBaseUrl(): Promise<string> {
  if (resolvedBase) return resolvedBase
  if (!resolving) {
    resolving = probeEmbedded()
      .then((ok) => {
        resolvedBase = ok ? EMBEDDED_FONTS_ROOT : serverCadDataBaseUrl()
        console.log(`[cad-fonts] using ${ok ? 'embedded' : 'server'} fonts root: ${resolvedBase}`)
        return resolvedBase
      })
      .catch(() => {
        resolvedBase = serverCadDataBaseUrl()
        console.log(`[cad-fonts] probe failed, fallback to server fonts root: ${resolvedBase}`)
        return resolvedBase
      })
  }
  return resolving
}
