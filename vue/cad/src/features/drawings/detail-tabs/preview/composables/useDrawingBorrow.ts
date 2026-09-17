import { computed, ref, type ComputedRef } from 'vue'

import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'

/** 存档项目借用被拒时的统一解释；引导入口由页面注入路由。 */
const ARCHIVED_HINT = '存档图纸处于只读保护。如需把其他项目的零件挂到当前图纸，请先发起变更工单，经管理员审批后再修改。'

interface UseDrawingBorrowOptions {
  currentItem: ComputedRef<DrawingSummaryView | PartView | null>
  canManageDrawingFiles: ComputedRef<boolean>
  archivedProject: ComputedRef<boolean>
  /** 「去发起变更工单」的跳转由页面提供，本 composable 不碰路由。 */
  onOpenChangeWorkOrder: () => void
}

/**
 * 借用零件：项目/零件搜索与选型、弹窗状态、借用幂等键与提交。
 * 借用本身是既有 Application 操作（drawingOperationsStore.borrowPartToProject），不再包一层 service。
 */
export function useDrawingBorrow(options: UseDrawingBorrowOptions) {
  const drawingStore = useDrawingStore()
  const drawingOperationsStore = useDrawingOperationsStore()
  const uiStore = useUiStore()

  const isBorrowing = ref(false)
  const borrowSearchMode = ref<'by-project' | 'global-part'>('by-project')
  const projectSearchQuery = ref('')
  const partSearchQuery = ref('')
  const selectedSourceProjectNo = ref('')
  const selectedSourcePartNo = ref('')
  const borrowReasonInput = ref('')
  const isSubmittingBorrow = ref(false)
  const borrowIdempotencyKey = ref('')
  const borrowKeyBoundSignature = ref('')

  // 获取除当前项目外的所有可选项目（支持名称、图号、厂商模糊过滤）
  const candidateProjects = computed(() => {
    const curNo = options.currentItem.value?.no || ''
    const q = projectSearchQuery.value.trim().toLowerCase()
    const list = drawingStore.drawings.filter((d) => d.no !== curNo)
    if (!q) return list
    return list.filter((d) =>
      d.no.toLowerCase().includes(q) ||
      d.name.toLowerCase().includes(q) ||
      (d.vendor && d.vendor.toLowerCase().includes(q)) ||
      (d.project && d.project.toLowerCase().includes(q))
    )
  })

  // 当前选中的源项目对象
  const selectedProjectDetail = computed(() => {
    if (!selectedSourceProjectNo.value) return candidateProjects.value[0] || null
    return drawingStore.drawings.find((d) => d.no === selectedSourceProjectNo.value) || null
  })

  // 根据模式和筛选条件获取零件列表
  const candidateParts = computed(() => {
    const q = partSearchQuery.value.trim().toLowerCase()

    if (borrowSearchMode.value === 'global-part') {
      // 全库全局穿透搜索（排除当前项目自身的零件）
      const curNo = options.currentItem.value?.no || ''
      const allOtherParts = drawingStore.parts.filter((p) => p.parentNo !== curNo && isBorrowablePart(p))
      if (!q) return allOtherParts.slice(0, 100) // 默认展示前 100 项
      return allOtherParts.filter((p) =>
        p.no.toLowerCase().includes(q) ||
        p.name.toLowerCase().includes(q) ||
        (p.material && p.material.toLowerCase().includes(q)) ||
        (p.spec && p.spec.toLowerCase().includes(q)) ||
        (p.parentNo && p.parentNo.toLowerCase().includes(q))
      )
    }

    // 按项目浏览选型
    const pNo = selectedSourceProjectNo.value || candidateProjects.value[0]?.no
    if (!pNo) return []

    const parts = drawingStore.parts.filter((p) => isBorrowablePart(p) && (p.parentNo === pNo || p.no.startsWith(`${pNo}-`)))
    if (!q) return parts

    return parts.filter((p) =>
      p.no.toLowerCase().includes(q) ||
      p.name.toLowerCase().includes(q) ||
      (p.material && p.material.toLowerCase().includes(q)) ||
      (p.spec && p.spec.toLowerCase().includes(q))
    )
  })

  // 计算各个项目的零件数量
  function isBorrowablePart(part: PartView): boolean {
    return part.status === 'published' || part.status === 'archived'
  }

  function getProjectPartCount(pNo: string): number {
    return drawingStore.parts.filter((p) => isBorrowablePart(p) && (p.parentNo === pNo || p.no.startsWith(`${pNo}-`))).length
  }

  // 获取零件所属项目的名称
  function getPartProjectName(part: PartView): string {
    const p = drawingStore.drawings.find((d) => d.no === part.parentNo)
    return p ? p.name : (part.parentNo || '未知项目')
  }

  const selectedPartDetail = computed(() => {
    if (!selectedSourcePartNo.value) return null
    return drawingStore.parts.find((p) => p.no === selectedSourcePartNo.value) || null
  })

  function showArchivedHint() {
    uiStore.confirm('图纸已存档，无法直接借用零件', ARCHIVED_HINT, {
      confirmText: '去发起变更工单',
      onConfirm: options.onOpenChangeWorkOrder,
    })
  }

  function openBorrowModal() {
    if (!options.canManageDrawingFiles.value) {
      uiStore.toast('只有图纸负责人、创建人或管理员可以借用零件', 'warn')
      return
    }
    if (options.archivedProject.value) {
      showArchivedHint()
      return
    }
    isBorrowing.value = true
    borrowSearchMode.value = 'by-project'
    projectSearchQuery.value = ''
    partSearchQuery.value = ''
    selectedSourceProjectNo.value = candidateProjects.value[0]?.no || ''
    selectedSourcePartNo.value = ''
    borrowReasonInput.value = ''
  }

  function cancelBorrowModal() {
    if (isSubmittingBorrow.value) return
    isBorrowing.value = false
    borrowIdempotencyKey.value = ''
    borrowKeyBoundSignature.value = ''
    projectSearchQuery.value = ''
    partSearchQuery.value = ''
    selectedSourcePartNo.value = ''
  }

  function selectProject(projNo: string) {
    selectedSourceProjectNo.value = projNo
    selectedSourcePartNo.value = ''
  }

  async function confirmBorrowPart() {
    if (isSubmittingBorrow.value) return
    if (!selectedSourcePartNo.value || !options.currentItem.value) return
    const curNo = options.currentItem.value.no
    const reason = borrowReasonInput.value.trim() || '跨项目工程设计借用'
    const signature = `${curNo}::${selectedSourcePartNo.value}::${reason}`

    if (!borrowIdempotencyKey.value || borrowKeyBoundSignature.value !== signature) {
      borrowIdempotencyKey.value = `borrow-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
      borrowKeyBoundSignature.value = signature
    }

    isSubmittingBorrow.value = true
    let borrowedPartName = ''
    let borrowedPartNo = ''
    try {
      const borrowed = await drawingOperationsStore.borrowPartToProject(
        curNo,
        selectedSourcePartNo.value,
        reason,
        borrowIdempotencyKey.value,
      )
      borrowedPartName = borrowed.name
      borrowedPartNo = borrowed.no
      try {
        await drawingStore.refresh()
      } catch (refreshErr) {
        console.warn('借用后刷新图纸列表失败，但不影响借用结果:', refreshErr)
      }
      uiStore.toast(`成功借用零件「${borrowedPartName} (${borrowedPartNo})」到当前项目`, 'ok')
      borrowIdempotencyKey.value = ''
      borrowKeyBoundSignature.value = ''
      isBorrowing.value = false
      selectedSourcePartNo.value = ''
    } catch (err: unknown) {
      console.error('借用失败', err)
      const message = err instanceof Error ? err.message : '借用零件失败，请重试'
      if (options.archivedProject.value || /存档|归档|变更工单/.test(message)) {
        showArchivedHint()
        return
      }
      uiStore.toast(message, 'warn')
    } finally {
      isSubmittingBorrow.value = false
    }
  }

  return {
    isBorrowing,
    borrowSearchMode,
    projectSearchQuery,
    partSearchQuery,
    selectedSourceProjectNo,
    selectedSourcePartNo,
    borrowReasonInput,
    isSubmittingBorrow,
    candidateProjects,
    selectedProjectDetail,
    candidateParts,
    selectedPartDetail,
    isBorrowablePart,
    getProjectPartCount,
    getPartProjectName,
    openBorrowModal,
    cancelBorrowModal,
    selectProject,
    confirmBorrowPart,
  }
}
