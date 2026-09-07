import { getApiBaseUrl } from '@/services/api-base.service'

// CAD 字体库基地址：优先使用 Tauri 内嵌资源（完全离线可用），
// 探测失败时回退到后端静态伺服地址。
// 注意：@mlightcad/cad-simple-viewer 期望 baseUrl 指向 cad-data 根目录
// （内部会自行拼接 fonts/），不要传入 fonts 目录本身。
const EMBEDDED_FONTS_ROOT = '/cad-data/'

// CAXA/AutoCAD 图纸常把形位公差写在独立的 SHX 字体中。
// 显式预加载这些字体，避免首帧绘制时暂时使用 simplex 后不再重绘。
export const CAD_SYMBOL_FONT_NAMES = ['gdt', 'AIGDT', 'amgdt', 'amgdtans', 'whgdtxt', 'gbgdt', 'CXGDT'] as const

type CadFontManagerForDiagnostics = {
  baseUrl?: string
  avaiableFonts?: Array<{ file?: string; type?: string; name?: string[] }>
  loadFonts?: (fonts: string[]) => Promise<void>
  regen?: () => void
  curDocument?: { database?: any }
}

let removeFontDiagnostics: (() => void) | null = null

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

/**
 * 预加载本地 CAD 符号字体。
 * manager 存在时走 MLightCAD 的字体加载器，以便同时覆盖它的绘图线程；
 * manager 不存在时先配置全局字体管理器，供在线编辑器创建实例时继承。
 */
export async function preloadCadSymbolFonts(manager?: { loadFonts?: (fonts: string[]) => Promise<unknown> }) {
  const baseUrl = await resolveCadFontsBaseUrl()
  const { FontManager } = await import('@mlightcad/mtext-renderer')
  const fontManager = FontManager.instance
  fontManager.baseUrl = `${baseUrl.replace(/\/$/, '')}/fonts/`
  fontManager.setSymbolFonts([...CAD_SYMBOL_FONT_NAMES])

  let statuses: unknown = []
  try {
    // 分开等待 SHX 与 mesh，避免查看器将 AIGDT 的加载延后到打开图纸之后。
    if (manager?.loadFonts) {
      await manager.loadFonts(['gdt', 'amgdt', 'amgdtans', 'whgdtxt', 'CXGDT'])
      await manager.loadFonts(['AIGDT', 'gbgdt'])
    }
    statuses = await fontManager.loadFontsByNames(CAD_SYMBOL_FONT_NAMES)
  } catch (error) {
    // 字体缺失不能阻断整张图纸打开；调用方会继续使用 CAD 默认字体。
    console.warn('[CAD] 形位公差字体预加载失败', error)
  }

  const statusSummary = Array.isArray(statuses)
    ? statuses.map((item: any) => ({
      fontName: item?.fontName,
      status: item?.status,
      url: item?.url,
    }))
    : statuses
  console.info('[CAD] 形位公差字体已预加载', JSON.stringify({
    baseUrl,
    requested: [...CAD_SYMBOL_FONT_NAMES],
    statuses: statusSummary,
  }))
  return statuses
}

function summarizeAvailableFonts(manager: CadFontManagerForDiagnostics) {
  return (manager.avaiableFonts ?? []).map((font) => ({
    file: font.file,
    type: font.type,
    names: font.name,
  }))
}

function inspectCadEntities(database: any) {
  const typeCounts: Record<string, number> = {}
  const candidates: Array<{
    type: string
    layer?: string
    style?: string
    textLength: number
    firstCodePoint?: number
    hasGdtControl: boolean
    hasPercentControl: boolean
  }> = []
  let blockCount = 0
  let entityCount = 0

  for (const block of database?.tables?.blockTable?.newIterator?.(true) ?? []) {
    blockCount++
    for (const entity of block.newIterator?.() ?? []) {
      entityCount++
      const type = String(entity?.dxfTypeName || entity?.type || 'UNKNOWN').toUpperCase()
      typeCounts[type] = (typeCounts[type] ?? 0) + 1

      const rawText = [entity?.text, entity?.contents, entity?.textString]
        .find((value) => typeof value === 'string') as string | undefined
      const text = rawText ?? ''
      const layer = String(entity?.layer ?? entity?.layerName ?? '')
      const style = String(entity?.textStyleName ?? entity?.styleName ?? entity?.textStyle?.name ?? '')
      const isTextCandidate = ['TEXT', 'MTEXT', 'ATTRIB', 'ATTDEF', 'TOLERANCE'].includes(type)
        || /gdt|tol|公差|形位|dim/i.test(`${layer} ${style}`)
        || /\\F[gG][dD][tT]|%%/.test(text)
        || /^[A-Za-z]$/.test(text.trim())

      if (isTextCandidate) {
        candidates.push({
          type,
          ...(layer ? { layer } : {}),
          ...(style ? { style } : {}),
          textLength: text.length,
          ...(text ? { firstCodePoint: text.codePointAt(0) } : {}),
          hasGdtControl: /\\F[gG][dD][tT]/.test(text),
          hasPercentControl: /%%/.test(text),
        })
      }
    }
  }

  return { blockCount, entityCount, typeCounts, candidates }
}

/**
 * 记录 LibreDWG 解析完成、尚未进入 AcDbEntityConverter 前的原始实体类型。
 * 只输出类型、样式和字符编码，不输出图纸文字内容。
 */
export function logRawCadDwgModel(result: any) {
  // parse() 返回 AcDbParsingTaskResult，图纸数据库在 result.model 中。
  const model = result?.model
  if (!model?.tables) {
    console.warn('[CAD][DWG原始实体诊断] 解析结果缺少 model.tables')
    return
  }
  const typeCounts: Record<string, number> = {}
  const candidates: Array<{
    blockIndex: number
    entityIndex: number
    type: string
    layer?: string
    style?: string
    originalDxfName?: string
    textLength: number
    codePoints: number[]
    hasGdtControl: boolean
    hasPercentControl: boolean
  }> = []
  const blocks = Array.isArray(model?.tables?.BLOCK_RECORD?.entries)
    ? model.tables.BLOCK_RECORD.entries
    : []
  const modelEntities = Array.isArray(model?.entities) ? model.entities : []
  let entityCount = 0
  const inlineFonts = new Set<string>()
  const symbolRuns: Array<{ style: string; fonts: string[]; code: number }> = []

  const inspectEntity = (entity: any, blockIndex: number, entityIndex: number) => {
    entityCount++
    const type = String(entity?.type || entity?.dxfTypeName || 'UNKNOWN').toUpperCase()
    typeCounts[type] = (typeCounts[type] ?? 0) + 1
    const rawText = [entity?.text, entity?.contents, entity?.textString]
      .find((value) => typeof value === 'string') as string | undefined
    const text = rawText ?? ''
    const layer = String(entity?.layer ?? '')
    const style = String(entity?.styleName ?? entity?.textStyleName ?? '')
    const fonts = Array.from(text.matchAll(/\\[fF]([^;|{}]+)(?:\|[^;{}]*)?;/g), (match) => match[1]!)
    fonts.forEach((font) => inlineFonts.add(font))
    // 仅记录单个 ASCII 字母的编码；不要输出可还原图纸正文的编码序列。
    const plain = text.replace(/\\[A-Za-z][^;{}]*;/g, '').replace(/[{}]/g, '').trim()
    if (/^[A-Za-z]$/.test(plain)) {
      symbolRuns.push({ style, fonts, code: plain.charCodeAt(0) })
    }
    const isCandidate = type === 'TOLERANCE'
      || type === 'ACAD_PROXY_ENTITY'
      || /gdt|tol|公差|形位|dim/i.test(`${layer} ${style}`)
      || /\\F[gG][dD][tT]|%%/.test(text)
      || /^[A-Za-z]$/.test(text.trim())

    if (isCandidate) {
      candidates.push({
        blockIndex,
        entityIndex,
        type,
        ...(layer ? { layer } : {}),
        ...(style ? { style } : {}),
        ...(entity?.originalDxfName ? { originalDxfName: String(entity.originalDxfName) } : {}),
        textLength: text.length,
        codePoints: /^[A-Za-z]$/.test(text.trim()) ? [text.trim().charCodeAt(0)] : [],
        hasGdtControl: /\\F[gG][dD][tT]/.test(text),
        hasPercentControl: /%%/.test(text),
      })
    }
  }

  blocks.forEach((block: any, blockIndex: number) => {
    for (const [entityIndex, entity] of (block?.entities ?? []).entries()) {
      inspectEntity(entity, blockIndex, entityIndex)
    }
  })

  // model.entities 是块表中实体的副本，不能再次计数。

  const report = {
    blockCount: blocks.length,
    modelSpaceEntityCount: modelEntities.length,
    entityCount,
    typeCounts,
    candidates,
  }
  console.info('[CAD][字体引用]', JSON.stringify({
    typeCounts,
    styles: (model.tables.STYLE?.entries ?? []).map((style: any) => ({
      name: style.name, font: style.font, bigFont: style.bigFont, extendedFont: style.extendedFont,
    })),
    inlineFonts: [...inlineFonts],
    symbolRuns,
  }))
  return report
}

/**
 * 开发环境字体诊断：
 * - 暴露 window.cadDocManager，便于在 F12 中直接调用 loadFonts；
 * - 监听 MLightCAD 的字体缺失/加载失败事件；
 * - 日志只包含字体文件和 URL，不包含图纸文字内容。
 */
export async function installCadFontDiagnostics(manager: CadFontManagerForDiagnostics) {
  if (!import.meta.env.DEV || typeof window === 'undefined') return

  removeFontDiagnostics?.()
  const [{ eventBus }, { FontManager }] = await Promise.all([
    import('@mlightcad/cad-simple-viewer'),
    import('@mlightcad/mtext-renderer'),
  ])
  const fontManager = FontManager.instance

  const readFontState = () => ({
    loaded: [...CAD_SYMBOL_FONT_NAMES].map((fontName) => ({
      fontName,
      isLoaded: fontManager.isFontLoaded(fontName),
      type: fontManager.getFontType(fontName),
    })),
    missedFonts: { ...fontManager.missedFonts },
    unsupportedChars: Object.fromEntries(
      Object.entries(fontManager.unsupportedChars).map(([char, count]) => [
        `U+${char.codePointAt(0)?.toString(16).toUpperCase().padStart(4, '0') ?? '????'}`,
        count,
      ]),
    ),
  })

  const onFontsNotFound = ({ fonts }: { fonts: string[] }) => {
    console.warn('[CAD][字体诊断] fonts-not-found', JSON.stringify({ fonts }))
  }
  const onFontsNotLoaded = ({ fonts }: { fonts: Array<{ fontName: string; url: string }> }) => {
    console.error('[CAD][字体诊断] fonts-not-loaded', JSON.stringify({ fonts }))
  }
  const onFontNotFound = ({ fontName, count }: { fontName: string; count: number }) => {
    console.warn('[CAD][字体诊断] font-not-found', JSON.stringify({ fontName, count }))
  }
  const onAvailableFontsFailed = ({ url }: { url: string }) => {
    console.error('[CAD][字体诊断] failed-to-get-avaiable-fonts', JSON.stringify({ url }))
  }

  eventBus.on('fonts-not-found', onFontsNotFound)
  eventBus.on('fonts-not-loaded', onFontsNotLoaded)
  eventBus.on('font-not-found', onFontNotFound)
  eventBus.on('failed-to-get-avaiable-fonts', onAvailableFontsFailed)

  const loadGdtFonts = async () => {
    if (!manager.loadFonts) throw new Error('当前 CAD 管理器没有 loadFonts 方法')
    await manager.loadFonts([...CAD_SYMBOL_FONT_NAMES])
    const result = {
      baseUrl: manager.baseUrl,
      requested: [...CAD_SYMBOL_FONT_NAMES],
      available: summarizeAvailableFonts(manager),
      ...readFontState(),
    }
    console.info('[CAD][字体诊断] GDT 字体加载完成', JSON.stringify(result))
    manager.regen?.()
    return result
  }

  const inspectDrawingEntities = () => {
    const report = inspectCadEntities(manager.curDocument?.database)
    console.info('[CAD][实体诊断]', JSON.stringify(report))
    return report
  }

  window.cadDocManager = manager
  window.cadFontDiagnostics = {
    loadGdtFonts,
    inspectDrawingEntities,
    inspect: () => ({
      baseUrl: manager.baseUrl,
      requested: [...CAD_SYMBOL_FONT_NAMES],
      available: summarizeAvailableFonts(manager),
      ...readFontState(),
    }),
  }

  console.info('[CAD][字体诊断] 已启用', JSON.stringify({
    baseUrl: manager.baseUrl,
    requested: [...CAD_SYMBOL_FONT_NAMES],
    available: summarizeAvailableFonts(manager),
    consoleHint: 'await window.cadFontDiagnostics.loadGdtFonts(); window.cadFontDiagnostics.inspectDrawingEntities()',
  }))

  removeFontDiagnostics = () => {
    eventBus.off('fonts-not-found', onFontsNotFound)
    eventBus.off('fonts-not-loaded', onFontsNotLoaded)
    eventBus.off('font-not-found', onFontNotFound)
    eventBus.off('failed-to-get-avaiable-fonts', onAvailableFontsFailed)
    if (window.cadDocManager === manager) delete window.cadDocManager
    delete window.cadFontDiagnostics
    removeFontDiagnostics = null
  }
}

function wrapGdtSymbolCell(cell: string): string {
  const match = cell.match(/^(\s*)([A-Za-z])(\s*)$/)
  if (!match) return cell
  return `${match[1]}{\\Fgdt.shx|b0|i0|c134|p6;${match[2]}}${match[3]}`
}

/**
 * CAXA 导出的 TOLERANCE 有时把 GDT 第一格写成裸字母（例如 r），
 * 而不是 AutoCAD 常见的 `\\Fgdt.shx...;r`。MLightCAD 只能按普通文字
 * 绘制前一种数据，因此在进入渲染器前补齐字体控制码。
 */
export function normalizeCadToleranceEntities(database: any): number {
  let normalizedCount = 0
  for (const block of database?.tables?.blockTable?.newIterator?.(true) ?? []) {
    for (const entity of block.newIterator?.() ?? []) {
      const type = String(entity?.dxfTypeName || entity?.type || '').toUpperCase()
      if (type !== 'TOLERANCE' || typeof entity.text !== 'string' || /\\F[gG][dD][tT]/.test(entity.text)) continue

      const original = entity.text
      const parts = original.split(/(\\P|\r\n|\r|\n)/i)
      for (let index = 0; index < parts.length; index += 2) {
        const row = parts[index] || ''
        const separatorIndex = row.indexOf('|')
        const firstCell = separatorIndex >= 0 ? row.slice(0, separatorIndex) : row
        const wrapped = wrapGdtSymbolCell(firstCell)
        parts[index] = separatorIndex >= 0
          ? `${wrapped}${row.slice(separatorIndex)}`
          : wrapped
      }
      entity.text = parts.join('')
      if (entity.text !== original) normalizedCount++
    }
  }
  if (normalizedCount > 0) {
    console.info('[CAD] 已补齐裸形位公差符号字体', { normalizedCount })
  }
  return normalizedCount
}
