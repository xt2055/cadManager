import type { TitleField } from '../features/drawings/detail-tabs/preview/cad-title-block.ts'

export interface SavedTitleSpace { id: string; name: string; textCount: number; fields: TitleField[]; warnings?: string[] }
export interface TitlePayload { spaces: SavedTitleSpace[]; error?: string }
export interface TitleSnapshot {
  attachmentId: string
  versionId: string
  fileName: string
  canWrite: boolean
  hasPrevious: boolean
  payload: TitlePayload | null
  extractedAt: string
}
interface ExtractionIO {
  load(id: string): Promise<TitleSnapshot>
  parse(id: string): Promise<TitlePayload>
  save(id: string, versionId: string, payload: TitlePayload): Promise<unknown>
}
export async function extractTitleBlockRecord(id: string, force: boolean, io: ExtractionIO): Promise<TitleSnapshot> {
  const snapshot = await io.load(id)
  if (!force && snapshot.payload && !snapshot.payload.error) return snapshot
  if (!snapshot.canWrite) throw new Error('没有保存此文件标题栏信息的权限')
  let payload: TitlePayload
  try { payload = await io.parse(id) }
  catch (error) { payload = { spaces: [], error: (error instanceof Error ? error.message : '提取失败').slice(0, 1000) } }
  await io.save(id, snapshot.versionId, payload)
  const result = await io.load(id)
  if (payload.error) throw new Error(payload.error)
  return result
}
export async function extractTitleBlockBatch(files: Array<{ id: string; name: string }>, extract: (id: string) => Promise<unknown>, progress: (completed: number, total: number) => void): Promise<string[]> {
  const unique = files.filter((file, index, all) => /\.(dwg|dxf|exb)$/i.test(file.name) && all.findIndex(item => item.id === file.id) === index)
  const failures: string[] = []
  for (const [index, file] of unique.entries()) {
    progress(index, unique.length)
    try { await extract(file.id) }
    catch (error) { failures.push(`${file.name}：${error instanceof Error ? error.message : '提取或保存失败'}`) }
  }
  progress(unique.length, unique.length)
  return failures
}
