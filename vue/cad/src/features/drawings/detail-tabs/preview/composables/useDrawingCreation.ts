import { computed, onBeforeUnmount, onMounted, ref, type ComputedRef } from 'vue'

import { useAuthStore } from '@/stores/auth.store'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import { formatCurrentTime, formatFileSize } from '../drawing-preview-format'
import type { ProjectDrawingFile } from '../drawing-preview-files'

export type DrawingCreationStage = 'preparing' | 'uploading' | 'converting' | 'opening'

interface UseDrawingCreationOptions {
  /** 建档权限（页面上下文提供）。 */
  canCreateDrawing: ComputedRef<boolean>
  /** 项目根图号：新零件挂到它下面。 */
  rootDrawingNo: ComputedRef<string>
  /** 创建成功后用它找回刚保存的附件。 */
  allFiles: ComputedRef<ProjectDrawingFile[]>
  /**
   * 创建成功后自动呼出本地 CAD。
   * 由页面注入，本 composable 不直接依赖编辑会话 composable。
   */
  openEditor: (file: DrawingFile) => Promise<void>
}

/**
 * 新建图纸（追加零件图）：空白 EXB 模板校验 → 建档 → 刷新 → 自动呼出本地 CAD。
 * 离开保护随本流程一起管理，页面不再自己注册 beforeunload。
 */
export function useDrawingCreation(options: UseDrawingCreationOptions) {
  const authStore = useAuthStore()
  const drawingStore = useDrawingStore()
  const drawingOperationsStore = useDrawingOperationsStore()
  const uiStore = useUiStore()

  const isCreatingDrawing = ref(false)
  const creatingDrawing = ref(false)
  const drawingCreationStage = ref<DrawingCreationStage>('preparing')
  const drawingCreationError = ref('')
  const newDrawingName = ref('')
  const newDrawingNo = ref('')
  const drawingCreationProgress = computed(() => ({
    preparing: { step: 1, title: '正在准备空白图纸', detail: '正在校验 CAXA 模板，请稍候…' },
    uploading: { step: 2, title: '正在创建图纸', detail: '正在创建草稿零件并保存空白模板…' },
    converting: { step: 3, title: '正在同步新建图纸', detail: '正在读取刚创建的零件和附件信息…' },
    opening: { step: 4, title: '正在准备并打开 CAXA', detail: '正在等待 DWG 就绪并启动本地编辑，首次可能需要几十秒…' },
  })[drawingCreationStage.value])

  function openCreateDrawing() {
    newDrawingName.value = ''
    newDrawingNo.value = ''
    drawingCreationError.value = ''
    drawingCreationStage.value = 'preparing'
    isCreatingDrawing.value = true
  }

  function closeCreateDrawing() {
    if (creatingDrawing.value) return
    isCreatingDrawing.value = false
    drawingCreationError.value = ''
  }

  function guardDrawingCreationUnload(event: BeforeUnloadEvent) {
    if (!creatingDrawing.value) return
    event.preventDefault()
    event.returnValue = ''
  }

  async function createDrawing() {
    if (creatingDrawing.value || !options.canCreateDrawing.value) return
    const name = newDrawingName.value.trim()
    const no = newDrawingNo.value.trim()
    const parentNo = options.rootDrawingNo.value
    if (!name || !no) {
      uiStore.toast('请填写名称和图号', 'warn')
      return
    }
    const normalize = (value: string) => value.replace(/[\s/\\]/g, '').toLowerCase()
    if ([...drawingStore.drawings, ...drawingStore.parts].some((item) => normalize(item.no) === normalize(no))) {
      uiStore.toast('图号已存在，请使用其他图号', 'warn')
      return
    }
    drawingCreationError.value = ''
    drawingCreationStage.value = 'preparing'
    creatingDrawing.value = true
    let created = false
    try {
      const response = await fetch(`${import.meta.env.BASE_URL}templates/blank.exb`)
      if (!response.ok) throw new Error('空白 EXB 模板读取失败')
      const content = await response.blob()
      const signature = new Uint8Array(await content.slice(0, 8).arrayBuffer())
      if (signature.length !== 8 || ![0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1].every((byte, index) => signature[index] === byte)) {
        throw new Error('空白 EXB 模板无效')
      }
      const filename = `${no}(${name})`.replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_') + '.exb'
      const file: DrawingFile = {
        id: crypto.randomUUID(), name: filename, size: formatFileSize(content.size),
        role: 'part', drawingNo: parentNo, partNo: no, version: 'v1.0',
        uploadedBy: authStore.currentUser?.displayName || '', uploadedAt: formatCurrentTime(), previewable: true,
      }
      drawingCreationStage.value = 'uploading'
      await drawingOperationsStore.createPartWithFile(parentNo, {
        no, name, parentNo, project: drawingStore.getDrawing(parentNo)?.project || parentNo,
        material: '—', spec: '', weight: 0, surfaceTreatment: '', partType: '自制件',
        qty: 1, status: 'draft', ver: 'v1.0', hasFile: true, files: [file],
      }, file, content)
      created = true
      drawingCreationStage.value = 'converting'
      await drawingStore.refresh()
      // 新建零件仅有这张附件；EXB 转换完成后展示名可能已经变为 DWG。
      const savedFile = options.allFiles.value.find((entry) => entry.ownerNo === no && entry.role === 'part')
      uiStore.toast(`图纸「${name}」已创建`, 'ok')
      if (savedFile) {
        drawingCreationStage.value = 'opening'
        await options.openEditor(savedFile)
      }
      else uiStore.toast('图纸已创建，请刷新列表后点击「本地编辑」', 'warn')
      isCreatingDrawing.value = false
    } catch (error) {
      const message = created ? '图纸已创建，但自动打开失败；请刷新列表后点击「本地编辑」' : error instanceof Error ? error.message : '新建图纸失败'
      if (created) isCreatingDrawing.value = false
      else drawingCreationError.value = message
      uiStore.toast(message, 'warn')
    } finally {
      creatingDrawing.value = false
    }
  }

  // 建档期间禁止关闭窗口：原先由页面 onMounted / onBeforeUnmount 注册，随流程一起搬进来。
  onMounted(() => { window.addEventListener('beforeunload', guardDrawingCreationUnload) })
  onBeforeUnmount(() => { window.removeEventListener('beforeunload', guardDrawingCreationUnload) })

  return {
    isCreatingDrawing,
    creatingDrawing,
    drawingCreationStage,
    drawingCreationError,
    newDrawingName,
    newDrawingNo,
    drawingCreationProgress,
    openCreateDrawing,
    closeCreateDrawing,
    createDrawing,
  }
}
