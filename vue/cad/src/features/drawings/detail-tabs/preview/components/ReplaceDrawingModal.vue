<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'ReplaceDrawingModal' })

/**
 * 替换图纸文件（版本替换）确认弹窗：只做展示与输入回抛。
 * 不引入任何 store / composable；展示串由父页面按 versionDisplayLabel / formatFileSize 备好后传入，
 * 「替换原因」通过 update:reason 回抛给持有状态的 composable。
 */
defineProps<{
  /** 原文件名（未选中时为空串）。 */
  originalName: string
  /** 原文件版本展示串（如 v1.2；未选中时为 v1.0）。 */
  originalVersion: string
  /** 新选文件名（未选中时为空串）。 */
  newName: string
  /** 新文件大小展示串（如 1.2 MB；未选中时为 0 B）。 */
  newSize: string
  /** 版本更新说明 / 替换原因。 */
  reason: string
}>()

const emit = defineEmits<{
  close: []
  confirm: []
  'update:reason': [value: string]
}>()
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal card replace-modal">
      <div class="modal-head">
        <div class="modal-title">
          <DemoIcon name="refresh-cw" :size="18" />
          <span>替换图纸文件并生成新版本</span>
        </div>
        <button class="btn sm close-btn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modal-body">
        <div class="replace-meta-box">
          <div class="meta-row">
            <span class="lbl">原文件：</span>
            <span class="val mono bold">{{ originalName }}</span>
            <span class="tag info">{{ originalVersion }}</span>
          </div>
          <div class="meta-row">
            <span class="lbl">新文件：</span>
            <span class="val mono bold text-accent">{{ newName }}</span>
            <span class="tag ok">({{ newSize }})</span>
          </div>
        </div>

        <div class="field">
          <label class="bold">版本更新说明 / 替换原因</label>
          <input
            type="text"
            class="inp"
            placeholder="例如：修改活塞密封槽倒角与公差，重新出图"
            :value="reason"
            @input="emit('update:reason', ($event.target as HTMLInputElement).value)"
          />
        </div>

        <div class="note info-note">
          <DemoIcon name="shield-check" :size="15" />
          <div>替换将保留原文件所有图纸历史树与下载凭据，版本号自动递进，全程留痕。</div>
        </div>
      </div>

      <div class="modal-foot">
        <button class="btn" type="button" @click="emit('close')">取消</button>
        <button class="btn primary" type="button" @click="emit('confirm')">
          <DemoIcon name="check" :size="14" />确认替换升级
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 只放本弹窗专属样式；弹窗外壳来自上面的共享 modal-chrome。 */
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
</style>
