<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'

import { changeRequestService, CHANGE_STATUS_LABELS, type ChangeRequest } from '@/services/change-request.service'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AdminChangeApprovalsPage' })

const uiStore = useUiStore()
const loading = ref(false)
const busyId = ref('')
const items = ref<ChangeRequest[]>([])
const selectedId = ref('')
const selected = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
const approveForm = reactive({ opinion: '', requireVerify: true, waiveReason: '' })
const decision = reactive({ opinion: '' })

const pending = computed(() => items.value.filter((item) => item.status === 'pending_approval' || item.status === 'pending_verify'))

async function load() {
  loading.value = true
  try {
    const list = await changeRequestService.list({ open: true })
    items.value = await Promise.all(list.map((item) => changeRequestService.get(item.id)))
    if (!selected.value && pending.value[0]) selectedId.value = pending.value[0].id
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '变更工单读取失败', 'warn')
  } finally {
    loading.value = false
  }
}

async function run(id: string, action: () => Promise<unknown>, success: string) {
  busyId.value = id
  try {
    await action()
    uiStore.toast(success, 'ok')
    await load()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '操作失败', 'warn')
  } finally {
    busyId.value = ''
  }
}

function approve(item: ChangeRequest) {
  if (!approveForm.opinion.trim()) {
    uiStore.toast('请填写审批意见', 'warn')
    return
  }
  if (!approveForm.requireVerify && !approveForm.waiveReason.trim()) {
    uiStore.toast('关闭验收必须填写免验收原因', 'warn')
    return
  }
  void run(item.id, () => changeRequestService.approve(item.id, approveForm.opinion.trim(), approveForm.requireVerify, approveForm.waiveReason.trim()), '已批准，图纸进入变更执行')
}

function reject(item: ChangeRequest) {
  if (!decision.opinion.trim()) {
    uiStore.toast('驳回必须填写意见', 'warn')
    return
  }
  void run(item.id, () => changeRequestService.reject(item.id, decision.opinion.trim()), '已驳回变更申请')
}

function verify(item: ChangeRequest) {
  if (!decision.opinion.trim()) {
    uiStore.toast('验收通过必须填写意见', 'warn')
    return
  }
  if (!item.currentSubmissionId) {
    uiStore.toast('请刷新并加载完整提交快照后再验收', 'warn')
    return
  }
  void run(item.id, () => changeRequestService.verify(item.id, decision.opinion.trim(), item.currentSubmissionId), '验收通过，正式版本已发布')
}

function returnForEdit(item: ChangeRequest) {
  if (!decision.opinion.trim()) {
    uiStore.toast('退回修改必须填写意见', 'warn')
    return
  }
  void run(item.id, () => changeRequestService.returnForEdit(item.id, decision.opinion.trim()), '已退回修改')
}

onMounted(() => { void load() })
</script>

<template>
  <div class="page admin-page">
    <div class="section-head">
      <h3>变更审批</h3>
      <span class="lib-count">批准待审工单，验收已提交的变更成果</span>
      <button class="btn sm" type="button" :disabled="loading" @click="load">刷新</button>
    </div>

    <div class="layout">
      <section class="card list-card">
        <p v-if="loading" class="hint">加载中…</p>
        <p v-else-if="!pending.length" class="hint">暂无待审批或待验收工单</p>
        <button
          v-for="item in pending"
          :key="item.id"
          class="row"
          :class="{ active: item.id === selectedId }"
          type="button"
          @click="selectedId = item.id"
        >
          <strong>{{ item.requestNo }}</strong>
          <span>{{ item.drawingNo }} · {{ CHANGE_STATUS_LABELS[item.status] }}</span>
          <small>{{ item.applicantName }} · {{ item.createdAt.slice(0, 16).replace('T', ' ') }}</small>
        </button>
      </section>

      <section v-if="selected" class="card detail-card">
        <header>
          <h4>{{ selected.requestNo }}</h4>
          <span class="tag">{{ CHANGE_STATUS_LABELS[selected.status] }}</span>
        </header>
        <dl>
          <div><dt>图纸</dt><dd>{{ selected.drawingNo }} · {{ selected.title }}</dd></div>
          <div><dt>原因</dt><dd>{{ selected.reason }}</dd></div>
          <div><dt>范围</dt><dd class="pre">{{ selected.scope }}</dd></div>
          <div><dt>执行人</dt><dd>{{ selected.executorName }}</dd></div>
        </dl>
        <div v-if="selected.actualChanges" class="block"><b>实际修改</b><p>{{ selected.actualChanges }}</p></div>
        <div v-if="selected.diffs?.length" class="block">
          <b>差异</b>
          <p v-for="(diff, index) in selected.diffs" :key="index">{{ diff.field }}：{{ diff.oldValue || '（空）' }} → {{ diff.newValue || '（空）' }}</p>
        </div>

        <div v-if="selected.status === 'pending_approval'" class="actions">
          <label>审批意见<input v-model="approveForm.opinion" type="text" /></label>
          <label class="inline"><input v-model="approveForm.requireVerify" type="checkbox" /><span>修改完成后需要验收</span></label>
          <label v-if="!approveForm.requireVerify">免验收原因<textarea v-model="approveForm.waiveReason" rows="2" /></label>
          <div class="btns">
            <button class="btn primary" type="button" :disabled="busyId === selected.id" @click="approve(selected)">批准</button>
            <input v-model="decision.opinion" type="text" placeholder="驳回意见" />
            <button class="btn danger" type="button" :disabled="busyId === selected.id" @click="reject(selected)">驳回</button>
          </div>
        </div>

        <div v-else-if="selected.status === 'pending_verify'" class="actions">
          <label>验收意见<input v-model="decision.opinion" type="text" /></label>
          <div class="btns">
            <button class="btn primary" type="button" :disabled="busyId === selected.id" @click="verify(selected)">验收并发布</button>
            <button class="btn" type="button" :disabled="busyId === selected.id" @click="returnForEdit(selected)">退回修改</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 16px; }
.section-head h3 { font-family: var(--font-display); font-size: 16px; font-weight: 800; }
.lib-count { color: var(--text-3); font-size: 11.5px; margin-right: auto; }
.layout { display: grid; grid-template-columns: minmax(280px, 0.8fr) minmax(0, 1.4fr); gap: 16px; }
.list-card, .detail-card { padding: 16px; }
.row { display: flex; flex-direction: column; gap: 4px; width: 100%; padding: 10px; border-radius: 10px; text-align: left; }
.row.active, .row:hover { background: var(--active); }
.row span, .row small, .hint { color: var(--text-3); font-size: 12px; }
.detail-card header { display: flex; align-items: center; gap: 10px; }
.detail-card h4 { margin: 0; }
dl { display: grid; gap: 8px; }
dt { color: var(--text-3); font-size: 11px; }
.pre { white-space: pre-wrap; }
.block { margin: 12px 0; }
.actions, .btns { display: flex; flex-direction: column; gap: 10px; }
.btns { flex-direction: row; align-items: center; }
label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; }
.inline { flex-direction: row; align-items: center; gap: 8px; }
input, textarea { font: inherit; padding: 6px 8px; border: 1px solid var(--line); border-radius: 6px; }
@media (max-width: 900px) { .layout { grid-template-columns: 1fr; } }
</style>
