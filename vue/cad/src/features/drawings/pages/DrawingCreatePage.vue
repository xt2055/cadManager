<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import CategoryTreeSelect from '@/components/common/CategoryTreeSelect.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import { dataManager } from '@/services/data-manager'
import type { Drawing, DrawingFile, StructurePart } from '@/types/domain.types'
import { directParentDrawingNo, isEquivalentAssemblyNo, isSameDrawingFamily, parseDrawingNumber, parseStandaloneDrawingFileName } from '@/utils/drawing-number-parser'

defineOptions({
  name: 'DrawingCreatePage',
})

const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()

const formProject = ref('')
const formProjectNo = ref('')
const formDrawingNo = ref('')
const formRemark = ref('')
const formCategoryId = ref('')
const isIdentifyingAssembly = ref(false)
const assemblyIdentifyMessage = ref('')
let assemblyIdentifySequence = 0

const createMode = ref<'blank' | 'fork'>('blank')
const selectedForkSourceNo = ref('')

const existingDrawings = computed(() => domainStore.drawings)

function onForkSourceChange() {
  const source = domainStore.drawings.find((item) => item.no === selectedForkSourceNo.value)
  if (!source) return
  // 继承源项目的基本信息（均可修改）；总图图号预填源号，提交前必须改成新号。
  formProject.value = `${source.name} (改进版)`
  formProjectNo.value = source.project || ''
  formDrawingNo.value = source.no
  formRemark.value = `分叉自 ${source.no} · 继承图纸、备料与工艺文件及零件结构`
  formCategoryId.value = source.categoryId ?? ''
}

interface UploadedAssembly {
  name: string
  size: string
  file?: File
}

interface UploadedPart {
  id: string
  name: string
  size: string
  file?: File
}

const assemblyFile = ref<UploadedAssembly | null>(null)
const partFiles = ref<UploadedPart[]>([])
const isDraggingAssembly = ref(false)
const isDraggingParts = ref(false)
const isCreating = ref(false)
const createStatus = ref('正在准备创建')

const assemblyFileInput = ref<HTMLInputElement | null>(null)
const partFilesInput = ref<HTMLInputElement | null>(null)
const partFolderInput = ref<HTMLInputElement | null>(null)

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function handleAssemblySelected(file: File) {
  assemblyFile.value = {
    name: file.name,
    size: formatFileSize(file.size),
    file,
  }
  const sequence = ++assemblyIdentifySequence
  const fileNameIdentity = parseStandaloneDrawingFileName(file.name)
  formDrawingNo.value = ''
  assemblyIdentifyMessage.value = '正在读取总图标题栏中的真实图号…'
  isIdentifyingAssembly.value = true
  if (!formProject.value) {
    formProject.value = fileNameIdentity.name || file.name.replace(/\.[^/.]+$/, '')
  }
  if (!formProjectNo.value) {
    formProjectNo.value = projectNoFromFile(file)
  }
  if (file && supportsDrawingNumberIdentification(file.name)) {
    try {
      const identity = await dataManager.identifyDrawingFile(file, file.name, { titleBlockOnly: true })
      if (sequence !== assemblyIdentifySequence) return
      if (identity.partNoSource !== 'titleBlock' || !identity.partNo.trim()) {
        assemblyIdentifyMessage.value = '标题栏未识别到总图图号，请核对图纸后手动填写。'
      } else {
        formDrawingNo.value = identity.partNo.trim()
        assemblyIdentifyMessage.value = '已从总图标题栏读取真实图号。'
      }
    } catch (error) {
      if (sequence !== assemblyIdentifySequence) return
      assemblyIdentifyMessage.value = error instanceof Error ? error.message : '读取总图标题栏失败，请手动填写总图图号。'
    } finally {
      if (sequence === assemblyIdentifySequence) isIdentifyingAssembly.value = false
    }
  } else {
    isIdentifyingAssembly.value = false
    assemblyIdentifyMessage.value = '当前文件不是可读取标题栏的 CAD 格式，请手动填写总图图号。'
  }
  uiStore.toast(`总图 ${file.name} 已选择，现可继续添加零件图`, 'ok')
}

function onAssemblyChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (file) {
    void handleAssemblySelected(file)
  }
}

function onAssemblyDrop(event: DragEvent) {
  isDraggingAssembly.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) {
    void handleAssemblySelected(file)
  }
}

function appendPartFiles(fileList: FileList | null) {
  if (!fileList || !fileList.length) return
  const added: UploadedPart[] = []
  for (let i = 0; i < fileList.length; i++) {
    const file = fileList[i]
    if (!file) continue
    const exists = partFiles.value.some((p) => p.name === file.name)
    if (!exists) {
      added.push({
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        name: file.name,
        size: formatFileSize(file.size),
        file,
      })
    }
  }
  partFiles.value.push(...added)
  uiStore.toast(`已加入 ${added.length} 个零件图文件`, 'ok')
}

function onPartsChange(event: Event) {
  const target = event.target as HTMLInputElement
  appendPartFiles(target.files)
}

function onFolderChange(event: Event) {
  const target = event.target as HTMLInputElement
  appendPartFiles(target.files)
}

function onPartsDrop(event: DragEvent) {
  isDraggingParts.value = false
  appendPartFiles(event.dataTransfer?.files ?? null)
}

function removeAssembly() {
  assemblyFile.value = null
  partFiles.value = []
  uiStore.toast('已移除总图文件，关联的零件图已重置', 'warn')
}

function removePart(id: string) {
  partFiles.value = partFiles.value.filter((p) => p.id !== id)
}

function clearAllParts() {
  partFiles.value = []
}

function supportsDrawingNumberIdentification(name: string): boolean {
  const extension = name.slice(name.lastIndexOf('.')).toLowerCase()
  return ['.exb', '.dwg', '.dxf'].includes(extension)
}

function projectNoFromFile(file: File): string {
  const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || ''
  const pathParts = relativePath.split(/[\\/]/).filter(Boolean)
  const candidates = [...pathParts, file.name]

  // 优先读取路径或文件名中的明确项目号，例如 PRJ-2026-782。
  for (const candidate of candidates) {
    const baseName = candidate.replace(/\.[^/.]+$/, '').trim()
    const projectMatch = baseName.match(/^(PRJ[-_]\d+(?:[-_][A-Za-z0-9]+)*)/i)
    if (projectMatch?.[1]) return projectMatch[1]
  }

  // 没有 PRJ 前缀时，项目号仍取文件名中的工程编号；它可以与总图图号字符串相同，语义由字段区分。
  const fileBaseName = file.name.replace(/\.[^/.]+$/, '').trim()
  const fileIdentity = parseStandaloneDrawingFileName(file.name)
  if (fileIdentity.isStandard) return fileIdentity.no
  return fileBaseName.split(/[（(【\[]/, 1)[0]?.trim() || ''
}

function isDetailListFile(name: string): boolean {
  return /密封件|外购件/.test(name) && name.includes('明细表')
}

function triggerAssemblyPick() {
  assemblyFileInput.value?.click()
}

function triggerPartsPick() {
  partFilesInput.value?.click()
}

function triggerFolderPick() {
  partFolderInput.value?.click()
}

function handleCancel() {
  router.push({ name: 'drawing-library' })
}

function handleSubmit() {
  if (isCreating.value) return
  isCreating.value = true
  createStatus.value = '正在准备创建'
  void performCreate().finally(() => {
    isCreating.value = false
    createStatus.value = '正在准备创建'
  })
}

async function performCreate() {
  const projectName = formProject.value.trim()
  if (!projectName) {
    uiStore.toast('请填写项目名称', 'warn')
    return
  }

  const projectNo = formProjectNo.value.trim()
  const drawingNo = formDrawingNo.value.trim()
  if (!projectNo) {
    uiStore.toast('请填写项目号；项目号只能来自总图文件名或用户手动输入', 'warn')
    return
  }
  if (isIdentifyingAssembly.value) {
    uiStore.toast('正在读取总图标题栏，请稍候再保存', 'warn')
    return
  }
  if (!drawingNo) {
    uiStore.toast('请填写总图图号；总图图号必须来自标题栏识别或用户核对后的手动输入', 'warn')
    return
  }

  if (createMode.value === 'fork') {
    if (!selectedForkSourceNo.value) {
      uiStore.toast('请选择要分叉的源图纸', 'warn')
      return
    }
    if (selectedForkSourceNo.value === drawingNo) {
      uiStore.toast('分叉新图号不能与源图号相同', 'warn')
      return
    }
    try {
      createStatus.value = '正在继承源图纸结构与文件'
      await domainStore.forkDrawing(
        selectedForkSourceNo.value,
        drawingNo,
        projectName,
        undefined,
        formRemark.value.trim(),
        undefined,
        projectNo,
      )
      domainStore.openDrawing(drawingNo)
      uiStore.toast(`已基于「${selectedForkSourceNo.value}」成功分叉项目「${projectNo}」，总图图号为「${drawingNo}」`, 'ok')
      router.push({ name: 'drawing-preview', params: { drawingId: drawingNo } })
      return
    } catch (error) {
      console.error('分叉图纸失败', error)
      uiStore.toast(error instanceof Error ? error.message : '分叉图纸失败，请重试', 'warn')
      return
    }
  }

  const newProjectDrawing: Drawing = {
    no: drawingNo,
    name: projectName,
    kind: '总图',
    project: projectNo,
    material: '—',
    vendor: '内部项目部',
    status: 'draft',
    ver: 'v1.0',
    updated: '刚刚',
     by: '当前用户',
    borrow: 0,
    hasFile: Boolean(assemblyFile.value),
    ...(formCategoryId.value ? { categoryId: formCategoryId.value } : {}),
    signers: {},
  }

  const identifiedPartFiles: Array<{ part: UploadedPart; parsed: ReturnType<typeof parseDrawingNumber>; material: string }> = []
  try {
    for (const [index, part] of partFiles.value.entries()) {
      if (!part.file) throw new Error(`零件文件「${part.name}」缺少文件内容`)
      if (!supportsDrawingNumberIdentification(part.name) || isDetailListFile(part.name)) {
        identifiedPartFiles.push({
          part,
          parsed: parseDrawingNumber(''),
          material: '—',
        })
        continue
      }
      createStatus.value = `正在识别零件图号（${index + 1}/${partFiles.value.length}）`
      const identity = await dataManager.identifyDrawingFile(part.file, part.name)
      const parsed = parseDrawingNumber(identity.partNo)
      if (!parsed.isStandard) {
        // 没有可靠工程图号的标准件、明细表等文件保留为其他文件，不能臆造零件号。
        identifiedPartFiles.push({ part, parsed, material: identity.material || '—' })
        continue
      }
      identifiedPartFiles.push({
        part,
        parsed,
        material: identity.material || identity.titleBlock?.['材料名称'] || identity.titleBlock?.['材料'] || identity.titleBlock?.['材质'] || '—',
      })
    }
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '读取零件图号失败，请检查图纸标题栏', 'warn')
    return
  }

  const projectFamilyNo = drawingNo
  const parsedPartFiles = identifiedPartFiles.map(({ part, parsed, material }) => ({
    part,
    parsed,
    material,
    isBorrowed: parsed.isStandard && !isSameDrawingFamily(parsed.no, projectFamilyNo),
  }))
  const assemblyNos = [
    drawingNo,
  ].filter(Boolean)
  const structuredPartFiles = parsedPartFiles.filter(({ parsed }) => parsed.isStandard && !isEquivalentAssemblyNo(parsed.no, projectFamilyNo, assemblyNos))
  const validPartEntries = structuredPartFiles.map(({ part, parsed, isBorrowed, material }, index) => ({
    part,
    parsed,
    isBorrowed,
    material,
    file: {
      id: `${Date.now()}-part-${index}`,
      name: part.name,
      size: part.size,
      role: 'part' as const,
       drawingNo,
      partNo: parsed.no,
      version: 'v1.0',
      uploadedBy: '当前用户',
      uploadedAt: '刚刚',
      previewable: true,
    } satisfies DrawingFile,
  }))

  const groupedPartEntries = [...validPartEntries.reduce((groups, entry) => {
    const group = groups.get(entry.parsed.no) ?? []
    group.push(entry)
    groups.set(entry.parsed.no, group)
    return groups
  }, new Map<string, typeof validPartEntries>()).values()]
  const duplicatePartFileCount = validPartEntries.length - groupedPartEntries.length

  const partsForStructure: StructurePart[] = groupedPartEntries.map((entries) => {
    const firstEntry = entries[0]
    if (!firstEntry) throw new Error('零件文件分组为空')
    const cleanName = firstEntry.part.name.replace(/\.[^/.]+$/, '')
    const partNo = firstEntry.parsed.no
    const localPartNos = new Set(structuredPartFiles.map((entry) => entry.parsed.no))
    const directParentNo = directParentDrawingNo(partNo)
     const parentNo = directParentNo && localPartNos.has(directParentNo) ? directParentNo : drawingNo
    return {
      no: partNo,
      name: firstEntry.parsed.name || cleanName,
       parentNo: firstEntry.isBorrowed ? drawingNo : parentNo,
       project: projectNo,
      material: firstEntry.material || '—',
      spec: '',
      weight: 0,
      surfaceTreatment: '',
      partType: '自制件',
      qty: 1,
      status: 'draft',
      ver: 'v1.0',
      hasFile: true,
      signers: {},
      files: entries.map((entry) => entry.file),
      ...(firstEntry.isBorrowed
        ? { borrowFrom: firstEntry.parsed.rootNo ?? firstEntry.parsed.no }
        : {}),
    }
  })
  const otherFileEntries = parsedPartFiles
    .filter(({ parsed }) => !parsed.isStandard || isEquivalentAssemblyNo(parsed.no, projectFamilyNo, assemblyNos))
  const otherDrawingFiles: DrawingFile[] = otherFileEntries
    .map(({ part }, index) => ({
      id: `${Date.now()}-other-${index}`,
      name: part.name,
      size: part.size,
      role: 'other' as const,
       drawingNo,
      version: 'v1.0',
       uploadedBy: '当前用户',
      uploadedAt: '刚刚',
      previewable: true,
    }))

  const assemblyDrawingFile: DrawingFile | undefined = assemblyFile.value
    ? {
        id: `${Date.now()}-assembly`,
        name: assemblyFile.value.name,
        size: assemblyFile.value.size,
        role: 'assembly' as const,
         drawingNo,
        version: 'v1.0',
         uploadedBy: '当前用户',
        uploadedAt: '刚刚',
        previewable: true,
      }
    : undefined

  newProjectDrawing.files = assemblyDrawingFile ? [assemblyDrawingFile] : []
  newProjectDrawing.otherFiles = otherDrawingFiles

  const attachments = [
    ...(assemblyFile.value
      ? [{ id: assemblyDrawingFile?.id ?? '', content: assemblyFile.value.file }]
      : []),
    ...validPartEntries.map((entry) => ({ id: entry.file.id, content: entry.part.file })),
    ...otherFileEntries
       .filter(({ parsed, isBorrowed }) => !isBorrowed && (!parsed.isStandard || parsed.level <= 0 || parsed.rootNo !== drawingNo))
       .map(({ part }, index) => ({ id: otherDrawingFiles[index]?.id ?? '', content: part.file })),
  ]

  newProjectDrawing.remark = formRemark.value.trim()

  try {
    createStatus.value = '正在保存项目结构并上传图纸文件'
    await domainStore.addDrawing(newProjectDrawing, partsForStructure, attachments.filter((item): item is { id: string; content: File } => Boolean(item.id && item.content)))
  } catch (error) {
    console.error('保存新建图纸失败', error)
    uiStore.toast('项目创建失败，数据未能保存', 'warn')
    return
  }

   domainStore.openDrawing(newProjectDrawing.no)

  const borrowedPartCount = groupedPartEntries.filter((entries) => entries[0]?.isBorrowed).length
   uiStore.toast(`项目「${projectNo}」已成功创建，总图图号为「${drawingNo}」${borrowedPartCount ? `，${borrowedPartCount} 个借用组件已关联` : ''}${duplicatePartFileCount ? `，${duplicatePartFileCount} 个同图号文件已合并到对应零件` : ''}${otherDrawingFiles.length ? `，${otherDrawingFiles.length} 个文件归入其他文件` : ''}`, 'ok')
  router.push({ name: 'drawing-preview', params: { drawingId: newProjectDrawing.no } })
}
</script>

<template>
  <div class="page drawing-create-view">
    <div v-if="isCreating" class="create-loading-overlay" role="status" aria-live="polite">
      <div class="create-loading-card">
        <span class="create-spinner" aria-hidden="true"></span>
        <strong>正在创建图纸</strong>
        <span>{{ createStatus }}</span>
        <small>请勿关闭页面或重复点击</small>
      </div>
    </div>
    <div class="create-topbar">
      <div class="topbar-left">
        <button class="btn icon-only" type="button" title="返回图纸库" @click="handleCancel">
          <DemoIcon name="arrow-left" :size="16" />
        </button>
        <div>
          <h1 class="create-title">新建项目图纸</h1>
          <p class="create-subtitle">创建项目基础信息与图纸档案，支持先立项后补传，或即时上传总图及零件图。</p>
        </div>
      </div>
      <div class="topbar-actions">
        <button class="btn" type="button" :disabled="isCreating" @click="handleCancel">取消</button>
        <button class="btn primary" type="button" :disabled="isCreating" @click="handleSubmit">
          <span v-if="isCreating" class="button-spinner" aria-hidden="true"></span>
          <DemoIcon v-else name="check" :size="14" />{{ isCreating ? '创建中…' : '保存并创建' }}
        </button>
      </div>
    </div>

    <div class="create-content-grid">
      <div class="create-main-col">
        <section class="card form-section">
          <div class="section-head">
            <DemoIcon name="folder-plus" :size="16" />
            <h2>项目基本信息与创建模式</h2>
            <span class="section-tip">支持空白立项，或基于已有图纸完整分叉继承所有结构与元标签</span>
          </div>

          <div class="create-mode-selector">
            <label class="mode-option" :class="{ active: createMode === 'blank' }">
              <input v-model="createMode" type="radio" value="blank" />
              <span>新建空白图纸</span>
            </label>
            <label class="mode-option" :class="{ active: createMode === 'fork' }">
              <input v-model="createMode" type="radio" value="fork" />
              <span>从已有图纸分叉 (继承结构/零件/文件)</span>
            </label>
          </div>

          <div v-if="createMode === 'fork'" class="fork-source-row">
            <label for="fork-source-select">选择要分叉的源图纸 *</label>
            <select id="fork-source-select" v-model="selectedForkSourceNo" class="inp" @change="onForkSourceChange">
              <option value="">请选择已有图纸作为模板…</option>
              <option v-for="item in existingDrawings" :key="item.no" :value="item.no">
                {{ item.no }} · {{ item.name }} ({{ item.vendor || '内部项目部' }})
              </option>
            </select>
            <p class="fork-tip">分叉将继承源图纸的项目信息、零件结构以及全部图纸、备料与工艺文件，以下表单已预填、均可修改；提交前请将总图图号改为新号。</p>
          </div>

          <div class="form-grid">
            <div class="form-item required">
              <label for="create-project-name">项目名称</label>
              <input
                id="create-project-name"
                v-model="formProject"
                class="inp"
                placeholder="例如：智能回转减速传动装置"
              />
            </div>

            <div class="form-item required">
              <label for="create-project-no">项目号</label>
              <input
                id="create-project-no"
                v-model="formProjectNo"
                class="inp"
                placeholder="从总图文件名自动带入，也可手动修改"
              />
              <small class="field-help">用于项目分类和文件夹目录，不是总图图号。</small>
            </div>

            <div class="form-item required">
              <label for="create-drawing-no">总图图号</label>
              <input
                id="create-drawing-no"
                v-model="formDrawingNo"
                class="inp"
                :placeholder="isIdentifyingAssembly ? '正在读取标题栏…' : '从总图标题栏自动识别，也可核对后修改'"
                :disabled="isIdentifyingAssembly"
              />
              <small
                class="field-help"
                :class="{ error: (createMode === 'fork' && formDrawingNo === selectedForkSourceNo) || (assemblyIdentifyMessage && !formDrawingNo && !isIdentifyingAssembly) }"
              >
                {{ createMode === 'fork' && formDrawingNo === selectedForkSourceNo ? '分叉需要新的总图图号，请修改后再提交。' : assemblyIdentifyMessage || '总图图号来自图纸标题栏，不使用项目号代替。' }}
              </small>
            </div>

            <div class="form-item">
              <label for="create-remark">项目说明与备忘</label>
              <input
                id="create-remark"
                v-model="formRemark"
                class="inp"
                placeholder="填写项目背景、技术交底要求或交付期限等"
              />
            </div>
            <div class="form-item">
              <label for="create-category">图纸分类归属</label>
              <CategoryTreeSelect
                v-model="formCategoryId"
                placeholder="请选择或快捷新建分类（如 油缸 / 耳环安装 / 焊接工艺）"
                :allow-quick-create="true"
              />
            </div>
          </div>
        </section>
      </div>

      <div class="create-side-col">
        <section class="card form-section file-upload-section">
          <div class="section-head">
            <DemoIcon name="file-up" :size="16" />
            <h2>图纸文件上传</h2>
          </div>

          <input
            ref="assemblyFileInput"
            type="file"
            accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
            class="hidden-input"
            @change="onAssemblyChange"
          />

          <div
            class="upload-box assembly-box"
            :class="{ active: isDraggingAssembly, 'has-file': Boolean(assemblyFile) }"
            @dragover.prevent="isDraggingAssembly = true"
            @dragleave.prevent="isDraggingAssembly = false"
            @drop.prevent="onAssemblyDrop"
          >
            <template v-if="!assemblyFile">
              <div class="upload-icon-wrap">
                <DemoIcon name="layers" :size="28" />
              </div>
              <div class="upload-texts">
                <b>上传项目总图</b>
              <p>支持 .exb / .dwg / .dxf / .pdf · 单文件 ≤ 100MB；选择后读取标题栏图号</p>
              </div>
              <div class="upload-actions">
                <button class="btn sm primary" type="button" @click="triggerAssemblyPick">
                  <DemoIcon name="upload" :size="13" />选择总图文件
                </button>
              </div>
              <span class="upload-hint">也可以先跳过，立项后再补传</span>
            </template>

            <template v-else>
              <div class="file-picked-card">
                <div class="picked-main">
                  <div class="picked-icon">
                    <DemoIcon name="file-check-2" :size="20" />
                  </div>
                  <div class="picked-meta">
                    <div class="picked-name" :title="assemblyFile.name">{{ assemblyFile.name }}</div>
                    <div class="picked-sub">总图 · {{ assemblyFile.size }} · 已就绪</div>
                  </div>
                </div>
                <div class="picked-ops">
                  <button class="btn sm" type="button" @click="triggerAssemblyPick">重新选择</button>
                  <button class="btn sm danger" type="button" @click="removeAssembly">移除</button>
                </div>
              </div>
            </template>
          </div>

          <div class="parts-upload-container">
            <div class="parts-header">
              <div class="parts-title-wrap">
                <h3>零件图文件上传</h3>
                <span class="badge muted-badge">{{ partFiles.length }} 个</span>
              </div>
              <span class="parts-lock-tip">
                <DemoIcon name="info" :size="12" />可先跳过，立项后随时补传
              </span>
            </div>

            <input
              ref="partFilesInput"
              type="file"
              multiple
              accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
              class="hidden-input"
              @change="onPartsChange"
            />
            <input
              ref="partFolderInput"
              type="file"
              multiple
              webkitdirectory
              class="hidden-input"
              @change="onFolderChange"
            />

            <div
              class="upload-box parts-box"
              :class="{ active: isDraggingParts }"
              @dragover.prevent="isDraggingParts = true"
              @dragleave.prevent="isDraggingParts = false"
              @drop.prevent="onPartsDrop($event)"
            >
              <div class="upload-icon-wrap">
                <DemoIcon name="boxes" :size="24" />
              </div>
              <div class="upload-texts">
                <b>批量上传零件图（可选）</b>
                <p>支持多选多个零件文件或直接选择整目录文件夹</p>
              </div>
              <div class="upload-actions">
                <button class="btn sm" type="button" @click="triggerPartsPick">
                  <DemoIcon name="files" :size="13" />多选文件
                </button>
                <button class="btn sm" type="button" @click="triggerFolderPick">
                  <DemoIcon name="folder-up" :size="13" />选择文件夹
                </button>
              </div>
            </div>

            <div v-if="partFiles.length" class="parts-list-card">
              <div class="parts-list-head">
                <span>待关联零件 ({{ partFiles.length }})</span>
                <button class="text-btn danger" type="button" @click="clearAllParts">清空列表</button>
              </div>
              <div class="parts-list-body">
                <div v-for="part in partFiles" :key="part.id" class="part-item-row">
                  <DemoIcon name="file" :size="14" />
                  <span class="part-name" :title="part.name">{{ part.name }}</span>
                  <span class="part-size">{{ part.size }}</span>
                  <button class="icon-btn xs" type="button" title="移除" @click="removePart(part.id)">
                    <DemoIcon name="x" :size="12" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.drawing-create-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: calc(100vh - 120px);
  padding: 6px 4px 20px;
}

.create-loading-overlay {
  position: fixed;
  z-index: 50;
  inset: 0;
  display: grid;
  place-items: center;
  padding: 24px;
  background: color-mix(in srgb, var(--bg) 72%, transparent);
  backdrop-filter: blur(3px);
}

.create-loading-card {
  display: flex;
  min-width: 240px;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 26px 30px;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--panel);
  box-shadow: 0 18px 50px rgb(0 0 0 / 18%);
  color: var(--text-1);
  text-align: center;
}

.create-loading-card span:not(.create-spinner) {
  color: var(--text-2);
  font-size: 12px;
}

.create-loading-card small {
  color: var(--text-3);
  font-size: 11px;
}

.create-spinner,
.button-spinner {
  display: inline-block;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: create-spin 0.75s linear infinite;
}

.create-spinner {
  width: 28px;
  height: 28px;
  margin-bottom: 4px;
  color: var(--accent);
}

.button-spinner {
  width: 13px;
  height: 13px;
}

@keyframes create-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .create-spinner,
  .button-spinner {
    animation-duration: 1.5s;
  }
}

.create-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 4px 0 12px;
  border-bottom: 1px solid var(--line);
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.create-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 800;
}

.create-subtitle {
  margin: 3px 0 0;
  color: var(--text-3);
  font-size: 12px;
}

.topbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.create-content-grid {
  display: grid;
  grid-template-columns: 1.15fr 0.85fr;
  gap: 16px;
  align-items: start;
}

.create-main-col,
.create-side-col {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-section {
  padding: 20px;
}

.section-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--line);
}

.section-head svg {
  color: var(--accent);
}

.section-head h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}

.section-tip {
  margin-left: auto;
  color: var(--text-3);
  font-size: 11.5px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 14px;
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-item label {
  color: var(--text-2);
  font-size: 12px;
  font-weight: 500;
}

.form-item.required label::after {
  content: ' *';
  color: var(--danger);
}

.form-item:last-child {
  grid-column: span 2;
}

.create-mode-selector {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.mode-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--panel-2);
  cursor: pointer;
  font-size: 12.5px;
  color: var(--text-2);
  transition: all 0.2s ease;
}

.mode-option.active {
  border-color: var(--accent);
  color: var(--accent);
  background: var(--accent-soft);
  font-weight: 600;
}

.fork-source-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 16px;
  padding: 12px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line));
  background: color-mix(in srgb, var(--accent-soft) 40%, var(--panel-2));
}

.fork-source-row label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-1);
}

.fork-tip {
  margin: 0;
  font-size: 11px;
  color: var(--text-3);
  line-height: 1.5;
}

.hidden-input {
  display: none;
}

.upload-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 20px 16px;
  border: 1.5px dashed var(--line);
  border-radius: 12px;
  background: var(--panel-2);
  transition: all 0.25s;
}

.upload-box.active {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.upload-box.disabled {
  opacity: 0.55;
  filter: grayscale(0.2);
  cursor: not-allowed;
}

.upload-box.has-file {
  padding: 10px;
  border-style: solid;
  border-color: var(--accent);
  background: var(--panel);
}

.upload-icon-wrap {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  margin-bottom: 8px;
  border-radius: 50%;
  background: var(--panel);
  color: var(--accent);
}

.upload-texts b {
  font-size: 13px;
  color: var(--text-1);
}

.upload-texts p {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.field-help {
  display: block;
  margin-top: 5px;
  color: var(--text-3);
  font-size: 11px;
  line-height: 1.45;
}

.field-help.error {
  color: var(--danger);
}

.upload-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}

.upload-hint {
  margin-top: 8px;
  color: var(--text-3);
  font-size: 11px;
}

.file-picked-card {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px;
}

.picked-main {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.picked-icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  flex: none;
  border-radius: 8px;
  background: var(--accent-soft);
  color: var(--accent);
}

.picked-meta {
  min-width: 0;
  text-align: left;
}

.picked-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-1);
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.picked-sub {
  color: var(--text-3);
  font-size: 11px;
}

.picked-ops {
  display: flex;
  gap: 6px;
  flex: none;
}

.parts-upload-container {
  margin-top: 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.parts-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.parts-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}

.parts-title-wrap h3 {
  margin: 0;
  font-size: 13.5px;
  font-weight: 700;
}

.parts-lock-tip {
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--warn);
  font-size: 11px;
}

.parts-list-card {
  margin-top: 6px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  overflow: hidden;
}

.parts-list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid var(--line);
  background: var(--panel);
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-2);
}

.parts-list-body {
  max-height: 180px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

.part-item-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--line);
  font-size: 12px;
}

.part-item-row:last-child {
  border-bottom: none;
}

.part-item-row svg {
  color: var(--text-3);
  flex: none;
}

.part-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--text-1);
}

.part-size {
  color: var(--text-3);
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  flex: none;
}

.text-btn {
  border: none;
  background: transparent;
  padding: 0;
  cursor: pointer;
  font-size: 11px;
}

.text-btn.danger {
  color: var(--danger);
}

.text-btn.danger:hover {
  text-decoration: underline;
}

.icon-btn.xs {
  width: 20px;
  height: 20px;
  border-radius: 4px;
}

@media (max-width: 1024px) {
  .create-content-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .form-item:last-child {
    grid-column: span 1;
  }
}
</style>
