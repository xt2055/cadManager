import { ref, type ComputedRef } from 'vue'

import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import { isCadPartFile, planPartUpload } from '@/modules/upload/drawing-part-upload-plan'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import { extractAndSaveTitleBlock } from '@/services/drawing-title-block.service'
import type { TitleSnapshot } from '@/services/title-block-workflow'
import { readLocalTitleBlock, type LocalTitleBlockResult } from '../local-title-block'
import { waitForConversion } from '../conversion-wait'
import type { PartNumberInputRow } from '../components/PartNumberInputModal.vue'
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

/** EXB 中间态：已上传但归属未定的附件，及它在本地文件列表里的 id（回退时要按它撤掉）。 */
interface StagedAttachment {
  attachmentId: string
  storageKey: string
  fileId: string
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

  // ———— 手工补录（兜底）：图幅里读不到图号时才用 ————
  const partNumberRows = ref<PartNumberInputRow[]>([])
  const partNumberOpen = ref(false)
  const partNumberBusy = ref(false)
  let resolvePartNumbers: ((entries: Array<{ id: string; partNo: string }> | null) => void) | null = null

  function askPartNumbers(rows: PartNumberInputRow[]): Promise<Array<{ id: string; partNo: string }> | null> {
    partNumberRows.value = rows
    partNumberOpen.value = true
    return new Promise((resolve) => { resolvePartNumbers = resolve })
  }

  function settlePartNumbers(entries: Array<{ id: string; partNo: string }> | null) {
    partNumberOpen.value = false
    partNumberRows.value = []
    const resolve = resolvePartNumbers
    resolvePartNumbers = null
    resolve?.(entries)
  }

  // 确认后弹窗立即关闭，建零件进度由 toast 汇报；busy 表达「正在按补录的图号落库」这段状态。
  function confirmPartNumbers(entries: Array<{ id: string; partNo: string }>) {
    partNumberBusy.value = true
    settlePartNumbers(entries)
  }

  /** 取消 = 整批不落库；已上传的 EXB 中间态保留为「其他文件」，不静默丢弃。 */
  function cancelPartNumbers() {
    settlePartNumbers(null)
  }

  /** 从已保存的标题栏快照里取图号（转换完成后读到的就是图纸真值）。 */
  function partNoFromTitleSnapshot(payload: TitleSnapshot['payload']): string {
    if (!payload) return ''
    for (const space of payload.spaces) {
      const field = space.fields.find((item) => item.key === 'number')
      if (field?.value) return field.value
      const candidate = field?.candidates[0]
      if (candidate) return candidate
    }
    return ''
  }

  /** 本地读不到图号的文件：记录原因，供弹窗展示。 */
  function manualReason(result: LocalTitleBlockResult): string {
    if (result.kind === 'error') return `图幅解析失败：${result.message}`
    if (result.kind === 'not-found') return '图幅里没有找到图号'
    return '需要手工填写图号'
  }

  async function onPartFilesChange(event: Event) {
    const target = event.target as HTMLInputElement
    const files = target.files
    if (!files || !files.length || !options.currentItem.value) return

    const rootNo = options.rootDrawingNo.value
    const failures: string[] = []
    let createdCount = 0
    let otherCount = 0
    // 已由图幅确定图号、等待落库的文件；EXB 中间态已上传，带上服务端标识与本地 fileId。
    const resolved: Array<{ file: File; partNo: string; attachment: StagedAttachment | null }> = []
    const manualRows: PartNumberInputRow[] = []
    const manualFiles = new Map<string, File>()
    // 需要手工补录、但已经上传过中间态的文件：补录后要么落成零件，要么按 id 撤掉，不能重复上传。
    const manualAttachments = new Map<string, StagedAttachment>()
    let uploadedOther = 0

    try {
      // 阶段一：逐个文件定图号。图号取自图幅；EXB 先上传等转换，再读图幅。
      for (const file of Array.from(files)) {
        const key = `${file.name}:${file.size}:${file.lastModified}`
        try {
          if (!isCadPartFile(file.name)) {
            const otherFile: DrawingFile = { ...baseFile(file), role: 'other', drawingNo: rootNo, version: 'v1.0' }
            await drawingOperationsStore.uploadOtherFile(rootNo, otherFile, file)
            otherCount += 1
            continue
          }

          const local = await readLocalTitleBlock(file)
          if (local.kind === 'ok' && local.partNo) {
            resolved.push({ file, partNo: local.partNo, attachment: null })
            continue
          }
          if (local.kind === 'ok') {
            manualRows.push({ id: key, name: file.name, candidates: local.candidates, reason: '图幅里有多个图号候选，请选择或填写' })
            manualFiles.set(key, file)
            continue
          }

          // EXB：浏览器解析不了，先上传触发后台转换，转完读 DWG 图幅。
          if (local.kind === 'unsupported') {
            // 中间态用「其他文件」上传（这是唯一不要求先有零件归属的上传路径）；
            // 记下它的本地 id，后面确定图号后要么落成零件、要么按 id 撤掉这个中间态。
            const staged: DrawingFile = { ...baseFile(file), role: 'other', drawingNo: rootNo, version: 'v1.0' }
            const ids = await drawingOperationsStore.uploadOtherFile(rootNo, staged, file)
            const attachment = { ...ids, fileId: staged.id }
            uploadedOther += 1
            if (!attachment.attachmentId) {
              failures.push(`${file.name}：上传后未返回附件标识，无法读取图幅`)
              continue
            }
            const conversion = await waitForConversion([attachment.attachmentId], {
              onProgress: (done, total) => { if (total) uiStore.toast(`正在转换 ${file.name}（${done}/${total}）`, 'info') },
            })
            const failed = conversion.failed[0]
            if (failed) {
              failures.push(`${file.name}：图纸转换失败（${failed.error}）`)
              continue
            }
            if (conversion.pending.length) {
              failures.push(`${file.name}：图纸仍在转换中，稍后可在文件列表用「重新识别图号」补齐`)
              continue
            }
            try {
              const snapshot = await extractAndSaveTitleBlock(attachment.attachmentId)
              const partNo = partNoFromTitleSnapshot(snapshot.payload)
              if (partNo) {
                resolved.push({ file, partNo, attachment })
                // 中间态已计入其他文件；确定归属后会被落成零件，不再重复统计。
                uploadedOther -= 1
                continue
              }
              manualRows.push({ id: key, name: file.name, candidates: [], reason: '已转换并读取图幅，但图幅里没有图号' })
              manualFiles.set(key, file)
              manualAttachments.set(key, attachment)
            } catch (error: unknown) {
              failures.push(`${file.name}：读取图幅失败（${error instanceof Error ? error.message : '未知原因'}）`)
            }
            continue
          }

          manualRows.push({ id: key, name: file.name, candidates: [], reason: manualReason(local) })
          manualFiles.set(key, file)
        } catch (error: unknown) {
          // 单个文件失败不放弃整批：与批量校正一致，逐文件收集原因。
          failures.push(`${file.name}：${error instanceof Error ? error.message : '处理失败'}`)
        }
      }

      // 阶段二：兜底手工补录。用户只填后几位，项目号自动补上；取消则整批不落库。
      if (manualRows.length) {
        const entries = await askPartNumbers(manualRows)
        if (!entries) {
          await drawingStore.refresh()
          uiStore.toast(
            uploadedOther
              ? `已取消图号补录；${uploadedOther} 个已上传文件暂保留为「其他文件」`
              : '已取消图号补录',
            'warn',
          )
          return
        }
        for (const entry of entries) {
          const file = manualFiles.get(entry.id)
          if (!file) continue
          const attachment = manualAttachments.get(entry.id) ?? null
          const partNo = entry.partNo.trim()
          // 留空 = 跳过：中间态已经是「其他文件」，据实计入；没有中间态就什么都不做。
          if (!partNo) {
            if (attachment) { otherCount += 1; uploadedOther -= 1 }
            continue
          }
          resolved.push({ file, partNo, attachment })
        }
      }

      // 阶段三：按图号决定落库路径。规则仍是 planPartUpload，只换了图号来源。
      for (const item of resolved) {
        try {
          const plan = planPartUpload({
            rootDrawingNo: rootNo,
            identity: { partNo: item.partNo },
            parts: drawingStore.parts,
            drawingNos: drawingStore.drawings.map((drawing) => drawing.no),
          })
          const newFile: DrawingFile = {
            ...baseFile(item.file),
            role: plan.role,
            drawingNo: rootNo,
            ...(plan.partNo ? { partNo: plan.partNo } : {}),
            version: 'v1.0',
          }

          // EXB 图号已在中间态上传时定下：直接落成零件，避免同一文件上传两次。
          // 借用件例外——借用关系只能由建档路径写入，这里的 reidentify 只建自有零件。
          if (item.attachment && plan.kind === 'create-part' && !plan.borrowFrom) {
            const staged = { ...newFile, id: item.attachment.attachmentId, storageKey: item.attachment.storageKey }
            await drawingOperationsStore.reidentifyDrawingFile(staged, plan.partNo)
            createdCount += 1
            continue
          }
          if (item.attachment) {
            // 不是「新建自有零件」：撤掉中间态，回到统一的建档路径，保证借用/挂靠语义一致。
            await drawingOperationsStore.deleteOtherFile(rootNo, item.attachment.fileId).catch(() => undefined)
          }

          if (plan.kind === 'other-file') {
            await drawingOperationsStore.uploadOtherFile(rootNo, newFile, item.file)
            otherCount += 1
            continue
          }

          if (plan.kind === 'attach-to-existing-part') {
            const existingPart = drawingStore.parts.find((part) => part.no === plan.partNo && part.parentNo === rootNo)
            if (existingPart && plan.material !== '—') existingPart.material = plan.material
            await drawingOperationsStore.uploadDrawingFile(plan.partNo, newFile, item.file)
            createdCount += 1
            continue
          }

          await drawingOperationsStore.createPartWithFile(plan.parentNo, {
            no: plan.partNo,
            name: item.file.name.replace(/\.[^/.]+$/, ''),
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
          }, newFile, item.file)
          createdCount += 1
        } catch (error: unknown) {
          failures.push(`${item.file.name}：${error instanceof Error ? error.message : '落库失败'}`)
        }
      }

      await drawingStore.refresh()

      const summary = `已整理 ${createdCount} 个规范零件文件${otherCount ? `，${otherCount} 个文件归入其他文件` : ''}`
      if (failures.length) uiStore.toast(`${summary}；${failures.length} 个未处理：${failures.join('；')}`, 'warn')
      else uiStore.toast(summary, 'ok')
    } catch (error: unknown) {
      console.error('上传零件图失败', error)
      uiStore.toast(`上传零件图失败：${error instanceof Error ? error.message : '未知原因'}`, 'warn')
    } finally {
      partNumberBusy.value = false
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
    partNumberRows,
    partNumberOpen,
    partNumberBusy,
    confirmPartNumbers,
    cancelPartNumbers,
  }
}
