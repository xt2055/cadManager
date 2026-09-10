import { reactive } from 'vue'
import { createAutoPartIndexer } from '@/features/part-index/part-index.auto'
import { extractAndSaveTitleBlock } from './drawing-title-block.service'
import { partIndexService } from './part-index.service'

export const automaticPartIndex = reactive({ running: false, completed: 0, total: 0, failed: 0, revision: 0, attachmentId: '' })

export function startAutomaticPartIndex(): () => void {
  let stopped = false
  let timer: ReturnType<typeof setTimeout> | undefined
  const controller = new AbortController()
  const worker = createAutoPartIndexer({
    list: (status, page) => partIndexService.list({ status, page, pageSize: 100 }, controller.signal),
    stopped: () => stopped,
    extract: async (item) => {
      const snapshot = await extractAndSaveTitleBlock(item.attachmentId, item.extractionStatus === 'failed', controller.signal)
      if (stopped) return
      if (snapshot.snapshotRevision < 1) throw new Error('标题栏尚未保存')
      await partIndexService.rebuild(item.attachmentId, snapshot.versionId, snapshot.snapshotRevision)
    },
    progress: (progress) => Object.assign(automaticPartIndex, progress),
    updated: (attachmentId) => {
      automaticPartIndex.attachmentId = attachmentId
      automaticPartIndex.revision++
    },
  })
  async function tick() {
    try { await worker.run() } catch {
      // Network errors are retried by the next cycle without interrupting navigation.
    } finally {
      if (!stopped) timer = setTimeout(() => void tick(), 60_000)
    }
  }
  void tick()
  return () => {
    stopped = true
    controller.abort()
    clearTimeout(timer)
    Object.assign(automaticPartIndex, { running: false, completed: 0, total: 0, failed: 0, attachmentId: '' })
  }
}
