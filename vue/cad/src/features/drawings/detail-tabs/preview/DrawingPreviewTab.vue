<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { dataManager } from '@/services/data-manager'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { Drawing, DrawingFile, StructurePart } from '@/types/domain.types'
import { parseDrawingNumber } from '@/utils/drawing-number-parser'

defineOptions({
  name: 'DrawingPreviewTab',
})

const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()

const currentItem = computed(() => domainStore.currentDrawing)
const isAssembly = computed(() => !currentItem.value || !('parentNo' in currentItem.value))
const rootDrawingNo = computed(() => {
  if (!currentItem.value) return ''
  if (!('parentNo' in currentItem.value)) return currentItem.value.no

  let currentNo = currentItem.value.no
  const visited = new Set<string>()
  while (!visited.has(currentNo)) {
    visited.add(currentNo)
    const part = domainStore.structure.find((item) => item.no === currentNo)
    if (!part) return currentItem.value.parentNo
    if (domainStore.drawings.some((drawing) => drawing.no === part.parentNo)) return part.parentNo
    currentNo = part.parentNo
  }
  return currentItem.value.parentNo
})

// 汇聚当前对象关联的所有图纸文件
const allFiles = computed<DrawingFile[]>(() => {
  if (!currentItem.value) return []
  const files: DrawingFile[] = []

  // 如果是总图，递归汇总总图、零件和其他文件。
  if (isAssembly.value) {
    const mainDrawing = currentItem.value as Drawing
    files.push(...(mainDrawing.files ?? []), ...(mainDrawing.otherFiles ?? []))
    const belongsToDrawing = (part: StructurePart): boolean => {
      if (part.parentNo === mainDrawing.no || part.no.startsWith(`${mainDrawing.no}-`)) return true

      const visited = new Set<string>()
      let parentNo = part.parentNo
      while (parentNo && !visited.has(parentNo)) {
        if (parentNo === mainDrawing.no) return true
        visited.add(parentNo)
        const parent = domainStore.structure.find((candidate) => candidate.no === parentNo)
        if (!parent) return part.no.startsWith(`${mainDrawing.no}-`)
        parentNo = parent.parentNo
      }
      return false
    }

    domainStore.structure
      .filter(belongsToDrawing)
      .forEach((part) => files.push(...(part.files ?? []), ...(part.otherFiles ?? [])))
  } else {
    // 如果当前选中的就是零件图
    const part = currentItem.value as StructurePart
    files.push(...(part.files ?? []), ...(part.otherFiles ?? []))
  }

  return files.filter((file, index, sourceFiles) => sourceFiles.findIndex((candidate) => candidate.id === file.id) === index)
})

const hasAssemblyFile = computed(() => {
  if (isAssembly.value) {
    const mainDrawing = currentItem.value as Drawing
    return Boolean(mainDrawing?.files?.some((f) => f.role === 'assembly'))
  }
  return true // 零件图单独查看时
})

// 文件上传 input ref
const assemblyInput = ref<HTMLInputElement | null>(null)
const partInput = ref<HTMLInputElement | null>(null)
const replaceInput = ref<HTMLInputElement | null>(null)

// 替换弹窗与正在替换的目标文件
const isReplacing = ref(false)
const targetReplaceFile = ref<DrawingFile | null>(null)
const replaceReasonInput = ref('')
const selectedReplaceBlob = ref<File | null>(null)

// 借用零件弹窗与多维度智能选型系统
const isBorrowing = ref(false)
const isReidentifyingAll = ref(false)
const borrowSearchMode = ref<'by-project' | 'global-part'>('by-project')
const projectSearchQuery = ref('')
const partSearchQuery = ref('')
const selectedSourceProjectNo = ref('')
const selectedSourcePartNo = ref('')
const borrowReasonInput = ref('')

// 获取除当前项目外的所有可选项目（支持名称、图号、厂商模糊过滤）
const candidateProjects = computed(() => {
  const curNo = currentItem.value?.no || ''
  const q = projectSearchQuery.value.trim().toLowerCase()
  const list = domainStore.drawings.filter((d) => d.no !== curNo)
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
  return domainStore.drawings.find((d) => d.no === selectedSourceProjectNo.value) || null
})

// 根据模式和筛选条件获取零件列表
const candidateParts = computed(() => {
  const q = partSearchQuery.value.trim().toLowerCase()

  if (borrowSearchMode.value === 'global-part') {
    // 全库全局穿透搜索（排除当前项目自身的零件）
    const curNo = currentItem.value?.no || ''
    const allOtherParts = domainStore.structure.filter((p) => p.parentNo !== curNo)
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

  const parts = domainStore.structure.filter((p) => p.parentNo === pNo || p.no.startsWith(`${pNo}-`))
  if (!q) return parts

  return parts.filter((p) =>
    p.no.toLowerCase().includes(q) ||
    p.name.toLowerCase().includes(q) ||
    (p.material && p.material.toLowerCase().includes(q)) ||
    (p.spec && p.spec.toLowerCase().includes(q))
  )
})

// 计算各个项目的零件数量
function getProjectPartCount(pNo: string): number {
  return domainStore.structure.filter((p) => p.parentNo === pNo || p.no.startsWith(`${pNo}-`)).length
}

// 获取零件所属项目的名称
function getPartProjectName(part: StructurePart): string {
  const p = domainStore.drawings.find((d) => d.no === part.parentNo)
  return p ? p.name : (part.parentNo || '未知项目')
}

const selectedPartDetail = computed(() => {
  if (!selectedSourcePartNo.value) return null
  return domainStore.structure.find((p) => p.no === selectedSourcePartNo.value) || null
})

function openBorrowModal() {
  isBorrowing.value = true
  borrowSearchMode.value = 'by-project'
  projectSearchQuery.value = ''
  partSearchQuery.value = ''
  selectedSourceProjectNo.value = candidateProjects.value[0]?.no || ''
  selectedSourcePartNo.value = ''
  borrowReasonInput.value = ''
}

function cancelBorrowModal() {
  isBorrowing.value = false
  projectSearchQuery.value = ''
  partSearchQuery.value = ''
  selectedSourcePartNo.value = ''
}

function selectProject(projNo: string) {
  selectedSourceProjectNo.value = projNo
  selectedSourcePartNo.value = ''
}

async function confirmBorrowPart() {
  if (!selectedSourcePartNo.value || !currentItem.value) return
  const curNo = currentItem.value.no

  try {
    const borrowed = await domainStore.borrowPartToProject(
      curNo,
      selectedSourcePartNo.value,
      borrowReasonInput.value.trim() || '跨项目工程设计借用',
    )
    uiStore.toast(`成功借用零件「${borrowed.name} (${borrowed.no})」到当前项目`, 'ok')
    isBorrowing.value = false
    selectedSourcePartNo.value = ''
  } catch (err: any) {
    console.error('借用失败', err)
    uiStore.toast(err.message || '借用零件失败，请重试', 'warn')
  }
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatCurrentTime(): string {
  const current = new Date()
  const year = current.getFullYear()
  const month = String(current.getMonth() + 1).padStart(2, '0')
  const day = String(current.getDate()).padStart(2, '0')
  const hours = String(current.getHours()).padStart(2, '0')
  const minutes = String(current.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}`
}

function openBrowse(file: DrawingFile) {
  if (!currentItem.value) return
  router.push({
    name: 'drawing-viewer',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

function openEditor(file: DrawingFile) {
  if (!currentItem.value) return
  router.push({
    name: 'drawing-editor',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

function openHistory(file: DrawingFile) {
  if (!currentItem.value) return
  router.push({
    name: 'drawing-file-history',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

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
  if (!targetReplaceFile.value || !selectedReplaceBlob.value) return
  const cur = targetReplaceFile.value
  const targetNo = cur.partNo || cur.drawingNo || currentItem.value?.no || ''

  try {
    const updated = await domainStore.replaceDrawingFile(
      targetNo,
      cur.id,
      {
        name: selectedReplaceBlob.value.name,
        size: formatFileSize(selectedReplaceBlob.value.size),
        replaceReason: replaceReasonInput.value.trim() || '版本替换更新',
      },
      selectedReplaceBlob.value,
    )
    uiStore.toast(`文件已成功替换为 ${updated.version} · 历史版本已归档留痕`, 'ok')
    isReplacing.value = false
    targetReplaceFile.value = null
    selectedReplaceBlob.value = null
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

async function handleDeleteFile(file: DrawingFile) {
  if (!currentItem.value) return
  if (!window.confirm(`确定要删除图纸文件「${file.name}」吗？`)) return

  try {
    const targetNo = file.partNo || file.drawingNo || currentItem.value.no
    if (file.role === 'other') {
      await domainStore.deleteOtherFile(targetNo, file.id)
    } else {
      await domainStore.deleteDrawingFile(targetNo, file.id)
    }
    uiStore.toast(`已删除文件 ${file.name}`)
  } catch (error) {
    console.error('删除文件失败', error)
    uiStore.toast('删除文件失败，请重试', 'warn')
  }
}

function triggerUploadAssembly() {
  assemblyInput.value?.click()
}

function triggerUploadPart() {
  if (!hasAssemblyFile.value && isAssembly.value) {
    uiStore.toast('请先上传总图文件，再进行零件图上传', 'warn')
    return
  }
  partInput.value?.click()
}

async function onAssemblyFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file || !currentItem.value) return

  const newFile: DrawingFile = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    name: file.name,
    size: formatFileSize(file.size),
    role: 'assembly',
    drawingNo: currentItem.value.no,
    version: currentItem.value.ver || 'v1.0',
        uploadedBy: ('by' in (currentItem.value || {})) ? (currentItem.value as any).by : '当前用户',
    uploadedAt: formatCurrentTime(),
    previewable: true,
  }

  try {
    await domainStore.uploadDrawingFile(currentItem.value.no, newFile, file)
    uiStore.toast(`总图文件「${file.name}」上传成功`, 'ok')
    openBrowse(newFile)
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
  if (!files || !files.length || !currentItem.value) return

  let createdCount = 0
  let otherCount = 0
  try {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      if (!file) continue
      const cleanName = file.name.replace(/\.[^/.]+$/, '')
      const rootNo = rootDrawingNo.value
      const extension = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] || ''
      const isCadPart = extension === '.exb' || extension === '.dwg' || extension === '.dxf'
      if (!isCadPart) {
        const newFile: DrawingFile = {
          id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
          name: file.name,
          size: formatFileSize(file.size),
          role: 'other',
          drawingNo: rootNo,
          version: 'v1.0',
          uploadedBy: ('by' in currentItem.value ? currentItem.value.by : '当前用户') || '当前用户',
          uploadedAt: formatCurrentTime(),
          previewable: true,
        }
        await domainStore.uploadOtherFile(rootNo, newFile, file)
        otherCount += 1
        continue
      }

      const identity = await dataManager.identifyDrawingFile(file, file.name)
      const parsed = parseDrawingNumber(identity.partNo)
      const isBorrowed = parsed.rootNo !== rootNo
      const isStructuredPart = parsed.no !== rootNo
      const parentNo = isBorrowed ? rootNo : parsed.parentNo ?? rootNo
      const partNo = parsed.no

      const newFile: DrawingFile = {
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        name: file.name,
        size: formatFileSize(file.size),
        role: isStructuredPart ? 'part' : 'other',
        drawingNo: rootNo,
        ...(isStructuredPart ? { partNo } : {}),
        version: 'v1.0',
        uploadedBy: ('by' in currentItem.value ? currentItem.value.by : '当前用户') || '当前用户',
        uploadedAt: formatCurrentTime(),
        previewable: true,
      }

      const parentExists = Boolean(
        domainStore.drawings.some((drawing) => drawing.no === parentNo)
        || domainStore.structure.some((part) => part.no === parentNo),
      )
      const existingPart = partNo
        ? domainStore.structure.find((part) => part.no === partNo && part.parentNo === rootNo)
        : undefined

      if (!isStructuredPart || !parentExists) {
        await domainStore.uploadOtherFile(rootNo, newFile, file)
        otherCount += 1
      } else if (existingPart) {
        await domainStore.uploadDrawingFile(partNo, newFile, file)
        createdCount += 1
      } else {
        await domainStore.createPartWithFile(parentNo, {
          no: partNo,
          name: cleanName,
          parentNo,
          project: '',
          material: 'HT200',
          spec: '',
          weight: 0,
          surfaceTreatment: '',
          partType: '自制件',
          qty: 1,
          status: 'draft',
          ver: 'v1.0',
          hasFile: true,
           files: [newFile],
            ...(isBorrowed ? { borrowFrom: parsed.rootNo ?? parsed.no } : {}),
         }, newFile, file)
        createdCount += 1
      }
    }
    uiStore.toast(`已整理 ${createdCount} 个规范零件文件${otherCount ? `，${otherCount} 个文件归入其他文件` : ''}`, 'ok')
  } catch (error) {
    console.error('上传零件图失败', error)
    uiStore.toast('上传零件图失败', 'warn')
  } finally {
    target.value = ''
  }
}

// 批量识别校正弹窗状态
const isReidentifyModalOpen = ref(false)
const reidentifyList = ref<Array<{ file: DrawingFile; oldPartNo: string; newPartNo: string; checked: boolean }>>([])
const reidentifyFailures = ref<string[]>([])
const isExecutingReidentify = ref(false)

function isNonPartCadFile(fileName: string): boolean {
  const lower = fileName.toLowerCase()
  return (
    lower.includes('明细表') ||
    lower.includes('外购件') ||
    lower.includes('标准件') ||
    lower.includes('密封件') ||
    lower.includes('汇总表') ||
    lower.includes('目录') ||
    lower.includes('bom')
  )
}

async function reidentifyAllPartFiles() {
  if (isReidentifyingAll.value) return
  const currentRootNo = rootDrawingNo.value
  const cadFiles = allFiles.value.filter((file) => {
    const extension = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] || ''
    // 排除总图、非 CAD 以及明细表/BOM等表格文件
    return (
      file.role !== 'assembly' &&
      Boolean(file.storageKey) &&
      ['.exb', '.dwg', '.dxf'].includes(extension) &&
      !isNonPartCadFile(file.name)
    )
  })
  if (!cadFiles.length) {
    uiStore.toast('当前图纸没有可重新识别的零件 CAD 文件（已自动过滤明细表与表格）', 'warn')
    return
  }

  isReidentifyingAll.value = true
  try {
    const results: Array<{ file: DrawingFile; oldPartNo: string; newPartNo: string; checked: boolean }> = []
    const failures: string[] = []
    for (const file of cadFiles) {
      try {
        const content = await dataManager.readAttachment(file.storageKey as string)
        const identity = await dataManager.identifyDrawingFile(content, file.name)
        const identifiedNo = identity.partNo.trim()

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
      uiStore.toast(failures.length ? `没有发现图号变化，${failures.length} 个文件识别失败` : '所有零件图号均已与标题栏一致', failures.length ? 'warn' : 'ok')
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
        await domainStore.reidentifyDrawingFile(item.file, item.newPartNo)
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

function closeReidentifyModal() {
  if (isExecutingReidentify.value) return
  isReidentifyModalOpen.value = false
}
</script>

<template>
  <div class="drawing-preview-view">
    <!-- 隐藏式文件选择框 -->
    <input
      ref="assemblyInput"
      type="file"
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onAssemblyFileChange"
    />
    <input
      ref="partInput"
      type="file"
      multiple
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onPartFilesChange"
    />
    <input
      ref="replaceInput"
      type="file"
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onReplaceFileSelected"
    />

    <!-- 顶部操作栏：规范的上传总图 / 零件图入口 -->
    <div class="preview-actions-header card card-pad">
      <div class="header-info">
        <DemoIcon name="layers" :size="18" />
        <div>
          <h3>图纸文件管理与在线浏览</h3>
          <p>先上传总图建立主框架，后上传关联零件图；点击任意文件「浏览」开启矢量画布控制。</p>
        </div>
      </div>

      <div class="header-buttons">
        <button class="btn" type="button" title="从其他工程项目借用零件图及关联文件" @click="openBorrowModal">
          <DemoIcon name="share-2" :size="14" />借用零件
        </button>
        <button class="btn primary" type="button" @click="triggerUploadAssembly">
          <DemoIcon name="upload" :size="14" />上传总图文件
        </button>
        <button
          class="btn"
          :class="{ primary: hasAssemblyFile }"
          type="button"
          :disabled="!hasAssemblyFile && isAssembly"
          :title="!hasAssemblyFile && isAssembly ? '请先上传总图' : '上传零件图'"
          @click="triggerUploadPart"
        >
          <DemoIcon name="files" :size="14" />上传零件图
        </button>
      </div>
    </div>

    <!-- 文件总览列表 -->
    <div class="card files-table-card">
      <div class="card-title files-title-row">
        <DemoIcon name="file-text" :size="16" />
        已关联图纸文件清单 ({{ allFiles.length }})
        <span class="hint">支持 DWG / DXF / EXB / PDF / STEP</span>
        <button class="btn sm" type="button" :disabled="isReidentifyingAll" title="读取全部零件 CAD 文件标题栏并批量校正图号" @click="reidentifyAllPartFiles">
          <DemoIcon name="scan" :size="13" />
          {{ isReidentifyingAll ? '识别中...' : '全部重新识别图号' }}
        </button>
      </div>

      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>文件类型</th>
              <th>文件名</th>
              <th>关联图号</th>
              <th>文件大小</th>
              <th>版本</th>
              <th>上传人</th>
              <th>上传时间</th>
              <th style="width: 250px; text-align: right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="file in allFiles" :key="file.id">
              <td>
                <span class="tag" :class="file.role === 'assembly' ? 'plain' : file.role === 'other' ? 'mute' : 'info'">
                  {{ file.role === 'assembly' ? '项目总图' : file.role === 'other' ? '其他文件' : '零件图' }}
                </span>
              </td>
              <td class="file-name-cell">
                <DemoIcon name="file-check-2" :size="16" />
                <b>{{ file.name }}</b>
              </td>
              <td class="num mono">{{ file.partNo || file.drawingNo }}</td>
              <td class="num">{{ file.size }}</td>
              <td class="num"><span class="ver-badge">{{ file.version }}</span></td>
              <td>{{ file.uploadedBy }}</td>
              <td class="num updated">{{ file.uploadedAt }}</td>
              <td class="row-actions" style="text-align: right">
                <button class="btn sm primary" type="button" title="在线 CAD 矢量浏览" @click="openBrowse(file)">
                  <DemoIcon name="eye" :size="13" />浏览
                </button>
                <button class="btn sm" type="button" title="在线 CAD 编辑器" @click="openEditor(file)">
                  <DemoIcon name="edit" :size="13" />在线编辑
                </button>
                <button class="btn sm" type="button" title="替换当前图纸文件并生成新版本" @click="triggerReplace(file)">
                  <DemoIcon name="refresh-cw" :size="13" />替换
                </button>
                <button class="btn sm" type="button" title="查看该文件所有历史版本树与演进" @click="openHistory(file)">
                  <DemoIcon name="history" :size="13" />历史
                  <span v-if="file.history?.length" class="hist-count">{{ file.history.length }}</span>
                </button>
                <button class="btn sm danger" type="button" title="删除文件" @click="handleDeleteFile(file)">
                  <DemoIcon name="trash-2" :size="13" />删除
                </button>
              </td>
            </tr>
            <tr v-if="!allFiles.length">
              <td colspan="8">
                <div class="empty">
                  <DemoIcon name="file-up" :size="36" />
                  <div class="t">尚未上传任何图纸文件</div>
                  <p>请点击上方「上传总图文件」开始建立工程档案</p>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 批量重新识别并校正图号弹窗 -->
    <div v-if="isReidentifyModalOpen" class="modal-backdrop">
      <div class="modal card reidentify-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="scan" :size="18" />
            <span>批量校正零件图号</span>
          </div>
          <button class="btn sm close-btn" type="button" :disabled="isExecutingReidentify" @click="closeReidentifyModal">✕</button>
        </div>

        <div class="modal-body reidentify-modal-body">
          <div class="reidentify-hint">
            <DemoIcon name="info" :size="14" />
            <span>系统已从图纸内部标题栏读取到真实图号，并已自动过滤明细表和非零件图文件。请核对并勾选需校正的项：</span>
          </div>

          <div class="reidentify-table-wrap">
            <table class="tbl compact-tbl">
              <thead>
                <tr>
                  <th style="width: 40px; text-align: center">
                    <input
                      type="checkbox"
                      :checked="reidentifyList.length > 0 && reidentifyList.every((i) => i.checked)"
                      @change="reidentifyList.forEach((i) => (i.checked = ($event.target as HTMLInputElement).checked))"
                    />
                  </th>
                  <th>文件名</th>
                  <th>当前关联图号</th>
                  <th>识别图号 (标题栏)</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in reidentifyList" :key="item.file.id">
                  <td style="text-align: center">
                    <input v-model="item.checked" type="checkbox" />
                  </td>
                  <td class="file-name-cell">
                    <DemoIcon name="file" :size="14" />
                    <span>{{ item.file.name }}</span>
                  </td>
                  <td class="num mono text-muted">{{ item.oldPartNo }}</td>
                  <td class="num mono bold text-accent">
                    {{ item.newPartNo }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="reidentifyFailures.length" class="reidentify-fail-box">
            <div class="fail-title">
              <DemoIcon name="alert-triangle" :size="13" />
              <span>以下 {{ reidentifyFailures.length }} 个文件未能在标题栏中找到规范图号（已忽略）：</span>
            </div>
            <ul>
              <li v-for="(msg, idx) in reidentifyFailures" :key="idx">{{ msg }}</li>
            </ul>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" :disabled="isExecutingReidentify" @click="closeReidentifyModal">取消</button>
          <button class="btn primary" type="button" :disabled="isExecutingReidentify" @click="confirmBatchReidentify">
            <DemoIcon name="check" :size="14" />
            {{ isExecutingReidentify ? '校正中...' : `确认校正 (${reidentifyList.filter((i) => i.checked).length} 项)` }}
          </button>
        </div>
      </div>
    </div>

    <!-- 替换图纸确认与说明弹窗 -->
    <div v-if="isReplacing" class="modal-backdrop">
      <div class="modal card replace-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="refresh-cw" :size="18" />
            <span>替换图纸文件并生成新版本</span>
          </div>
          <button class="btn sm close-btn" type="button" @click="cancelReplace">✕</button>
        </div>

        <div class="modal-body">
          <div class="replace-meta-box">
            <div class="meta-row">
              <span class="lbl">原文件：</span>
              <span class="val mono bold">{{ targetReplaceFile?.name }}</span>
              <span class="tag info">{{ targetReplaceFile?.version || 'v1.0' }}</span>
            </div>
            <div class="meta-row">
              <span class="lbl">新文件：</span>
              <span class="val mono bold text-accent">{{ selectedReplaceBlob?.name }}</span>
              <span class="tag ok">({{ formatFileSize(selectedReplaceBlob?.size || 0) }})</span>
            </div>
          </div>

          <div class="field">
            <label class="bold">版本更新说明 / 替换原因</label>
            <input
              v-model="replaceReasonInput"
              type="text"
              class="inp"
              placeholder="例如：修改活塞密封槽倒角与公差，重新出图"
            />
          </div>

          <div class="note info-note">
            <DemoIcon name="shield-check" :size="15" />
            <div>替换将保留原文件所有图纸历史树与下载凭据，版本号自动递进，全程留痕。</div>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" @click="cancelReplace">取消</button>
          <button class="btn primary" type="button" @click="confirmReplace">
            <DemoIcon name="check" :size="14" />确认替换升级
          </button>
        </div>
      </div>
    </div>
    <!-- 借用其他项目零件弹窗 (支持千级项目/海量零件双模智能选型体系) -->
    <div v-if="isBorrowing" class="modal-backdrop">
      <div class="modal card borrow-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="share-2" :size="18" />
            <span>跨工程项目借用零件与图纸</span>
          </div>
          <div class="modal-mode-tabs">
            <button
              class="mode-tab-btn"
              :class="{ active: borrowSearchMode === 'by-project' }"
              type="button"
              @click="borrowSearchMode = 'by-project'"
            >
              <DemoIcon name="folder" :size="13" />按工程项目选型
            </button>
            <button
              class="mode-tab-btn"
              :class="{ active: borrowSearchMode === 'global-part' }"
              type="button"
              @click="borrowSearchMode = 'global-part'"
            >
              <DemoIcon name="search" :size="13" />全库全局穿透搜索
            </button>
          </div>
          <button class="btn sm close-btn" type="button" @click="cancelBorrowModal">✕</button>
        </div>

        <div class="modal-body borrow-modal-body">
          <!-- 模式 1：按工程项目三栏分级导航与选型（专为几千个项目设计） -->
          <div v-if="borrowSearchMode === 'by-project'" class="borrow-three-grid">
            <!-- 栏 1：项目库快速检索与选择 -->
            <div class="borrow-panel-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="folder" :size="13" />工程项目库 ({{ candidateProjects.length }})</span>
              </div>
              <div class="search-input-wrap compact">
                <DemoIcon name="search" :size="13" />
                <input
                  v-model="projectSearchQuery"
                  type="text"
                  class="inp filter-inp"
                  placeholder="搜索项目名称/图号/厂商..."
                />
              </div>
              <div class="scroll-select-list">
                <div
                  v-for="proj in candidateProjects"
                  :key="proj.no"
                  class="project-item-card"
                  :class="{ active: (selectedSourceProjectNo || candidateProjects[0]?.no) === proj.no }"
                  @click="selectProject(proj.no)"
                >
                  <div class="proj-card-title">{{ proj.name }}</div>
                  <div class="proj-card-meta">
                    <span class="mono">{{ proj.no }}</span>
                    <span class="tag tag-no-dot plain tag-xs">{{ getProjectPartCount(proj.no) }} 个零件</span>
                  </div>
                </div>
                <div v-if="!candidateProjects.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="20" />
                  <span>未找到匹配的项目</span>
                </div>
              </div>
            </div>

            <!-- 栏 2：当前项目下的零件列表 -->
            <div class="borrow-panel-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="file" :size="13" />可选零件清单 ({{ candidateParts.length }})</span>
                <span class="col-badge mono">{{ selectedProjectDetail?.no }}</span>
              </div>
              <div class="search-input-wrap compact">
                <DemoIcon name="filter" :size="13" />
                <input
                  v-model="partSearchQuery"
                  type="text"
                  class="inp filter-inp"
                  placeholder="过滤零件图号/名称/材质..."
                />
              </div>
              <div class="scroll-select-list">
                <div
                  v-for="part in candidateParts"
                  :key="part.no"
                  class="part-candidate-card"
                  :class="{ active: selectedSourcePartNo === part.no }"
                  @click="selectedSourcePartNo = part.no"
                >
                  <div class="part-card-head">
                    <span class="part-name">{{ part.name }}</span>
                    <span class="tag tag-no-dot mono tag-xs">{{ part.no }}</span>
                  </div>
                  <div class="part-card-sub">
                    <span>材质: {{ part.material || '—' }}</span>
                    <span>数量: {{ part.qty || 1 }}</span>
                    <span class="has-file-badge" :class="{ ok: part.files?.length }">
                      {{ part.files?.length ? `${part.files.length} 份图纸` : '无图纸' }}
                    </span>
                  </div>
                </div>
                <div v-if="!candidateParts.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="20" />
                  <span>该项目暂无匹配零件</span>
                </div>
              </div>
            </div>

            <!-- 栏 3：选中零件档案核对与借用理由 -->
            <div class="borrow-panel-col borrow-detail-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
              </div>

              <div v-if="selectedPartDetail" class="borrow-card-detail-content">
                <div class="preview-hero-card">
                  <DemoIcon name="file-check-2" :size="22" />
                  <div>
                    <h4>{{ selectedPartDetail.name }}</h4>
                    <span class="mono text-accent">{{ selectedPartDetail.no }}</span>
                  </div>
                </div>

                <div class="preview-spec-grid">
                  <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ selectedProjectDetail?.name }}</span></div>
                  <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPartDetail.partType }}</span></div>
                  <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPartDetail.material }} {{ selectedPartDetail.spec }}</span></div>
                  <div class="spec-row"><span class="k">单重/数量：</span><span class="v">{{ selectedPartDetail.weight ? `${selectedPartDetail.weight} kg` : '—' }} / {{ selectedPartDetail.qty }} 件</span></div>
                  <div class="spec-row"><span class="k">表面处理：</span><span class="v">{{ selectedPartDetail.surfaceTreatment || '—' }}</span></div>
                  <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPartDetail.files?.[0]?.name || '（无独立图纸文件）' }}</span></div>
                </div>

                <div class="field" style="margin-top: auto;">
                  <label class="bold">借用备注说明 / 选型原因</label>
                  <input
                    v-model="borrowReasonInput"
                    type="text"
                    class="inp"
                    placeholder="例如：复用成熟导向套设计，缩短加工周期"
                  />
                </div>
              </div>

              <div v-else class="empty-preview-prompt">
                <DemoIcon name="mouse-pointer-click" :size="32" />
                <p>请点击中间列表选择需要借用的零件</p>
              </div>
            </div>
          </div>

          <!-- 模式 2：全库全局穿透搜索（直击数万图纸） -->
          <div v-else class="borrow-two-grid">
            <div class="borrow-panel-col">
              <div class="search-input-wrap">
                <DemoIcon name="search" :size="15" />
                <input
                  v-model="partSearchQuery"
                  type="text"
                  class="inp global-search-inp"
                  placeholder="在企业全库中穿透搜索：输入图号如 04、活塞、HT200、或项目名称..."
                  autofocus
                />
              </div>

              <div class="scroll-select-list global-list" style="margin-top: 10px;">
                <div
                  v-for="part in candidateParts"
                  :key="part.no"
                  class="global-part-card"
                  :class="{ active: selectedSourcePartNo === part.no }"
                  @click="selectedSourcePartNo = part.no; selectedSourceProjectNo = part.parentNo"
                >
                  <div class="gp-top">
                    <span class="gp-name">{{ part.name }}</span>
                    <span class="mono gp-no">{{ part.no }}</span>
                    <span class="tag info tag-xs">{{ getPartProjectName(part) }}</span>
                  </div>
                  <div class="gp-btm">
                    <span>材质: {{ part.material || '—' }}</span>
                    <span>数量: {{ part.qty || 1 }}</span>
                    <span>图纸: {{ part.files?.[0]?.name || '无文件' }}</span>
                  </div>
                </div>
                <div v-if="!candidateParts.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="24" />
                  <span>全库中未找到匹配的零件，请尝试更简短的关键词</span>
                </div>
              </div>
            </div>

            <!-- 右侧详情 -->
            <div class="borrow-panel-col borrow-detail-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
              </div>

              <div v-if="selectedPartDetail" class="borrow-card-detail-content">
                <div class="preview-hero-card">
                  <DemoIcon name="file-check-2" :size="22" />
                  <div>
                    <h4>{{ selectedPartDetail.name }}</h4>
                    <span class="mono text-accent">{{ selectedPartDetail.no }}</span>
                  </div>
                </div>

                <div class="preview-spec-grid">
                  <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ getPartProjectName(selectedPartDetail) }}</span></div>
                  <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPartDetail.partType }}</span></div>
                  <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPartDetail.material }} {{ selectedPartDetail.spec }}</span></div>
                  <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPartDetail.files?.[0]?.name || '（无文件）' }}</span></div>
                </div>

                <div class="field" style="margin-top: auto;">
                  <label class="bold">借用备注说明</label>
                  <input
                    v-model="borrowReasonInput"
                    type="text"
                    class="inp"
                    placeholder="输入借用说明..."
                  />
                </div>
              </div>

              <div v-else class="empty-preview-prompt">
                <DemoIcon name="mouse-pointer-click" :size="32" />
                <p>请在搜索结果中点击选定要借用的零件</p>
              </div>
            </div>
          </div>

          <div class="note info-note" style="margin-top: 12px;">
            <DemoIcon name="shield-check" :size="14" />
            <div>系统将自动克隆图纸零件并挂载至当前工程，在借用记录台账中建立双向可追溯凭据，不污染源工程。</div>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" @click="cancelBorrowModal">取消</button>
          <button
            class="btn primary"
            type="button"
            :disabled="!selectedSourcePartNo"
            @click="confirmBorrowPart"
          >
            <DemoIcon name="check" :size="14" />确认借入此零件
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ver-badge {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
  font-weight: 700;
  color: var(--accent);
  background: var(--panel-2);
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid var(--line);
}

.hist-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10.5px;
  font-weight: 700;
  min-width: 16px;
  height: 16px;
  border-radius: 8px;
  background: var(--accent);
  color: #fff;
  padding: 0 4px;
  margin-left: 2px;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.replace-modal {
  width: 520px;
  max-width: 90vw;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 16px 36px rgba(0, 0, 0, 0.4);
  display: flex;
  flex-direction: column;
}

.reidentify-modal {
  width: 680px;
  max-width: 92vw;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.45);
  display: flex;
  flex-direction: column;
  max-height: 85vh;
}

.reidentify-modal-body {
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  min-height: 0;
}

.reidentify-hint {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12.5px;
  color: var(--text-2);
  line-height: 1.5;
  background: var(--panel-2);
  padding: 8px 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.reidentify-table-wrap {
  border: 1px solid var(--line);
  border-radius: 6px;
  overflow: hidden;
  max-height: 320px;
  overflow-y: auto;
}

.compact-tbl th,
.compact-tbl td {
  padding: 8px 10px;
  font-size: 12.5px;
}

.text-muted {
  color: var(--text-3);
}

.reidentify-fail-box {
  background: rgba(234, 179, 8, 0.08);
  border: 1px dashed rgba(234, 179, 8, 0.35);
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  color: var(--text-2);
}

.reidentify-fail-box .fail-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--warn, #eab308);
  margin-bottom: 6px;
}

.reidentify-fail-box ul {
  margin: 0;
  padding-left: 18px;
  color: var(--text-3);
  max-height: 80px;
  overflow-y: auto;
}

.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--line);
}

.modal-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-1);
}

.modal-title svg {
  color: var(--accent);
}

.close-btn {
  border: none;
  background: transparent;
  font-size: 14px;
  color: var(--text-3);
  cursor: pointer;
}

.close-btn:hover {
  color: var(--text-1);
}

.modal-body {
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.replace-meta-box {
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.meta-row .lbl {
  color: var(--text-3);
  min-width: 60px;
}

.meta-row .val {
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 260px;
}

.text-accent {
  color: var(--accent) !important;
}

.info-note {
  margin: 0;
}

.modal-foot {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 18px;
  border-top: 1px solid var(--line);
  background: var(--panel-2);
  border-bottom-left-radius: inherit;
  border-bottom-right-radius: inherit;
}

.files-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.files-title-row .hint {
  flex: 1;
  min-width: 180px;
}

.files-title-row .btn {
  flex: 0 0 auto;
  white-space: nowrap;
}

/* 借用零件高阶选型体系弹窗 */
.borrow-modal {
  width: 960px;
  max-width: 96vw;
  height: 640px;
  max-height: 92vh;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.modal-mode-tabs {
  display: flex;
  gap: 6px;
  background: var(--panel-2);
  padding: 3px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.mode-tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--text-3);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.mode-tab-btn:hover {
  color: var(--text-1);
}

.mode-tab-btn.active {
  background: var(--panel);
  color: var(--accent);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
}

.borrow-modal-body {
  flex: 1;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

/* 模式 1：三栏式自适应布局 */
.borrow-three-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 260px 310px 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

/* 模式 2：双栏穿透搜索布局 */
.borrow-two-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

.borrow-panel-col {
  display: flex;
  flex-direction: column;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 12px;
  min-height: 0;
  overflow: hidden;
}

.col-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.col-title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  display: flex;
  align-items: center;
  gap: 6px;
}

.col-title svg {
  color: var(--accent);
}

.col-badge {
  font-size: 11px;
  color: var(--text-3);
  background: var(--panel);
  padding: 1px 5px;
  border-radius: 3px;
  border: 1px solid var(--line);
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.compact {
  margin-top: 0 !important;
  margin-bottom: 8px;
}

.filter-inp {
  font-size: 12px !important;
  height: 30px !important;
  padding-left: 28px !important;
}

.global-search-inp {
  padding-left: 36px !important;
  height: 38px !important;
  font-size: 13px !important;
}

.scroll-select-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-right: 2px;
}

.project-item-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.project-item-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.project-item-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.proj-card-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.proj-card-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.part-candidate-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.part-candidate-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.part-candidate-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.part-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.part-name {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.part-card-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.has-file-badge {
  color: var(--text-3);
}

.has-file-badge.ok {
  color: var(--accent);
}

.global-part-card {
  padding: 10px 12px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.global-part-card:hover {
  background: var(--hover);
  border-color: var(--accent);
}

.global-part-card.active {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.gp-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.gp-name {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.gp-no {
  font-size: 12px;
  color: var(--text-2);
}

.gp-btm {
  display: flex;
  align-items: center;
  gap: 14px;
  font-size: 11.5px;
  color: var(--text-3);
}

.borrow-detail-col {
  background: var(--panel);
  padding: 14px;
}

.borrow-card-detail-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow-y: auto;
}

.preview-hero-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
}

.preview-hero-card svg {
  color: var(--accent);
}

.preview-hero-card h4 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.preview-spec-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--panel-2);
  padding: 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.spec-row {
  display: flex;
  align-items: center;
  font-size: 12.5px;
  gap: 6px;
}

.spec-row .k {
  color: var(--text-3);
  width: 75px;
  flex-shrink: 0;
}

.spec-row .v {
  color: var(--text-1);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
}

.empty-list-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px 10px;
  color: var(--text-3);
  gap: 6px;
  font-size: 12px;
}

.empty-preview-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--text-3);
  gap: 8px;
  font-size: 13px;
}

@media (max-width: 900px) {
  .borrow-three-grid {
    grid-template-columns: 1fr;
  }
  .borrow-two-grid {
    grid-template-columns: 1fr;
  }
}




<style scoped>
.drawing-preview-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}

.drawing-preview-view > * {
  max-width: 100%;
  min-width: 0;
}

.hidden-file-input {
  display: none;
}

.preview-actions-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.header-info {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-info svg {
  color: var(--accent);
}

.header-info h3 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.header-info p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.header-buttons {
  display: flex;
  gap: 10px;
}

.files-table-card {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}

.files-table-card .tbl {
  width: 100%;
  min-width: 0;
  table-layout: fixed;
}

.files-table-card .tbl th,
.files-table-card .tbl td {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.files-table-card .tbl th:nth-child(1),
.files-table-card .tbl td:nth-child(1) {
  width: 10%;
}

.files-table-card .tbl th:nth-child(2),
.files-table-card .tbl td:nth-child(2) {
  width: 25%;
}

.files-table-card .tbl th:nth-child(3),
.files-table-card .tbl td:nth-child(3) {
  width: 14%;
}

.files-table-card .tbl th:nth-child(4),
.files-table-card .tbl td:nth-child(4) {
  width: 9%;
}

.files-table-card .tbl th:nth-child(5),
.files-table-card .tbl td:nth-child(5) {
  width: 8%;
}

.files-table-card .tbl th:nth-child(6),
.files-table-card .tbl td:nth-child(6) {
  width: 10%;
}

.files-table-card .tbl th:nth-child(7),
.files-table-card .tbl td:nth-child(7) {
  width: 14%;
}

.table-pad {
  width: 100%;
  min-width: 0;
  padding: 10px 14px;
  max-height: none;
  overflow-x: hidden;
  overflow-y: hidden;
}

.actions-heading {
  width: 10%;
  min-width: 0;
  text-align: right !important;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 260px;
}

.file-name-cell svg {
  color: var(--accent);
}

.file-name-cell b {
  display: block;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
}

.row-actions {
  display: flex;
  position: relative;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  width: 100%;
  min-width: 0;
  white-space: nowrap;
  text-align: right;
}

.row-actions .btn {
  width: 28px;
  min-width: 28px;
  height: 28px;
  padding: 5px;
  gap: 0;
  font-size: 0;
  white-space: nowrap;
  flex: 0 0 auto;
}

.row-actions .btn :deep(svg) {
  width: 14px;
  height: 14px;
}

.row-actions .hist-count {
  position: absolute;
  margin-left: 16px;
  margin-top: -15px;
  min-width: 13px;
  padding: 1px 3px;
  border-radius: 99px;
  background: var(--accent);
  color: var(--accent-ink);
  font-size: 9px;
  line-height: 12px;
  text-align: center;
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  color: var(--text-3);
  gap: 8px;
}

.empty .t {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-2);
}

.empty p {
  font-size: 12px;
  margin: 0;
}
</style>
