<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useDrawingStore } from '@/stores/drawing.store'
import { useAuthStore } from '@/stores/auth.store'
import { useUiStore } from '@/stores/ui.store'
import { lifecycleApi } from '@/services/lifecycle.service'
import {
  changeRequestService,
  CHANGE_STATUS_LABELS,
  type ChangeRequest,
} from '@/services/change-request.service'
import { buildChangeTargetGroups, changeTargetRoleLabel } from '../components/detail/change-targets'
import DemoIcon from '@/components/common/DemoIcon.vue'
import EvidenceDocuments from '../components/EvidenceDocuments.vue'
import ChangeEvidenceList from '../components/ChangeEvidenceList.vue'
import { formatReadableDateTime } from '@/utils/date-time'

const route = useRoute()
const drawingStore = useDrawingStore()
const authStore = useAuthStore()
const uiStore = useUiStore()

const drawing = computed(() => drawingStore.getDrawing(String(route.params.drawingId ?? '')))
const requests = ref<ChangeRequest[]>([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const loadFailed = ref(false)
const search = ref('')
const selected = ref<string[]>([])
const activeHistoryId = ref('')
const historyLoading = ref(false)
const activeView = ref<'current' | 'history'>('current')
const creating = ref(false)
const historySearch = ref('')
const historyStatus = ref('')

const form = reactive({ reason: '', scope: '' })

const evidenceFiles = ref<{ id: string; file: File }[]>([])
const evidenceDragging = ref(false)
const evidenceInput = ref<HTMLInputElement | null>(null)
const evidenceRef = ref<{ load: () => Promise<void> } | null>(null)
let evidenceSeq = 0

function formatEvidenceSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}
function addEvidence(list: FileList | null | undefined) {
  if (!list?.length) return
  for (const item of Array.from(list)) {
    if (item.size === 0) {
      error.value = `「${item.name}」为空文件，已忽略`
      continue
    }
    if (item.size > 100 * 1024 * 1024) {
      error.value = `「${item.name}」超过单文件 100 MB 上限，已忽略`
      continue
    }
    if (evidenceFiles.value.some((entry) => entry.file.name === item.name && entry.file.size === item.size)) continue
    evidenceFiles.value.push({ id: `evidence-${++evidenceSeq}`, file: item })
  }
}
function pickEvidence(event: Event) {
  const target = event.target as HTMLInputElement
  addEvidence(target.files)
  target.value = ''
}
function onEvidenceDrop(event: DragEvent) {
  evidenceDragging.value = false
  addEvidence(event.dataTransfer?.files)
}
function removeEvidence(id: string) {
  evidenceFiles.value = evidenceFiles.value.filter((entry) => entry.id !== id)
}
function clearEvidence() {
  evidenceFiles.value = []
  evidenceDragging.value = false
  if (evidenceInput.value) evidenceInput.value.value = ''
}

const current = computed(() =>
  requests.value.find((item) => ['pending_approval', 'executing', 'pending_verify'].includes(item.status)),
)
const history = computed(() =>
  requests.value.filter((item) => ['completed', 'cancelled', 'rejected'].includes(item.status)),
)
const filteredHistory = computed(() =>
  history.value.filter(
    (item) =>
      (!historyStatus.value || item.status === historyStatus.value) &&
      `${item.requestNo} ${item.title || ''} ${item.reason} ${item.executorName || ''} ${item.applicantName || ''}`
        .toLowerCase()
        .includes(historySearch.value.trim().toLowerCase()),
  ),
)
const activeHistory = computed(
  () => requests.value.find((item) => item.id === activeHistoryId.value) || null,
)

const stepIndex = computed(() =>
  current.value?.status === 'pending_verify' ? 2 : current.value?.status === 'executing' ? 1 : 0,
)
// 变更申请通过后由被指定设计员完成修改，并由其在变更工单页直接提交发起完整审核。
const canSubmitCurrent = computed(() => Boolean(
  current.value && current.value.status === 'executing' && authStore.currentUser?.id === current.value.executorId,
))

const groups = computed(() =>
  buildChangeTargetGroups(drawing.value, drawing.value ? drawingStore.getStructure(drawing.value.no) : []),
)

const drawingTargets = computed(() =>
  groups.value.flatMap((group) =>
    group.files
      .filter((file) => file.category === 'drawing2d')
      .map((file) => ({ id: file.id, name: file.name, no: group.no, ownerName: group.name, role: file.role })),
  ),
)

const filteredTargets = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return drawingTargets.value
  return drawingTargets.value.filter((item) =>
    `${item.no} ${item.ownerName} ${item.name}`.toLowerCase().includes(keyword),
  )
})

const allIds = computed(() => [...new Set(drawingTargets.value.map((item) => item.id))])
const allSelected = computed(() => allIds.value.length > 0 && allIds.value.every((id) => selected.value.includes(id)))
const selectedIds = computed(() => selected.value.filter((id) => allIds.value.includes(id)))
const ready = computed(
  () => !busy.value && !loadFailed.value && form.reason.trim() && form.scope.trim() && selectedIds.value.length > 0,
)

let loadSequence = 0

async function refresh() {
  const id = drawing.value?.id
  const sequence = ++loadSequence
  if (!id) {
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  loadFailed.value = false
  try {
    const list = await changeRequestService.listByDrawing(id)
    const open = list.find((item) => ['pending_approval', 'executing', 'pending_verify'].includes(item.status))
    const detail = open ? await changeRequestService.get(open.id) : null
    if (sequence !== loadSequence) return
    requests.value = list.map((item) => (item.id === detail?.id ? detail : item))
  } catch (cause) {
    if (sequence === loadSequence) {
      loadFailed.value = true
      error.value = cause instanceof Error ? cause.message : '工单加载失败，请重试'
    }
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

watch(
  () => drawing.value?.id,
  () => {
    requests.value = []
    selected.value = []
    search.value = ''
    form.reason = ''
    form.scope = ''
    clearEvidence()
    activeHistoryId.value = ''
    activeView.value = 'current'
    creating.value = false
    historySearch.value = ''
    historyStatus.value = ''
    void refresh()
  },
  { immediate: true },
)

function selectAll() {
  selected.value = allSelected.value ? [] : [...allIds.value]
}

async function run(action: () => Promise<ChangeRequest>, message: string): Promise<ChangeRequest | undefined> {
  if (busy.value) return undefined
  const drawingId = drawing.value?.id
  busy.value = true
  error.value = ''
  try {
    const updated = await action()
    if (drawingId !== drawing.value?.id) return undefined
    requests.value = [updated, ...requests.value.filter((item) => item.id !== updated.id)]
    form.reason = ''
    form.scope = ''
    selected.value = []
    search.value = ''
    uiStore.toast(message, 'ok')
    creating.value = false
    if (['completed', 'cancelled', 'rejected'].includes(updated.status)) {
      activeView.value = 'history'
      historySearch.value = ''
      historyStatus.value = ''
      activeHistoryId.value = updated.id
    }
    try {
      await drawingStore.refresh()
    } catch {
      uiStore.toast('工单已保存，图纸信息暂未刷新，请稍后刷新页面', 'warn')
    }
    return updated
  } catch (cause) {
    if (drawingId === drawing.value?.id) {
      error.value = cause instanceof Error ? cause.message : '操作失败，请重试'
    }
    return undefined
  } finally {
    busy.value = false
  }
}

async function submitChangeReview() {
  const request = current.value
  if (!request || busy.value) return
  const last = request.submissions?.[request.submissions.length - 1]
  busy.value = true
  error.value = ''
  try {
    await changeRequestService.submit(
      request.id,
      last?.actualChanges?.trim() || '完成图纸修改，提交完整审核',
      last?.proposedAttributes ?? {},
    )
    await refresh()
    uiStore.toast('已提交修改成果并发起完整审核，请按节点顺序签署', 'ok')
  } catch (e) {
    error.value = (e as Error).message
    uiStore.toast(error.value || '提交审核失败', 'warn')
  } finally {
    busy.value = false
  }
}

async function create() {
  if (!drawing.value || !ready.value) return
  const pendingEvidence = [...evidenceFiles.value]
  const created = await run(
    () =>
      changeRequestService.create({
        drawingId: drawing.value!.id,
        reason: form.reason.trim(),
        scope: form.scope.trim(),
        attachmentIds: selectedIds.value,
        requireVerify: true,
      }),
    pendingEvidence.length ? '申请已提交，正在归档变更佐证资料…' : '申请已提交，等待管理员审批',
  )
  if (!created) return
  clearEvidence()
  if (!pendingEvidence.length) return
  busy.value = true
  let failed = 0
  for (const entry of pendingEvidence) {
    try {
      const body = new FormData()
      body.set('file', entry.file)
      body.set('title', entry.file.name.replace(/\.[^/.]+$/, ''))
      body.set('category', '变更依据')
      body.set('description', '发起变更工单时随附')
      body.set('drawingId', created.drawingId)
      body.set('changeRequestId', created.id)
      body.set('source', '变更工单')
      await lifecycleApi('/lifecycle-documents', { method: 'POST', body })
    } catch {
      failed += 1
    }
  }
  busy.value = false
  if (failed) {
    error.value = `${failed} 份变更佐证资料上传失败，请在下方「变更证明依据与材料」区域重新上传`
    return
  }
  uiStore.toast(`已随工单归档 ${pendingEvidence.length} 份变更佐证资料`, 'ok')
  await nextTick()
  evidenceRef.value?.load()
}

async function openHistory(item: ChangeRequest) {
  activeHistoryId.value = item.id
  if (item.actions) return
  historyLoading.value = true
  try {
    const detail = await changeRequestService.get(item.id)
    requests.value = requests.value.map((entry) => (entry.id === detail.id ? detail : entry))
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '历史加载失败'
  } finally {
    historyLoading.value = false
  }
}

function closeHistoryDetail() {
  activeHistoryId.value = ''
}

function switchView(view: 'current' | 'history') {
  activeView.value = view
  activeHistoryId.value = ''
}

function getStatusTagClass(status: string): string {
  if (status === 'completed') return 'ok'
  if (status === 'executing' || status === 'pending_verify') return 'plain'
  if (status === 'pending_approval') return 'warn'
  if (status === 'rejected' || status === 'cancelled') return 'danger'
  return 'mute'
}

const CHANGE_ACTION_LABELS: Record<string, string> = {
  create: '发起变更',
  approve: '审批通过',
  reject: '驳回申请',
  submit: '提交修改成果',
  verify: '审核验收通过',
  return: '退回修改',
  cancel: '终止工单',
  waive_verify: '免验收放行',
}

function actionLabel(action: string): string {
  return CHANGE_ACTION_LABELS[action] || action
}

function actionTone(action: string): string {
  if (action === 'approve' || action === 'verify') return 'ok'
  if (action === 'reject' || action === 'cancel') return 'danger'
  if (action === 'return') return 'warn'
  if (action === 'waive_verify') return 'mute'
  return 'plain'
}
</script>

<template>
  <main class="changes-page" :aria-busy="loading || busy">
    <!-- 头部横幅与操作 -->
    <header class="changes-heading card">
      <div class="heading-title">
        <div class="heading-icon">
          <DemoIcon name="clipboard-list" :size="24" />
        </div>
        <div>
          <div class="title-wrap">
            <h2>工程图纸变更工单</h2>
            <span class="tag plain">受控变更闭环</span>
          </div>
          <p>严格受控的变更审批与设计审核流程，每次修订均固定基线、证据链与多级节点签署记录。</p>
        </div>
      </div>
      <div class="heading-actions">
        <button class="btn sm" type="button" :disabled="loading || busy" title="刷新最新工单状态" @click="refresh">
          <DemoIcon name="rotate-cw" :size="13" />
          刷新
        </button>
        <button
          v-if="!current && drawing?.status === 'archived' && !creating"
          class="btn sm primary"
          type="button"
          :disabled="loading || busy || loadFailed"
          @click="creating = true; switchView('current')"
        >
          <DemoIcon name="plus" :size="13" />
          发起变更工单
        </button>
        <RouterLink class="btn sm" :to="{ name: 'drawing-preview', params: { drawingId: route.params.drawingId } }">
          <DemoIcon name="arrow-left" :size="13" />
          返回图纸文件
        </RouterLink>
      </div>
    </header>

    <!-- 顶部分页导航 -->
    <nav class="workorder-tabs" aria-label="工单视图">
      <button
        type="button"
        class="tab-item-btn"
        :class="{ selected: activeView === 'current' }"
        :aria-pressed="activeView === 'current'"
        @click="switchView('current')"
      >
        <DemoIcon name="activity" :size="14" />
        <span>当前进行中工单</span>
        <span class="count-pill">{{ current ? 1 : 0 }}</span>
      </button>
      <button
        type="button"
        class="tab-item-btn"
        :class="{ selected: activeView === 'history' }"
        :aria-pressed="activeView === 'history'"
        @click="switchView('history')"
      >
        <DemoIcon name="history" :size="14" />
        <span>历史变更存档</span>
        <span class="count-pill">{{ history.length }}</span>
      </button>
    </nav>

    <!-- 错误横幅 -->
    <div v-if="error" class="card card-pad error-banner" role="alert">
      <DemoIcon name="alert-circle" :size="18" />
      <span class="err-txt">{{ error }}</span>
      <button v-if="loadFailed && !busy" class="btn sm" type="button" @click="refresh">重新加载</button>
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="card card-pad loading-banner" role="status">
      <DemoIcon name="rotate-cw" :size="24" class="spin-icon" />
      <span>正在读取变更工单与任务数据…</span>
    </div>

    <template v-else-if="drawing">
      <!-- 视图 1：当前工单 / 发起工单 -->
      <template v-if="activeView === 'current'">
        <!-- 表单：新建变更申请 -->
        <form
          v-if="creating && !current && !loadFailed && drawing.status === 'archived'"
          class="card card-pad form-card"
          @submit.prevent="create"
        >
          <div class="card-section-head">
            <div class="head-lead">
              <div class="lead-icon"><DemoIcon name="file-plus" :size="18" /></div>
              <div>
                <h3>发起图纸变更申请</h3>
                <p>选择本次涉及修改的图纸或模型，并详细说明变更缘由</p>
              </div>
            </div>
            <button class="btn sm" type="button" :disabled="busy" @click="creating = false">
              <DemoIcon name="x" :size="13" />
              收起取消
            </button>
          </div>

          <fieldset :disabled="busy" class="form-fieldset">
            <!-- 步骤 1：选择要修改的文件 -->
            <div class="sub-form-block">
              <div class="block-title-row">
                <h4>
                  <span class="step-num">1</span>
                  选择授权变更文件清单
                </h4>
                <span class="selected-stat">
                  已选 <b>{{ selectedIds.length }}</b> / {{ allIds.length }} 个文件
                </span>
              </div>

              <div class="file-picker-toolbar">
                <div class="search-box">
                  <DemoIcon name="search" :size="14" />
                  <input
                    v-model="search"
                    type="search"
                    class="inp search-inp"
                    placeholder="快速搜索图号、零件或文件名…"
                    aria-label="搜索变更文件"
                  />
                  <button v-if="search" class="clear-search-btn" type="button" @click="search = ''">
                    <DemoIcon name="x" :size="12" />
                  </button>
                </div>
                <button type="button" class="btn sm" :disabled="!allIds.length" @click="selectAll">
                  <DemoIcon name="check-square" :size="13" />
                  {{ allSelected ? '取消全选' : '全选所有文件' }}
                </button>
              </div>

              <div class="target-list-scroll">
                <div class="drawing-target-list">
                  <label
                    v-for="item in filteredTargets"
                    :key="item.id"
                    class="file-checkbox-card flat-card"
                    :class="{ 'is-checked': selected.includes(item.id) }"
                  >
                    <input v-model="selected" type="checkbox" :value="item.id" class="hidden-chk" />
                    <div class="chk-indicator">
                      <DemoIcon v-if="selected.includes(item.id)" name="check" :size="12" />
                    </div>
                    <span class="file-category-badge">2D</span>
                    <span class="file-name-text" :title="item.name">{{ item.name }}</span>
                    <span class="target-kind-badge">{{ changeTargetRoleLabel(item.role) }}</span>
                  </label>
                </div>

                <div v-if="!filteredTargets.length" class="empty-target-msg">
                  <DemoIcon name="search-x" :size="24" />
                  <span>{{ allIds.length ? '没有匹配的 2D 图纸，请调整搜索关键词' : '当前工程下没有可变更的 2D 图纸' }}</span>
                </div>
              </div>
            </div>

            <!-- 步骤 2：说明变更与执行人 -->
            <div class="sub-form-block">
              <div class="block-title-row">
                <h4>
                  <span class="step-num">2</span>
                  说明变更原因与计划范围
                </h4>
              </div>

              <div class="input-grid">
                <div class="form-field full-width">
                  <label class="field-label">变更原因 <span class="req">*</span></label>
                  <input
                    v-model="form.reason"
                    class="inp"
                    required
                    placeholder="例如：装配干涉校验不通过 / 客户要求提升承载强度"
                  />
                </div>

                <div class="form-field full-width">
                  <label class="field-label">计划修改内容与技术要求 <span class="req">*</span></label>
                  <textarea
                    v-model="form.scope"
                    class="inp"
                    required
                    rows="3"
                    placeholder="例如：需调整关键连接孔距由 45mm 至 48mm，并重新校核强度"
                  />
                </div>
              </div>
            </div>

            <!-- 步骤 3：上传变更佐证资料 -->
            <div class="sub-form-block">
              <div class="block-title-row">
                <h4>
                  <span class="step-num">3</span>
                  上传变更佐证资料（可选）
                </h4>
                <span class="selected-stat">
                  已选 <b>{{ evidenceFiles.length }}</b> 份
                </span>
              </div>

              <div
                class="evidence-dropzone"
                :class="{ dragging: evidenceDragging }"
                role="button"
                tabindex="0"
                @click="evidenceInput?.click()"
                @keydown.enter.prevent="evidenceInput?.click()"
                @keydown.space.prevent="evidenceInput?.click()"
                @dragover.prevent="evidenceDragging = true"
                @dragleave.prevent="evidenceDragging = false"
                @drop.prevent="onEvidenceDrop"
              >
                <DemoIcon name="upload" :size="22" />
                <div class="dropzone-text">
                  <b>点击选择或拖拽文件到此处</b>
                  <span>支持客户确认图、变更依据、会议纪要、技术要求等，单文件不超过 100 MB</span>
                </div>
                <input ref="evidenceInput" type="file" multiple class="hidden-file" @change="pickEvidence" />
              </div>

              <ul v-if="evidenceFiles.length" class="evidence-file-list">
                <li v-for="entry in evidenceFiles" :key="entry.id">
                  <DemoIcon name="file-text" :size="14" />
                  <span class="evidence-name" :title="entry.file.name">{{ entry.file.name }}</span>
                  <span class="evidence-size">{{ formatEvidenceSize(entry.file.size) }}</span>
                  <button class="evidence-remove" type="button" :aria-label="`移除 ${entry.file.name}`" @click="removeEvidence(entry.id)">
                    <DemoIcon name="x" :size="13" />
                  </button>
                </li>
              </ul>

              <p class="evidence-hint">
                <DemoIcon name="info" :size="13" />
                <span>此步骤可跳过：审批与提交完整审核都不校验变更依据材料；材料可在工单内的「变更证明依据与材料」区域随时补充，仅作为过程留痕。</span>
              </p>
            </div>

            <footer class="form-submit-bar">
              <div class="submit-note">
                <DemoIcon name="info" :size="13" />
                <span>提交申请后需经管理员审批，获准后仅解锁所选目标文件的修改权限。</span>
              </div>
              <div class="submit-actions">
                <button class="btn" type="button" :disabled="busy" @click="creating = false">取消</button>
                <button class="btn primary" :disabled="!ready" type="submit">
                  <DemoIcon v-if="busy" name="rotate-cw" :size="13" class="spin-icon" />
                  <span>{{ busy ? '正在提交…' : '确认并提交变更申请' }}</span>
                </button>
              </div>
            </footer>
          </fieldset>
        </form>

        <!-- 展示：当前处于进行中的工单 -->
        <section v-else-if="current" class="card card-pad current-card">
          <div class="current-top-head">
            <div class="head-left-col">
              <div class="order-no-row">
                <span class="order-no-badge mono">{{ current.requestNo }}</span>
                <span class="tag" :class="getStatusTagClass(current.status)">
                  {{ CHANGE_STATUS_LABELS[current.status] }}
                </span>
              </div>
              <h3 class="current-title">{{ current.title || drawing.name }}</h3>
            </div>
            <div class="head-right-col">
              <div class="meta-tag-item">
                <DemoIcon name="users" :size="13" />
                <span>申请人: <b>{{ current.applicantName }}</b></span>
              </div>
              <div class="meta-tag-item">
                <DemoIcon name="clock" :size="13" />
                <span>发起于: <b>{{ formatReadableDateTime(current.createdAt) }}</b></span>
              </div>
            </div>
          </div>

          <!-- 现代化全流程进度条 -->
          <div class="progress-stepper-wrap">
            <ol class="progress-stepper" aria-label="工单进展阶段">
              <li
                v-for="(step, idx) in ['1. 申请审批', '2. 设计员执行', '3. 完整审核签署', '4. 发布新版本']"
                :key="step"
                class="step-item"
                :class="{ reached: idx <= stepIndex, current: idx === stepIndex }"
                :aria-current="idx === stepIndex ? 'step' : undefined"
              >
                <div class="step-indicator">
                  <DemoIcon v-if="idx < stepIndex" name="check" :size="14" />
                  <span v-else>{{ idx + 1 }}</span>
                </div>
                <div class="step-label-wrap">
                  <span class="step-label">{{ step }}</span>
                </div>
              </li>
            </ol>
          </div>

          <!-- 变更核心诉求卡片 -->
          <div class="info-grid-card">
            <div class="info-item">
              <span class="info-lbl">变更原因</span>
              <div class="info-val strong-val">{{ current.reason }}</div>
            </div>
            <div class="info-item">
              <span class="info-lbl">计划修改内容</span>
              <div class="info-val">{{ current.scope }}</div>
            </div>
            <div class="info-item">
              <span class="info-lbl">指定设计员</span>
              <div class="info-val user-val">
                <DemoIcon name="user-check" :size="14" />
                {{ current.executorName || '待管理员审批时指派' }}
              </div>
            </div>
          </div>

          <!-- 授权修改的目标文件清单 -->
          <details class="modern-details" open>
            <summary>
              <div class="details-summary-content">
                <DemoIcon name="folder-lock" :size="14" />
                <span>已授权修改的文件清单</span>
                <span class="badge muted-badge">{{ current.targets?.length ?? 0 }} 个文件</span>
              </div>
            </summary>
            <div class="authorized-targets-grid">
              <div
                v-for="t in current.targets"
                :key="t.attachmentId"
                class="target-file-pill"
              >
                <DemoIcon name="file" :size="13" />
                <span class="mono part-no">{{ t.partNo || t.drawingNo }}</span>
                <span class="file-name" :title="t.name">{{ t.name }}</span>
              </div>
            </div>
          </details>

          <!-- 证明材料归档组件（内联） -->
          <div class="evidence-embed-wrap">
            <EvidenceDocuments
              ref="evidenceRef"
              :key="current.id"
              :drawing-id="drawing.id"
              :change-request-id="current.id"
              :read-only="current.status === 'pending_verify'"
            />
          </div>

          <!-- 工单状态提示 -->
          <div class="card card-pad state-prompt-card">
            <div class="prompt-icon">
              <DemoIcon
                :name="current.status === 'pending_verify' ? 'clipboard-check' : 'clock'"
                :size="20"
              />
            </div>
            <div class="prompt-text">
              <b>{{ current.status === 'pending_approval' ? '变更申请已就绪，等待管理员审批' : current.status === 'pending_verify' ? '修改成果已提交，正在审核中心进行完整审核' : `等待指定设计员「${current.executorName || '未指定'}」完成本地图纸编辑` }}</b>
              <p>{{ current.status === 'pending_approval' ? '管理员同意后即可开辟修改通道，不校验变更依据材料；如需补充客户确认图或依据资料，可在下方「变更证明依据与材料」区域随时上传。' : current.status === 'pending_verify' ? '审核在「审核中心」进行，全部节点签署通过后系统将自动发布最新版本。' : '设计人员完成本地 CAD 文件修改后，可直接在此提交并发起完整审核，不要求先上传证明材料。' }}</p>
            </div>
            <button
              v-if="canSubmitCurrent"
              class="btn primary prompt-action"
              type="button"
              :disabled="busy"
              @click="submitChangeReview"
            >
              <DemoIcon name="check-circle-2" :size="14" />
              提交并发起完整审核
            </button>
          </div>

          <!-- 提交轮次与操作历史记录 -->
          <details
            v-if="current.submissions?.length || current.actions?.length || current.actualChanges || current.diffs?.length"
            class="modern-details"
          >
            <summary>
              <div class="details-summary-content">
                <DemoIcon name="history" :size="14" />
                <span>工单操作记录与提交历程</span>
              </div>
            </summary>
            <div class="timeline-container">
              <article
                v-for="entry in current.submissions"
                :key="entry.id"
                class="timeline-node submission-node"
              >
                <div class="node-indicator"></div>
                <div class="node-body card card-pad">
                  <div class="node-head">
                    <span class="round-badge">{{ entry.legacyHistory ? '历史提交' : `第 ${entry.round} 轮修改成果` }}</span>
                  </div>
                  <p class="changes-txt">{{ entry.actualChanges }}</p>
                  <div v-if="entry.diffs?.length" class="diff-list">
                    <span v-for="(diff, idx) in entry.diffs" :key="idx" class="diff-pill">
                      {{ diff.field }}: <em>{{ diff.oldValue || '无' }}</em> → <strong>{{ diff.newValue || '无' }}</strong>
                    </span>
                  </div>
                </div>
              </article>

              <div
                v-for="act in current.actions"
                :key="act.id"
                class="timeline-node action-node"
              >
                <div class="node-indicator tiny"></div>
                <div class="action-body">
                  <span class="mono act-time">{{ act.createdAt.slice(0, 16).replace('T', ' ') }}</span>
                  <span class="act-actor">{{ act.actorName || '系统' }}</span>
                  <span class="act-opinion">{{ act.opinion }}</span>
                </div>
              </div>
            </div>
          </details>
        </section>

        <!-- 空状态：暂无进行中工单 -->
        <section v-else-if="!error" class="card card-pad empty-card">
          <div class="empty-icon-wrap">
            <DemoIcon name="clipboard-check" :size="40" />
          </div>
          <h3>暂无正在进行的变更工单</h3>
          <p>
            {{ drawing.status === 'archived' ? '图纸已正式存档。当需要针对设计进行工程变更时，可发起工单指定修改范围。' : '图纸当前尚未正式存档，存档发布后即可发起受控变更工单。' }}
          </p>
          <div class="empty-btn-group">
            <button
              v-if="drawing.status === 'archived'"
              class="btn primary"
              type="button"
              @click="creating = true"
            >
              <DemoIcon name="plus" :size="13" />
              新建变更工单
            </button>
            <button
              v-if="history.length"
              class="btn"
              type="button"
              @click="switchView('history')"
            >
              <DemoIcon name="history" :size="13" />
              查看已归档的 {{ history.length }} 条历史工单
            </button>
          </div>
        </section>
      </template>

      <!-- 视图 2：历史工单列表 -->
      <section v-else class="card card-pad history-section">
        <div class="card-section-head">
          <div class="head-lead">
            <div class="lead-icon"><DemoIcon name="history" :size="18" /></div>
            <div>
              <h3>已归档变更历史工单</h3>
              <p>完整保留历次变更原因、涉及图纸清单、各轮修改总结与审核签章记录</p>
            </div>
          </div>
          <span class="badge muted-badge">共 {{ history.length }} 条</span>
        </div>

        <div class="history-toolbar">
          <div class="search-box">
            <DemoIcon name="search" :size="14" />
            <input
              v-model="historySearch"
              type="search"
              class="inp"
              placeholder="搜索工单号、变更原因或执行人…"
              aria-label="搜索历史工单"
            />
          </div>
          <select v-model="historyStatus" class="inp status-filter-select" aria-label="筛选状态">
            <option value="">全部状态</option>
            <option value="completed">已完成</option>
            <option value="rejected">已驳回</option>
            <option value="cancelled">已终止</option>
          </select>
        </div>

        <div class="history-list-wrap">
          <article
            v-for="item in filteredHistory"
            :key="item.id"
            class="card history-card"
          >
            <button
              type="button"
              class="history-card-header"
              @click="openHistory(item)"
            >
              <div class="history-title-block">
                <div class="row-top">
                  <span class="mono req-no">{{ item.requestNo }}</span>
                  <span class="tag" :class="getStatusTagClass(item.status)">
                    {{ CHANGE_STATUS_LABELS[item.status] }}
                  </span>
                </div>
                <strong class="history-title">{{ item.title || item.reason }}</strong>
              </div>

              <div class="history-meta-block">
                <span class="meta-person">执行人: <b>{{ item.executorName || item.applicantName }}</b></span>
                <time class="meta-date mono text-time">{{ formatReadableDateTime(item.completedAt || item.createdAt) }}</time>
                <div class="expand-icon-box">
                  <DemoIcon name="chevron-right" :size="16" />
                </div>
              </div>
            </button>
          </article>

          <div v-if="!filteredHistory.length" class="card empty-card">
            <DemoIcon name="search-x" :size="32" />
            <strong>{{ history.length ? '没有匹配的工单记录' : '暂无历史工单存档' }}</strong>
            <p>{{ history.length ? '请尝试更换搜索关键词或状态筛选条件' : '已完成、已驳回或已终止的变更工单将在此处永久归档' }}</p>
          </div>
        </div>
      </section>
    </template>

    <!-- 历史工单详情：层叠页面 -->
    <div v-if="activeHistory" class="history-detail-layer">
      <div class="history-detail-mask" @click="closeHistoryDetail"></div>
      <aside class="history-detail-page" role="dialog" aria-modal="true" aria-label="历史变更工单详情">
        <header class="hd-page-head">
          <div class="hd-page-title">
            <div class="hd-page-icon"><DemoIcon name="history" :size="18" /></div>
            <div class="hd-page-title-text">
              <div class="row-top">
                <span class="mono req-no">{{ activeHistory.requestNo }}</span>
                <span class="tag" :class="getStatusTagClass(activeHistory.status)">
                  {{ CHANGE_STATUS_LABELS[activeHistory.status] }}
                </span>
              </div>
              <h3>{{ activeHistory.title || activeHistory.reason }}</h3>
            </div>
          </div>
          <button class="btn sm" type="button" @click="closeHistoryDetail">
            <DemoIcon name="x" :size="13" />关闭
          </button>
        </header>

        <div class="hd-page-body">
          <div class="info-grid-card">
            <div class="info-item">
              <span class="info-lbl">变更原因</span>
              <div class="info-val">{{ activeHistory.reason }}</div>
            </div>
            <div class="info-item">
              <span class="info-lbl">计划修改范围</span>
              <div class="info-val">{{ activeHistory.scope }}</div>
            </div>
            <div class="info-item">
              <span class="info-lbl">责任设计员</span>
              <div class="info-val">{{ activeHistory.executorName || '未指定' }}</div>
            </div>
          </div>

          <div v-if="activeHistory.targets?.length" class="history-targets-block">
            <span class="sub-lbl">涉及文件清单:</span>
            <div class="authorized-targets-grid">
              <div v-for="target in activeHistory.targets" :key="target.attachmentId" class="target-file-pill">
                <span class="mono part-no">{{ target.partNo || target.drawingNo }}</span>
                <span class="file-name">{{ target.name }}</span>
              </div>
            </div>
          </div>

          <!-- 变更依据材料：仅展示本次变更上传的凭证 -->
          <div class="history-evidence-block">
            <span class="sub-lbl">变更依据材料:</span>
            <ChangeEvidenceList
              :drawing-id="activeHistory.drawingId"
              :change-request-id="activeHistory.id"
              :submission-ids="activeHistory.submissions?.map((sub) => sub.id) || []"
            />
          </div>

          <div v-if="historyLoading" class="loading-hint">
            <DemoIcon name="rotate-cw" :size="16" class="spin-icon" />
            <span>正在获取完整历史日志…</span>
          </div>

          <div v-if="activeHistory.submissions?.length" class="history-submissions-list">
            <span class="sub-lbl">修改提交成果:</span>
            <div
              v-for="sub in activeHistory.submissions"
              :key="sub.id"
              class="card card-pad submission-item-card"
            >
              <div class="submission-head">
                <span class="round-badge">{{ sub.legacyHistory ? '历史提交' : `第 ${sub.round} 轮提交` }}</span>
                <span class="sub-actor"><DemoIcon name="users" :size="11" />{{ sub.actorName || '未知提交人' }}</span>
                <time class="mono sub-time">{{ formatReadableDateTime(sub.createdAt) }}</time>
              </div>
              <p class="changes-txt">{{ sub.actualChanges }}</p>
              <div v-if="sub.diffs?.length" class="diff-block">
                <span class="diff-lbl">修改记录（{{ sub.diffs.length }} 项）:</span>
                <div class="diff-list">
                  <span v-for="(diff, idx) in sub.diffs" :key="idx" class="diff-pill">
                    {{ diff.field }}: <em>{{ diff.oldValue || '无' }}</em> → <strong>{{ diff.newValue || '无' }}</strong>
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div v-if="activeHistory.actions?.length" class="history-actions-list">
            <span class="sub-lbl">办理过程流转:</span>
            <div v-for="act in activeHistory.actions" :key="act.id" class="action-row">
              <span class="tag act-badge" :class="actionTone(act.action)">{{ actionLabel(act.action) }}</span>
              <div class="act-body">
                <div class="act-head">
                  <b class="act-actor">{{ act.actorName || '系统' }}</b>
                  <time class="mono act-time">{{ act.createdAt.slice(0, 16).replace('T', ' ') }}</time>
                </div>
                <p v-if="act.opinion" class="act-opinion">{{ act.opinion }}</p>
              </div>
            </div>
          </div>
        </div>
      </aside>
    </div>
  </main>
</template>

<style scoped>
.changes-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: min(100%, 1160px);
  margin: 0 auto;
  padding: 16px 20px 48px;
  box-sizing: border-box;
}

.changes-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 24px;
  flex-wrap: wrap;
}

.heading-title {
  display: flex;
  align-items: center;
  gap: 16px;
}

.heading-icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  flex-shrink: 0;
  border-radius: 12px;
  background: var(--accent-soft);
  color: var(--accent);
}

.title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
}

.heading-title h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 800;
}

.heading-title p {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 12px;
}

.heading-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

/* 导航 Tabs */
.workorder-tabs {
  display: flex;
  gap: 6px;
  border-bottom: 1px solid var(--line);
  padding-bottom: 2px;
}

.tab-item-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 18px;
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--text-2);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.tab-item-btn:hover {
  color: var(--text-1);
}

.tab-item-btn.selected {
  color: var(--accent);
  border-bottom-color: var(--accent);
  font-weight: 700;
}

.count-pill {
  padding: 1px 7px;
  border-radius: 99px;
  background: var(--panel-2);
  color: var(--text-3);
  font-size: 11px;
}

.tab-item-btn.selected .count-pill {
  background: var(--accent-soft);
  color: var(--accent);
}

/* 状态横幅 */
.error-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  border-color: var(--danger);
  background: rgb(248 113 113 / 8%);
  color: var(--danger);
}

.err-txt {
  flex: 1;
  font-size: 12.5px;
}

.loading-banner,
.empty-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 48px 24px;
  text-align: center;
  color: var(--text-3);
}

.empty-icon-wrap {
  display: grid;
  place-items: center;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: var(--panel-2);
  color: var(--accent);
}

.empty-card h3 {
  margin: 0;
  font-size: 16px;
  color: var(--text-1);
}

.empty-card p {
  margin: 0;
  font-size: 12.5px;
  max-width: 460px;
  line-height: 1.5;
}

.empty-btn-group {
  display: flex;
  gap: 10px;
  margin-top: 8px;
}

/* 卡片通用头 */
.card-section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
  gap: 12px;
}

.head-lead {
  display: flex;
  align-items: center;
  gap: 12px;
}

.lead-icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
  flex-shrink: 0;
}

.head-lead h3,
.head-lead h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-1);
}

.head-lead p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

/* 发起申请表单 */
.form-fieldset {
  border: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.sub-form-block {
  padding: 16px 18px;
  border-radius: 12px;
  background: var(--panel-2);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.block-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.block-title-row h4 {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  font-size: 13.5px;
  font-weight: 700;
}

.step-num {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--accent);
  color: var(--accent-ink);
  font-size: 11px;
  font-weight: 700;
}

.selected-stat {
  font-size: 12px;
  color: var(--text-3);
}

.selected-stat b {
  color: var(--accent);
}

.file-picker-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-box {
  position: relative;
  flex: 1;
  display: flex;
  align-items: center;
}

.search-box svg {
  position: absolute;
  left: 12px;
  color: var(--text-3);
  pointer-events: none;
}

.search-box input {
  padding-left: 36px;
  padding-right: 32px;
  height: 34px;
}

.clear-search-btn {
  position: absolute;
  right: 8px;
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
}

.target-list-scroll {
  max-height: 280px;
  overflow-y: auto;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel);
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.drawing-target-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.file-checkbox-card.flat-card {
  padding: 9px 12px;
  gap: 10px;
}

.flat-card .file-category-badge {
  flex: none;
}

.flat-card .file-name-text {
  flex: 1;
  min-width: 0;
  color: var(--text-1);
  font-weight: 600;
}

.flat-card .target-kind-badge {
  flex: none;
  margin-left: auto;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--panel-2);
  color: var(--text-3);
  font-size: 10.5px;
}

.file-checkbox-card {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid var(--line);
  background: var(--panel);
  cursor: pointer;
  transition: all 0.2s;
  user-select: none;
}

.file-checkbox-card:hover {
  border-color: var(--accent);
}

.file-checkbox-card.is-checked {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.hidden-chk {
  display: none;
}

.chk-indicator {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  border: 1.5px solid var(--line-strong);
  border-radius: 4px;
  background: var(--panel);
  flex-shrink: 0;
}

.file-checkbox-card.is-checked .chk-indicator {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--accent-ink);
}

.file-category-badge {
  font-size: 10px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 4px;
  background: var(--panel-2);
  color: var(--accent);
}

.file-name-text {
  font-size: 11.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-target-msg {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 24px;
  color: var(--text-3);
  font-size: 12px;
}

.input-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.full-width {
  grid-column: 1 / -1;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}

.req {
  color: var(--danger);
}

.form-submit-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 14px;
  border-top: 1px solid var(--line);
  gap: 12px;
  flex-wrap: wrap;
}

.submit-note {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-3);
  font-size: 11.5px;
}

.submit-actions {
  display: flex;
  gap: 10px;
}

/* 当前工单展示 */
.current-card {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.current-top-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--line);
}

.order-no-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}

.order-no-badge {
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 700;
  font-size: 12px;
}

.current-title {
  margin: 0;
  font-size: 18px;
  font-weight: 800;
  color: var(--text-1);
}

.head-right-col {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-end;
}

.meta-tag-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-3);
}

.meta-tag-item b {
  color: var(--text-1);
  font-weight: 500;
}

/* 步骤条 */
.progress-stepper-wrap {
  padding: 16px 20px;
  border-radius: 12px;
  background: var(--panel-2);
}

.progress-stepper {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  list-style: none;
  padding: 0;
  margin: 0;
}

.step-item {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--text-3);
}

.step-indicator {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 1.5px solid var(--line);
  background: var(--panel);
  font-size: 12px;
  font-weight: 700;
  flex-shrink: 0;
}

.step-item.reached {
  color: var(--accent);
}

.step-item.reached .step-indicator {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}

.step-item.current {
  font-weight: 700;
}

.step-item.current .step-indicator {
  background: var(--accent);
  color: var(--accent-ink);
  box-shadow: 0 0 10px var(--glow);
}

.step-label {
  font-size: 12.5px;
}

/* 核心信息卡片网格 */
.info-grid-card {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 10px;
  background: var(--panel-2);
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.info-lbl {
  font-size: 11px;
  color: var(--text-3);
  text-transform: uppercase;
}

.info-val {
  font-size: 13px;
  color: var(--text-1);
  white-space: pre-wrap;
  line-height: 1.5;
}

.strong-val {
  font-weight: 600;
}

.user-val {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--accent);
}

/* 折叠明细 */
.modern-details {
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel);
  overflow: hidden;
}

.modern-details summary {
  padding: 12px 16px;
  cursor: pointer;
  background: var(--panel-2);
  user-select: none;
}

.details-summary-content {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-1);
}

.authorized-targets-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 14px 16px;
}

.target-file-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  border-radius: 6px;
  border: 1px solid var(--line);
  background: var(--panel-2);
  font-size: 12px;
}

.part-no {
  color: var(--accent);
  font-weight: 600;
}

.file-name {
  color: var(--text-1);
}

.submit-action-card {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.attribute-edit-details {
  margin: 12px 0;
}

.attribute-input-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  padding: 14px 16px;
}

/* 提示卡片 */
.state-prompt-card {
  display: flex;
  align-items: center;
  gap: 16px;
  background: var(--panel-2);
}

.prompt-icon {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 50%;
  background: var(--panel);
  color: var(--accent);
  flex-shrink: 0;
}

.prompt-text {
  flex: 1;
}

.prompt-text b {
  font-size: 13.5px;
  color: var(--text-1);
}

.prompt-text p {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 12px;
}

.prompt-action {
  flex-shrink: 0;
}

.review-link-btn {
  flex-shrink: 0;
}

/* 时间线 */
.timeline-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
}

.timeline-node {
  display: flex;
  gap: 12px;
}

.node-indicator {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--accent);
  margin-top: 6px;
  flex-shrink: 0;
}

.node-indicator.tiny {
  width: 6px;
  height: 6px;
  margin-top: 8px;
}

.node-body {
  flex: 1;
  padding: 12px 14px;
}

.node-head {
  margin-bottom: 6px;
}

.round-badge {
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 11px;
  font-weight: 700;
}

.changes-txt {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--text-1);
}

.diff-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.diff-pill {
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--panel-2);
  font-size: 11px;
  color: var(--text-2);
}

.action-body {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  color: var(--text-3);
}

.act-time {
  font-size: 11px;
}

.act-actor {
  font-weight: 600;
  color: var(--text-2);
}

/* 历史卡片列表 */
.history-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.history-toolbar {
  display: flex;
  gap: 12px;
}

.status-filter-select {
  width: 150px;
  flex-shrink: 0;
}

.history-list-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.history-card {
  border-radius: 10px;
  overflow: hidden;
  transition: all 0.2s;
}

.history-card:hover {
  border-color: var(--accent);
}

.history-card-header {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  background: transparent;
  border: none;
  color: inherit;
  cursor: pointer;
  text-align: left;
  gap: 16px;
}

.history-title-block {
  flex: 1;
  min-width: 0;
}

.row-top {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.req-no {
  font-size: 11.5px;
  color: var(--accent);
  font-weight: 600;
}

.history-title {
  display: block;
  font-size: 14px;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-meta-block {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
}

.meta-person {
  font-size: 12px;
  color: var(--text-3);
}

.meta-person b {
  color: var(--text-1);
}

.meta-date {
  font-size: 11.5px;
  color: var(--text-3);
}

.expand-icon-box {
  color: var(--text-3);
}

/* 历史详情：层叠页面 */
.history-detail-layer {
  position: fixed;
  inset: 0;
  z-index: 2200;
  display: flex;
  justify-content: flex-end;
}

.history-detail-mask {
  position: absolute;
  inset: 0;
  background: rgb(0 0 0 / 45%);
  backdrop-filter: blur(3px);
}

.history-detail-page {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(100%, 760px);
  height: 100%;
  background: var(--panel);
  box-shadow: -24px 0 60px rgb(0 0 0 / 25%);
  animation: hd-slide-in 0.24s ease;
}

@keyframes hd-slide-in {
  from { transform: translateX(24px); opacity: 0; }
  to { transform: translateX(0); opacity: 1; }
}

.hd-page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--line);
  background: var(--panel-top);
}

.hd-page-title {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.hd-page-icon {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
}

.hd-page-title-text {
  min-width: 0;
}

.hd-page-title-text h3 {
  margin: 4px 0 0;
  font-size: 14.5px;
  color: var(--text-1);
}

.hd-page-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px 20px 40px;
}

.sub-lbl {
  display: block;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-2);
  margin-bottom: 6px;
}

.history-submissions-list,
.history-actions-list,
.history-evidence-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.submission-item-card {
  background: var(--panel);
}

.action-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  font-size: 12px;
  color: var(--text-3);
  padding: 8px 0;
  border-top: 1px dashed var(--line);
}

.action-row:first-of-type {
  border-top: none;
}

.act-badge {
  flex-shrink: 0;
  min-width: 84px;
  justify-content: center;
}

.act-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.act-head {
  display: flex;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
}

.act-actor {
  color: var(--text-1);
  font-size: 12.5px;
}

.act-time {
  color: var(--text-3);
  font-size: 11px;
}

.act-opinion {
  margin: 0;
  color: var(--text-2);
  line-height: 1.6;
  overflow-wrap: anywhere;
}

.submission-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}

.sub-actor {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--text-2);
  font-size: 11.5px;
}

.sub-time {
  color: var(--text-3);
  font-size: 11px;
}

.diff-block {
  margin-top: 8px;
}

.diff-lbl {
  display: block;
  margin-bottom: 5px;
  color: var(--text-3);
  font-size: 11px;
}

.loading-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--text-3);
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@media (max-width: 768px) {
  .changes-heading,
  .current-top-head,
  .history-card-header {
    flex-direction: column;
    align-items: flex-start;
  }
  .progress-stepper {
    grid-template-columns: 1fr 1fr;
  }
  .info-grid-card,
  .attribute-input-grid {
    grid-template-columns: 1fr;
  }
  .history-meta-block {
    margin-top: 8px;
    width: 100%;
    justify-content: space-between;
  }
}

/* 变更佐证资料上传 */
.evidence-dropzone {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  border: 1.5px dashed var(--line-strong);
  border-radius: 12px;
  background: var(--panel-2);
  color: var(--text-2);
  cursor: pointer;
  transition: border-color 0.25s, background 0.25s, color 0.25s;
}

.evidence-dropzone:hover,
.evidence-dropzone.dragging {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}

.evidence-dropzone > svg {
  flex: none;
}

.dropzone-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.dropzone-text b {
  font-size: 13px;
}

.dropzone-text span {
  color: var(--text-3);
  font-size: 11.5px;
  line-height: 1.5;
}

.hidden-file {
  display: none;
}

.evidence-file-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
}

.evidence-file-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  font-size: 12.5px;
}

.evidence-file-list li > svg {
  flex: none;
  color: var(--accent);
}

.evidence-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.evidence-size {
  flex: none;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.evidence-remove {
  display: grid;
  width: 24px;
  height: 24px;
  flex: none;
  place-items: center;
  border-radius: 7px;
  color: var(--text-3);
  transition: background 0.2s, color 0.2s;
}

.evidence-remove:hover {
  background: rgb(248 113 113 / 12%);
  color: var(--danger);
}

.evidence-hint {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  margin: 10px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
  line-height: 1.6;
}

.evidence-hint svg {
  flex: none;
  margin-top: 2px;
  color: var(--accent);
}
</style>
