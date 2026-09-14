<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { notificationService, type NotificationRecipient } from '@/services/notification.service'
import { useAuthStore } from '@/stores/auth.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'NotificationManagementPage' })

const auth = useAuthStore()
const ui = useUiStore()

const loading = ref(false)
const sending = ref(false)
const loadError = ref('')
const sendError = ref('')
const recipients = ref<NotificationRecipient[]>([])
const form = reactive({ title: '', content: '', broadcast: false, recipientIds: [] as string[] })

const pickerOpen = ref(false)
const keyword = ref('')
const pickerEl = ref<HTMLElement | null>(null)

const selectedUsers = computed(() =>
  form.recipientIds
    .map(id => recipients.value.find(user => user.id === id))
    .filter((user): user is NotificationRecipient => Boolean(user)),
)
const visibleTags = computed(() => selectedUsers.value.slice(0, 3))
const hiddenCount = computed(() => Math.max(0, selectedUsers.value.length - visibleTags.value.length))
const filteredRecipients = computed(() => {
  const query = keyword.value.trim().toLowerCase()
  if (!query) return recipients.value
  return recipients.value.filter(user => user.displayName.toLowerCase().includes(query) || user.account.toLowerCase().includes(query))
})
const recipientLabel = computed(() => (form.broadcast ? `全体 ${recipients.value.length} 位启用用户` : `${form.recipientIds.length} 位指定用户`))

function message(value: unknown) { return value instanceof Error ? value.message : '操作失败，请稍后重试' }

function toggleRecipient(id: string) {
  const index = form.recipientIds.indexOf(id)
  if (index >= 0) form.recipientIds.splice(index, 1)
  else form.recipientIds.push(id)
}

function selectVisible() {
  const merged = new Set(form.recipientIds)
  filteredRecipients.value.forEach(user => merged.add(user.id))
  form.recipientIds = [...merged]
}

function clearRecipients() { form.recipientIds = [] }

function onDocumentPointer(event: MouseEvent) {
  if (pickerOpen.value && pickerEl.value && !pickerEl.value.contains(event.target as Node)) pickerOpen.value = false
}

async function loadRecipients() {
  loading.value = true
  try {
    const users = await notificationService.recipients(auth.token)
    recipients.value = users.filter(user => user.status === 'active')
    loadError.value = ''
  } catch (e) {
    loadError.value = message(e)
  } finally {
    loading.value = false
  }
}

async function send() {
  if (sending.value) return
  sendError.value = ''
  if (!form.title.trim() || !form.content.trim()) { sendError.value = '请填写通知标题和正文'; return }
  if (!form.broadcast && !form.recipientIds.length) { sendError.value = '请至少选择一位收件人'; return }
  sending.value = true
  try {
    const result = await notificationService.send(auth.token, { ...form, recipientIds: form.broadcast ? [] : [...form.recipientIds] })
    ui.toast(`通知已发送给 ${result.sent} 位用户`)
    Object.assign(form, { title: '', content: '', broadcast: false, recipientIds: [] })
    pickerOpen.value = false
  } catch (e) {
    sendError.value = message(e)
  } finally {
    sending.value = false
  }
}

onMounted(() => {
  document.addEventListener('mousedown', onDocumentPointer)
  void loadRecipients()
})

onBeforeUnmount(() => document.removeEventListener('mousedown', onDocumentPointer))
</script>

<template>
  <div class="page notification-admin">
    <header class="page-head">
      <div>
        <h1>通知管理</h1>
        <p>向指定用户或全体成员发送系统公告，用于发布变更、审批与运维提醒。</p>
      </div>
      <button class="btn" type="button" :disabled="loading" @click="loadRecipients">
        <DemoIcon name="refresh-cw" :size="14" /><span>{{ loading ? '加载中…' : '刷新收件人' }}</span>
      </button>
    </header>

    <section class="card card-pad compose-card">
      <form class="compose-form" @submit.prevent="send">
        <div class="field">
          <label>发送范围</label>
          <div class="scope-options">
            <button type="button" class="scope-option" :class="{ active: !form.broadcast }" @click="form.broadcast = false">
              <span class="scope-radio" :class="{ on: !form.broadcast }"></span>
              <span class="scope-text"><strong>指定用户</strong><small>仅推送给选中的启用账号</small></span>
            </button>
            <button type="button" class="scope-option" :class="{ active: form.broadcast }" @click="form.broadcast = true">
              <span class="scope-radio" :class="{ on: form.broadcast }"></span>
              <span class="scope-text"><strong>全体用户</strong><small>推送给所有启用的账号</small></span>
            </button>
          </div>
        </div>

        <div v-if="!form.broadcast" class="field">
          <label>收件人<span class="field-hint">已选 {{ form.recipientIds.length }} / {{ recipients.length }} 人</span></label>
          <div ref="pickerEl" class="user-picker">
            <button type="button" class="picker-box" :class="{ open: pickerOpen }" @click="pickerOpen = !pickerOpen">
              <span class="picker-values">
                <span v-if="!selectedUsers.length" class="picker-placeholder">搜索姓名或账号，可选择多人</span>
                <span v-for="user in visibleTags" :key="user.id" class="picker-tag">
                  {{ user.displayName }}
                  <i role="button" aria-label="移除收件人" @click.stop="toggleRecipient(user.id)"><DemoIcon name="x" :size="10" /></i>
                </span>
                <span v-if="hiddenCount" class="picker-more">+{{ hiddenCount }}</span>
              </span>
              <DemoIcon name="chevron-down" :size="14" class="picker-caret" />
            </button>
            <div v-if="pickerOpen" class="picker-panel">
              <label class="picker-search">
                <DemoIcon name="search" :size="14" />
                <input v-model="keyword" type="text" placeholder="搜索姓名或账号" />
              </label>
              <div class="picker-tools">
                <span>{{ filteredRecipients.length }} 位匹配</span>
                <div class="picker-tools__actions">
                  <button type="button" @click="selectVisible">全选</button>
                  <button type="button" :disabled="!form.recipientIds.length" @click="clearRecipients">清空</button>
                </div>
              </div>
              <ul class="picker-list">
                <li v-for="user in filteredRecipients" :key="user.id">
                  <label class="picker-option" :class="{ checked: form.recipientIds.includes(user.id) }">
                    <input type="checkbox" :checked="form.recipientIds.includes(user.id)" @change="toggleRecipient(user.id)" />
                    <span class="picker-option__name">{{ user.displayName }}</span>
                    <span class="picker-option__account">{{ user.account }}</span>
                  </label>
                </li>
                <li v-if="!filteredRecipients.length" class="picker-empty">{{ loading ? '正在加载用户…' : '没有匹配的用户' }}</li>
              </ul>
            </div>
          </div>
        </div>
        <p v-else class="note"><DemoIcon name="info" :size="14" />将发送给所有启用的账号，包括当前管理员。</p>

        <div class="field">
          <label>标题<span class="field-hint">{{ form.title.length }} / 120</span></label>
          <input v-model="form.title" class="inp" maxlength="120" required placeholder="请输入通知标题" />
        </div>

        <div class="field field-last">
          <label>正文<span class="field-hint">{{ form.content.length }} / 5000</span></label>
          <textarea v-model="form.content" class="inp" maxlength="5000" rows="6" required placeholder="请输入通知内容" />
        </div>

        <p v-if="loadError" role="alert" class="compose-error">{{ loadError }}<button type="button" class="btn sm" @click="loadRecipients">重新加载</button></p>
        <p v-if="sendError" role="alert" class="compose-error">{{ sendError }}</p>

        <div class="compose-foot">
          <span class="compose-foot__hint">将发送给 <strong>{{ recipientLabel }}</strong></span>
          <button class="btn primary" type="submit" :disabled="sending || loading">
            <DemoIcon name="bell" :size="14" /><span>{{ sending ? '发送中…' : '发送通知' }}</span>
          </button>
        </div>
      </form>
    </section>
  </div>
</template>

<style scoped>
.notification-admin { display: flex; flex-direction: column; gap: 18px; width: 100%; }
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
.page-head h1 { font-size: 18px; }
.page-head p { margin-top: 4px; color: var(--text-3); font-size: 12px; }

.compose-card { max-width: 780px; }
.compose-form { display: flex; flex-direction: column; }
.compose-form .field > label { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.field-hint { color: var(--text-3); font-size: 11px; font-weight: 400; }

.scope-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 8px; }
.scope-option { display: flex; align-items: flex-start; gap: 10px; padding: 13px 15px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel-2); text-align: left; transition: border-color 0.25s, background-color 0.25s; }
.scope-option:hover { border-color: var(--line-strong); }
.scope-option.active { border-color: var(--accent); background: var(--accent-soft); box-shadow: 0 0 0 1px var(--accent-soft); }
.scope-radio { position: relative; width: 16px; height: 16px; flex: none; margin-top: 2px; border: 1.5px solid var(--line-strong); border-radius: 50%; transition: border-color 0.2s; }
.scope-option.active .scope-radio { border-color: var(--accent); }
.scope-radio.on::after { position: absolute; inset: 3px; border-radius: 50%; background: var(--accent); content: ''; }
.scope-text strong { display: block; color: var(--text-1); font-size: 12.5px; }
.scope-text small { display: block; margin-top: 3px; color: var(--text-3); font-size: 11px; }

.user-picker { position: relative; }
.picker-box { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 38px; padding: 6px 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--panel-2); transition: border-color 0.25s, box-shadow 0.25s; }
.picker-box:hover { border-color: var(--line-strong); }
.picker-box.open { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
.picker-values { display: flex; flex: 1; flex-wrap: wrap; align-items: center; gap: 6px; min-width: 0; }
.picker-placeholder { color: var(--text-3); font-size: 12.5px; }
.picker-tag { display: inline-flex; align-items: center; gap: 5px; padding: 3px 6px 3px 9px; border: 1px solid var(--accent); border-radius: 7px; background: var(--accent-soft); color: var(--text-1); font-size: 11.5px; }
.picker-tag i { display: grid; place-items: center; color: var(--text-3); cursor: pointer; transition: color 0.2s; }
.picker-tag i:hover { color: var(--danger); }
.picker-more { color: var(--text-3); font-size: 11.5px; }
.picker-caret { flex: none; color: var(--text-3); }

.picker-panel { position: absolute; z-index: 30; top: calc(100% + 6px); right: 0; left: 0; display: flex; flex-direction: column; overflow: hidden; border: 1px solid var(--line); border-radius: 11px; background: var(--panel); box-shadow: var(--shadow); }
.picker-search { display: flex; align-items: center; gap: 8px; padding: 9px 12px; border-bottom: 1px solid var(--line); color: var(--text-3); }
.picker-search input { flex: 1; border: none; outline: none; background: transparent; color: var(--text-1); font-size: 12.5px; }
.picker-tools { display: flex; justify-content: space-between; align-items: center; padding: 7px 12px; color: var(--text-3); font-size: 11px; }
.picker-tools__actions { display: flex; gap: 6px; }
.picker-tools__actions button { padding: 2px 9px; border: 1px solid var(--line); border-radius: 6px; background: transparent; color: var(--text-2); font-size: 11px; transition: all 0.2s; }
.picker-tools__actions button:hover:not(:disabled) { color: var(--accent); border-color: var(--accent); }
.picker-tools__actions button:disabled { opacity: 0.4; cursor: not-allowed; }
.picker-list { max-height: 250px; overflow-y: auto; padding: 6px; }
.picker-option { display: flex; align-items: center; gap: 9px; padding: 8px 10px; border-radius: 8px; cursor: pointer; transition: background-color 0.15s; }
.picker-option:hover { background: var(--hover); }
.picker-option.checked { background: var(--accent-soft); }
.picker-option__name { color: var(--text-1); font-size: 12.5px; }
.picker-option__account { margin-left: auto; color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.picker-empty { padding: 24px; color: var(--text-3); font-size: 12px; text-align: center; }

.note { margin-top: 12px; }
.compose-error { display: flex; align-items: center; gap: 10px; margin-top: 12px; color: var(--danger); font-size: 12px; }
.compose-foot { display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-top: 22px; padding-top: 16px; border-top: 1px solid var(--line); }
.compose-foot__hint { color: var(--text-3); font-size: 12px; }
.compose-foot__hint strong { color: var(--text-1); }
</style>
