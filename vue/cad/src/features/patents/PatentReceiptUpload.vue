<script setup lang="ts">
import { ref, watch } from 'vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { lifecycleApi } from '@/services/lifecycle.service'
import { useUiStore } from '@/stores/ui.store'

const props = defineProps<{ patentId: string; open: boolean }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'uploaded'): void }>()

const uiStore = useUiStore()
const file = ref<File | null>(null)
const title = ref('')
const description = ref('')
const busy = ref(false)
const isDragging = ref(false)
const input = ref<HTMLInputElement | null>(null)

watch(
  () => props.open,
  (open) => {
    if (open) reset()
  },
)

function reset() {
  file.value = null
  title.value = ''
  description.value = ''
  if (input.value) input.value.value = ''
}

function assignFile(selected: File) {
  file.value = selected
  if (!title.value.trim()) title.value = selected.name.replace(/\.[^/.]+$/, '')
}

function pick(event: Event) {
  const target = event.target as HTMLInputElement
  const chosen = target.files?.[0] || null
  if (chosen) assignFile(chosen)
}

function onDrop(event: DragEvent) {
  isDragging.value = false
  const dropped = event.dataTransfer?.files?.[0]
  if (dropped) assignFile(dropped)
}

function removeSelectedFile() {
  file.value = null
  if (input.value) input.value.value = ''
}

function formatFileSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

async function upload() {
  if (!file.value || !title.value.trim() || busy.value) return
  busy.value = true
  try {
    const form = new FormData()
    form.set('file', file.value)
    form.set('title', title.value.trim())
    form.set('category', '缴费凭证')
    form.set('description', description.value.trim())
    form.set('patentId', props.patentId)
    form.set('source', '专利缴费')
    await lifecycleApi('/lifecycle-documents', { method: 'POST', body: form })
    uiStore.toast(`缴费凭证「${title.value.trim()}」已上传`, 'ok')
    emit('uploaded')
    emit('close')
  } catch (e) {
    uiStore.toast((e as Error).message || '缴费凭证上传失败', 'warn')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div v-if="open" class="receipt-backdrop" @click.self="emit('close')">
    <div class="receipt-modal card" role="dialog" aria-modal="true" aria-label="上传缴费凭证">
      <header class="rm-head">
        <div class="rm-title">
          <span class="rm-icon"><DemoIcon name="file-up" :size="18" /></span>
          <div>
            <h3>上传缴费凭证</h3>
            <p>上传该专利的官方缴费收据或电子凭证，仅本专利可见</p>
          </div>
        </div>
        <button class="rm-close" type="button" aria-label="关闭" @click="emit('close')">
          <DemoIcon name="x" :size="16" />
        </button>
      </header>

      <form class="rm-body" @submit.prevent="upload">
        <input ref="input" type="file" class="hidden-file-input" @change="pick" />
        <div
          class="receipt-dropzone"
          :class="{ 'is-dragging': isDragging, 'has-file': Boolean(file) }"
          @dragover.prevent="isDragging = true"
          @dragleave.prevent="isDragging = false"
          @drop.prevent="onDrop"
          @click="input?.click()"
        >
          <template v-if="!file">
            <span class="dz-icon"><DemoIcon name="file-up" :size="30" /></span>
            <b>点击选择缴费凭证 或 拖拽文件至此</b>
            <p>支持 PDF、图片、扫描件等，单文件上限 100 MB</p>
          </template>
          <template v-else>
            <div class="dz-file" @click.stop>
              <span class="dz-file-icon"><DemoIcon name="file-text" :size="22" /></span>
              <div class="dz-file-text">
                <b :title="file.name">{{ file.name }}</b>
                <span>{{ formatFileSize(file.size) }} · 准备上传</span>
              </div>
              <div class="dz-file-actions">
                <button class="btn sm" type="button" @click="input?.click()">重新选择</button>
                <button class="btn sm danger" type="button" @click="removeSelectedFile">移除</button>
              </div>
            </div>
          </template>
        </div>

        <label class="field">
          <span>凭证名称 <i>*</i></span>
          <input v-model="title" required maxlength="200" placeholder="例如：2026 年度专利年费收据" />
        </label>
        <label class="field">
          <span>说明（可选）</span>
          <textarea v-model="description" rows="2" placeholder="缴费项目、金额或对应票据号等" />
        </label>

        <p class="rm-note"><DemoIcon name="shield-check" :size="13" />上传后自动计算 SHA-256 数字指纹，文件不可修改或覆盖。</p>

        <footer class="rm-foot">
          <button class="btn" type="button" :disabled="busy" @click="emit('close')">取消</button>
          <button class="btn primary" type="submit" :disabled="busy || !file || !title.trim()">
            <DemoIcon v-if="busy" name="rotate-cw" :size="13" class="spin" />
            {{ busy ? '正在上传…' : '上传凭证' }}
          </button>
        </footer>
      </form>
    </div>
  </div>
</template>

<style scoped>
.receipt-backdrop {
  position: fixed;
  inset: 0;
  z-index: 2200;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgb(0 0 0 / 50%);
  backdrop-filter: blur(4px);
  animation: receipt-fade 0.18s ease;
}

@keyframes receipt-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

.receipt-modal {
  display: flex;
  flex-direction: column;
  width: min(100%, 560px);
  max-height: 90vh;
  overflow: hidden;
  border-radius: 16px;
  box-shadow: 0 24px 60px rgb(0 0 0 / 30%);
}

.rm-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--line);
}

.rm-title {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.rm-icon {
  display: grid;
  width: 40px;
  height: 40px;
  flex: none;
  place-items: center;
  border-radius: 12px;
  background: var(--accent-soft);
  color: var(--accent);
}

.rm-head h3 {
  margin: 0;
  font-size: 15.5px;
  font-weight: 800;
}

.rm-head p {
  margin: 3px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.rm-close {
  display: grid;
  width: 30px;
  height: 30px;
  flex: none;
  place-items: center;
  border-radius: 8px;
  color: var(--text-3);
}

.rm-close:hover {
  background: var(--hover);
  color: var(--text-1);
}

.rm-body {
  display: flex;
  flex-direction: column;
  gap: 15px;
  padding: 20px;
  overflow-y: auto;
}

.hidden-file-input {
  display: none;
}

.receipt-dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 26px 18px;
  border: 2px dashed var(--line-strong);
  border-radius: 12px;
  background: var(--panel-2);
  color: var(--text-2);
  cursor: pointer;
  text-align: center;
  transition: border-color 0.2s, background 0.2s;
}

.receipt-dropzone:hover,
.receipt-dropzone.is-dragging {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.receipt-dropzone.has-file {
  padding: 12px;
  border-style: solid;
  border-color: var(--accent);
  background: var(--panel);
  cursor: default;
}

.dz-icon {
  display: grid;
  width: 54px;
  height: 54px;
  place-items: center;
  border-radius: 50%;
  background: var(--panel);
  color: var(--accent);
}

.receipt-dropzone b {
  font-size: 13px;
  color: var(--text-1);
}

.receipt-dropzone p {
  margin: 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.dz-file {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.dz-file-icon {
  display: grid;
  width: 42px;
  height: 42px;
  flex: none;
  place-items: center;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
}

.dz-file-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  flex: 1;
}

.dz-file-text b {
  overflow: hidden;
  font-size: 12.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dz-file-text span {
  color: var(--text-3);
  font-size: 11px;
}

.dz-file-actions {
  display: flex;
  gap: 8px;
  flex: none;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.field > span {
  color: var(--text-2);
  font-size: 12px;
}

.field > span i {
  color: var(--danger);
  font-style: normal;
}

.field input,
.field textarea {
  width: 100%;
  padding: 9px 11px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  color: var(--text-1);
  font: inherit;
  font-size: 12.5px;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.field textarea {
  resize: vertical;
  line-height: 1.6;
}

.field input:focus,
.field textarea:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}

.rm-note {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0;
  padding: 10px 12px;
  border: 1px dashed var(--line-strong);
  border-radius: 10px;
  color: var(--text-3);
  font-size: 11.5px;
}

.rm-note svg {
  flex: none;
  color: var(--accent);
}

.rm-foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.spin {
  animation: receipt-spin 1s linear infinite;
}

@keyframes receipt-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .receipt-backdrop,
  .spin {
    animation: none;
  }
}
</style>
