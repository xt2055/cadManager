<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

import { useAuthStore } from '@/stores/auth.store'
import { useUiStore } from '@/stores/ui.store'
import {
  changeRequestService,
  CHANGE_STATUS_LABELS,
  type ChangeRequest,
  type ChangeUserOption,
  type ProposedAttributes,
} from '@/services/change-request.service'

defineOptions({ name: 'ChangeRequestDialog' })

const props = defineProps<{
  visible: boolean
  drawingId: string
  drawingNo: string
  drawingName: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'changed'): void
}>()

const authStore = useAuthStore()
const uiStore = useUiStore()

const isAdmin = computed(() => authStore.currentUser?.roles?.includes('admin') ?? false)
const currentUserId = computed(() => authStore.currentUser?.id ?? '')

const loading = ref(false)
const requests = ref<ChangeRequest[]>([])
const users = ref<ChangeUserOption[]>([])

const openStatuses = new Set(['pending_approval', 'executing', 'pending_verify'])
const openRequest = computed(() => requests.value.find((item) => openStatuses.has(item.status)) ?? null)
const history = computed(() => requests.value.filter((item) => !openStatuses.has(item.status)))

const form = reactive({ reason: '', scope: '', title: '', requireVerify: true, executorId: '', autoApprove: false, approveOpinion: '', waiveReason: '' })
const approveForm = reactive({ opinion: '', requireVerify: true, waiveReason: '' })
const submitForm = reactive({ actualChanges: '', name: '', material: '', vendor: '', version: '' })
const decision = reactive({ opinion: '', cancelOpinion: '' })
const busy = ref(false)
const expandedId = ref('')
// openDetail 携带进行中工单的差异与操作记录（列表接口不含），供待验收展示与留痕查看。
const openDetail = ref<ChangeRequest | null>(null)

const displayOpen = computed(() => (openDetail.value && openRequest.value && openDetail.value.id === openRequest.value.id ? openDetail.value : openRequest.value))

async function refresh() {
  if (!props.drawingId) return
  loading.value = true
  try {
    requests.value = await changeRequestService.listByDrawing(props.drawingId)
    const open = requests.value.find((item) => openStatuses.has(item.status)) ?? null
    openDetail.value = open ? await changeRequestService.get(open.id) : null
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '变更工单加载失败', 'warn')
  } finally {
    loading.value = false
  }
}

async function loadUsers() {
  if (!isAdmin.value || users.value.length) return
  try {
    users.value = await changeRequestService.listUsers()
  } catch {
    users.value = []
  }
}

watch(() => props.visible, (value) => {
  if (value) {
    resetForms()
    void refresh()
    void loadUsers()
  }
})

function resetForms() {
  form.reason = ''
  form.scope = ''
  form.title = ''
  form.requireVerify = true
  form.executorId = ''
  form.autoApprove = false
  form.approveOpinion = ''
  form.waiveReason = ''
  approveForm.opinion = ''
  approveForm.requireVerify = true
  approveForm.waiveReason = ''
  submitForm.actualChanges = ''
  submitForm.name = ''
  submitForm.material = ''
  submitForm.vendor = ''
  submitForm.version = ''
  decision.opinion = ''
  decision.cancelOpinion = ''
  openDetail.value = null
  expandedId.value = ''
}

async function run(action: () => Promise<unknown>, success: string) {
  busy.value = true
  try {
    await action()
    uiStore.toast(success, 'ok')
    await refresh()
    emit('changed')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '操作失败', 'warn')
  } finally {
    busy.value = false
  }
}

function submitCreate() {
  if (!form.reason.trim() || !form.scope.trim()) {
    uiStore.toast('请填写变更原因和修改范围', 'warn')
    return
  }
  if (form.autoApprove && !form.approveOpinion.trim()) {
    uiStore.toast('管理员直接批准必须填写审批意见', 'warn')
    return
  }
  if (form.autoApprove && !form.requireVerify && !form.waiveReason.trim()) {
    uiStore.toast('免验收必须单独填写原因，不得复用审批意见', 'warn')
    return
  }
  void run(async () => changeRequestService.create({
    drawingId: props.drawingId,
    reason: form.reason.trim(),
    scope: form.scope.trim(),
    title: form.title.trim(),
    executorId: form.executorId || undefined,
    requireVerify: form.requireVerify,
    autoApprove: form.autoApprove && isAdmin.value,
    approveOpinion: form.autoApprove ? form.approveOpinion.trim() : undefined,
    waiveReason: form.autoApprove && !form.requireVerify ? form.waiveReason.trim() : undefined,
  }), '变更申请已提交')
}

function submitApprove() {
  const request = openRequest.value
  if (!request) return
  if (!approveForm.opinion.trim()) {
    uiStore.toast('请填写审批意见', 'warn')
    return
  }
  if (!approveForm.requireVerify && !approveForm.waiveReason.trim()) {
    uiStore.toast('关闭验收必须填写免验收原因', 'warn')
    return
  }
  void run(() => changeRequestService.approve(request.id, approveForm.opinion.trim(), approveForm.requireVerify, approveForm.waiveReason.trim()), '已批准，图纸进入变更执行')
}

function submitReject() {
  const request = openRequest.value
  if (!request) return
  if (!decision.opinion.trim()) {
    uiStore.toast('驳回必须填写意见', 'warn')
    return
  }
  void run(() => changeRequestService.reject(request.id, decision.opinion.trim()), '已驳回变更申请')
}

function buildProposed(): ProposedAttributes {
  const payload: ProposedAttributes = {}
  if (submitForm.name.trim()) payload.name = submitForm.name.trim()
  if (submitForm.material.trim()) payload.material = submitForm.material.trim()
  if (submitForm.vendor.trim()) payload.vendor = submitForm.vendor.trim()
  if (submitForm.version.trim()) payload.version = submitForm.version.trim()
  return payload
}

function submitDone() {
  const request = openRequest.value
  if (!request) return
  if (!submitForm.actualChanges.trim()) {
    uiStore.toast('请填写实际修改说明', 'warn')
    return
  }
  void run(() => changeRequestService.submit(request.id, submitForm.actualChanges.trim(), buildProposed()), '变更已提交')
}

function submitVerify() {
  const request = openRequest.value
  if (!request) return
  if (!decision.opinion.trim()) {
    uiStore.toast('验收通过必须填写意见', 'warn')
    return
  }
  // 携带当前待验收轮次 ID：若工单在此期间被退回重提，后端将拒绝旧页面的过期验收。
  const submissionId = openDetail.value?.id === request.id ? openDetail.value.currentSubmissionId : ''
  if (!submissionId || loading.value) {
    uiStore.toast('请刷新并加载完整提交快照后再验收', 'warn')
    return
  }
  void run(() => changeRequestService.verify(request.id, decision.opinion.trim(), submissionId), '验收通过，正式版本已发布')
}

function submitReturn() {
  const request = openRequest.value
  if (!request) return
  if (!decision.opinion.trim()) {
    uiStore.toast('退回修改必须填写意见', 'warn')
    return
  }
  void run(() => changeRequestService.returnForEdit(request.id, decision.opinion.trim()), '已退回修改')
}

function submitCancel() {
  const request = openRequest.value
  if (!request) return
  if (!decision.cancelOpinion.trim()) {
    uiStore.toast('终止工单必须填写原因', 'warn')
    return
  }
  uiStore.confirm('终止变更工单', `确定终止工单「${request.requestNo}」吗？\n终止后不再发布本次修改，图纸保持存档。`, {
    confirmText: '终止',
    onConfirm: () => run(() => changeRequestService.cancel(request.id, decision.cancelOpinion.trim()), '工单已终止'),
  })
}

async function toggleExpand(item: ChangeRequest) {
  if (expandedId.value === item.id) {
    expandedId.value = ''
    return
  }
  expandedId.value = item.id
  if (!item.actions) {
    try {
      const detail = await changeRequestService.get(item.id)
      const index = requests.value.findIndex((entry) => entry.id === item.id)
      if (index >= 0) requests.value[index] = detail
    } catch {
      /* 忽略：展开失败不影响列表 */
    }
  }
}

function close() {
  emit('update:visible', false)
}

function statusLabel(status: string): string {
  return CHANGE_STATUS_LABELS[status as keyof typeof CHANGE_STATUS_LABELS] ?? status
}

const canEdit = computed(() => {
  const request = openRequest.value
  if (!request) return false
  return request.status === 'executing' && (request.executorId === currentUserId.value || isAdmin.value)
})

const canCancel = computed(() => {
  const request = openRequest.value
  if (!request) return false
  return isAdmin.value || request.applicantId === currentUserId.value
})
</script>

<template>
  <div v-if="visible" class="cr-overlay" @click.self="close">
    <div class="cr-modal">
      <header class="cr-head">
        <div>
          <h3>图纸变更工单</h3>
          <span class="cr-sub">{{ drawingNo }} · {{ drawingName }}</span>
        </div>
        <button class="cr-x" type="button" aria-label="关闭" @click="close">✕</button>
      </header>

      <div class="cr-body">
        <p v-if="loading" class="cr-hint">加载中…</p>

        <!-- 无进行中工单：发起变更 -->
        <section v-else-if="!openRequest" class="cr-section">
          <div class="cr-note">该图纸处于存档保护中。发起变更工单并经管理员审批后，指定执行人方可修改；修改结果默认需验收后才发布正式版。</div>
          <label>变更原因<input v-model="form.reason" type="text" placeholder="为什么要改" /></label>
          <label>修改范围/内容<textarea v-model="form.scope" rows="3" placeholder="计划修改什么" /></label>
          <label>工单标题（可选）<input v-model="form.title" type="text" :placeholder="drawingName" /></label>
          <label v-if="isAdmin" class="cr-inline">
            <input v-model="form.requireVerify" type="checkbox" />
            <span>修改完成后需要验收</span>
          </label>
          <label v-if="isAdmin">执行人（默认本人）
            <select v-model="form.executorId">
              <option value="">我本人</option>
              <option v-for="user in users" :key="user.id" :value="user.id">{{ user.displayName }}</option>
            </select>
          </label>
          <template v-if="isAdmin">
            <label class="cr-inline"><input v-model="form.autoApprove" type="checkbox" /><span>以管理员身份直接批准本申请</span></label>
            <label v-if="form.autoApprove">审批意见<textarea v-model="form.approveOpinion" rows="2" placeholder="必填：审批意见" /></label>
            <label v-if="form.autoApprove && !form.requireVerify">免验收原因<textarea v-model="form.waiveReason" rows="2" placeholder="必填：单独填写免验收原因，不复用审批意见" /></label>
          </template>
          <button class="cr-primary" type="button" :disabled="busy" @click="submitCreate">提交变更申请</button>
        </section>

        <!-- 进行中工单 -->
        <section v-else class="cr-section">
          <div class="cr-open">
            <div class="cr-open-head">
              <b>{{ openRequest.requestNo }}</b>
              <span class="cr-badge">{{ statusLabel(openRequest.status) }}</span>
              <span v-if="openRequest.directAdminApproval" class="cr-tag">管理员直接批准</span>
            </div>
            <dl class="cr-dl">
              <div><dt>原因</dt><dd>{{ openRequest.reason }}</dd></div>
              <div><dt>范围</dt><dd>{{ openRequest.scope }}</dd></div>
              <div><dt>申请人</dt><dd>{{ openRequest.applicantName }}</dd></div>
              <div><dt>执行人</dt><dd>{{ openRequest.executorName }}</dd></div>
              <div><dt>需要验收</dt><dd>{{ openRequest.requireVerify ? '是' : '否（免验收）' }}</dd></div>
            </dl>
            <div v-if="displayOpen?.actualChanges" class="cr-sub-block">
              <p class="cr-diff-title">实际修改说明</p>
              <p class="cr-act">{{ displayOpen.actualChanges }}</p>
            </div>
            <div v-if="displayOpen?.diffs?.length" class="cr-sub-block">
              <p class="cr-diff-title">本次修改内容（当前提交轮次快照）</p>
              <p v-for="(diff, di) in displayOpen.diffs" :key="'d' + di" class="cr-diff">
                <b>{{ diff.field }}</b>：{{ diff.oldValue || '（空）' }} → {{ diff.newValue || '（空）' }}
              </p>
            </div>
            <div v-if="displayOpen?.submissions?.length" class="cr-sub-block">
              <p class="cr-diff-title">提交轮次历史</p>
              <div v-for="sub in displayOpen?.submissions" :key="sub.id" class="cr-act">
                <b>{{ sub.legacyHistory ? '旧格式历史（无法准确还原轮次）' : `第 ${sub.round} 轮` }} · {{ sub.status === 'pending' ? '待验收' : sub.status === 'accepted' ? '已发布' : '已退回' }}</b>
                <p v-if="sub.legacyHistory && openRequest.status === 'pending_verify'">旧格式快照不可直接验收，请退回后重新提交，以生成可核对的独立快照。</p>
                · {{ sub.actorName || '执行人' }}：{{ sub.actualChanges }}
                <p v-for="(diff, index) in sub.diffs" :key="index" class="cr-diff">{{ diff.field }}：{{ diff.oldValue || '（空）' }} → {{ diff.newValue || '（空）' }}</p>
              </div>
            </div>
            <div v-if="displayOpen?.actions?.length" class="cr-sub-block">
              <p class="cr-diff-title">操作记录</p>
              <p v-for="act in displayOpen.actions" :key="act.id" class="cr-act">
                <b>{{ act.actorName || '系统' }}</b> · {{ act.opinion }}
              </p>
            </div>
          </div>

          <!-- 待审批：管理员通过/驳回 -->
          <div v-if="openRequest.status === 'pending_approval' && isAdmin" class="cr-actions">
            <label>审批意见<input v-model="approveForm.opinion" type="text" placeholder="必填" /></label>
            <label class="cr-inline"><input v-model="approveForm.requireVerify" type="checkbox" /><span>修改完成后需要验收</span></label>
            <label v-if="!approveForm.requireVerify">免验收原因<textarea v-model="approveForm.waiveReason" rows="2" placeholder="必填" /></label>
            <div class="cr-btn-row">
              <button class="cr-primary" type="button" :disabled="busy" @click="submitApprove">批准</button>
              <input v-model="decision.opinion" type="text" placeholder="驳回意见" />
              <button class="cr-danger" type="button" :disabled="busy" @click="submitReject">驳回</button>
            </div>
          </div>

          <!-- 执行中：执行人提交完成 -->
          <div v-else-if="openRequest.status === 'executing' && canEdit" class="cr-actions">
            <div class="cr-note">你可在 CAXA 中修改该图纸并保存；在下方填写实际修改说明与调整后的技术属性（留空表示不修改），提交后进入验收/发布。</div>
            <label>实际修改说明<textarea v-model="submitForm.actualChanges" rows="2" placeholder="必填" /></label>
            <div class="cr-grid">
              <label>名称<input v-model="submitForm.name" type="text" placeholder="不修改留空" /></label>
              <label>材料<input v-model="submitForm.material" type="text" placeholder="不修改留空" /></label>
              <label>供应商<input v-model="submitForm.vendor" type="text" placeholder="不修改留空" /></label>
              <label>版本<input v-model="submitForm.version" type="text" placeholder="不修改留空" /></label>
            </div>
            <button class="cr-primary" type="button" :disabled="busy" @click="submitDone">提交完成</button>
          </div>
          <div v-else-if="openRequest.status === 'executing'" class="cr-note">变更执行中，等待指定执行人（{{ openRequest.executorName }}）完成修改并提交。</div>

          <!-- 待验收：管理员验收/退回 -->
          <div v-else-if="openRequest.status === 'pending_verify' && isAdmin" class="cr-actions">
            <label>验收意见<input v-model="decision.opinion" type="text" placeholder="必填：验收说明" /></label>
            <div class="cr-btn-row">
              <button class="cr-primary" type="button" :disabled="busy || loading || !openDetail?.currentSubmissionId" @click="submitVerify">验收并发布</button>
              <button class="cr-danger" type="button" :disabled="busy" @click="submitReturn">退回修改</button>
            </div>
          </div>

          <div v-if="canCancel" class="cr-actions cr-cancel-row">
            <label>终止原因<input v-model="decision.cancelOpinion" type="text" placeholder="必填：终止本工单的原因" /></label>
            <div class="cr-btn-row">
              <button class="cr-ghost" type="button" :disabled="busy" @click="submitCancel">终止工单</button>
            </div>
          </div>
        </section>

        <!-- 历史工单 -->
        <section v-if="history.length" class="cr-section">
          <h4>历史工单</h4>
          <ul class="cr-history">
            <li v-for="item in history" :key="item.id">
              <button type="button" class="cr-hist-btn" @click="toggleExpand(item)">
                <span class="mono">{{ item.requestNo }}</span>
                <span class="cr-badge muted">{{ statusLabel(item.status) }}</span>
                <span class="cr-hist-date">{{ item.createdAt.slice(0, 10) }}</span>
              </button>
              <div v-if="expandedId === item.id" class="cr-detail">
                <div v-for="sub in item.submissions" :key="sub.id" class="cr-sub-block">
                  <b>{{ sub.legacyHistory ? '旧格式历史（无法准确还原轮次）' : `第 ${sub.round} 轮` }}</b>
                  <p>{{ sub.actualChanges }}</p>
                  <p v-for="(diff, index) in sub.diffs" :key="index" class="cr-diff">{{ diff.field }}：{{ diff.oldValue || '（空）' }} → {{ diff.newValue || '（空）' }}</p>
                </div>
                <div v-if="item.actions?.length">
                  <p v-for="act in item.actions" :key="act.id" class="cr-act">
                    <b>{{ act.actorName || '系统' }}</b> · {{ act.opinion }}
                  </p>
                </div>
                <div v-if="item.diffs?.length">
                  <p class="cr-diff-title">修改内容</p>
                  <p v-for="(diff, i) in item.diffs" :key="i" class="cr-diff">
                    {{ diff.field }}：{{ diff.oldValue || '（空）' }} → {{ diff.newValue || '（空）' }}
                  </p>
                </div>
                <p v-if="!item.actions?.length && !item.diffs?.length" class="cr-hint">暂无记录</p>
              </div>
            </li>
          </ul>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cr-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1200;
}
.cr-modal {
  width: min(680px, 94vw);
  max-height: 88vh;
  background: var(--bg-card, #fff);
  color: var(--text, #1f2937);
  border-radius: 12px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.28);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.cr-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border, #e5e7eb);
}
.cr-head h3 {
  margin: 0;
  font-size: 16px;
}
.cr-sub {
  font-size: 12px;
  opacity: 0.7;
}
.cr-x {
  background: none;
  border: none;
  cursor: pointer;
  color: inherit;
  display: inline-flex;
}
.cr-body {
  padding: 16px 18px;
  overflow-y: auto;
}
.cr-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.cr-section h4 {
  margin: 6px 0 0;
  font-size: 14px;
}
label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 13px;
}
.cr-inline {
  flex-direction: row;
  align-items: center;
  gap: 8px;
}
input,
textarea,
select {
  font: inherit;
  padding: 6px 8px;
  border: 1px solid var(--border, #d1d5db);
  border-radius: 6px;
  background: var(--bg, #fff);
  color: inherit;
}
.cr-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
button {
  font: inherit;
  cursor: pointer;
  border-radius: 6px;
  padding: 8px 14px;
  border: 1px solid transparent;
}
.cr-primary {
  background: var(--accent, #2563eb);
  color: #fff;
}
.cr-danger {
  background: #dc2626;
  color: #fff;
}
.cr-ghost {
  background: transparent;
  border: 1px solid var(--border, #d1d5db);
  color: inherit;
}
button:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.cr-btn-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.cr-btn-row input {
  flex: 1;
}
.cr-cancel-row {
  margin-top: 4px;
}
.cr-note {
  font-size: 12px;
  line-height: 1.5;
  opacity: 0.85;
  background: var(--bg-soft, #f3f4f6);
  border-radius: 8px;
  padding: 8px 10px;
}
.cr-actions {
  display: flex;
  flex-direction: column;
  gap: 10px;
  border-top: 1px dashed var(--border, #e5e7eb);
  padding-top: 12px;
}
.cr-open {
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 10px;
  padding: 12px;
}
.cr-open-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}
.cr-badge {
  font-size: 12px;
  background: var(--accent, #2563eb);
  color: #fff;
  border-radius: 999px;
  padding: 1px 8px;
}
.cr-badge.muted {
  background: var(--border, #9ca3af);
}
.cr-tag {
  font-size: 12px;
  color: #b45309;
}
.cr-dl {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px 16px;
  margin: 0;
  font-size: 13px;
}
.cr-dl dt {
  opacity: 0.6;
  display: inline;
}
.cr-dl dd {
  display: inline;
  margin: 0;
}
.cr-dl > div {
  min-width: 0;
}
.cr-history {
  list-style: none;
  margin: 0;
  padding: 0;
}
.cr-history li {
  border-bottom: 1px solid var(--border, #eef0f3);
}
.cr-hist-btn {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  background: none;
  border: none;
  color: inherit;
  padding: 8px 2px;
  text-align: left;
}
.cr-hist-date {
  margin-left: auto;
  font-size: 12px;
  opacity: 0.6;
}
.mono {
  font-family: ui-monospace, monospace;
}
.cr-detail {
  padding: 4px 2px 10px;
  font-size: 12px;
}
.cr-sub-block {
  margin-top: 10px;
  border-top: 1px solid var(--border, #eef0f3);
  padding-top: 8px;
  font-size: 13px;
}
.cr-act {
  margin: 2px 0;
}
.cr-diff-title {
  margin: 6px 0 2px;
  font-weight: 600;
}
.cr-diff {
  margin: 2px 0;
}
.cr-hint {
  font-size: 12px;
  opacity: 0.6;
}
</style>
