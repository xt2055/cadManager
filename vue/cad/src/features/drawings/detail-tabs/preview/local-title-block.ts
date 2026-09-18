import { collectTitleSpaces, extractTitleFields } from './cad-title-block'
import { getCadWorkerUrls } from './cad-worker-assets'

/**
 * 读取**本地文件**的图幅（标题栏）。
 *
 * 图号的主来源是图幅，不是文件名：文件名可能忘改，中文文件名还会让后端 `resolvePartNo` 直接返回空。
 * 浏览器只能解析 DWG / DXF（libredwg / 原生 DXF 转换器），EXB 解析不了 ——
 * EXB 必须走上传 → 后端 CAXA 转 DWG → 再读（见 conversion-wait.ts 与 useDrawingFileUpload）。
 */

export type LocalTitleBlockResult =
  | { kind: 'ok'; partNo: string; material: string; candidates: string[] }
  | { kind: 'unsupported' }
  | { kind: 'not-found' }
  | { kind: 'error'; message: string }

/** 单文件解析上限：wasm worker 卡住时及时放弃，不拖死整批上传。 */
const PARSE_TIMEOUT_MS = 60_000

function extensionOf(name: string): string {
  const index = name.lastIndexOf('.')
  return index < 0 ? '' : name.slice(index).toLowerCase()
}

let registration: Promise<void> | null = null

/** 转换器是全局单例注册，多个文件连续解析只需注册一次。 */
function ensureConverters(): Promise<void> {
  if (!registration) {
    registration = import('@/services/cad-converters.service')
      .then(({ registerCadConverters }) => registerCadConverters(getCadWorkerUrls().dwgParser))
      .catch((error: unknown) => {
        registration = null
        throw error
      })
  }
  return registration
}

function withTimeout<T>(task: Promise<T>, timeoutMs: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = window.setTimeout(() => reject(new Error('读取图纸图幅超时')), timeoutMs)
    task.then(
      (value) => { window.clearTimeout(timer); resolve(value) },
      (error: unknown) => { window.clearTimeout(timer); reject(error) },
    )
  })
}

export function supportsLocalTitleBlock(fileName: string): boolean {
  const extension = extensionOf(fileName)
  return extension === '.dwg' || extension === '.dxf'
}

/**
 * 解析本地 DWG/DXF 并取出图幅图号。
 * EXB 直接回 `unsupported`（不读文件内容、不报错），由调用方改走"先转换再读"。
 */
export async function readLocalTitleBlock(file: File): Promise<LocalTitleBlockResult> {
  const extension = extensionOf(file.name)
  if (!supportsLocalTitleBlock(file.name)) return { kind: 'unsupported' }

  try {
    await ensureConverters()
    const [{ AcDbDatabase, AcDbFileType }, content] = await Promise.all([
      import('@mlightcad/data-model'),
      file.arrayBuffer(),
    ])
    const fileType = extension === '.dxf' ? AcDbFileType.DXF : AcDbFileType.DWG
    const database = new AcDbDatabase()
    await withTimeout((async () => {
      await database.read(content, { readOnly: true }, fileType)
      if (database.lastOpenError) throw new Error('CAD 数据解析失败')
    })(), PARSE_TIMEOUT_MS)

    const spaces = collectTitleSpaces(database)
    // 图号取所有空间里最高优先级的解析结果；多候选时原样带回，交给调用方让用户选。
    let partNo = ''
    const candidates = new Set<string>()
    let material = ''
    for (const space of spaces) {
      for (const field of extractTitleFields(space.texts)) {
        if (field.key === 'number') {
          if (field.value && !partNo) partNo = field.value
          for (const candidate of field.candidates) candidates.add(candidate)
        } else if (field.key === 'material' && field.value && !material) {
          material = field.value
        }
      }
    }

    const unique = [...candidates]
    if (partNo) return { kind: 'ok', partNo, material, candidates: unique }
    if (unique.length === 1) return { kind: 'ok', partNo: unique[0]!, material, candidates: unique }
    // 多候选且没有唯一值：图号留空，由调用方交给用户从 candidates 里选。
    if (unique.length > 1) return { kind: 'ok', partNo: '', material, candidates: unique }
    return { kind: 'not-found' }
  } catch (error: unknown) {
    return { kind: 'error', message: error instanceof Error ? error.message : '读取图纸图幅失败' }
  }
}
