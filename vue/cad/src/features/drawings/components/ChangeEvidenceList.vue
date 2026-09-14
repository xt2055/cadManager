<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { lifecycleApi, lifecycleFetch, downloadEvidence, type EvidenceDocument } from '@/services/lifecycle.service'
import { formatReadableDateTime } from '@/utils/date-time'
import { useUiStore } from '@/stores/ui.store'

const props = defineProps<{
  drawingId: string
  changeRequestId: string
  submissionIds?: string[]
}>()

const uiStore = useUiStore()
const documents = ref<EvidenceDocument[]>([])
const loading = ref(false)
const imageUrls = ref<Record<string, string>>({})
const lightbox = ref<{ url: string; title: string } | null>(null)

const IMAGE_EXTS = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.bmp', '.svg']
const IMAGE_PREVIEW_LIMIT = 12 * 1024 * 1024

function isImage(name: string): boolean {
  const index = name.lastIndexOf('.')
  return index >= 0 && IMAGE_EXTS.includes(name.slice(index).toLowerCase())
}

function formatFileSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

function revokeImages() {
  Object.values(imageUrls.value).forEach((url) => URL.revokeObjectURL(url))
  imageUrls.value = {}
}

async function loadImagePreviews() {
  for (const doc of documents.value) {
    if (!isImage(doc.fileName) || imageUrls.value[doc.id] || doc.size > IMAGE_PREVIEW_LIMIT) continue
    try {
      const blob = await (await lifecycleFetch(`/lifecycle-documents/${doc.id}`)).blob()
      imageUrls.value = { ...imageUrls.value, [doc.id]: URL.createObjectURL(blob) }
    } catch {
      // 预览失败时退回文件图标，不影响下载
    }
  }
}

function openLightbox(doc: EvidenceDocument) {
  const url = imageUrls.value[doc.id]
  if (url) lightbox.value = { url, title: doc.title }
}

async function load() {
  if (!props.drawingId) return
  loading.value = true
  try {
    const base = await lifecycleApi<EvidenceDocument[]>(
      `/lifecycle-documents?drawingId=${encodeURIComponent(props.drawingId)}`,
    )
    const matched = new Map<string, EvidenceDocument>()
    for (const doc of base) {
      if (doc.changeRequestId === props.changeRequestId) matched.set(doc.id, doc)
    }
    for (const submissionId of props.submissionIds || []) {
      const list = await lifecycleApi<EvidenceDocument[]>(
        `/lifecycle-documents?drawingId=${encodeURIComponent(props.drawingId)}&submissionId=${encodeURIComponent(submissionId)}`,
      )
      for (const doc of list) matched.set(doc.id, doc)
    }
    revokeImages()
    documents.value = [...matched.values()].sort((a, b) => b.createdAt.localeCompare(a.createdAt))
    void loadImagePreviews()
  } catch (e) {
    uiStore.toast((e as Error).message || '变更依据材料加载失败', 'warn')
  } finally {
    loading.value = false
  }
}

async function download(doc: EvidenceDocument) {
  try {
    await downloadEvidence(`/lifecycle-documents/${doc.id}`, doc.fileName)
  } catch (e) {
    uiStore.toast((e as Error).message || '下载失败', 'warn')
  }
}

watch(
  () => [props.drawingId, props.changeRequestId, (props.submissionIds || []).join(',')],
  load,
  { immediate: true },
)

onBeforeUnmount(revokeImages)
defineExpose({ load })
</script>

<template>
  <div class="change-evidence-list">
    <p v-if="loading" class="ce-state">
      <DemoIcon name="rotate-cw" :size="13" class="spin-icon" />
      正在加载本次变更上传的凭证…
    </p>
    <p v-else-if="!documents.length" class="ce-state">
      <DemoIcon name="inbox" :size="13" />
      本次变更未上传依据材料
    </p>
    <ul v-else class="ce-items">
      <li v-for="doc in documents" :key="doc.id" class="ce-item">
        <img
          v-if="isImage(doc.fileName) && imageUrls[doc.id]"
          class="ce-thumb"
          :src="imageUrls[doc.id]"
          :alt="doc.title"
          @click="openLightbox(doc)"
        />
        <span v-else class="ce-icon">
          <DemoIcon :name="isImage(doc.fileName) ? 'image' : 'file-text'" :size="15" />
        </span>
        <div class="ce-main">
          <b :title="doc.title">{{ doc.title }}</b>
          <small>
            {{ doc.category }} · {{ doc.fileName }} · {{ formatFileSize(doc.size) }} ·
            {{ doc.createdBy }} · {{ formatReadableDateTime(doc.createdAt) }}
          </small>
        </div>
        <button class="btn sm" type="button" @click="download(doc)">
          <DemoIcon name="download" :size="12" />下载
        </button>
      </li>
    </ul>

    <div v-if="lightbox" class="ce-lightbox" @click.self="lightbox = null">
      <div class="ce-lightbox-inner">
        <img :src="lightbox.url" :alt="lightbox.title" />
        <button class="btn sm ce-lightbox-close" type="button" @click="lightbox = null">
          <DemoIcon name="x" :size="12" />关闭
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.change-evidence-list {
  display: flex;
  flex-direction: column;
}

.ce-state {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--panel);
  color: var(--text-3);
  font-size: 12px;
}

.ce-items {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.ce-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--line);
  border-radius: 9px;
  background: var(--panel);
}

.ce-icon {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  border-radius: 8px;
  background: var(--accent-soft);
  color: var(--accent);
}

.ce-thumb {
  width: 44px;
  height: 44px;
  flex-shrink: 0;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-2);
  object-fit: cover;
  cursor: zoom-in;
}

.ce-main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.ce-main b {
  font-size: 12.5px;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ce-main small {
  color: var(--text-3);
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ce-lightbox {
  position: fixed;
  inset: 0;
  z-index: 2300;
  display: grid;
  place-items: center;
  padding: 32px;
  background: rgb(0 0 0 / 82%);
  backdrop-filter: blur(4px);
}

.ce-lightbox-inner {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 12px;
  align-items: center;
  max-width: 100%;
  max-height: 100%;
}

.ce-lightbox-inner img {
  max-width: 100%;
  max-height: calc(100vh - 120px);
  border-radius: 10px;
  object-fit: contain;
}

.ce-lightbox-close {
  align-self: flex-end;
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
