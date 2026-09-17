import { computed, ref, type ComputedRef } from 'vue'

import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import {
  buildDownloadZip,
  candidateHasFormat,
  downloadFormatLabel,
  saveDownloadZip,
  toDownloadCandidates,
  type DownloadCandidate,
  type DownloadFormat,
} from '@/services/drawing-batch-download.service'
import { useUiStore } from '@/stores/ui.store'
import type { ProjectDrawingFile } from '../drawing-preview-files'

interface UseDrawingBatchDownloadOptions {
  allFiles: ComputedRef<ProjectDrawingFile[]>
  currentItem: ComputedRef<DrawingSummaryView | PartView | null>
}

/**
 * 批量下载弹窗：格式选择、勾选状态、进度与提示。
 * 读取 / 转换 / 打包 / 保存都在 drawing-batch-download.service，这里只编排 UI 状态。
 */
export function useDrawingBatchDownload(options: UseDrawingBatchDownloadOptions) {
  const uiStore = useUiStore()

  const isDownloadOpen = ref(false)
  const downloadFormat = ref<DownloadFormat>('dwg')
  const downloadFileIds = ref<Set<string>>(new Set())
  const isDownloading = ref(false)
  const downloadProgress = ref('')

  const downloadCandidates = computed<DownloadCandidate[]>(() => toDownloadCandidates(options.allFiles.value))

  const allDownloadSelected = computed(() =>
    downloadCandidates.value.length > 0
    && downloadCandidates.value.filter((item) => candidateHasFormat(item, downloadFormat.value)).every((item) => downloadFileIds.value.has(item.file.id)),
  )

  function openDownloadModal() {
    downloadFormat.value = 'dwg'
    downloadFileIds.value = new Set(downloadCandidates.value.filter((item) => candidateHasFormat(item, 'dwg')).map((item) => item.file.id))
    isDownloadOpen.value = true
  }

  /** 关闭弹窗（取消与右上角关闭共用；打包中由弹窗自身禁用按钮）。 */
  function closeDownloadModal() {
    isDownloadOpen.value = false
  }

  function onFormatChange(format: DownloadFormat) {
    downloadFormat.value = format
    // 切换格式时自动保留已选且当前格式可用的项，或者默认全选当前可用项
    const available = downloadCandidates.value.filter((item) => candidateHasFormat(item, format)).map((item) => item.file.id)
    const currentSelectedAvailable = available.filter((id) => downloadFileIds.value.has(id))
    downloadFileIds.value = new Set(currentSelectedAvailable.length ? currentSelectedAvailable : available)
  }

  function toggleDownloadFile(id: string) {
    const next = new Set(downloadFileIds.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    downloadFileIds.value = next
  }

  function toggleAllDownloadFiles() {
    if (allDownloadSelected.value) {
      downloadFileIds.value = new Set()
    } else {
      downloadFileIds.value = new Set(downloadCandidates.value.filter((item) => candidateHasFormat(item, downloadFormat.value)).map((item) => item.file.id))
    }
  }

  async function executeDownload() {
    const format = downloadFormat.value
    const selected = downloadCandidates.value.filter((item) => downloadFileIds.value.has(item.file.id) && candidateHasFormat(item, format))
    if (!selected.length) {
      uiStore.toast('请至少选择一个当前格式可用的文件', 'warn')
      return
    }
    isDownloading.value = true
    try {
      const { bytes, failed } = await buildDownloadZip({
        format,
        selected,
        onProgress: (message) => { downloadProgress.value = message },
      })
      const defaultZipName = `${options.currentItem.value?.no || '图纸文件'}-${downloadFormatLabel(format)}.zip`
      const saved = await saveDownloadZip(defaultZipName, bytes)
      // 用户在保存框中取消：保持弹窗打开，不提示
      if (saved.via === 'cancelled') return

      isDownloadOpen.value = false
      const okCount = selected.length - failed
      if (saved.via === 'tauri') {
        uiStore.toast(`已成功保存至：${saved.savedPath}（共 ${okCount} 个文件）`, 'ok')
      } else if (saved.via === 'picker') {
        uiStore.toast(`已成功保存所选图纸（共 ${okCount} 个文件）`, 'ok')
      } else {
        uiStore.toast(`已打包下载 ${okCount} 个文件${failed ? `，${failed} 个获取失败已跳过` : ''}`, failed ? 'warn' : 'ok')
      }
    } catch (error) {
      console.error('批量下载失败', error)
      uiStore.toast('批量下载失败，请重试', 'warn')
    } finally {
      isDownloading.value = false
      downloadProgress.value = ''
    }
  }

  return {
    isDownloadOpen,
    downloadFormat,
    downloadFileIds,
    downloadCandidates,
    allDownloadSelected,
    candidateHasFormat,
    isDownloading,
    downloadProgress,
    openDownloadModal,
    closeDownloadModal,
    onFormatChange,
    toggleDownloadFile,
    toggleAllDownloadFiles,
    executeDownload,
  }
}
