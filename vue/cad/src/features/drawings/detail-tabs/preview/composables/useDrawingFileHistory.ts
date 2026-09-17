import type { ComputedRef } from 'vue'

import type { DrawingFile } from '@/types/domain.types'

const HISTORY_READ_STORAGE_KEY = 'cad:read-file-history:v1'

interface UseDrawingFileHistoryOptions {
  /** 原文以「当前项存在」作为打开历史的前置条件，这里保持同一语义。 */
  currentItem: ComputedRef<{ no: string } | null>
  /** 跳转到历史版本树由页面提供，本 composable 不碰路由。 */
  onOpenHistory: (fileId: string) => void
}

/**
 * 文件历史已读状态：localStorage 记忆、未读标记与打开历史。
 * 只服务当前预览页，不进 Store，也不做跨页同步。
 */
export function useDrawingFileHistory(options: UseDrawingFileHistoryOptions) {
  function readHistoryIds(): Set<string> {
    try {
      const raw = window.localStorage.getItem(HISTORY_READ_STORAGE_KEY)
      const ids = raw ? JSON.parse(raw) : []
      return new Set(Array.isArray(ids) ? ids.filter((id): id is string => typeof id === 'string') : [])
    } catch {
      return new Set()
    }
  }

  function markHistoryAsRead(fileId: string) {
    const ids = readHistoryIds()
    ids.add(fileId)
    window.localStorage.setItem(HISTORY_READ_STORAGE_KEY, JSON.stringify([...ids]))
  }

  function isHistoryUnread(file: DrawingFile): boolean {
    return Boolean(file.history?.length) && !readHistoryIds().has(file.id)
  }

  function openHistory(file: DrawingFile) {
    if (!options.currentItem.value) return
    markHistoryAsRead(file.id)
    options.onOpenHistory(file.id)
  }

  return {
    isHistoryUnread,
    openHistory,
  }
}
