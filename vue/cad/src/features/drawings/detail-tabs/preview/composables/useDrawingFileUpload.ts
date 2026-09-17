import { ref, type ComputedRef } from 'vue'

import { drawingFileService } from '@/app/container'
import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import { isCadPartFile, planPartUpload } from '@/modules/upload/drawing-part-upload-plan'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import { formatCurrentTime, formatFileSize } from '../drawing-preview-format'

const MANAGE_DENIED_HINT = '只有图纸负责人、创建人或管理员可以为该图纸上传文件'
const ASSEMBLY_FIRST_HINT = '请先上传总图文件，再进行零件图上传'

interface UseDrawingFileUploadOptions {
  currentItem: ComputedRef<DrawingSummaryView | PartView | null>
  rootDrawingNo: ComputedRef<string>
  isAssembly: ComputedRef<boolean>
  hasAssemblyFile: ComputedRef<boolean>
  canManageDrawingFiles: ComputedRef<boolean>
  /** 总图上传成功后打开预览：导航由页面注入，本 composable 不碰路由。 */
  onAssemblyUploaded: (file: DrawingFile) => void
}

/**
 * 上传总图 / 批量上传零件图：只管 UI 状态、input、toast、刷新与调用领域服务。
 * 「这个文件该走哪条持久化路径」由 drawing-part-upload-plan 的纯函数决定。
 */
export function useDrawingFileUpload(options: UseDrawingFileUploadOptions) {
  const authStore = useAuthStore()
  const drawingStore = useDrawingStore()
  const drawingOperationsStore = useDrawingOperationsStore()
  const uiStore = useUiStore()

  // 文件上传 input ref
  const assemblyInput = ref<HTMLInputElement | null>(null)
  const partInput = ref<HTMLInputElement | null>(null)

  function uploadedBy(): string {
    return authStore.currentUser?.displayName || authStore.currentUser?.account || '未知'
  }

  function baseFile(file: File) {
    return {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      name: file.name,
      size: formatFileSize(file.size),
      uploadedBy: uploadedBy(),
      uploadedAt: formatCurrentTime(),
      previewable: true,
    }
  }

  function triggerUploadAssembly() {
    if (!options.canManageDrawingFiles.value) {
      uiStore.toast(MANAGE_DENIED_HINT, 'warn')
      return
    }
    assemblyInput.value?.click()
  }

  function triggerUploadPart() {
    if (!options.canManageDrawingFiles.value) {
      uiStore.toast(MANAGE_DENIED_HINT, 'warn')
      return
    }
    if (!options.hasAssemblyFile.value && options.isAssembly.value) {
      uiStore.toast(ASSEMBLY_FIRST_HINT, 'warn')
      return
    }
    partInput.value?.click()
  }

  async function onAssemblyFileChange(event: Event) {
    const target = event.target as HTMLInputElement
    const file = target.files?.[0]
    const item = options.currentItem.value
    if (!file || !item) return

    const newFile: DrawingFile = {
      ...baseFile(file),
      role: 'assembly',
      drawingNo: item.no,
      version: item.version || 'v1.0',
    }

    try {
      await drawingOperationsStore.uploadDrawingFile(item.no, newFile, file)
      await drawingStore.refresh()
      uiStore.toast(`总图文件「${file.name}」上传成功`, 'ok')
      options.onAssemblyUploaded(newFile)
    } catch (error) {
      console.error('上传总图失败', error)
      uiStore.toast('上传总图失败', 'warn')
    } finally {
      target.value = ''
    }
  }

  async function onPartFilesChange(event: Event) {
    const target = event.target as HTMLInputElement
    const files = target.files
    if (!files || !files.length || !options.currentItem.value) return

    let createdCount = 0
    let otherCount = 0
    try {
      for (let i = 0; i < files.length; i++) {
        const file = files[i]
        if (!file) continue
        const rootNo = options.rootDrawingNo.value

        if (!isCadPartFile(file.name)) {
          const otherFile: DrawingFile = { ...baseFile(file), role: 'other', drawingNo: rootNo, version: 'v1.0' }
          await drawingOperationsStore.uploadOtherFile(rootNo, otherFile, file)
          await drawingStore.refresh()
          otherCount += 1
          continue
        }

        const identity = await drawingFileService.identify(file, file.name)
        const plan = planPartUpload({
          rootDrawingNo: rootNo,
          identity,
          parts: drawingStore.parts,
          drawingNos: drawingStore.drawings.map((drawing) => drawing.no),
        })
        const newFile: DrawingFile = {
          ...baseFile(file),
          role: plan.role,
          drawingNo: rootNo,
          ...(plan.partNo ? { partNo: plan.partNo } : {}),
          version: 'v1.0',
        }

        if (plan.kind === 'other-file') {
          await drawingOperationsStore.uploadOtherFile(rootNo, newFile, file)
          await drawingStore.refresh()
          otherCount += 1
          continue
        }

        if (plan.kind === 'attach-to-existing-part') {
          const existingPart = drawingStore.parts.find((part) => part.no === plan.partNo && part.parentNo === rootNo)
          if (existingPart && plan.material !== '—') existingPart.material = plan.material
          await drawingOperationsStore.uploadDrawingFile(plan.partNo, newFile, file)
          await drawingStore.refresh()
          createdCount += 1
          continue
        }

        await drawingOperationsStore.createPartWithFile(plan.parentNo, {
          no: plan.partNo,
          name: file.name.replace(/\.[^/.]+$/, ''),
          parentNo: plan.parentNo,
          project: '',
          material: plan.material,
          spec: '',
          weight: 0,
          surfaceTreatment: '',
          partType: '自制件',
          qty: 1,
          status: 'draft',
          ver: 'v1.0',
          hasFile: true,
          files: [newFile],
          ...(plan.borrowFrom ? { borrowFrom: plan.borrowFrom } : {}),
        }, newFile, file)
        await drawingStore.refresh()
        createdCount += 1
      }
      uiStore.toast(`已整理 ${createdCount} 个规范零件文件${otherCount ? `，${otherCount} 个文件归入其他文件` : ''}`, 'ok')
    } catch (error) {
      console.error('上传零件图失败', error)
      uiStore.toast('上传零件图失败', 'warn')
    } finally {
      target.value = ''
    }
  }

  return {
    assemblyInput,
    partInput,
    triggerUploadAssembly,
    triggerUploadPart,
    onAssemblyFileChange,
    onPartFilesChange,
  }
}
