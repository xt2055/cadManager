<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'

import { changeRequestService, CHANGE_STATUS_LABELS, type ChangeRequest, type ChangeUserOption } from '@/services/change-request.service'
import EvidenceDocuments from '@/features/drawings/components/EvidenceDocuments.vue'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AdminChangeApprovalsPage' })

const uiStore = useUiStore()
const loading = ref(false)
const busyId = ref('')
const items = ref<ChangeRequest[]>([])
const selectedId = ref('')
const selected = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
const approveForm = reactive({ opinion: '', requireVerify: true, waiveReason: '', executorId: '' })
const users = ref<ChangeUserOption[]>([])
watch(selected, (item) => { approveForm.executorId = item?.executorId || ''; approveForm.opinion = ''; decision.opinion = '' })
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
  if (!approveForm.executorId) { uiStore.toast('请指定负责修改的设计员', 'warn'); return }
  void run(item.id, () => changeRequestService.approve(item.id, approveForm.opinion.trim(), true, '', approveForm.executorId), '已批准并指定设计员，修改后将重新进行完整审核')
}

function reject(item: ChangeRequest) {
  if (!decision.opinion.trim()) {
    uiStore.toast('驳回必须填写意见', 'warn')
    return
  }
  void run(item.id, () => changeRequestService.reject(item.id, decision.opinion.trim()), '已驳回变更申请')
}

onMounted(() => { void load(); void changeRequestService.listUsers().then(result => { users.value = result }).catch(() => uiStore.toast('人员列表加载失败，请刷新', 'warn')) })
</script>

<template>
  <div class="page admin-page">
    <div class="section-head">
      <h3>变更审批</h3>
      <span class="lib-count">审批变更申请并指定设计员；技术审核由完整流程负责</span>
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
          <label>指定设计员<select v-model="approveForm.executorId" required><option value="">请选择设计员</option><option v-for="user in users" :key="user.id" :value="user.id">{{ user.displayName }}</option></select></label>
          <p>修改完成后必须重新经过全部审核节点，通过后系统自动发布新版。</p>
          <div class="btns">
            <button class="btn primary" type="button" :disabled="busyId === selected.id" @click="approve(selected)">批准</button>
            <input v-model="decision.opinion" type="text" placeholder="驳回意见" />
            <button class="btn danger" type="button" :disabled="busyId === selected.id" @click="reject(selected)">驳回</button>
          </div>
        </div>

        <div v-else-if="selected.status === 'pending_verify'" class="actions">
          <p>成果正在进行完整审核，由当前节点责任人签署。</p>
          <RouterLink class="btn primary" :to="`/reviews/task/${encodeURIComponent(selected.drawingNo)}`">进入审核中心</RouterLink>
        </div>
        <EvidenceDocuments :key="selected.id" :drawing-id="selected.drawingId" :change-request-id="selected.id" read-only />
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
