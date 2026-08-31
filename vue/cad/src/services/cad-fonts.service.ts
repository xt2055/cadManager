// CAD 字体库基地址：优先使用 Tauri 内嵌资源（完全离线可用），
// 探测失败时回退到后端静态伺服地址（/cad-data/fonts/）。
const EMBEDDED_FONTS_BASE = '/cad-data/fonts/'

let resolvedBase: string | null = null
let resolving: Promise<string> | null = null

export function cadFontsBaseUrl(): string {
  return resolvedBase ?? serverFontsBaseUrl()
}

function serverFontsBaseUrl(): string {
  const api = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
  const root = api.replace(/\/api$/, '')
  return `${root}/cad-data/fonts/`
}

async function probeEmbedded(): Promise<boolean> {
  try {
    const resp = await fetch(EMBEDDED_FONTS_BASE + 'fonts.json', { cache: 'no-store' })
    if (!resp.ok) return false
    const text = await resp.text()
    const trimmed = text.trimStart()
    return trimmed.startsWith('{') && trimmed.includes('"')
  } catch {
    return false
  }
}

export async function resolveCadFontsBaseUrl(): Promise<string> {
  if (resolvedBase) return resolvedBase
  if (!resolving) {
    resolving = probeEmbedded()
      .then((ok) => {
        resolvedBase = ok ? EMBEDDED_FONTS_BASE : serverFontsBaseUrl()
        console.log(`[cad-fonts] using ${ok ? 'embedded' : 'server'} fonts: ${resolvedBase}`)
        return resolvedBase
      })
      .catch(() => {
        resolvedBase = serverFontsBaseUrl()
        console.log(`[cad-fonts] probe failed, fallback to server fonts: ${resolvedBase}`)
        return resolvedBase
      })
  }
  return resolving
}
