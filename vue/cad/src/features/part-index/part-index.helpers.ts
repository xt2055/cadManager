import type { PartIndexBackfillError, PartIndexFields, PartIndexItem, PartIndexPage, PartIndexStatus, PartIndexProject } from '../../services/part-index.service'
import type { SavedTitleSpace } from '../../services/title-block-workflow'

export const partIndexStatusLabels: Record<PartIndexStatus, string> = {
  pending: '待提取',
  failed: '提取失败',
  needs_confirmation: '需检查',
  recognized: '已识别',
  edited: '已人工修订',
  confirmed: '已人工确认',
  recheck: '换版待检查',
}

export function uniquePartIndexProjects(projects: PartIndexProject[] | null | undefined): PartIndexProject[] {
  return [...new Map((projects ?? []).map(project => [project.drawingId, project])).values()]
}

export function partIndexProjectLabel(project: PartIndexProject): string {
  return [...new Set([project.projectCode || project.drawingNo, project.projectName].map(value => value?.trim()).filter(Boolean))].join(' · ')
}

export function partIndexQueryString(filters: {
  page: number
  pageSize: number
  keyword?: string
  projectId?: string
  material?: string
  designer?: string
  dateFrom?: string
  dateTo?: string
  status?: string
}): string {
  const query = new URLSearchParams()
  const values: Record<string, string | number | undefined> = {
    page: filters.page,
    page_size: filters.pageSize,
    keyword: filters.keyword,
    project_id: filters.projectId,
    material: filters.material,
    designer: filters.designer,
    date_from: filters.dateFrom,
    date_to: filters.dateTo,
    status: filters.status,
  }
  Object.entries(values).forEach(([key, value]) => {
    if ((typeof value === 'string' || typeof value === 'number') && value !== '') query.set(key, String(value))
  })
  const text = query.toString()
  return text ? `?${text}` : ''
}

export function emptyPartIndexFields(): PartIndexFields {
  return {
    drawingNo: '', partName: '', material: '', designer: '', checker: '', approver: '',
    drawingDateRaw: '', scale: '', sheetSize: '', process: '', standard: '', company: '',
  }
}

export function fieldsFromTitleSpace(space: SavedTitleSpace, existingSheetSize = ''): PartIndexFields {
  const fields = emptyPartIndexFields()
  fields.sheetSize = existingSheetSize
  for (const field of space.fields) {
    const value = field.value.trim()
    if (field.key === 'number') fields.drawingNo = value
    if (field.key === 'name') fields.partName = value
    if (field.key === 'material') fields.material = value
    if (field.key === 'designer') fields.designer = value
    if (field.key === 'checker') fields.checker = value
    if (field.key === 'approver') fields.approver = value
    if (field.key === 'date') fields.drawingDateRaw = value
    if (field.key === 'scale') fields.scale = value
    if (field.key === 'process') fields.process = value
    if (field.key === 'standard') fields.standard = value
    if (field.key === 'company') fields.company = value
  }
  return fields
}

export function applyTitleCandidate(fields: PartIndexFields, sourceKey: string, candidate: string): PartIndexFields {
  const mapping: Partial<Record<string, keyof PartIndexFields>> = {
    number: 'drawingNo', name: 'partName', material: 'material', designer: 'designer',
    checker: 'checker', approver: 'approver', date: 'drawingDateRaw', scale: 'scale',
    process: 'process', standard: 'standard', company: 'company',
  }
  const target = mapping[sourceKey]
  return target ? { ...fields, [target]: candidate.trim() } : fields
}

export function canBatchExtract(item: PartIndexItem): boolean {
  return item.canWrite && (item.extractionStatus === 'pending' || item.extractionStatus === 'failed')
}

export function togglePartIndexPageSelection(items: PartIndexItem[], selected: string[]): string[] {
  const writable = items.filter((item) => item.canWrite).map((item) => item.attachmentId)
  return writable.every((id) => selected.includes(id)) ? [] : writable
}

export async function collectPendingPartIndexes(
  loadPage: (page: number) => Promise<PartIndexPage>,
  shouldStop: () => boolean,
  progress: (scanned: number, total: number) => void,
): Promise<{ items: PartIndexItem[], stopped: boolean }> {
  const items = new Map<string, PartIndexItem>()
  let page = 1
  let scanned = 0
  while (!shouldStop()) {
    const result = await loadPage(page)
    if (shouldStop()) return { items: [], stopped: true }
    for (const item of result.list) {
      if (canBatchExtract(item)) items.set(item.attachmentId, item)
    }
    scanned += result.list.length
    progress(scanned, result.total)
    if (page * result.pageSize >= result.total) return { items: [...items.values()], stopped: false }
    if (!result.list.length || result.pageSize < 1) throw new Error('图纸列表发生变化，请刷新后重新生成索引')
    page++
  }
  return { items: [], stopped: true }
}

export function isValidPartIndexDrawingDate(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim())
  if (!match) return false
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (year < 1 || year > 9999 || month < 1 || month > 12 || day < 1) return false
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0)
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]
  return day <= days[month - 1]!
}

export interface ExtractedTitleSnapshot {
  versionId: string
  snapshotRevision: number
}

export async function extractAndRebuildPartIndex(
  extract: () => Promise<ExtractedTitleSnapshot>,
  rebuild: (versionId: string, snapshotRevision: number) => Promise<unknown>,
): Promise<void> {
  const snapshot = await extract()
  if (snapshot.snapshotRevision < 1) throw new Error('标题栏快照尚未保存，无法重建零件索引')
  await rebuild(snapshot.versionId, snapshot.snapshotRevision)
}

export function appendPartIndexBackfillErrors(
  current: readonly PartIndexBackfillError[],
  incoming: readonly PartIndexBackfillError[],
  maximum = 100,
): { items: PartIndexBackfillError[], omitted: number } {
  const items = [...current]
  let omitted = 0
  const limit = Math.max(0, Math.floor(maximum))
  for (const error of incoming) {
    const attachmentId = typeof error?.attachmentId === 'string' ? error.attachmentId.trim() : ''
    const message = typeof error?.message === 'string' ? error.message.trim() : ''
    if (!attachmentId || !message) continue
    if (items.length >= limit) {
      omitted++
      continue
    }
    items.push({ attachmentId, message })
  }
  return { items, omitted }
}

export interface BatchRunResult {
  total: number
  completed: number
  succeeded: number
  currentFile: string
  failures: string[]
  failedItems: PartIndexItem[]
  stopped: boolean
}

export async function runSerialPartIndexBatch(
  items: PartIndexItem[],
  shouldStop: () => boolean,
  extract: (item: PartIndexItem) => Promise<unknown>,
  progress: (result: BatchRunResult) => void,
): Promise<BatchRunResult> {
  const queue = [...items]
  const result: BatchRunResult = { total: queue.length, completed: 0, succeeded: 0, currentFile: '', failures: [], failedItems: [], stopped: false }
  const report = () => progress({ ...result, failures: [...result.failures], failedItems: [...result.failedItems] })
  for (const item of queue) {
    if (shouldStop()) {
      result.stopped = true
      break
    }
    result.currentFile = item.fileName
    report()
    try {
      await extract(item)
      result.succeeded++
    } catch (error) {
      result.failures.push(`${item.fileName}：${error instanceof Error ? error.message : '读取失败'}`)
      result.failedItems.push(item)
    }
    result.completed++
    report()
  }
  result.currentFile = ''
  report()
  return result
}
