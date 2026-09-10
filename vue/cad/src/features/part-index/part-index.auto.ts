import type { PartIndexItem, PartIndexPage } from '../../services/part-index.service'

export interface AutoIndexProgress { running: boolean; completed: number; total: number; failed: number }

// Collect before writing: extraction changes the pending/failed result sets and their pages.
export function createAutoPartIndexer(io: {
  list: (status: 'pending' | 'failed', page: number) => Promise<PartIndexPage>
  extract: (item: PartIndexItem) => Promise<unknown>
  stopped: () => boolean
  progress: (value: AutoIndexProgress) => void
  updated: (attachmentId: string) => void
  now?: () => number
}) {
  const attempts = new Map<string, { failures: number; next: number }>()
  let running: Promise<void> | null = null
  const now = io.now ?? Date.now
  async function process() {
    const queue = new Map<string, PartIndexItem>()
    for (const status of ['pending', 'failed'] as const) {
      for (let page = 1; !io.stopped(); page++) {
        const result = await io.list(status, page)
        if (io.stopped()) return
        for (const item of result.list) {
          if (!item.canWrite || !['pending', 'failed'].includes(item.extractionStatus)) continue
          const key = `${item.attachmentId}:${item.versionId}`
          if ((attempts.get(key)?.next ?? 0) <= now()) queue.set(item.attachmentId, item)
        }
        if (page * result.pageSize >= result.total) break
        if (!result.list.length || result.pageSize < 1) throw new Error('零件列表已变化')
      }
    }
    const progress = { running: true, completed: 0, total: queue.size, failed: 0 }
    if (queue.size) io.progress({ ...progress })
    try {
      for (const item of queue.values()) {
        if (io.stopped()) break
        const key = `${item.attachmentId}:${item.versionId}`
        try {
          await io.extract(item)
          // Avoid another read while a stale list response still contains this version.
          attempts.set(key, { failures: 0, next: now() + 30 * 60_000 })
        } catch {
          const failures = (attempts.get(key)?.failures ?? 0) + 1
          attempts.set(key, { failures, next: now() + Math.min(30, 5 ** (failures - 1)) * 60_000 })
          progress.failed++
        }
        if (io.stopped()) break
        progress.completed++
        io.updated(item.attachmentId)
        io.progress({ ...progress })
      }
    } finally {
      if (!io.stopped()) io.progress({ ...progress, running: false })
    }
  }
  return {
    run(): Promise<void> {
      if (!running) running = process().finally(() => { running = null })
      return running
    },
  }
}
