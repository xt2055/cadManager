<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'
import type { Drawing, DrawingFile, StructurePart } from '@/types/demo.types'

defineOptions({
  name: 'DrawingCreatePage',
})

const router = useRouter()
const demoStore = useDemoStore()
const uiStore = useUiStore()

const formProject = ref('')
const formProjectNo = ref('')
const formVendor = ref('')
const formRemark = ref('')

const signers = ref<Array<{ role: string; user: string; required: boolean }>>([
  { role: '设计', user: '张工', required: true },
  { role: '校对', user: '王工', required: true },
  { role: '审核', user: '李工', required: true },
  { role: '工艺', user: '刘工', required: true },
  { role: '标准化', user: '孙工', required: false },
  { role: '批准', user: '赵总', required: true },
])

const optionalCandidateUsers = ['张工', '王工', '李工', '刘工', '孙工', '赵总', '周工', '钱工']

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

const assemblyFileInput = ref<HTMLInputElement | null>(null)
const partFilesInput = ref<HTMLInputElement | null>(null)
const partFolderInput = ref<HTMLInputElement | null>(null)

const canUploadParts = computed(() => Boolean(assemblyFile.value))

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function handleAssemblySelected(file: File) {
  assemblyFile.value = {
    name: file.name,
    size: formatFileSize(file.size),
    file,
  }
  if (!formProject.value) {
    formProject.value = file.name.replace(/\.[^/.]+$/, '')
  }
  uiStore.toast(`总图 ${file.name} 已选择，现可继续添加零件图`, 'ok')
}

function onAssemblyChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (file) {
    handleAssemblySelected(file)
  }
}

function onAssemblyDrop(event: DragEvent) {
  isDraggingAssembly.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) {
    handleAssemblySelected(file)
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

async function handleSubmit() {
  const projectName = formProject.value.trim()
  if (!projectName) {
    uiStore.toast('请填写项目名称', 'warn')
    return
  }

  const generatedNo = formProjectNo.value.trim() || `PRJ-${new Date().getFullYear()}-${Math.floor(100 + Math.random() * 900)}`

  const signerMap: Record<string, string> = {}
  signers.value.forEach((item) => {
    signerMap[item.role] = item.user
  })

  const newProjectDrawing: Drawing = {
    no: generatedNo,
    name: projectName,
    kind: '总图',
    project: projectName,
    material: '—',
    vendor: formVendor.value.trim() || '内部项目部',
    status: 'draft',
    ver: 'v1.0',
    updated: '刚刚',
    by: signers.value[0]?.user ?? '当前用户',
    borrow: 0,
    hasFile: Boolean(assemblyFile.value),
    signers: signerMap,
  }

  const partsForStructure: StructurePart[] = partFiles.value.map((part, index) => {
    const cleanName = part.name.replace(/\.[^/.]+$/, '')
    return {
      no: `${generatedNo}-${String(index + 1).padStart(2, '0')}`,
      name: cleanName,
      material: 'HT200',
      qty: 1,
      status: 'draft',
      ver: 'v1.0',
      hasFile: true,
    }
  })

  const files: DrawingFile[] = [
    ...(assemblyFile.value
      ? [{ name: assemblyFile.value.name, size: assemblyFile.value.size, role: 'assembly' as const }]
      : []),
    ...partFiles.value.map((part, index) => ({
      name: part.name,
      size: part.size,
      role: 'part' as const,
      partNo: partsForStructure[index]?.no,
    })),
  ]

  newProjectDrawing.remark = formRemark.value.trim()
  newProjectDrawing.files = files

  try {
    await demoStore.addDrawing(newProjectDrawing, partsForStructure)
  } catch (error) {
    console.error('保存新建图纸失败', error)
    uiStore.toast('项目创建失败，数据未能保存', 'warn')
    return
  }

  demoStore.openDrawing(newProjectDrawing.no)

  uiStore.toast(`项目「${projectName}」已成功创建并保存`, 'ok')
  router.push({ name: 'drawing-library' })
}
</script>

<template>
  <div class="page drawing-create-view">
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
        <button class="btn" type="button" @click="handleCancel">取消</button>
        <button class="btn primary" type="button" @click="handleSubmit">
          <DemoIcon name="check" :size="14" />保存并创建
        </button>
      </div>
    </div>

    <div class="create-content-grid">
      <div class="create-main-col">
        <section class="card form-section">
          <div class="section-head">
            <DemoIcon name="folder-plus" :size="16" />
            <h2>项目基本信息</h2>
            <span class="section-tip">项目级属性定义（零件材料等专属属性在零件详情页维护）</span>
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

            <div class="form-item">
              <label for="create-project-no">项目 / 图纸编号</label>
              <input
                id="create-project-no"
                v-model="formProjectNo"
                class="inp"
                placeholder="留空自动按 PRJ-2026-xxx 规则生成"
              />
            </div>

            <div class="form-item">
              <label for="create-vendor">承制厂商 / 责任单位</label>
              <input
                id="create-vendor"
                v-model="formVendor"
                class="inp"
                placeholder="例如：华辰重工 / 研发二组"
              />
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
          </div>
        </section>

        <section class="card form-section">
          <div class="section-head">
            <DemoIcon name="users" :size="16" />
            <h2>签署与审批人员</h2>
            <span class="section-tip">规范清晰的人员矩阵，确保图纸审批流转可追溯</span>
          </div>

          <div class="signers-layout">
            <div
              v-for="item in signers"
              :key="item.role"
              class="signer-card"
              :class="{ required: item.required }"
            >
              <div class="signer-role-wrap">
                <span class="signer-badge">{{ item.role }}</span>
                <span v-if="item.required" class="role-tag-req">必需</span>
                <span v-else class="role-tag-opt">可选</span>
              </div>
              <div class="signer-select-wrap">
                <select v-model="item.user" class="inp signer-select">
                  <option v-for="user in optionalCandidateUsers" :key="user" :value="user">
                    {{ user }}
                  </option>
                  <option value="待定">待定（稍后指定）</option>
                </select>
              </div>
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
                <p>支持 .exb / .dwg / .dxf / .pdf · 单文件 ≤ 100MB</p>
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
              <span v-if="!canUploadParts" class="parts-lock-tip">
                <DemoIcon name="lock" :size="12" />请先上传总图以解锁零件图批量上传
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
              :class="{
                active: isDraggingParts && canUploadParts,
                disabled: !canUploadParts,
              }"
              @dragover.prevent="canUploadParts && (isDraggingParts = true)"
              @dragleave.prevent="isDraggingParts = false"
              @drop.prevent="canUploadParts && onPartsDrop($event)"
            >
              <div class="upload-icon-wrap">
                <DemoIcon name="boxes" :size="24" />
              </div>
              <div class="upload-texts">
                <b>批量上传零件图（可选）</b>
                <p>支持多选多个零件文件或直接选择整目录文件夹</p>
              </div>
              <div class="upload-actions">
                <button
                  class="btn sm"
                  type="button"
                  :disabled="!canUploadParts"
                  @click="triggerPartsPick"
                >
                  <DemoIcon name="files" :size="13" />多选文件
                </button>
                <button
                  class="btn sm"
                  type="button"
                  :disabled="!canUploadParts"
                  @click="triggerFolderPick"
                >
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

.signers-layout {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}

.signer-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  transition: all 0.2s ease;
}

.signer-card:hover {
  border-color: var(--accent);
}

.signer-role-wrap {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.signer-badge {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
}

.role-tag-req {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 10px;
  font-weight: 600;
}

.role-tag-opt {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--panel);
  color: var(--text-3);
  font-size: 10px;
}

.signer-select {
  width: 100%;
  height: 32px;
  font-size: 12px;
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
  .signers-layout {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 640px) {
  .signers-layout {
    grid-template-columns: 1fr;
  }
  .form-grid {
    grid-template-columns: 1fr;
  }
  .form-item:last-child {
    grid-column: span 1;
  }
}
</style>
