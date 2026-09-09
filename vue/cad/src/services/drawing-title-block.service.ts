import { reactive } from 'vue'
import { getApiBaseUrl } from './api-base.service'
import { collectTitleSpaces, extractTitleFields } from '@/features/drawings/detail-tabs/preview/cad-title-block'
import { extractTitleBlockRecord, extractTitleBlockBatch, type TitlePayload, type TitleSnapshot } from './title-block-workflow'
export const savedTitleBlocks = reactive<Record<string, TitleSnapshot>>({})
const pending = new Map<string, Promise<TitleSnapshot>>()
function headers() {
  const token = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
  return { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }
}
async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, { ...init, headers: headers(), credentials: 'include', signal: AbortSignal.timeout(30000) })
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body.message || `图纸信息请求失败：HTTP ${response.status}`)
  return body.data ?? body
}
export async function loadTitleBlock(id: string): Promise<TitleSnapshot> {
  const result = await request<TitleSnapshot>(`/cad/title-blocks/${encodeURIComponent(id)}`)
  savedTitleBlocks[id] = result
  return result
}
async function parseTitleBlock(id: string): Promise<TitlePayload> {
  const [{ AcDbDatabase, AcDbFileType }, { registerCadConverters }, { getCadWorkerUrls }] = await Promise.all([
    import('@mlightcad/data-model'), import('./cad-converters.service'),
    import('@/features/drawings/detail-tabs/preview/cad-worker-assets'),
  ])
  await registerCadConverters(getCadWorkerUrls().dwgParser)
  const response = await fetch(`${getApiBaseUrl()}/cad/source?attachmentId=${encodeURIComponent(id)}`, { headers: headers(), credentials: 'include', signal: AbortSignal.timeout(120000) })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.message || `读取 CAD 文件失败：HTTP ${response.status}`)
  }
  const content = await response.arrayBuffer()
  const db = new AcDbDatabase()
  await db.read(content, { readOnly: true }, AcDbFileType.DWG)
  if (db.lastOpenError) throw new Error('CAD 数据解析失败')
  return { spaces: collectTitleSpaces(db).map(space => ({ id: space.id, name: space.name, textCount: space.texts.length, fields: extractTitleFields(space.texts), warnings: space.warnings })) }
}
export function extractAndSaveTitleBlock(id: string, force = false): Promise<TitleSnapshot> {
  const existing = pending.get(id)
  if (existing) return existing
  const task = extractTitleBlockRecord(id, force, {
    load: loadTitleBlock,
    parse: parseTitleBlock,
    save: (attachmentId, versionId, payload) => request(`/cad/title-blocks/${encodeURIComponent(attachmentId)}`, { method: 'PUT', body: JSON.stringify({ versionId, payload }) }),
  }).finally(() => pending.delete(id))
  pending.set(id, task)
  return task
}

export async function extractCreationTitleBlocks(files: Array<{ id: string; name: string }>, progress: (completed: number, total: number) => void): Promise<string[]> {
  return extractTitleBlockBatch(files, id => extractAndSaveTitleBlock(id), progress)
}

export function savedDesigner(ids: string[]): string {
  const values = ids.flatMap(id => savedTitleBlocks[id]?.payload?.spaces.flatMap(space => space.fields.filter(field => field.key === 'designer' && field.value).map(field => field.value)) ?? [])
  const unique = [...new Set(values)]
  return unique.length === 1 ? unique[0]! : ''
}
