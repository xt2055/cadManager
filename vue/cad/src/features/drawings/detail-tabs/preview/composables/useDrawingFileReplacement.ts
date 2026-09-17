import { ref, type ComputedRef } from 'vue'

import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import { formatFileSize } from '../drawing-preview-format'

interface UseDrawingFileReplacementOptions {
  currentItem: ComputedRef<DrawingSummaryView | PartView | null>
}

/**
 * 替换图纸文件（版本替换）：input、弹窗状态、替换原因与确认/取消。
 * 版本归档、历史留痕仍由 drawingOperationsStore.replaceDrawingFile 负责。
 */
export function useDrawingFileReplacement(options: UseDrawingFileReplacementOptions) {
  const drawingStore = useDrawingStore()
  const drawingOperationsStore = useDrawingOperationsStore()
  const uiStore = useUiStore()

  const replaceInput = ref<HTMLInputElement | null>(null)
  const isReplacing = ref(false)
  const targetReplaceFile = ref<DrawingFile | null>(null)
  const replaceReasonInput = ref('')
  const selectedReplaceBlob = ref<File | null>(null)

  function triggerReplace(file: DrawingFile) {
    targetReplaceFile.value = file
    replaceReasonInput.value = ''
    selectedReplaceBlob.value = null
    replaceInput.value?.click()
  }

  function onReplaceFileSelected(event: Event) {
    const target = event.target as HTMLInputElement
    const file = target.files?.[0]
    if (!file || !targetReplaceFile.value) return
    selectedReplaceBlob.value = file
    isReplacing.value = true
    target.value = ''
  }

  async function confirmReplace() {
    const blob = selectedReplaceBlob.value
    const cur = targetReplaceFile.value
    if (!cur || !blob) return
    const targetNo = cur.partNo || cur.drawingNo || options.currentItem.value?.no || ''

    try {
      const updated = await drawingOperationsStore.replaceDrawingFile(
        targetNo,
        cur.id,
        {
          name: blob.name,
          size: formatFileSize(blob.size),
          replaceReason: replaceReasonInput.value.trim() || '版本替换更新',
        },
        blob,
      )
      await drawingStore.refresh()
      uiStore.toast(`文件已成功替换为 ${updated.version} · 历史版本已归档留痕`, 'ok')
      cancelReplace()
    } catch (error) {
      console.error('替换文件失败', error)
      uiStore.toast('替换文件失败，请重试', 'warn')
    }
  }

  function cancelReplace() {
    isReplacing.value = false
    targetReplaceFile.value = null
    selectedReplaceBlob.value = null
  }

  return {
    replaceInput,
    isReplacing,
    targetReplaceFile,
    replaceReasonInput,
    selectedReplaceBlob,
    triggerReplace,
    onReplaceFileSelected,
    confirmReplace,
    cancelReplace,
  }
}
