import type { TitleField } from '../features/drawings/detail-tabs/preview/cad-title-block.ts'

export interface SavedTitleSpace { id: string; name: string; textCount: number; fields: TitleField[]; warnings?: string[] }
export interface TitlePayload { spaces: SavedTitleSpace[]; error?: string }
export interface StatusError extends Error { status?: number }
export interface TitleSnapshot {
  attachmentId: string
  versionId: string
  fileName: string
  canWrite: boolean
  hasPrevious: boolean
  payload: TitlePayload | null
  extractedAt: string
  snapshotRevision: number
}

function recordOf(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : null
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function strings(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : []
}

function normalizeTitleField(value: unknown): TitleField {
  const field = recordOf(value) ?? {}
  return {
    key: stringValue(field.key),
    label: stringValue(field.label),
    value: stringValue(field.value),
    source: stringValue(field.source),
    candidates: strings(field.candidates),
  }
}

function normalizeTitleSpace(value: unknown): SavedTitleSpace {
  const space = recordOf(value) ?? {}
  const textCount = typeof space.textCount === 'number' && Number.isSafeInteger(space.textCount) && space.textCount >= 0
    ? space.textCount
    : 0
  return {
    id: stringValue(space.id),
    name: stringValue(space.name),
    textCount,
    fields: Array.isArray(space.fields) ? space.fields.map(normalizeTitleField) : [],
    warnings: strings(space.warnings),
  }
}

export function normalizeTitlePayload(value: unknown): TitlePayload {
  const payload = recordOf(value) ?? {}
  return {
    spaces: Array.isArray(payload.spaces) ? payload.spaces.map(normalizeTitleSpace) : [],
    ...(typeof payload.error === 'string' && payload.error ? { error: payload.error } : {}),
  }
}

function normalizeTitleSnapshot(snapshot: TitleSnapshot): TitleSnapshot {
  return {
    ...snapshot,
    payload: snapshot.payload === null ? null : normalizeTitlePayload(snapshot.payload),
  }
}

function versionConflict(): StatusError {
  return Object.assign(new Error('文件版本已变化，请重新提取'), { status: 409 })
}

interface ExtractionIO {
  load(id: string): Promise<TitleSnapshot>
  parse(id: string, versionId: string): Promise<TitlePayload>
  save(id: string, versionId: string, payload: TitlePayload): Promise<unknown>
}
export async function extractTitleBlockRecord(id: string, force: boolean, io: ExtractionIO): Promise<TitleSnapshot> {
  const snapshot = normalizeTitleSnapshot(await io.load(id))
  if (!force && snapshot.payload && !snapshot.payload.error) return snapshot
  if (!snapshot.canWrite) throw new Error('没有保存此文件标题栏信息的权限')
  let payload: TitlePayload
  try { payload = normalizeTitlePayload(await io.parse(id, snapshot.versionId)) }
  catch (error) {
    if (typeof error === 'object' && error !== null && (error as StatusError).status === 409) throw error
    payload = { spaces: [], error: (error instanceof Error ? error.message : '提取失败').slice(0, 1000) }
  }
  await io.save(id, snapshot.versionId, payload)
  const result = normalizeTitleSnapshot(await io.load(id))
  if (result.versionId !== snapshot.versionId) throw versionConflict()
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
