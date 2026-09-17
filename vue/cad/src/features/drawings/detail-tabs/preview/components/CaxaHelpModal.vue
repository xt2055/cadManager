<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'CaxaHelpModal' })

defineProps<{
  /** 失败详情：缺少的 CAXA 路径或需要配置的内容。 */
  detail: string
  /** 正在保存 CAXA 路径：禁用两个「选择程序」按钮。 */
  saving: boolean
}>()

const emit = defineEmits<{
  close: []
  pick: []
  openSystemApps: []
}>()
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal card caxa-help-modal">
      <div class="modal-head">
        <div class="modal-title">
          <DemoIcon name="alert-triangle" :size="18" />
          <span>未找到本机 CAD 程序</span>
        </div>
        <button class="btn sm close-btn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modal-body">
        <p class="caxa-help-detail">{{ detail }}</p>
        <p class="caxa-help-text">请选择一种处理方式；指定一次后系统会自动记住，之后无需重复选择：</p>
        <div class="caxa-help-actions">
          <button class="btn primary" type="button" :disabled="saving" @click="emit('pick')">
            <DemoIcon name="folder-open" :size="14" />
            {{ saving ? '处理中...' : '选择 CAXA 程序（CDRAFT_M.exe）' }}
          </button>
          <button class="btn" type="button" @click="emit('openSystemApps')">
            <DemoIcon name="settings" :size="14" />打开系统「默认应用」设置
          </button>
        </div>
      </div>

      <div class="modal-foot">
        <button class="btn" type="button" @click="emit('close')">稍后处理</button>
        <button class="btn primary" type="button" :disabled="saving" @click="emit('pick')">选择程序并重试</button>
      </div>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 弹窗外壳（backdrop / head / title / close-btn / body / foot）由本组件自己引入的共享 modal-chrome 持有，
   父页面不再用 :deep() 兜本组件的内部结构；这里只放本组件专属样式。 */
.caxa-help-modal {
  max-width: 540px;
  width: min(540px, calc(100vw - 48px));
}

.caxa-help-detail {
  margin: 0 0 10px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--warn-soft, rgba(234, 179, 8, 0.12));
  color: var(--warn, #b45309);
  font-size: 12.5px;
  line-height: 1.7;
  word-break: break-all;
}

.caxa-help-text {
  margin: 0 0 12px;
  font-size: 13px;
  color: var(--text-2, #4b5563);
}

.caxa-help-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.caxa-help-actions .btn {
  justify-content: center;
}
</style>
