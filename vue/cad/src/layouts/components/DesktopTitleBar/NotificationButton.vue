<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import DemoIcon from '@/components/common/DemoIcon.vue'
import NotificationAlertDialog from '@/components/feedback/NotificationAlertDialog.vue'
import { createNotificationAlertTracker } from '@/features/notifications/notification-alert-tracker'
import { notificationService, type NotificationItem } from '@/services/notification.service'
import { connectNotifications, type NotificationConnectionState } from '@/services/notification-socket'
import { useAuthStore } from '@/stores/auth.store'
import { useUiStore } from '@/stores/ui.store'
import { formatReadableDateTime } from '@/utils/date-time'

const auth = useAuthStore()
const ui = useUiStore()
const router = useRouter()
const open = ref(false)
const unread = ref(0)
const items = ref<NotificationItem[]>([])
const total = ref(0)
const page = ref(1)
const unreadOnly = ref(false)
const loading = ref(false)
const error = ref('')
const updating = ref(false)
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / 20)))
let alive = true
let listVersion = 0
let syncing = false
let syncAgain = false
const ALERT_QUEUE_LIMIT = 20
const alerts = ref<NotificationItem[]>([])
const summaryUnread = ref<number | null>(null)
const alertHandled = ref(0)
const alertBusy = ref(false)
const tracker = createNotificationAlertTracker()
let disconnectSocket: (() => void) | undefined
const connection = ref<NotificationConnectionState>('connecting')
const connectionLabel = computed(() => ({ connecting: '正在连接实时通知…', connected: '实时通知已连接', disconnected: '连接中断，正在重连…', unauthorized: '登录已失效，请重新登录' })[connection.value])

function message(value: unknown) { return value instanceof Error ? value.message : '操作失败，请稍后重试' }
function sameSession(token: string) { return alive && token === auth.token }

async function load() {
  const version = ++listVersion
  const token = auth.token
  if (!token) return
  loading.value = true
  try {
    const result = await notificationService.list(token, page.value, unreadOnly.value)
    if (!sameSession(token) || version !== listVersion) return
    items.value = result.items
    total.value = result.total
    unread.value = result.unread
    error.value = ''
    if (page.value > pageCount.value) page.value = pageCount.value
  } catch (e) {
    if (sameSession(token) && version === listVersion) error.value = message(e)
  } finally {
    if (sameSession(token) && version === listVersion) loading.value = false
  }
}

async function sync() {
  if (!auth.token) return
  if (syncing) { syncAgain = true; return }
  const token = auth.token
  syncing = true
  try {
    const result = await notificationService.list(token)
    if (!sameSession(token)) return
    const batch = tracker.collect(result.items)
    if (batch.seeded) {
      if (result.unread > 0) summaryUnread.value = result.unread
    } else if (batch.alerts.length) {
      enqueueAlerts(batch.alerts)
    }
    unread.value = result.unread
    error.value = ''
    if (open.value) await load()
  } catch (e) {
    if (sameSession(token)) error.value = message(e)
  } finally {
    syncing = false
    if (alive && syncAgain) { syncAgain = false; void sync() }
  }
}

async function markRead(item?: NotificationItem) {
  if (updating.value) return
  const token = auth.token
  updating.value = true
  try {
    if (item) await notificationService.read(token, item.id)
    else await notificationService.readAll(token)
    if (sameSession(token)) await load()
  } catch (e) { if (sameSession(token)) ui.toast(message(e), 'warn') }
  finally { if (sameSession(token)) updating.value = false }
}

async function viewRelated(item: NotificationItem) {
  if (!item.readAt) await markRead(item)
  open.value = false
  await router.push(`/drawings/${encodeURIComponent(item.drawingId)}/${item.kind === 'change' ? 'changes' : 'review'}`)
}

function enqueueAlerts(incoming: NotificationItem[]) {
  const queued = new Set(alerts.value.map(item => item.id))
  for (const item of incoming) {
    if (alerts.value.length >= ALERT_QUEUE_LIMIT) break
    if (queued.has(item.id)) continue
    queued.add(item.id)
    alerts.value.push(item)
  }
}

function dropAlert(id: string) {
  const index = alerts.value.findIndex(item => item.id === id)
  if (index < 0) return
  alerts.value.splice(index, 1)
  alertHandled.value += 1
  if (!alerts.value.length) alertHandled.value = 0
}

/** 弹窗必须确认：单条「知道了」= 标记该条已读并出队；汇总模式只关闭，不批量已读。 */
async function acknowledgeAlert() {
  if (alertBusy.value) return
  if (summaryUnread.value !== null) { summaryUnread.value = null; return }
  const item = alerts.value[0]
  if (!item) return
  alertBusy.value = true
  try {
    // 标记已读失败也会关闭弹窗（角标保持未读、可在通知中心重试），避免强制弹窗把用户卡住。
    await markRead(item)
  } finally {
    alertBusy.value = false
    dropAlert(item.id)
  }
}

async function viewRelatedAlert(item: NotificationItem) {
  if (alertBusy.value) return
  alertBusy.value = true
  dropAlert(item.id)
  try {
    await viewRelated(item)
  } finally {
    alertBusy.value = false
  }
}

function openCenterFromAlert() {
  summaryUnread.value = null
  open.value = true
}

function startSocket() {
  disconnectSocket?.()
  if (!auth.token) return
  disconnectSocket = connectNotifications(auth.token, {
    ready: () => void sync(),
    changed: () => void sync(),
    state: state => { connection.value = state },
  })
}
function onVisible() { if (document.visibilityState === 'visible') void sync() }
watch(open, value => { if (value) void load() })
watch(unreadOnly, () => { if (page.value !== 1) page.value = 1; else void load() })
watch(page, () => void load())
watch(() => auth.token, () => {
  ++listVersion
  items.value = []; unread.value = 0; total.value = 0; error.value = ''
  tracker.reset()
  alerts.value = []; summaryUnread.value = null; alertHandled.value = 0; alertBusy.value = false
  open.value = false
  loading.value = false; updating.value = false
  startSocket()
  void sync()
})
onMounted(() => {
  void sync()
  startSocket()
  document.addEventListener('visibilitychange', onVisible)
})
onUnmounted(() => {
  alive = false; ++listVersion
  disconnectSocket?.()
  document.removeEventListener('visibilitychange', onVisible)
})
</script>

<template>
  <button class="icon-btn notification-bell" type="button" :title="error ? '通知暂未更新，点击重试' : `通知 · ${unread} 条未读`" :aria-label="`通知，${unread} 条未读`" :aria-expanded="open" @click="open = true">
    <DemoIcon name="bell" />
    <span v-if="unread" class="notification-count">{{ unread > 99 ? '99+' : unread }}</span>
    <span v-else-if="error" class="notification-count">!</span>
  </button>
  <Teleport to="body">
    <Transition name="notification-drawer">
      <div v-if="open" class="notification-overlay" @mousedown.self="open = false">
        <aside class="notification-panel" role="dialog" aria-modal="true" aria-label="通知中心">
          <header class="notification-panel__head">
            <div class="notification-panel__title"><DemoIcon name="bell" :size="16" /><span>通知中心</span></div>
            <button class="notification-close" type="button" aria-label="关闭" @click="open = false"><DemoIcon name="x" :size="16" /></button>
          </header>
          <div class="notification-panel__body">
            <div class="notification-center">
      <p class="notification-muted" role="status">{{ connectionLabel }}</p>
      <div class="notification-toolbar">
        <div class="notification-filters">
          <span class="notification-muted">{{ unread }} 条未读</span>
          <label class="notification-radio"><input v-model="unreadOnly" type="checkbox" />只看未读</label>
        </div>
        <div class="notification-actions"><button class="btn" type="button" :disabled="loading" @click="load">刷新</button><button class="btn" type="button" :disabled="!unread || updating" @click="markRead()">全部已读</button></div>
      </div>
      <p v-if="error" role="alert" class="notification-error">{{ error }} <button class="btn" type="button" @click="load">重试</button></p>
      <p v-if="loading && !items.length" class="notification-empty">正在加载通知…</p>
      <div v-else-if="!items.length && !error" class="notification-empty"><DemoIcon name="bell" :size="32" /><p>{{ unreadOnly ? '没有未读通知' : '暂无通知' }}</p></div>
      <div class="notification-list" :aria-busy="loading">
        <article v-for="item in items" :key="item.id" class="notification-card" :class="{ unread: !item.readAt }">
          <div class="notification-card-heading"><strong>{{ item.title }}</strong><span v-if="!item.readAt" class="notification-unread-label">未读</span></div>
          <p class="notification-content">{{ item.content }}</p>
          <p class="notification-muted">{{ item.kind === 'announcement' ? item.senderName : '系统通知' }} · {{ formatReadableDateTime(item.createdAt) }}</p>
          <div class="notification-actions"><button v-if="item.drawingId" class="btn" type="button" :disabled="updating" @click="viewRelated(item)">{{ item.kind === 'change' ? '查看变更工单' : '查看图纸审批' }}</button><button v-if="!item.readAt" class="btn" type="button" :disabled="updating" @click="markRead(item)">标为已读</button></div>
        </article>
      </div>
      <div v-if="total > 20" class="notification-toolbar notification-pages"><button class="btn" type="button" :disabled="page === 1 || loading" @click="page--">上一页</button><span>{{ page }} / {{ pageCount }} · 共 {{ total }} 条</span><button class="btn" type="button" :disabled="page >= pageCount || loading" @click="page++">下一页</button></div>
            </div>
          </div>
        </aside>
      </div>
    </Transition>
      <NotificationAlertDialog
        :item="alerts[0] || null"
        :summary-unread="summaryUnread"
        :remaining="alerts.length"
        :position="alertHandled + 1"
        :total="alertHandled + alerts.length"
        :busy="alertBusy"
        @acknowledge="acknowledgeAlert"
        @view-related="viewRelatedAlert"
        @open-center="openCenterFromAlert"
      />
  </Teleport>
</template>

<style scoped>
.notification-bell { position: relative; }
.notification-count { position: absolute; top: -5px; right: -8px; min-width: 16px; padding: 1px 4px; border-radius: 10px; background: var(--danger); color: white; font-size: 10px; line-height: 14px; }
.notification-center { color: var(--text-1); font-size: 13px; }
.notification-toolbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 20px; }
.notification-filters { display: flex; align-items: center; gap: 14px; }
.notification-toolbar .notification-muted { margin: 0; }
.notification-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.notification-muted { color: var(--text-3); font-size: 12px; margin: 12px 0; }
.notification-list { display: flex; flex-direction: column; gap: 12px; }
.notification-card { padding: 18px; border: 1px solid var(--line); border-radius: 12px; background: var(--panel); }
.notification-card.unread { border-left: 3px solid var(--accent); }
.notification-card-heading { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; overflow-wrap: anywhere; }
.notification-unread-label { flex-shrink: 0; color: var(--accent); font-size: 11px; }
.notification-content { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.8; margin: 12px 0; }
.notification-error { color: var(--danger); line-height: 1.6; }
.notification-empty { padding: 60px 0; text-align: center; color: var(--text-3); }
.notification-pages { margin-top: 20px; }
.notification-radio { display: inline-flex; align-items: center; gap: 7px; margin-right: 16px; cursor: pointer; }
.notification-overlay { position: fixed; inset: 0; z-index: 2100; display: flex; justify-content: flex-end; background: rgb(0 0 0 / 45%); }
.notification-panel { display: flex; flex-direction: column; width: min(560px, 100vw); height: 100%; border-left: 1px solid var(--line); background: var(--panel-top); box-shadow: -24px 0 60px rgb(0 0 0 / 30%); }
.notification-panel__head { display: flex; justify-content: space-between; align-items: center; padding: 18px 20px; border-bottom: 1px solid var(--line); }
.notification-panel__title { display: flex; align-items: center; gap: 9px; color: var(--text-1); font-family: var(--font-display); font-size: 15px; font-weight: 900; }
.notification-panel__title svg { color: var(--accent); }
.notification-close { display: grid; width: 30px; height: 30px; place-items: center; border-radius: 8px; color: var(--text-3); transition: all 0.2s; }
.notification-close:hover { background: var(--hover); color: var(--text-1); }
.notification-panel__body { flex: 1; overflow-y: auto; padding: 18px 20px; }

.notification-drawer-enter-active,
.notification-drawer-leave-active { transition: opacity 0.25s ease; }
.notification-drawer-enter-active .notification-panel,
.notification-drawer-leave-active .notification-panel { transition: transform 0.3s cubic-bezier(0.22, 0.8, 0.3, 1); }
.notification-drawer-enter-from,
.notification-drawer-leave-to { opacity: 0; }
.notification-drawer-enter-from .notification-panel,
.notification-drawer-leave-to .notification-panel { transform: translateX(100%); }
</style>
