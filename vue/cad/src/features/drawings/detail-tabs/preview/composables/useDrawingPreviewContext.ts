import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { isDrawingDecider } from '@/modules/drawing/drawing-authority'
import type { DrawingSummaryView, FileView, PartView, StructureNodeView } from '@/modules/drawing'
import { changeRequestService } from '@/services/change-request.service'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useReviewStore } from '@/stores/review.store'
import type { DrawingFile } from '@/types/domain.types'
import { editableChangeTargets } from '../../../components/detail/change-edit-access'
import { canDeleteDrawingFiles } from '../drawing-file-delete'
import { collectProjectFiles } from '../drawing-preview-files'

/**
 * 总图预览页的上下文与权限：当前项、项目根图号、项目级文件清单，
 * 以及建档/上传/编辑/删除四项权限判定。
 *
 * 「变更工单编辑权」跟随 canEditFile 一起放在这里：存档项目的编辑权由执行中的变更工单决定，
 * 拆开会让两处状态互相依赖。
 */
export function useDrawingPreviewContext() {
  const route = useRoute()
  const authStore = useAuthStore()
  const drawingStore = useDrawingStore()
  const reviewStore = useReviewStore()

  const currentItem = computed<DrawingSummaryView | PartView | null>(() => {
    const id = String(route.params.drawingId ?? '')
    return drawingStore.getDrawing(id) ?? drawingStore.getPart(id)
  })
  const isAssembly = computed(() => !currentItem.value || !('parentNo' in currentItem.value))
  const rootDrawingNo = computed(() => {
    if (!currentItem.value) return ''
    if (!('parentNo' in currentItem.value)) return currentItem.value.no

    let currentNo = currentItem.value.no
    const visited = new Set<string>()
    while (!visited.has(currentNo)) {
      visited.add(currentNo)
      const part = drawingStore.parts.find((item) => item.no === currentNo)
      if (!part) return currentItem.value.parentNo
      if (drawingStore.drawings.some((drawing) => drawing.no === part.parentNo)) return part.parentNo
      currentNo = part.parentNo
    }
    return currentItem.value.parentNo
  })

  const allFiles = computed(() => collectProjectFiles({
    currentItem: currentItem.value,
    isAssembly: isAssembly.value,
    rootDrawingNo: rootDrawingNo.value,
    parts: drawingStore.parts,
    structureOf: (drawingNo) => drawingStore.getStructure(drawingNo),
  }))

  const hasAssemblyFile = computed(() => {
    if (isAssembly.value) {
      const mainDrawing = currentItem.value as DrawingSummaryView
      return Boolean(mainDrawing?.files?.some((f) => f.role === 'assembly'))
    }
    return true // 零件图单独查看时
  })

  // 总图清单包含零件文件：按所属项目读取工单，再按附件 ID 判定授权。
  const changeTargetIds = ref<Set<string>>(new Set())
  const archivedProject = computed(() => drawingStore.getDrawing(rootDrawingNo.value)?.status === 'archived' || currentItem.value?.status === 'archived')
  let changeAccessSequence = 0
  async function refreshChangeEditAccess() {
    const sequence = ++changeAccessSequence
    const current = authStore.currentUser
    changeTargetIds.value = new Set()
    // 未登录（恢复会话未完成）时不要发请求：无鉴权请求只会拿回 401，并把「有编辑权」误判成无编辑权。
    if (!archivedProject.value || !current || !authStore.isAuthenticated) return
    const drawingId = drawingStore.getDrawing(rootDrawingNo.value)?.id
    if (!drawingId) return
    try {
      const list = await changeRequestService.listByDrawing(drawingId)
      const details = await Promise.all(list.filter((entry) => entry.status === 'executing' && entry.executorId === current.id)
        .map((entry) => changeRequestService.get(entry.id)))
      if (sequence === changeAccessSequence) changeTargetIds.value = editableChangeTargets(details, current.id)
    } catch {
      if (sequence === changeAccessSequence) changeTargetIds.value = new Set()
    }
  }
  watch(
    [() => currentItem.value, rootDrawingNo, archivedProject, () => authStore.currentUser?.id, () => authStore.isAuthenticated],
    () => { void refreshChangeEditAccess() },
    { immediate: true },
  )

  // 新建图纸（追加零件图）属于编制动作：负责人 = 原创建人权限。
  const canCreateDrawing = computed(() => {
    const project = drawingStore.getDrawing(rootDrawingNo.value)
    const user = authStore.currentUser
    if (!project || !user || !['draft', 'published'].includes(project.status)) return false
    return isDrawingDecider(project, user)
  })

  // 上传总图文件、上传零件图同样属于编制动作：只有控制权持有人（负责人，未指派时回落创建人）可操作。
  // 判定与后端 drawing_decision_owner 同构，避免非负责人向他人项目上传文件。
  const canManageDrawingFiles = computed(() => {
    const project = drawingStore.getDrawing(rootDrawingNo.value)
    const user = authStore.currentUser
    if (!project || !user || project.status === 'archived') return false
    return isDrawingDecider(project, user)
  })

  // 编辑权限矩阵（前端显隐；后端 editing.Open 同步强校验）：
  // 未存档 → 创建者或管理员；审核中另允许当前节点责任人；
  // 存档 → 仅当当前用户持有执行中的变更工单时可编辑。
  function canEditFile(file: DrawingFile): boolean {
    const item = currentItem.value
    const current = authStore.currentUser
    if (!item || !current) return false
    const admin = current.roles?.includes('admin') ?? false
    if (archivedProject.value) return changeTargetIds.value.has(file.id)
    // 控制权判定统一走 isDrawingDecider：有负责人时归负责人，无负责人时回落创建人。
    const authority = drawingAuthorityTarget(item)
    if (isDrawingDecider(authority, current)) return true
    if (item.status === 'reviewing') {
      if (admin) return true
      return reviewStore.myPendingReviews().some((reviewCase) => reviewCase.no === item.no)
    }
    return admin
  }

  /**
   * 图纸及零件项的创建人回退值。
   * 总图以图纸库记录为准；零件项自身可能只带创建人与更新人，因此逐个回退，
   * 避免「文件属于零件」时控制权判定拿不到创建人而误判为无权限。
   */
  function drawingAuthorityTarget(item: DrawingSummaryView | PartView | StructureNodeView | FileView) {
    const project = drawingStore.getDrawing(rootDrawingNo.value)
    if (project) return project
    const rawCreator = (item as { createdBy?: unknown }).createdBy
    const createdBy = typeof rawCreator === 'string' ? rawCreator : ('by' in item && typeof item.by === 'string' ? item.by : '')
    return { createdBy }
  }

  const canDeleteFiles = computed(() => {
    const item = currentItem.value
    const current = authStore.currentUser
    if (!item || !current) return false
    // 删除权与编辑权共用同一判定，避免出现「能编辑却不能删文件」的矛盾提示。
    return canDeleteDrawingFiles({
      status: archivedProject.value ? 'archived' : item.status,
      decides: isDrawingDecider(drawingAuthorityTarget(item), current),
    })
  })

  return {
    currentItem,
    isAssembly,
    rootDrawingNo,
    allFiles,
    hasAssemblyFile,
    archivedProject,
    canCreateDrawing,
    canManageDrawingFiles,
    canEditFile,
    canDeleteFiles,
  }
}
