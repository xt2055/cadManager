import { defineStore } from 'pinia'
import { ref } from 'vue'
import { appContainer, drawingUploadCoordinator } from '@/app/container'
import type { PendingDrawingUploadEntry } from '@/modules/upload'
import type { DrawingUploadPlan } from '@/modules/upload'
import type { UploadSession, UploadSessionItem, UploadSessionSnapshot } from '@/services/data-manager/data-provider'

/** 上传会话 Read Model：保存进度、失败项和恢复状态，不保存上传事务细节。 */
export const useUploadStore = defineStore('upload', () => {
  const session = ref<UploadSession | null>(null)
  const items = ref<UploadSessionItem[]>([])
  const progress = ref<Record<string, number>>({})
  const recovering = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)

  function applySnapshot(snapshot: UploadSessionSnapshot) {
    session.value = snapshot.session
    items.value = snapshot.items
  }

  async function load(sessionId: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      applySnapshot(await appContainer.uploadGateway.getSession(sessionId))
    } catch (loadError: unknown) {
      error.value = loadError instanceof Error ? loadError.message : String(loadError)
      throw loadError
    } finally {
      loading.value = false
    }
  }

  async function refresh(): Promise<void> {
    if (session.value) await load(session.value.id)
  }

  async function recover(sessionId: string): Promise<void> {
    recovering.value = true
    try {
      await load(sessionId)
    } finally {
      recovering.value = false
    }
  }

  function setProgress(itemId: string, value: number) {
    progress.value = { ...progress.value, [itemId]: Math.max(0, Math.min(100, value)) }
  }

  async function create(plan: DrawingUploadPlan) {
    const result = await drawingUploadCoordinator.create({
      ...plan,
      onProgress: (itemId, value) => {
        setProgress(itemId, value)
        plan.onProgress?.(itemId, value)
      },
    })
    await load(result.session.id)
    return result
  }

  async function retryItem(itemId: string, entries: Map<string, PendingDrawingUploadEntry>): Promise<void> {
    if (!session.value) throw new Error('当前没有上传会话')
    await drawingUploadCoordinator.retryOne(session.value.id, itemId, entries, (id, value) => setProgress(id, value))
    await refresh()
  }

  async function retryAll(entries: Map<string, PendingDrawingUploadEntry>): Promise<Record<string, unknown>> {
    if (!session.value) throw new Error('当前没有上传会话')
    const result = await drawingUploadCoordinator.retryFailed(session.value.id, entries, (id, value) => setProgress(id, value))
    await refresh()
    return result
  }

  async function cancel(): Promise<void> {
    if (!session.value) return
    await appContainer.uploadGateway.cancelSession(session.value.id)
    session.value = null
    items.value = []
    progress.value = {}
  }

  function clear() {
    session.value = null
    items.value = []
    progress.value = {}
    error.value = null
  }

  return {
    session,
    items,
    progress,
    recovering,
    loading,
    error,
    load,
    refresh,
    recover,
    setProgress,
    create,
    retryItem,
    retryAll,
    cancel,
    clear,
  }
})
