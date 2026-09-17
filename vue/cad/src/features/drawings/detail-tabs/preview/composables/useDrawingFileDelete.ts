import type { ComputedRef } from 'vue'

import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import type { ProjectDrawingFile } from '../drawing-preview-files'

interface UseDrawingFileDeleteOptions {
  currentItem: ComputedRef<DrawingSummaryView | PartView | null>
}

/**
 * 删除图纸文件：二次确认 → 调 Application 操作 → 刷新列表。
 * 与上传 / 替换同属文件操作层，从页面搬出来，页面只保留「确认删除」这一个入口。
 */
export function useDrawingFileDelete(options: UseDrawingFileDeleteOptions) {
  const drawingStore = useDrawingStore()
  const drawingOperationsStore = useDrawingOperationsStore()
  const uiStore = useUiStore()

  function confirmDeleteFile(file: ProjectDrawingFile) {
    uiStore.confirm('删除图纸文件', `确定要删除图纸文件「${file.name}」吗？`, {
      confirmText: '删除',
      danger: true,
      onConfirm: () => deleteFile(file),
    })
  }

  async function deleteFile(file: DrawingFile) {
    const item = options.currentItem.value
    if (!item) return
    try {
      const targetNo = file.partNo || file.drawingNo || item.no
      if (file.role === 'other') {
        await drawingOperationsStore.deleteOtherFile(targetNo, file.id)
      } else {
        await drawingOperationsStore.deleteDrawingFile(targetNo, file.id)
      }
      await drawingStore.refresh()
      uiStore.toast(`已删除文件 ${file.name}`)
    } catch (error) {
      console.error('删除文件失败', error)
      uiStore.toast('删除文件失败，请重试', 'warn')
    }
  }

  return { confirmDeleteFile }
}
