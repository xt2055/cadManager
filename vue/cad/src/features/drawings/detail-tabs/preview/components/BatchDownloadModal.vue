<script setup lang="ts">
import { computed } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'BatchDownloadModal' })

/**
 * 批量下载弹窗：只做展示与意图收集。
 * 不引入任何 store / composable——候选行（含「当前格式是否可用」）由父页面按 useDrawingBatchDownload
 * 的状态投影进来，勾选 / 切格式 / 打包都是 emit 出去的意图。
 */
const props = defineProps<{
  /** 当前下载格式。 */
  format: 'exb' | 'dwg' | 'pdf'
  /** 候选文件行（父页面：downloadCandidates × downloadFormat 的投影）。 */
  rows: { id: string; name: string; sizeLabel: string; available: boolean; checked: boolean }[]
  /** 当前格式可用的行是否已全选。 */
  allSelected: boolean
  /** 正在打包：禁用主按钮并显示进度。 */
  busy: boolean
  /** 打包进度文案。 */
  progress: string
}>()

const emit = defineEmits<{
  close: []
  confirm: []
  formatChange: [format: 'exb' | 'dwg' | 'pdf']
  /** 勾选 / 取消勾选一行。 */
  toggle: [id: string]
  /** 全选 / 全不选（父页面只作用于当前格式可用的行）。 */
  toggleAll: []
}>()

const selectedCount = computed(() => props.rows.filter((row) => row.checked).length)
const formatLabel = computed(() => props.format.toUpperCase())
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal card download-modal">
      <div class="modal-head">
        <div class="modal-title">
          <DemoIcon name="download" :size="18" />
          <span>批量下载图纸文件</span>
        </div>
        <button class="btn sm close-btn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modal-body">
        <div class="download-format-row">
          <span class="lbl bold">下载格式：</span>
          <label class="mode-option" :class="{ active: format === 'exb' }">
            <input type="radio" :checked="format === 'exb'" @change="emit('formatChange', 'exb')" />
            <span>EXB 原始格式</span>
          </label>
          <label class="mode-option" :class="{ active: format === 'dwg' }">
            <input type="radio" :checked="format === 'dwg'" @change="emit('formatChange', 'dwg')" />
            <span>DWG 格式</span>
          </label>
          <label class="mode-option" :class="{ active: format === 'pdf' }">
            <input type="radio" :checked="format === 'pdf'" @change="emit('formatChange', 'pdf')" />
            <span>PDF 格式 (矢量)</span>
          </label>
        </div>

        <div class="download-list-head">
          <label class="download-check-all">
            <input type="checkbox" :checked="allSelected" @change="emit('toggleAll')" />
            <b>全选</b>
          </label>
          <span class="hint">已选 {{ selectedCount }} / {{ rows.length }} 个文件 · {{ format === 'pdf' ? '支持 CAD 转矢量 PDF 及 PDF 附件' : '按所选格式下载' }}</span>
        </div>

        <div class="download-file-list">
          <label
            v-for="row in rows"
            :key="row.id"
            class="download-file-row"
            :class="{ unavailable: !row.available }"
          >
            <input
              type="checkbox"
              :checked="row.checked"
              :disabled="!row.available"
              @change="emit('toggle', row.id)"
            />
            <span class="file-name mono" :title="row.name">{{ row.name }}</span>
            <span class="dl-size">{{ row.sizeLabel }}</span>
            <span class="tag" :class="row.available ? 'ok' : 'mute'">
              {{ row.available ? formatLabel : '无此格式' }}
            </span>
          </label>
          <div v-if="!rows.length" class="empty compact-empty">
            <DemoIcon name="file" :size="28" />
            <div class="t">当前图纸暂无可下载的文件</div>
          </div>
        </div>

        <div class="note info-note">
          <DemoIcon name="info" :size="15" />
          <div>所选文件将打包为一个 zip 压缩包；没有对应格式文件的行会被跳过并标注。</div>
        </div>
      </div>

      <div class="modal-foot">
        <button class="btn" type="button" @click="emit('close')">取消</button>
        <button class="btn primary" type="button" :disabled="busy || selectedCount === 0" @click="emit('confirm')">
          <span v-if="busy" class="local-edit-spinner" aria-hidden="true"></span>
          <DemoIcon v-else name="download" :size="14" />
          {{ busy ? (progress || '正在打包...') : `打包下载 (${selectedCount})` }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 只放本弹窗专属样式；弹窗外壳来自上面的共享 modal-chrome。 */
.download-modal {
  width: 560px;
}

.download-format-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.download-list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-bottom: 1px solid var(--line);
}

.download-check-all {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  cursor: pointer;
  font-size: 12.5px;
}

.download-file-list {
  display: flex;
  max-height: 320px;
  flex-direction: column;
  overflow-y: auto;
  border: 1px solid var(--line);
  border-radius: 10px;
}

.download-file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--line);
  cursor: pointer;
  font-size: 12.5px;
}

.download-file-row:last-child {
  border-bottom: none;
}

.download-file-row:hover {
  background: var(--panel-2);
}

.download-file-row.unavailable {
  cursor: not-allowed;
  opacity: 0.55;
}

.download-file-row .file-name {
  flex: 1;
  overflow: hidden;
  min-width: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.download-file-row .dl-size {
  flex-shrink: 0;
  color: var(--text-3);
  font-size: 11.5px;
}

/* .local-edit-spinner 与父页面文件表的同名工具类共用，父页面那份带的是它的 scope id、作用不到本组件，
   因此这里保留一份内容相同的副本（含 keyframes；scoped 下 keyframes 名会各自带 scope，不冲突）。 */
.local-edit-spinner {
  width: 13px;
  height: 13px;
  flex: none;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: local-edit-spin 0.75s linear infinite;
}

@keyframes local-edit-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .local-edit-spinner {
    animation-duration: 1.5s;
  }
}
</style>
