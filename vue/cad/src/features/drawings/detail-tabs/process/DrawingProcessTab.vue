<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import DocxPreviewModal from './DocxPreviewModal.vue'
import { useDrawingStore } from '@/stores/drawing.store'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useUiStore } from '@/stores/ui.store'
import type { CraftFile } from '@/types/domain.types'
import type { CraftFileView } from '@/modules/drawing'
import { saveFilesAsZip } from '@/utils/download-file'
import { versionDisplayLabel } from '@/modules/versioning/versioning-service'
import { selectAllFiles, selectedFiles, toggleFileSelection } from '../file-bulk-selection'

defineOptions({ name: 'DrawingProcessTab' })

const drawingOperationsStore = useDrawingOperationsStore()
const route = useRoute()
const drawingStore = useDrawingStore()
const uiStore = useUiStore()

const currentItem = computed(() => {
  const id = String(route.params.drawingId ?? '')
  return drawingStore.getDrawing(id) ?? drawingStore.getPart(id)
})
const crafts = computed<CraftFileView[]>(() => currentItem.value?.craftFiles ?? [])
const fileInput = ref<HTMLInputElement | null>(null)
const replaceInput = ref<HTMLInputElement | null>(null)
const replacingFileId = ref<string | null>(null)

const previewVisible = ref(false)
const previewFile = ref<CraftFileView | null>(null)
const selectedFileIds = ref<Set<string>>(new Set())
const bulkLoading = ref(false)
const selectedCrafts = computed(() => selectedFiles(crafts.value, selectedFileIds.value))
const allCraftsSelected = computed(() => crafts.value.length > 0 && crafts.value.every((file) => selectedFileIds.value.has(file.id)))

function openPreview(file: CraftFileView) {
  if (!file.storageKey) {
    uiStore.toast('该工艺文件尚未保存在本地或后端，暂无法预览', 'warn')
    return
  }
  previewFile.value = file
  previewVisible.value = true
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function triggerUpload() {
  fileInput.value?.click()
}

function triggerReplace(file: CraftFileView) {
  replacingFileId.value = file.id
  replaceInput.value?.click()
}

async function onReplaceChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  const fileId = replacingFileId.value
  if (!file || !fileId || !currentItem.value) return
  try {
    await drawingOperationsStore.replaceCraftFile(currentItem.value.no, fileId, file)
    await drawingStore.refresh()
    uiStore.toast(`工艺文件已替换：${file.name}`, 'ok')
  } catch (error) {
    console.error('替换工艺文件失败', error)
    uiStore.toast('替换工艺文件失败', 'warn')
  } finally {
    target.value = ''
    replacingFileId.value = null
  }
}

function formatCurrentTime(): string {
  const d = new Date()
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}`
}

async function onFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const files = Array.from(target.files ?? [])
  if (!files.length || !currentItem.value) return

  let uploadedCount = 0
  let failedCount = 0
  try {
    for (const file of files) {
      const newFile: CraftFile = {
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        drawingNo: currentItem.value.no,
        name: file.name,
        op: '机加工与装配工艺',
        ver: currentItem.value.version || 'v1.0',
        by: '张工',
        date: formatCurrentTime(),
        size: formatFileSize(file.size),
        previewable: true,
        scanned: false,
      }

      try {
        await drawingOperationsStore.uploadCraftFile(currentItem.value.no, newFile, file)
        await drawingStore.refresh()
        uploadedCount += 1
      } catch (error) {
        failedCount += 1
        console.error(`上传工艺文件失败：${file.name}`, error)
      }
    }

    if (failedCount === 0) {
      uiStore.toast(`已上传 ${uploadedCount} 个工艺文件`, 'ok')
    } else if (uploadedCount > 0) {
      uiStore.toast(`已上传 ${uploadedCount} 个工艺文件，${failedCount} 个上传失败`, 'warn')
    } else {
      uiStore.toast('工艺文件上传失败', 'warn')
    }
  } finally {
    target.value = ''
  }
}

function toggleCraftSelection(fileId: string) {
  selectedFileIds.value = toggleFileSelection(selectedFileIds.value, fileId)
}

function toggleAllCrafts() {
  selectedFileIds.value = selectAllFiles(crafts.value.map((file) => file.id), selectedFileIds.value)
}

function handleDeleteCraft(file: CraftFileView) {
  uiStore.confirm('删除工艺文件', `确定要移除工艺文件「${file.name}」吗？`, {
    confirmText: '删除',
    danger: true,
    onConfirm: () => deleteCraftFiles([file]),
  })
}

function handleDeleteSelectedCrafts() {
  if (!selectedCrafts.value.length) return
  uiStore.confirm('删除所选工艺文件', `确定要删除已选的 ${selectedCrafts.value.length} 个工艺文件吗？删除后无法恢复。`, {
    confirmText: '删除所选',
    danger: true,
    onConfirm: () => deleteCraftFiles(selectedCrafts.value),
  })
}

async function deleteCraftFiles(files: CraftFileView[]) {
  if (!currentItem.value) return
  bulkLoading.value = true
  let failed = 0
  let failureReason = ''
  try {
    for (const file of files) {
      try {
        await drawingOperationsStore.deleteCraftFile(currentItem.value.no, file.id)
      } catch (error) {
        failed += 1
        failureReason = error instanceof Error ? error.message : String(error)
        console.error(`删除工艺文件失败：${file.name}`, error)
      }
    }
    selectedFileIds.value = new Set()
    await drawingStore.refresh()
    const succeeded = files.length - failed
    uiStore.toast(failed ? `已删除 ${succeeded} 个工艺文件，${failed} 个删除失败：${failureReason}` : `已删除 ${succeeded} 个工艺文件`, failed ? 'warn' : 'ok')
  } finally {
    bulkLoading.value = false
  }
}

async function handleDownloadSelectedCrafts() {
  if (!currentItem.value || !selectedCrafts.value.length) return
  bulkLoading.value = true
  let failed = 0
  try {
    const files: Array<{ name: string; content: Blob }> = []
    for (const file of selectedCrafts.value) {
      if (!file.storageKey) { failed += 1; continue }
      try {
        files.push({ name: file.name, content: await drawingOperationsStore.readAttachmentContent(file) })
      } catch (error) {
        failed += 1
        console.error(`读取工艺文件失败：${file.name}`, error)
      }
    }
    if (!files.length) {
      uiStore.toast('所选工艺文件均无法下载', 'warn')
      return
    }
    const saved = await saveFilesAsZip(`${currentItem.value.no}_工艺文件.zip`, files)
    if (saved) uiStore.toast(failed ? `已保存 ${files.length} 个工艺文件，${failed} 个读取失败` : `已保存 ${files.length} 个工艺文件`, failed ? 'warn' : 'ok')
  } finally {
    bulkLoading.value = false
  }
}

async function handleDownloadCraft(file: CraftFileView) {
  try {
    if (await drawingOperationsStore.downloadAttachment(file)) {
      uiStore.toast(`已保存 ${file.name}`, 'ok')
    }
  } catch (error) {
    console.error('下载工艺文件失败', error)
    uiStore.toast('下载工艺文件失败：附件可能尚未保存', 'warn')
  }
}

onMounted(async () => {
  await Promise.all([drawingStore.load(), drawingOperationsStore.initialize()])
  await drawingStore.refresh()
})

</script>

<template>
  <div class="process-page-view">
    <input
      ref="fileInput"
      type="file"
      multiple
      accept=".pdf,.doc,.docx,.crd,.step,.xlsx"
      class="hidden-file-input"
      @change="onFileChange"
    />
    <input
      ref="replaceInput"
      type="file"
      multiple
      accept=".pdf,.doc,.docx,.crd,.step,.xlsx"
      class="hidden-file-input"
      @change="onReplaceChange"
    />

    <!-- 顶部操作栏 -->
    <div class="card card-pad process-top-bar">
      <div class="process-info">
        <DemoIcon name="file-text" :size="18" />
        <div>
          <h3>工艺文件与规程管理</h3>
          <p>维护机加、装配、焊接及热处理等工艺规程文件，大卡片视窗便于快速预览。</p>
        </div>
      </div>
      <div class="process-actions">
        <button v-if="crafts.length" class="btn sm" type="button" @click="toggleAllCrafts">
          <DemoIcon name="check-square" :size="14" />{{ allCraftsSelected ? '取消全选' : '全选' }}
        </button>
        <button v-if="selectedCrafts.length" class="btn sm" type="button" :disabled="bulkLoading" @click="handleDownloadSelectedCrafts">
          <DemoIcon name="download" :size="14" />下载所选（{{ selectedCrafts.length }}）
        </button>
        <button v-if="selectedCrafts.length" class="btn sm danger" type="button" :disabled="bulkLoading" @click="handleDeleteSelectedCrafts">
          <DemoIcon name="trash-2" :size="14" />删除所选（{{ selectedCrafts.length }}）
        </button>
        <button class="btn primary" type="button" @click="triggerUpload">
          <DemoIcon name="upload" :size="14" />上传工艺文件
        </button>
      </div>
    </div>

    <!-- 工艺文件宽卡片网格布局 -->
    <div class="craft-grid">
      <div v-for="file in crafts" :key="file.id" class="card card-pad craft-wide-card">
        <div class="craft-card-top">
          <label class="file-select" :title="`选择 ${file.name}`">
            <input type="checkbox" :checked="selectedFileIds.has(file.id)" @change="toggleCraftSelection(file.id)" />
          </label>
          <div class="file-icon-wrap">
            <DemoIcon name="file-text" :size="24" />
          </div>
          <div class="craft-main-meta">
            <div class="craft-name-row">
              <b :title="file.name">{{ file.name }}</b>
              <span class="tag plain">{{ versionDisplayLabel(file.ver) }}</span>
            </div>
            <div class="craft-op-tag">{{ file.op }}</div>
          </div>
        </div>

        <div class="craft-detail-fields">
            <div class="field-item">
              <span class="lbl">编制人员</span>
              <span class="val">{{ file.author || '未识别' }}</span>
            </div>
          <div class="field-item">
            <span class="lbl">上传时间</span>
            <span class="val">{{ file.date && file.date !== '刚刚' ? file.date : formatCurrentTime() }}</span>
          </div>
          <div class="field-item">
            <span class="lbl">文件大小</span>
            <span class="val">{{ file.size }}</span>
          </div>
        </div>

        <div class="craft-card-footer">
          <button class="btn sm" type="button" @click="openPreview(file)">
            <DemoIcon name="eye" :size="13" />在线预览
          </button>
          <button class="btn sm" type="button" @click="handleDownloadCraft(file)">
            <DemoIcon name="download" :size="13" />下载
          </button>
          <button class="btn sm" type="button" @click="triggerReplace(file)">
            <DemoIcon name="refresh-cw" :size="13" />替换
          </button>
          <button class="btn sm danger" type="button" @click="handleDeleteCraft(file)">
            <DemoIcon name="trash-2" :size="13" />删除
          </button>
        </div>
      </div>
    </div>

    <div v-if="!crafts.length" class="card empty">
      <DemoIcon name="file-text" :size="38" />
      <div class="t">暂无工艺文件</div>
      <p>请点击右上角「上传工艺文件」上传规程与指导卡</p>
    </div>

    <!-- DOCX / PDF 阅读弹窗 -->
    <DocxPreviewModal
      v-model:visible="previewVisible"
      :title="previewFile?.name || '工艺规程阅读'"
      :file-name="previewFile?.name"
      :storage-key="previewFile?.storageKey"
    />
  </div>
</template>

<style scoped>
.process-page-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 480px;
}
.hidden-file-input {
  display: none;
}
.process-top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.process-info {
  display: flex;
  align-items: center;
  gap: 12px;
}
.process-info svg {
  color: var(--accent);
}
.process-info h3 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}
.process-info p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}
.craft-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(380px, 1fr));
  gap: 16px;
}
.craft-wide-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 180px;
  border-radius: var(--radius);
  transition: transform 0.2s ease, border-color 0.2s ease;
}
.process-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 8px;
}
.craft-card-top {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}
.file-select {
  display: grid;
  place-items: center;
  padding-top: 2px;
}
.file-icon-wrap {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  flex: none;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
}
.craft-main-meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.craft-name-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.craft-name-row b {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.craft-op-tag {
  color: var(--text-3);
  font-size: 11.5px;
}
.craft-detail-fields {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--panel-2);
}
.field-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.field-item .lbl {
  color: var(--text-3);
  font-size: 10.5px;
}
.field-item .val {
  font-size: 11.5px;
  font-weight: 500;
}
.craft-card-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: auto;
  padding-top: 6px;
  border-top: 1px solid var(--line);
}
@media (max-width: 760px) {
  .craft-grid {
    grid-template-columns: 1fr;
  }
}
</style>
