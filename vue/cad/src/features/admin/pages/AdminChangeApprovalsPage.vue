<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import EvidenceDocuments from '@/features/drawings/components/EvidenceDocuments.vue'
import { changeRequestService, CHANGE_STATUS_LABELS, type ChangeRequest, type ChangeUserOption } from '@/services/change-request.service'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AdminChangeApprovalsPage' })

const uiStore = useUiStore()
const loading = ref(false)
const busyId = ref('')
const items = ref<ChangeRequest[]>([])
const selectedId = ref('')
const users = ref<ChangeUserOption[]>([])
const approveForm = reactive({ opinion: '', executorId: '' })
const rejectOpinion = ref('')
const evidenceOpen = ref(false)

const selected = computed(() => items.value.find((item) => item.id === selectedId.value) ?? null)
const pending = computed(() => items.value.filter((item) => item.status === 'pending_approval'))
const verifying = computed(() => items.value.filter((item) => item.status === 'pending_verify'))
// 创建工单时会默认把执行人写成申请人，待审批阶段应视为尚未指派。
const assignedExecutor = computed(() => {
  const item = selected.value
  if (!item) return ''
  if (item.status === 'pending_approval' && (!item.executorId || item.executorId === item.applicantId)) return ''
  return item.executorName || ''
})

const FILE_CATEGORY_LABELS: Record<string, string> = { drawing2d: '二维图纸', model3d: '三维模型', other: '其他文件', auto: '文件' }
const FILE_CATEGORY_ICONS: Record<string, string> = { drawing2d: 'file-text', model3d: 'box', other: 'file', auto: 'file' }
const STATUS_TAG: Record<string, string> = { pending_approval: 'warn', executing: 'plain', pending_verify: 'info', completed: 'ok', rejected: 'danger', cancelled: 'mute' }

function categoryLabel(category: string) { return FILE_CATEGORY_LABELS[category] ?? '文件' }
function categoryIcon(category: string) { return FILE_CATEGORY_ICONS[category] ?? 'file' }
function formatTime(value?: string) { return value ? value.slice(0, 16).replace('T', ' ') : '—' }

watch(selected, () => {
  approveForm.executorId = assignedExecutor.value ? (selected.value?.executorId || '') : ''
  approveForm.opinion = ''
  rejectOpinion.value = ''
})

async function load() {
  loading.value = true
  try {
    const list = await changeRequestService.list({ open: true })
    const relevant = list.filter((item) => item.status === 'pending_approval' || item.status === 'pending_verify')
    items.value = await Promise.all(relevant.map((item) => changeRequestService.get(item.id)))
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
  if (!approveForm.opinion.trim()) { uiStore.toast('请填写审批意见', 'warn'); return }
  if (!approveForm.executorId) { uiStore.toast('请指定负责修改的设计员', 'warn'); return }
  void run(item.id, () => changeRequestService.approve(item.id, approveForm.opinion.trim(), true, '', approveForm.executorId), '已批准并指定设计员，修改后将重新进行完整审核')
}

function reject(item: ChangeRequest) {
  if (!rejectOpinion.value.trim()) { uiStore.toast('驳回必须填写意见', 'warn'); return }
  void run(item.id, () => changeRequestService.reject(item.id, rejectOpinion.value.trim()), '已驳回变更申请')
}

onMounted(() => {
  void load()
  void changeRequestService.listUsers()
    .then((result) => { users.value = result.filter((user) => user.status === 'active' && (user.roles ?? []).some((role) => role === 'designer' || role === 'admin')) })
    .catch(() => uiStore.toast('人员列表加载失败，请刷新', 'warn'))
})
</script>

<template>
  <div class="approvals-page">
    <header class="page-head">
      <div>
        <h1>变更审批</h1>
        <p>审批变更申请、核对涉及文件并指定设计员。</p>
      </div>
      <button class="btn" type="button" :disabled="loading" @click="load">
        <DemoIcon name="refresh-cw" :size="14" /><span>{{ loading ? '加载中…' : '刷新' }}</span>
      </button>
    </header>

    <div class="approvals-grid">
      <section class="card list-card">
        <div class="list-scroll">
          <p v-if="loading" class="hint">加载中…</p>
          <template v-else>
            <div v-if="pending.length" class="group">
              <div class="group-title"><DemoIcon name="clipboard-check" :size="13" /><span>待审批</span><b>{{ pending.length }}</b></div>
              <button v-for="item in pending" :key="item.id" class="item" :class="{ active: item.id === selectedId }" type="button" @click="selectedId = item.id">
                <span class="item__top">
                  <span class="item__no">{{ item.requestNo }}</span>
                  <span class="tag warn">{{ CHANGE_STATUS_LABELS[item.status] }}</span>
                </span>
                <span class="item__drawing">{{ item.drawingNo }}{{ item.title ? ` · ${item.title}` : '' }}</span>
                <span class="item__meta">{{ item.applicantName }} · {{ item.targets?.length ?? 0 }} 个文件 · {{ formatTime(item.createdAt) }}</span>
              </button>
            </div>

            <div v-if="verifying.length" class="group">
              <div class="group-title"><DemoIcon name="workflow" :size="13" /><span>完整审核中</span><b>{{ verifying.length }}</b></div>
              <button v-for="item in verifying" :key="item.id" class="item" :class="{ active: item.id === selectedId }" type="button" @click="selectedId = item.id">
                <span class="item__top">
                  <span class="item__no">{{ item.requestNo }}</span>
                  <span class="tag info">{{ CHANGE_STATUS_LABELS[item.status] }}</span>
                </span>
                <span class="item__drawing">{{ item.drawingNo }}{{ item.title ? ` · ${item.title}` : '' }}</span>
                <span class="item__meta">{{ item.applicantName }} · {{ item.targets?.length ?? 0 }} 个文件 · {{ formatTime(item.createdAt) }}</span>
              </button>
            </div>

            <p v-if="!pending.length && !verifying.length" class="hint">暂无待审批或审核中的变更工单</p>
          </template>
        </div>
      </section>

      <section v-if="selected" class="card detail-card">
        <div class="detail__head">
          <div class="detail__title">
            <h4>{{ selected.requestNo }}</h4>
            <p class="detail__drawing">{{ selected.drawingNo }}{{ selected.title ? ` · ${selected.title}` : '' }}</p>
          </div>
          <span class="tag" :class="STATUS_TAG[selected.status] ?? 'mute'">{{ CHANGE_STATUS_LABELS[selected.status] }}</span>
        </div>

        <div class="detail__scroll">
          <div class="meta-grid">
            <div><span>申请人</span><strong>{{ selected.applicantName || '—' }}</strong></div>
            <div><span>申请时间</span><strong>{{ formatTime(selected.createdAt) }}</strong></div>
            <div><span>指定设计员</span><strong>{{ assignedExecutor || '待指派' }}</strong></div>
            <div><span>验收方式</span><strong>{{ selected.requireVerify ? '完整审核' : '免验收' }}</strong></div>
          </div>

          <section class="panel">
            <div class="panel__head">
              <DemoIcon name="folder-lock" :size="15" />
              <h5>申请修改的图纸 / 文件</h5>
              <span class="count">{{ selected.targets?.length ?? 0 }} 个</span>
            </div>
            <div v-if="selected.targets?.length" class="target-list">
              <div v-for="target in selected.targets" :key="target.attachmentId" class="target-row">
                <span class="target-type"><DemoIcon :name="categoryIcon(target.fileCategory)" :size="13" />{{ categoryLabel(target.fileCategory) }}</span>
                <div class="target-main">
                  <strong class="mono">{{ target.partNo || target.drawingNo || '—' }}</strong>
                  <span class="file-name" :title="target.name">{{ target.name }}</span>
                </div>
                <span v-if="target.partNo && target.drawingNo" class="target-drawing">图纸 {{ target.drawingNo }}</span>
              </div>
            </div>
            <p v-else class="hint">该申请未指定修改文件</p>
          </section>

          <section class="panel">
            <div class="panel__head"><DemoIcon name="clipboard-list" :size="15" /><h5>申请说明</h5></div>
            <div class="text-field"><span>变更原因</span><p>{{ selected.reason || '—' }}</p></div>
            <div class="text-field"><span>计划修改内容</span><p class="pre">{{ selected.scope || '—' }}</p></div>
          </section>

          <section class="panel">
            <div class="panel__head">
              <DemoIcon name="archive" :size="15" />
              <h5>证明材料</h5>
              <button class="btn sm panel__action" type="button" @click="evidenceOpen = true"><DemoIcon name="eye" :size="13" />查看已上传材料</button>
            </div>
            <p class="panel-hint">查看申请人上传的合同、沟通记录、技术要求、验收证明等佐证资料。</p>
          </section>
        </div>

        <div v-if="selected.status === 'pending_approval'" class="detail__foot">
          <p class="foot-note"><DemoIcon name="info" :size="13" />批准后由设计员执行修改，提交后重新完整审核并自动发布新版本。</p>
          <div class="foot-fields">
            <label class="action-field"><span>审批意见</span><input v-model="approveForm.opinion" class="inp" placeholder="填写同意理由" /></label>
            <label class="action-field"><span>指定设计员</span>
              <select v-model="approveForm.executorId" class="inp" required>
                <option value="">请选择设计员</option>
                <option v-for="user in users" :key="user.id" :value="user.id">{{ user.displayName }}</option>
              </select>
            </label>
          </div>
          <div class="foot-buttons">
            <input v-model="rejectOpinion" class="inp" placeholder="驳回意见（必填）" />
            <button class="btn danger" type="button" :disabled="busyId === selected.id" @click="reject(selected)">驳回</button>
            <button class="btn primary" type="button" :disabled="busyId === selected.id" @click="approve(selected)"><DemoIcon name="check" :size="14" />批准并指派</button>
          </div>
        </div>

        <div v-else-if="selected.status === 'pending_verify'" class="detail__foot detail__foot--verify">
          <p class="foot-note"><DemoIcon name="info" :size="13" />成果正在进行完整审核，由当前节点责任人签署。</p>
          <RouterLink class="btn primary" :to="`/reviews/task/${encodeURIComponent(selected.drawingNo)}`"><DemoIcon name="arrow-up-right" :size="14" />进入审核中心</RouterLink>
        </div>
      </section>

      <section v-else-if="!loading" class="card detail-card empty-detail">
        <DemoIcon name="clipboard-check" :size="30" />
        <p>选择左侧工单查看申请详情</p>
      </section>
    </div>

    <Teleport to="body">
      <Transition name="approval-drawer">
        <div v-if="evidenceOpen && selected" class="evidence-overlay" @mousedown.self="evidenceOpen = false">
          <aside class="evidence-drawer" role="dialog" aria-modal="true" aria-label="证明材料">
            <header class="evidence-drawer__head">
              <div class="evidence-drawer__title"><DemoIcon name="archive" :size="16" /><span>证明材料 · {{ selected.requestNo }}</span></div>
              <button class="evidence-drawer__close" type="button" aria-label="关闭" @click="evidenceOpen = false"><DemoIcon name="x" :size="16" /></button>
            </header>
            <div class="evidence-drawer__body">
              <EvidenceDocuments :key="selected.id" :drawing-id="selected.drawingId" :change-request-id="selected.id" read-only />
            </div>
          </aside>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.approvals-page {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 14px;
  min-height: 0;
  padding: clamp(14px, 2vw, 22px) clamp(14px, 2.4vw, 26px);
  overflow: hidden;
}

.page-head { display: flex; flex: none; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
.page-head h1 { font-size: 18px; }
.page-head p { margin-top: 4px; color: var(--text-3); font-size: 12px; }

.approvals-grid {
  display: grid;
  flex: 1;
  grid-template-columns: minmax(300px, 0.82fr) minmax(0, 1.6fr);
  grid-template-rows: minmax(0, 1fr);
  gap: 16px;
  min-height: 0;
}

.list-card { display: flex; flex-direction: column; min-height: 0; padding: 0; }
.list-scroll { display: flex; flex: 1; flex-direction: column; gap: 14px; min-height: 0; overflow-y: auto; padding: 12px; }
.hint { padding: 18px 6px; color: var(--text-3); font-size: 12px; text-align: center; }

.group { display: flex; flex-direction: column; gap: 6px; }
.group-title { display: flex; align-items: center; gap: 7px; padding: 2px 8px; color: var(--text-3); font-size: 11.5px; letter-spacing: 0.4px; }
.group-title b { margin-left: auto; padding: 1px 8px; border-radius: 99px; background: var(--panel-2); color: var(--text-2); font-size: 11px; font-weight: 600; }
.item { display: flex; flex-direction: column; gap: 5px; width: 100%; padding: 11px 12px; border: 1px solid transparent; border-radius: 10px; text-align: left; transition: background-color 0.2s, border-color 0.2s; }
.item:hover { background: var(--hover); }
.item.active { border-color: var(--accent); background: var(--active); }
.item__top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.item__no { color: var(--text-1); font-family: 'JetBrains Mono', monospace; font-size: 12.5px; font-weight: 600; }
.item__drawing { overflow: hidden; color: var(--text-2); font-size: 12px; white-space: nowrap; text-overflow: ellipsis; }
.item__meta { color: var(--text-3); font-size: 11px; }

.detail-card { display: flex; flex-direction: column; min-height: 0; padding: 0; }
.detail__head { display: flex; flex: none; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 16px 18px 14px; border-bottom: 1px solid var(--line); }
.detail__title { min-width: 0; }
.detail__head h4 { font-family: var(--font-display); font-size: 16px; font-weight: 800; }
.detail__drawing { margin-top: 4px; color: var(--text-3); font-size: 12px; }
.detail__scroll { display: flex; flex: 1; flex-direction: column; min-height: 0; overflow-y: auto; padding: 16px 18px; }
.detail__foot { display: flex; flex: none; flex-direction: column; gap: 10px; padding: 13px 18px; border-top: 1px solid var(--line); background: var(--panel-top); }
.detail__foot--verify { flex-direction: row; align-items: center; justify-content: space-between; gap: 16px; }

.meta-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.meta-grid > div { padding: 11px 13px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel-2); }
.meta-grid span { display: block; margin-bottom: 4px; color: var(--text-3); font-size: 11px; }
.meta-grid strong { font-size: 12.5px; font-weight: 600; }

.panel { margin-top: 14px; padding: 14px 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--panel-2); }
.panel__head { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; }
.panel__head svg { color: var(--accent); }
.panel__head h5 { font-size: 13px; font-weight: 700; }
.panel__head .count { margin-left: auto; color: var(--text-3); font-size: 11px; }
.panel__action { margin-left: auto; }
.panel-hint { color: var(--text-3); font-size: 12px; line-height: 1.6; }

.target-list { display: flex; flex-direction: column; gap: 8px; }
.target-row { display: flex; align-items: center; gap: 12px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 9px; background: var(--panel); }
.target-type { display: inline-flex; align-items: center; gap: 5px; flex: none; padding: 3px 9px; border-radius: 6px; background: var(--accent-soft); color: var(--accent); font-size: 10.5px; }
.target-main { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 2px; }
.target-main strong { color: var(--text-1); font-size: 12px; }
.file-name { overflow: hidden; color: var(--text-3); font-size: 11.5px; white-space: nowrap; text-overflow: ellipsis; }
.target-drawing { flex: none; color: var(--text-3); font-size: 11px; }

.text-field + .text-field { margin-top: 12px; }
.text-field > span { display: block; margin-bottom: 5px; color: var(--text-3); font-size: 11px; }
.text-field p { color: var(--text-2); font-size: 12.5px; line-height: 1.7; }
.pre { white-space: pre-wrap; }

.foot-note { display: flex; align-items: center; gap: 7px; color: var(--text-3); font-size: 11.5px; }
.foot-note svg { flex: none; color: var(--accent); }
.foot-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.foot-buttons { display: flex; align-items: center; gap: 8px; }
.foot-buttons .inp { flex: 1; }
.action-field { display: flex; flex-direction: column; gap: 6px; }
.action-field > span { color: var(--text-3); font-size: 11px; }

.empty-detail { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px; color: var(--text-3); }
.empty-detail p { font-size: 12.5px; }

.evidence-overlay { position: fixed; inset: 0; z-index: 2100; display: flex; justify-content: flex-end; background: rgb(0 0 0 / 45%); }
.evidence-drawer { display: flex; flex-direction: column; width: min(1040px, 94vw); height: 100%; border-left: 1px solid var(--line); background: var(--bg); box-shadow: -24px 0 60px rgb(0 0 0 / 30%); }
.evidence-drawer__head { display: flex; align-items: center; justify-content: space-between; padding: 16px 20px; border-bottom: 1px solid var(--line); background: var(--panel-top); }
.evidence-drawer__title { display: flex; align-items: center; gap: 9px; color: var(--text-1); font-family: var(--font-display); font-size: 15px; font-weight: 800; }
.evidence-drawer__title svg { color: var(--accent); }
.evidence-drawer__close { display: grid; width: 30px; height: 30px; place-items: center; border-radius: 8px; color: var(--text-3); transition: all 0.2s; }
.evidence-drawer__close:hover { background: var(--hover); color: var(--text-1); }
.evidence-drawer__body { flex: 1; min-height: 0; overflow-y: auto; padding: 18px 20px; }

.approval-drawer-enter-active,
.approval-drawer-leave-active { transition: opacity 0.25s ease; }
.approval-drawer-enter-active .evidence-drawer,
.approval-drawer-leave-active .evidence-drawer { transition: transform 0.3s cubic-bezier(0.22, 0.8, 0.3, 1); }
.approval-drawer-enter-from,
.approval-drawer-leave-to { opacity: 0; }
.approval-drawer-enter-from .evidence-drawer,
.approval-drawer-leave-to .evidence-drawer { transform: translateX(100%); }

@media (max-width: 980px) {
  .approvals-page { overflow-y: auto; }
  .approvals-grid { grid-template-columns: 1fr; grid-template-rows: none; }
  .foot-fields { grid-template-columns: 1fr; }
}
</style>
