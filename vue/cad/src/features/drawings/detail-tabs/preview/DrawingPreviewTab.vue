<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { Drawing, DrawingFile, StructurePart } from '@/types/domain.types'
import { parseDrawingFileName, parseStandaloneDrawingFileName } from '@/utils/drawing-number-parser'

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

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function openBrowse(file: DrawingFile) {
  if (!currentItem.value) return
  router.push({
    name: 'drawing-viewer',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
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
    uploadedAt: '刚刚',
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
      const currentProjectParsed = parseDrawingFileName(file.name, rootNo)
      const standaloneParsed = parseStandaloneDrawingFileName(file.name)
      const parsed = currentProjectParsed.isStandard ? currentProjectParsed : standaloneParsed
      const isBorrowed = standaloneParsed.isStandard && standaloneParsed.rootNo !== rootNo
      const isStructuredPart = parsed.isStandard && (parsed.level > 0 && parsed.rootNo === rootNo || isBorrowed)
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
        uploadedAt: '刚刚',
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
          name: parsed.name || cleanName,
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
           ...(isBorrowed ? { borrowFrom: standaloneParsed.rootNo ?? standaloneParsed.no } : {}),
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
      <div class="card-title">
        <DemoIcon name="file-text" :size="16" />
        已关联图纸文件清单 ({{ allFiles.length }})
        <span class="hint">支持 DWG / DXF / EXB / PDF / STEP</span>
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
              <th style="width: 140px; text-align: right">操作</th>
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
              <td class="num">{{ file.version }}</td>
              <td>{{ file.uploadedBy }}</td>
              <td class="num updated">{{ file.uploadedAt }}</td>
              <td class="row-actions" style="text-align: right">
                <button class="btn sm primary" type="button" @click="openBrowse(file)">
                  <DemoIcon name="eye" :size="13" />浏览
                </button>
                <button class="btn sm danger" type="button" @click="handleDeleteFile(file)">
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
  </div>
</template>

<style scoped>
.drawing-preview-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
  overflow: visible;
}

.table-pad {
  padding: 10px 14px;
  max-height: none;
  overflow: auto;
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
  overflow-wrap: anywhere;
  line-height: 1.4;
}

.row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
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
