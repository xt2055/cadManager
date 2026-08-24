<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { CraftFile } from '@/types/domain.types'

defineOptions({ name: 'DrawingProcessTab' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

const currentItem = computed(() => domainStore.currentDrawing)
const fileInput = ref<HTMLInputElement | null>(null)

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function triggerUpload() {
  fileInput.value?.click()
}

async function onFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file || !currentItem.value) return

  const newFile: CraftFile = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    drawingNo: currentItem.value.no,
    name: file.name,
    op: '机加工与装配工艺',
    ver: currentItem.value.ver || 'v1.0',
    by: '张工',
    date: '刚刚',
    size: formatFileSize(file.size),
    previewable: true,
  }

  try {
    await domainStore.uploadCraftFile(currentItem.value.no, newFile, file)
    uiStore.toast(`工艺文件「${file.name}」上传成功`, 'ok')
  } catch (error) {
    console.error('上传工艺文件失败', error)
    uiStore.toast('上传工艺文件失败', 'warn')
  } finally {
    target.value = ''
  }
}

async function handleDeleteCraft(file: CraftFile) {
  if (!currentItem.value) return
  if (!window.confirm(`确定要移除工艺文件「${file.name}」吗？`)) return

  try {
    await domainStore.deleteCraftFile(currentItem.value.no, file.id || file.name)
    uiStore.toast(`已移除工艺文件 ${file.name}`)
  } catch (error) {
    console.error('删除工艺文件失败', error)
    uiStore.toast('删除工艺文件失败', 'warn')
  }
}

async function handleDownloadCraft(file: CraftFile) {
  try {
    await domainStore.downloadAttachment(file)
    uiStore.toast(`已开始下载 ${file.name}`, 'ok')
  } catch (error) {
    console.error('下载工艺文件失败', error)
    uiStore.toast('下载工艺文件失败：附件可能尚未保存', 'warn')
  }
}
</script>

<template>
  <div class="process-page-view">
    <input
      ref="fileInput"
      type="file"
      accept=".pdf,.doc,.docx,.crd,.step,.xlsx"
      class="hidden-file-input"
      @change="onFileChange"
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
        <button class="btn primary" type="button" @click="triggerUpload">
          <DemoIcon name="upload" :size="14" />上传工艺文件
        </button>
      </div>
    </div>

    <!-- 工艺文件宽卡片网格布局 -->
    <div class="craft-grid">
      <div v-for="file in domainStore.crafts" :key="file.id" class="card card-pad craft-wide-card">
        <div class="craft-card-top">
          <div class="file-icon-wrap">
            <DemoIcon name="file-text" :size="24" />
          </div>
          <div class="craft-main-meta">
            <div class="craft-name-row">
              <b :title="file.name">{{ file.name }}</b>
              <span class="tag plain">{{ file.ver }}</span>
            </div>
            <div class="craft-op-tag">{{ file.op }}</div>
          </div>
        </div>

        <div class="craft-detail-fields">
          <div class="field-item">
            <span class="lbl">编制人员</span>
            <span class="val">{{ file.by }}</span>
          </div>
          <div class="field-item">
            <span class="lbl">上传日期</span>
            <span class="val">{{ file.date }}</span>
          </div>
          <div class="field-item">
            <span class="lbl">文件大小</span>
            <span class="val">{{ file.size }}</span>
          </div>
        </div>

        <div class="craft-card-footer">
          <button class="btn sm" type="button" @click="uiStore.toast('工艺文件预览（Demo）', 'info')">
            <DemoIcon name="eye" :size="13" />在线预览
          </button>
          <button class="btn sm" type="button" @click="handleDownloadCraft(file)">
            <DemoIcon name="download" :size="13" />下载
          </button>
          <button class="btn sm danger" type="button" @click="handleDeleteCraft(file)">
            <DemoIcon name="trash-2" :size="13" />删除
          </button>
        </div>
      </div>
    </div>

    <div v-if="!domainStore.crafts.length" class="card empty">
      <DemoIcon name="file-text" :size="38" />
      <div class="t">暂无工艺文件</div>
      <p>请点击右上角「上传工艺文件」上传规程与指导卡</p>
    </div>
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
.craft-card-top {
  display: flex;
  align-items: flex-start;
  gap: 14px;
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
