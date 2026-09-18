import { ref, type ComputedRef } from 'vue'

import { extractAndSaveTitleBlock } from '@/services/drawing-title-block.service'
import type { TitleSnapshot } from '@/services/title-block-workflow'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import type { ProjectDrawingFile } from '../drawing-preview-files'
import { filterReidentifiableFiles } from '../drawing-reidentify'

/** 从已保存的标题栏快照里取图号；图幅没写图号时返回空串，由调用方按失败项处理。 */
function partNoFromSnapshot(snapshot: TitleSnapshot): string {
  for (const space of snapshot.payload?.spaces ?? []) {
    const field = space.fields.find((item) => item.key === 'number')
    if (field?.value) return field.value.trim()
    const candidate = field?.candidates[0]
    if (candidate) return candidate.trim()
  }
  return ''
}

/** 弹窗里的一行待校正记录。 */
export interface ReidentifyItem {
  file: DrawingFile
  oldPartNo: string
  newPartNo: string
  checked: boolean
}

interface UseDrawingReidentifyOptions {
  allFiles: ComputedRef<ProjectDrawingFile[]>
  rootDrawingNo: ComputedRef<string>
  canManageDrawingFiles: ComputedRef<boolean>
}

/**
 * 批量重新识别图号：读文件 → CAD identify → 收集变化 → 弹窗勾选 → 逐项校正。
 * 「哪些文件该参与」与「表格类文件判断」在 drawing-reidentify.ts 的纯函数里。
 */
export function useDrawingReidentify(options: UseDrawingReidentifyOptions) {
  const drawingStore = useDrawingStore()
  const drawingOperationsStore = useDrawingOperationsStore()
  const uiStore = useUiStore()

  const isReidentifyingAll = ref(false)
  const isReidentifyModalOpen = ref(false)
  const reidentifyList = ref<ReidentifyItem[]>([])
  const reidentifyFailures = ref<string[]>([])
  const isExecutingReidentify = ref(false)

  async function reidentifyAllPartFiles() {
    if (isReidentifyingAll.value) return
    if (!options.canManageDrawingFiles.value) {
      uiStore.toast('只有图纸负责人、创建人或管理员可以校正图号', 'warn')
      return
    }
    const currentRootNo = options.rootDrawingNo.value
    // 借用图不参与：它的图号由来源项目负责，借用方校正等于去改别人项目的零件。
    const cadFiles = filterReidentifiableFiles(options.allFiles.value.filter((file) => !file.borrowed))
    if (!cadFiles.length) {
      uiStore.toast('当前图纸没有可重新识别的零件 CAD 文件（已自动过滤明细表与表格）', 'warn')
      return
    }

    isReidentifyingAll.value = true
    try {
      const results: ReidentifyItem[] = []
      const failures: string[] = []
      for (const file of cadFiles) {
        try {
          // 图号真值在图幅里，不在文件名里：文件名可能忘改，后端按文件名解析等于白读。
          // EXB 未转换完成时这里会抛 409「图纸正在转换，完成后将自动提取标题栏」，按失败项收集。
          const snapshot = await extractAndSaveTitleBlock(file.id)
          const identifiedNo = partNoFromSnapshot(snapshot)
          if (!identifiedNo) {
            failures.push(`${file.name}：图幅里没有找到图号`)
            continue
          }

          // 过滤：如果识别出的图号与总图号完全相同，说明是附属文件或总图明细，跳过
          if (currentRootNo && identifiedNo === currentRootNo) {
            continue
          }

          if (identifiedNo !== (file.partNo || '')) {
            results.push({ file, oldPartNo: file.partNo || '未关联', newPartNo: identifiedNo, checked: true })
          }
        } catch (error) {
          failures.push(`${file.name}：${error instanceof Error ? error.message : String(error)}`)
        }
      }

      if (!results.length) {
        uiStore.toast(failures.length ? `没有发现图号变化，${failures.length} 个文件未能读取图幅` : '所有零件图号均已与图幅一致', failures.length ? 'warn' : 'ok')
        return
      }

      reidentifyList.value = results
      reidentifyFailures.value = failures
      isReidentifyModalOpen.value = true
    } finally {
      isReidentifyingAll.value = false
    }
  }

  async function confirmBatchReidentify() {
    const selected = reidentifyList.value.filter((item) => item.checked)
    if (!selected.length) {
      uiStore.toast('请至少勾选一个要校正的文件', 'warn')
      return
    }

    isExecutingReidentify.value = true
    let updatedCount = 0
    const executeFailures: string[] = []
    try {
      for (const item of selected) {
        try {
          await drawingOperationsStore.reidentifyDrawingFile(item.file, item.newPartNo)
          await drawingStore.refresh()
          updatedCount += 1
        } catch (error) {
          executeFailures.push(`${item.file.name}：${error instanceof Error ? error.message : String(error)}`)
        }
      }
      uiStore.toast(
        `已完成 ${updatedCount}/${selected.length} 个文件的图号校正${executeFailures.length ? `，${executeFailures.length} 个失败` : ''}`,
        executeFailures.length ? 'warn' : 'ok',
      )
      isReidentifyModalOpen.value = false
    } finally {
      isExecutingReidentify.value = false
    }
  }

  /** 弹窗勾选 / 取消勾选一行：勾选状态只在这里落账，弹窗组件不直接改 props。 */
  function toggleReidentifyItem(fileId: string, checked: boolean) {
    const item = reidentifyList.value.find((entry) => entry.file.id === fileId)
    if (item) item.checked = checked
  }

  /** 弹窗把图号改成手工修正值时回写：图幅识别的结果允许人工纠正。 */
  function updateReidentifyNo(fileId: string, newPartNo: string) {
    const item = reidentifyList.value.find((entry) => entry.file.id === fileId)
    if (item) item.newPartNo = newPartNo
  }

  /** 弹窗表头的全选 / 全不选。 */
  function toggleAllReidentifyItems(checked: boolean) {
    reidentifyList.value.forEach((entry) => {
      entry.checked = checked
    })
  }

  function closeReidentifyModal() {
    if (isExecutingReidentify.value) return
    isReidentifyModalOpen.value = false
  }

  return {
    isReidentifyingAll,
    isReidentifyModalOpen,
    reidentifyList,
    reidentifyFailures,
    isExecutingReidentify,
    reidentifyAllPartFiles,
    confirmBatchReidentify,
    closeReidentifyModal,
    toggleReidentifyItem,
    updateReidentifyNo,
    toggleAllReidentifyItems,
  }
}
