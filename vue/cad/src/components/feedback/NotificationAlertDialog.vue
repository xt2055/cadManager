<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { notificationActionLabel } from '@/features/notifications/notification-targets'
import type { NotificationItem } from '@/services/notification.service'
import { formatReadableDateTime } from '@/utils/date-time'

defineOptions({
  name: 'NotificationAlertDialog',
})

const props = withDefaults(
  defineProps<{
    /** 当前需要确认的单条通知；为空时走启动汇总模式 */
    item?: NotificationItem | null
    /** 本次会话启动时已存在的未读数量，仅用于汇总提醒 */
    summaryUnread?: number | null
    /** 队列中仍需确认的通知条数（含当前这条） */
    remaining?: number
    /** 当前是本次批量中的第几条 */
    position?: number
    /** 本次批量总数 */
    total?: number
    /** 确认请求进行中：按钮禁用，防止重复提交 */
    busy?: boolean
  }>(),
  { item: null, summaryUnread: null, remaining: 1, position: 1, total: 1, busy: false },
)

const emit = defineEmits<{
  acknowledge: []
  'view-related': [item: NotificationItem]
  'open-center': []
}>()

const alertIcons = {
  change: 'arrow-right-left',
  review: 'clipboard-check',
  task: 'user-check',
  announcement: 'bell',
} as const

const summaryMode = computed(() => !props.item && (props.summaryUnread ?? 0) > 0)
const visible = computed(() => Boolean(props.item) || summaryMode.value)
const heading = computed(() => (summaryMode.value ? `你有 ${props.summaryUnread} 条未读通知` : props.item?.title || '新通知'))
const bodyText = computed(() => {
  if (summaryMode.value) return `共 ${props.summaryUnread} 条未读通知，请打开通知中心查看处理。`
  return props.item?.content?.trim() || '无详细说明。'
})
const icon = computed(() => (props.item ? alertIcons[props.item.kind] ?? 'bell' : 'bell'))
const relatedLabel = computed(() => notificationActionLabel(props.item))
const queueLabel = computed(() => {
  if (!props.item || props.total <= 1) return ''
  const rest = Math.max(0, props.remaining - 1)
  return rest ? `第 ${props.position} / ${props.total} 条 · 还有 ${rest} 条待确认` : `第 ${props.position} / ${props.total} 条 · 最后一条`
})

const acknowledgeButton = ref<HTMLButtonElement | null>(null)

async function focusAcknowledge() {
  if (!visible.value) return
  await nextTick()
  acknowledgeButton.value?.focus()
}

watch([() => props.item?.id, () => props.summaryUnread, visible], focusAcknowledge, { immediate: true })
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="alert-overlay">
      <section
        class="modal alert-modal"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="notification-alert-title"
        aria-describedby="notification-alert-body"
      >
        <header class="modal-head">
          <h3 id="notification-alert-title">{{ heading }}</h3>
          <span class="alert-kind">{{ summaryMode ? '未读汇总' : '新通知' }}</span>
        </header>

        <div class="modal-body">
          <div class="confirm-body alert-body">
            <DemoIcon :name="icon" :size="30" />
            <p id="notification-alert-body" class="alert-content">{{ bodyText }}</p>
          </div>
          <p v-if="item" class="alert-meta">{{ item.senderName }} · {{ formatReadableDateTime(item.createdAt) }}</p>
          <p v-if="queueLabel" class="alert-queue"><DemoIcon name="bell" :size="13" />{{ queueLabel }}</p>
        </div>

        <footer class="modal-foot">
          <p v-if="busy" class="alert-busy" role="status">正在标记已读…</p>
          <template v-if="summaryMode">
            <button class="btn" type="button" :disabled="busy" @click="emit('open-center')">去通知中心</button>
          </template>
          <template v-else-if="item?.drawingId">
            <button class="btn" type="button" :disabled="busy" @click="emit('view-related', item)">{{ relatedLabel }}</button>
          </template>
          <button ref="acknowledgeButton" class="btn primary" type="button" :disabled="busy" @click="emit('acknowledge')">
            <DemoIcon name="check" :size="14" />知道了
          </button>
        </footer>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
/* 层级高于通知中心抽屉（z-index 2100），保证“不漏提醒”。 */
.alert-overlay {
  position: fixed;
  inset: 0;
  z-index: 2300;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgb(4 8 15 / 58%);
  backdrop-filter: blur(5px);
  animation: alert-fade-in 0.25s;
}

.alert-modal {
  width: 480px;
  animation: alert-modal-in 0.32s cubic-bezier(0.2, 0.9, 0.3, 1.15);
}

.alert-modal .modal-body {
  padding-bottom: 8px;
}

.alert-body {
  padding: 4px 4px 0;
}

.alert-content {
  max-height: 42vh;
  overflow-y: auto;
  padding: 0 2px;
}

.alert-kind {
  flex-shrink: 0;
  padding: 3px 9px;
  border: 1px solid var(--line);
  border-radius: 999px;
  color: var(--accent);
  font-size: 11px;
}

.alert-meta {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 12px;
  text-align: center;
}

.alert-queue {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  margin: 12px 0 0;
  padding-top: 10px;
  border-top: 1px dashed var(--line);
  color: var(--text-2);
  font-size: 12px;
}

.alert-modal .modal-foot { align-items: center; }

/* 处理中提示靠左，与右侧按钮分开。 */
.alert-busy {
  margin: 0 auto 0 0;
  color: var(--text-3);
  font-size: 12px;
}

@keyframes alert-fade-in {
  from { opacity: 0; }
}

@keyframes alert-modal-in {
  from { opacity: 0; transform: translateY(18px) scale(0.95); }
}

@media (prefers-reduced-motion: reduce) {
  .alert-overlay,
  .alert-modal { animation: none; }
}
</style>
