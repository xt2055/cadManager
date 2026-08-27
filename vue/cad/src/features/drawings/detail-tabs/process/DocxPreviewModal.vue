<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { renderAsync } from 'docx-preview'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { dataManager } from '@/services/data-manager'

interface Props {
  visible: boolean
  title: string
  storageKey?: string
  fileName?: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'close'): void
}>()

const docxContainer = ref<HTMLDivElement | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const pdfUrl = ref<string | null>(null)
const isPdf = ref(false)

async function loadDocument() {
  if (!props.storageKey) {
    error.value = '文件未找到存储键，无法预览'
    return
  }

  loading.value = true
  error.value = null
  isPdf.value = (props.fileName || '').toLowerCase().endsWith('.pdf')

  try {
    const blob = await dataManager.readAttachment(props.storageKey)
    if (isPdf.value) {
      if (pdfUrl.value) {
        URL.revokeObjectURL(pdfUrl.value)
      }
      pdfUrl.value = URL.createObjectURL(blob)
    } else {
      // 加载状态下原来的 v-else 不会挂载容器，必须先切换到内容视图再渲染 DOCX。
      loading.value = false
      await nextTick()
      if (!docxContainer.value) {
        throw new Error('DOCX 预览容器未挂载')
      }

      docxContainer.value.innerHTML = ''
      await renderAsync(blob, docxContainer.value, undefined, {
        inWrapper: true,
        ignoreWidth: false,
        ignoreHeight: false,
        breakPages: true,
        renderHeaders: true,
        renderFooters: true,
      })
    }
  } catch (err) {
    console.error('加载文档预览失败', err)
    error.value = `文档加载失败: ${err instanceof Error ? err.message : String(err)}`
  } finally {
    loading.value = false
  }
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      void loadDocument()
    } else {
      if (pdfUrl.value) {
        URL.revokeObjectURL(pdfUrl.value)
        pdfUrl.value = null
      }
      if (docxContainer.value) {
        docxContainer.value.innerHTML = ''
      }
    }
  },
)

function handleClose() {
  emit('update:visible', false)
  emit('close')
}
</script>

<template>
  <div v-if="visible" class="doc-preview-modal-backdrop" @click.self="handleClose">
    <div class="doc-preview-modal-dialog">
      <!-- 模态框头部 -->
      <div class="doc-preview-header">
        <div class="doc-preview-title">
          <DemoIcon :name="isPdf ? 'file-text' : 'file-text'" :size="18" />
          <span>{{ title || fileName || '文档阅读器' }}</span>
        </div>
        <div class="doc-preview-actions">
          <button class="icon-close-btn" title="关闭" type="button" @click="handleClose">
            <DemoIcon name="x" :size="18" />
          </button>
        </div>
      </div>

      <!-- 模态框主体内容 -->
      <div class="doc-preview-body">
        <div v-if="loading" class="preview-loading">
          <DemoIcon name="refresh-cw" :size="32" class="spin-icon" />
          <span>正在渲染文档，请稍候...</span>
        </div>

        <div v-else-if="error" class="preview-error">
          <DemoIcon name="alert-circle" :size="36" />
          <p>{{ error }}</p>
          <button class="btn sm" type="button" @click="loadDocument">重试</button>
        </div>

        <!-- PDF 嵌入视窗 -->
        <iframe
          v-else-if="isPdf && pdfUrl"
          :src="pdfUrl"
          class="pdf-viewer-frame"
          title="PDF Preview"
        ></iframe>

        <!-- DOCX 容器 -->
        <div
          v-else
          ref="docxContainer"
          class="docx-render-area"
        ></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.doc-preview-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.65);
  backdrop-filter: blur(4px);
}

.doc-preview-modal-dialog {
  width: 90vw;
  max-width: 1050px;
  height: 88vh;
  display: flex;
  flex-direction: column;
  background: var(--panel);
  border-radius: var(--radius-lg, 12px);
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.35);
  border: 1px solid var(--line);
  overflow: hidden;
}

.doc-preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  border-bottom: 1px solid var(--line);
  background: var(--panel-2);
}

.doc-preview-title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14.5px;
  font-weight: 600;
  color: var(--text-1);
}

.doc-preview-title svg {
  color: var(--accent);
}

.icon-close-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--text-2);
  cursor: pointer;
  transition: all 0.2s;
}

.icon-close-btn:hover {
  background: var(--panel-hover, rgba(255, 255, 255, 0.1));
  color: var(--text-1);
}

.doc-preview-body {
  flex: 1;
  overflow: auto;
  position: relative;
  background: #525659; /* 传统文档阅读器深色背景衬托 A4 白纸 */
}

.docx-render-area {
  width: 100%;
  min-height: 100%;
  padding: 20px 0;
  display: flex;
  justify-content: center;
}

:deep(.docx-wrapper) {
  background: transparent !important;
  padding: 0 !important;
}

:deep(.docx) {
  background: #ffffff !important;
  color: #111827 !important;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.4) !important;
  margin: 0 auto 24px auto !important;
  padding: 48px 64px !important;
  box-sizing: border-box;
}

:deep(.docx *) {
  color: #111827 !important;
}

:deep(.docx p),
:deep(.docx span),
:deep(.docx table),
:deep(.docx td),
:deep(.docx th) {
  color: #111827 !important;
}

.pdf-viewer-frame {
  width: 100%;
  height: 100%;
  border: none;
}

.preview-loading,
.preview-error {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--text-2);
}

.spin-icon {
  animation: spin 1s linear infinite;
  color: var(--accent);
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
