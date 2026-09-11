<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import { changeRequestService, CHANGE_STATUS_LABELS, type ChangeRequest, type ChangeUserOption, type ProposedAttributes } from '@/services/change-request.service'
import { buildChangeTargetGroups } from '../components/detail/change-targets'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { formatReadableDateTime } from '@/utils/date-time'

const route = useRoute()
const drawingStore = useDrawingStore()
const authStore = useAuthStore()
const uiStore = useUiStore()
const drawing = computed(() => drawingStore.getDrawing(String(route.params.drawingId ?? '')))
const isAdmin = computed(() => authStore.currentUser?.roles?.includes('admin') ?? false)
const requests = ref<ChangeRequest[]>([])
const users = ref<ChangeUserOption[]>([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const loadFailed = ref(false)
const search = ref('')
const selected = ref<string[]>([])
const expandedId = ref('')
const historyLoading = ref(false)
const activeView = ref<'current' | 'history'>('current')
const creating = ref(false)
const historySearch = ref('')
const historyStatus = ref('')
const form = reactive({ reason: '', scope: '', executorId: '' })
const submission = reactive({ actualChanges: '', name: '', material: '', vendor: '' })
const current = computed(() => requests.value.find((item) => ['pending_approval', 'executing', 'pending_verify'].includes(item.status)))
const history = computed(() => requests.value.filter((item) => ['completed', 'cancelled', 'rejected'].includes(item.status)))
const filteredHistory = computed(() => history.value.filter((item) =>
  (!historyStatus.value || item.status === historyStatus.value)
  && `${item.requestNo} ${item.title} ${item.reason} ${item.executorName}`.toLowerCase().includes(historySearch.value.trim().toLowerCase()),
))
const stepIndex = computed(() => current.value?.status === 'pending_verify' ? 2 : current.value?.status === 'executing' ? 1 : 0)
const groups = computed(() => buildChangeTargetGroups(drawing.value, drawing.value ? drawingStore.getStructure(drawing.value.no) : []))
const filteredGroups = computed(() => groups.value.map((group) => ({ ...group, files: group.files.filter((file) =>
  `${group.no} ${group.name} ${file.name}`.toLowerCase().includes(search.value.trim().toLowerCase()),
) })).filter((group) => group.files.length))
const allIds = computed(() => [...new Set(groups.value.flatMap((group) => group.files.map((file) => file.id)))])
const allSelected = computed(() => allIds.value.length > 0 && allIds.value.every((id) => selected.value.includes(id)))
const selectedIds = computed(() => selected.value.filter((id) => allIds.value.includes(id)))
const canSubmit = computed(() => current.value?.status === 'executing' && (isAdmin.value || current.value.executorId === authStore.currentUser?.id))
const ready = computed(() => !busy.value && !loadFailed.value && form.reason.trim() && form.scope.trim() && selectedIds.value.length > 0)
let loadSequence = 0

async function refresh() {
  const id = drawing.value?.id
  const sequence = ++loadSequence
  if (!id) { loading.value = false; return }
  loading.value = true
  error.value = ''
  loadFailed.value = false
  try {
    const list = await changeRequestService.listByDrawing(id)
    const open = list.find((item) => ['pending_approval', 'executing', 'pending_verify'].includes(item.status))
    const detail = open ? await changeRequestService.get(open.id) : null
    if (sequence !== loadSequence) return
    requests.value = list.map((item) => item.id === detail?.id ? detail : item)
  } catch (cause) {
    if (sequence === loadSequence) {
      loadFailed.value = true
      error.value = cause instanceof Error ? cause.message : '工单加载失败，请重试'
    }
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

watch(() => drawing.value?.id, () => {
  requests.value = []
  selected.value = []
  search.value = ''
  form.reason = ''; form.scope = ''; form.executorId = ''
  submission.actualChanges = ''; submission.name = ''; submission.material = ''; submission.vendor = ''
  expandedId.value = ''
  activeView.value = 'current'; creating.value = false
  historySearch.value = ''; historyStatus.value = ''
  void refresh()
}, { immediate: true })

watch(isAdmin, async (admin) => {
  if (!admin) { users.value = []; return }
  try { users.value = await changeRequestService.listUsers() } catch { users.value = [] }
}, { immediate: true })

function selectAll() {
  selected.value = allSelected.value ? [] : [...allIds.value]
}

async function run(action: () => Promise<ChangeRequest>, message: string) {
  if (busy.value) return
  const drawingId = drawing.value?.id
  busy.value = true
  error.value = ''
  try {
    const updated = await action()
    if (drawingId !== drawing.value?.id) return
    requests.value = [updated, ...requests.value.filter((item) => item.id !== updated.id)]
    form.reason = ''; form.scope = ''; form.executorId = ''
    selected.value = []; search.value = ''
    submission.actualChanges = ''; submission.name = ''; submission.material = ''; submission.vendor = ''
    uiStore.toast(message, 'ok')
    creating.value = false
    if (['completed', 'cancelled', 'rejected'].includes(updated.status)) {
      activeView.value = 'history'
      historySearch.value = ''; historyStatus.value = ''
      expandedId.value = updated.id
    }
    // 保留当前图纸，原位刷新；清空缓存会让父级详情页卸载为“未找到图纸”。
    try { await drawingStore.refresh() } catch {
      uiStore.toast('工单已保存，图纸信息暂未刷新，请稍后刷新页面', 'warn')
    }
  } catch (cause) {
    if (drawingId === drawing.value?.id) error.value = cause instanceof Error ? cause.message : '操作失败，请重试'
  } finally { busy.value = false }
}

function create() {
  if (!drawing.value || !ready.value) return
  void run(() => changeRequestService.create({
    drawingId: drawing.value!.id, reason: form.reason.trim(), scope: form.scope.trim(),
    attachmentIds: selectedIds.value, executorId: form.executorId || undefined, requireVerify: true,
  }), '申请已提交，等待审批')
}

function submit() {
  const request = current.value
  if (!request || !canSubmit.value || !submission.actualChanges.trim()) return
  const proposed: ProposedAttributes = {}
  for (const field of ['name', 'material', 'vendor'] as const) {
    if (submission[field].trim()) proposed[field] = submission[field].trim()
  }
  void run(() => changeRequestService.submit(request.id, submission.actualChanges.trim(), proposed), request.requireVerify ? '本轮修改已结束，等待验收' : '工单已结束并发布')
}

async function expand(item: ChangeRequest) {
  if (expandedId.value === item.id) { expandedId.value = ''; return }
  expandedId.value = item.id
  if (item.actions) return
  historyLoading.value = true
  try {
    const detail = await changeRequestService.get(item.id)
    requests.value = requests.value.map((entry) => entry.id === detail.id ? detail : entry)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '历史加载失败' }
  finally { historyLoading.value = false }
}
</script>

<template>
  <main class="changes-page" :aria-busy="loading || busy">
    <header class="changes-heading">
      <div class="heading-title"><span class="heading-icon"><DemoIcon name="clipboard-list" :size="24" /></span><div><h2>变更工单</h2><p>每一次修改，都有清晰的来处与记录。</p></div></div>
      <div class="heading-actions">
        <button class="btn" type="button" :disabled="loading || busy" @click="refresh">刷新状态</button>
        <button v-if="activeView === 'history' && !current && drawing?.status === 'archived'" class="btn primary" type="button" :disabled="loading || busy || loadFailed" @click="creating = true; activeView = 'current'">＋ 新建工单</button>
        <RouterLink class="btn" :to="{ name: 'drawing-preview', params: { drawingId: route.params.drawingId } }">返回图纸文件</RouterLink>
      </div>
    </header>
    <nav class="workorder-tabs" aria-label="工单视图">
      <button type="button" :class="{ selected: activeView === 'current' }" :aria-pressed="activeView === 'current'" @click="activeView = 'current'">当前工单 <span>{{ current ? 1 : 0 }}</span></button>
      <button type="button" :class="{ selected: activeView === 'history' }" :aria-pressed="activeView === 'history'" @click="activeView = 'history'">历史工单 <span>{{ history.length }}</span></button>
    </nav>
    <div v-if="error" class="message error" role="alert">{{ error }} <button v-if="loadFailed && !busy" class="btn sm" @click="refresh">重新加载</button></div>
    <p v-if="loading" class="message" role="status">正在加载工单…</p>
    <template v-else-if="drawing">
      <template v-if="activeView === 'current'">
      <form v-if="creating && !current && !loadFailed && drawing.status === 'archived'" class="sheet create-sheet" @submit.prevent="create">
        <div class="section-heading"><div><span class="eyebrow">NEW REQUEST</span><h3>发起一次变更</h3></div><button class="btn sm" type="button" :disabled="busy" @click="creating = false">收起</button></div>
        <fieldset :disabled="busy">
          <div class="section-heading"><h4>1. 选择要修改的文件</h4><span>已选 {{ selectedIds.length }} / {{ allIds.length }}</span></div>
          <div class="file-toolbar">
            <input v-model="search" aria-label="搜索变更文件" placeholder="搜索图号、零件或文件名" type="search" />
            <button type="button" class="btn sm" :disabled="!allIds.length" @click="selectAll">{{ allSelected ? '取消全选' : '选择全部文件' }}</button>
          </div>
          <div class="target-list">
            <section v-for="group in filteredGroups" :key="group.no" class="target-group">
              <h5>{{ group.no }} · {{ group.name }} <span>{{ group.kind }}</span></h5>
              <label v-for="file in group.files" :key="file.id" class="file-option">
                <input v-model="selected" type="checkbox" :value="file.id" />
                <span class="file-kind">{{ file.category === 'model3d' ? '3D' : file.category === 'drawing2d' ? '2D' : '附件' }}</span>
                <span>{{ file.name }}</span>
              </label>
            </section>
            <p v-if="!filteredGroups.length" class="muted">{{ allIds.length ? '没有匹配的文件，请调整搜索条件。' : '当前项目没有可变更的文件。' }}</p>
          </div>
          <h4>2. 说明变更</h4>
          <label class="field">变更原因<input v-model="form.reason" required placeholder="例如：装配间隙不足" /></label>
          <label class="field">计划修改内容<textarea v-model="form.scope" required rows="3" placeholder="例如：调整所选零件的孔位和尺寸" /></label>
          <details v-if="isAdmin" class="optional"><summary>执行人设置（默认本人）</summary><label class="field">执行人<select v-model="form.executorId"><option value="">我本人</option><option v-for="user in users" :key="user.id" :value="user.id">{{ user.displayName }}</option></select></label></details>
          <footer class="form-footer"><span class="muted">审批后仅开放所选文件；完成后提交验收。</span><button class="btn primary" :disabled="!ready" type="submit">{{ busy ? '正在提交…' : '提交申请' }}</button></footer>
        </fieldset>
      </form>
      <section v-else-if="current" class="sheet current-sheet">
        <div class="section-heading"><div><span class="eyebrow">{{ current.requestNo }}</span><h3>{{ current.title || drawing.name }}</h3></div><span class="status" :data-status="current.status">{{ CHANGE_STATUS_LABELS[current.status] }}</span></div>
        <ol class="progress-track" aria-label="工单进度"><li v-for="(step, index) in ['申请审批', '执行修改', '成果验收', '完成发布']" :key="step" :class="{ reached: index <= stepIndex, now: index === stepIndex }" :aria-current="index === stepIndex ? 'step' : undefined"><span>{{ index + 1 }}</span>{{ step }}</li></ol>
        <div class="request-meta"><span>申请人 <b>{{ current.applicantName }}</b></span><span>创建于 {{ formatReadableDateTime(current.createdAt) }}</span></div>
        <dl class="request-info"><div><dt>变更原因</dt><dd>{{ current.reason }}</dd></div><div><dt>计划修改</dt><dd>{{ current.scope }}</dd></div><div><dt>执行人</dt><dd>{{ current.executorName || '待指定' }}</dd></div></dl>
        <details class="optional"><summary>授权文件 · {{ current.targets?.length ?? 0 }}</summary><ul><li v-for="target in current.targets" :key="target.attachmentId">{{ target.partNo || target.drawingNo }} · {{ target.name }}</li></ul></details>
        <form v-if="canSubmit" @submit.prevent="submit">
          <fieldset :disabled="busy">
            <p class="message">保存修改并结束本地编辑后，填写说明，点击结束工单。</p>
            <label class="field">实际修改说明<textarea v-model="submission.actualChanges" required rows="3" placeholder="填写本次完成的修改" /></label>
            <details class="optional"><summary>同时调整总图属性（可选）</summary><div class="attribute-grid"><label class="field">名称<input v-model="submission.name" placeholder="留空保持原值" /></label><label class="field">材料<input v-model="submission.material" placeholder="留空保持原值" /></label><label class="field">供应商<input v-model="submission.vendor" placeholder="留空保持原值" /></label></div></details>
            <footer class="form-footer"><span class="muted">{{ current.requireVerify ? '结束后自动提交验收，通过后发布正式版本。' : '此工单已获免验收批准，结束后直接发布正式版本。' }}</span><button type="submit" class="btn primary" :disabled="busy || !submission.actualChanges.trim()">{{ busy ? '正在结束…' : '结束工单' }}</button></footer>
          </fieldset>
        </form>
        <p v-else class="message">{{ current.status === 'pending_approval' ? '申请已提交，等待管理员审批。' : current.status === 'pending_verify' ? '修改已提交，等待管理员验收。' : `等待执行人 ${current.executorName} 完成修改。` }}</p>
        <details v-if="current.submissions?.length || current.actions?.length || current.actualChanges || current.diffs?.length" class="optional"><summary>提交与操作记录</summary>
          <article v-if="!current.submissions?.length && (current.actualChanges || current.diffs?.length)" class="record"><p>{{ current.actualChanges }}</p><p v-for="(diff, index) in current.diffs" :key="index">{{ diff.field }}：{{ diff.oldValue || '空' }} → {{ diff.newValue || '空' }}</p></article>
          <article v-for="entry in current.submissions" :key="entry.id" class="record"><b>{{ entry.legacyHistory ? '历史提交' : `第 ${entry.round} 轮提交` }}</b><p>{{ entry.actualChanges }}</p><p v-for="(diff, index) in entry.diffs" :key="index">{{ diff.field }}：{{ diff.oldValue || '空' }} → {{ diff.newValue || '空' }}</p></article>
          <p v-for="action in current.actions" :key="action.id" class="record muted">{{ action.createdAt.slice(0, 16).replace('T', ' ') }} · {{ action.actorName || '系统' }} · {{ action.opinion }}</p>
        </details>
      </section>
      <section v-else-if="!error" class="sheet empty-state"><DemoIcon name="clipboard-check" :size="36" /><h3>暂无进行中的工单</h3><p>{{ drawing.status === 'archived' ? '需要调整图纸时，新建工单并选择本次修改的文件。' : '图纸存档后，即可发起变更工单。' }}</p><button v-if="drawing.status === 'archived'" class="btn primary" type="button" @click="creating = true">新建变更工单</button><button v-if="history.length" class="btn" type="button" @click="activeView = 'history'">查看 {{ history.length }} 条历史工单</button></section>
      </template>
      <section v-else class="sheet history">
        <div class="section-heading"><div><span class="eyebrow">CHANGE HISTORY</span><h3>历史工单</h3></div><span class="muted">共 {{ history.length }} 条</span></div>
        <div class="history-filters"><input v-model="historySearch" type="search" aria-label="搜索历史工单" placeholder="搜索工单号、变更原因或执行人" /><select v-model="historyStatus" aria-label="筛选工单状态"><option value="">全部状态</option><option value="completed">已完成</option><option value="rejected">已驳回</option><option value="cancelled">已终止</option></select></div>
        <article v-for="item in filteredHistory" :key="item.id" class="history-item">
          <button type="button" class="history-button" :aria-expanded="expandedId === item.id" @click="expand(item)"><span class="history-title"><b>{{ item.title || item.reason }}</b><small>{{ item.requestNo }} · {{ item.executorName || item.applicantName }}</small></span><span class="status" :data-status="item.status">{{ CHANGE_STATUS_LABELS[item.status] }}</span><time>{{ formatReadableDateTime(item.completedAt || item.createdAt) }}</time><DemoIcon :name="expandedId === item.id ? 'chevron-up' : 'chevron-down'" :size="16" /></button>
          <div v-if="expandedId === item.id" class="history-summary"><dl class="request-info"><div><dt>计划修改</dt><dd>{{ item.scope }}</dd></div><div><dt>执行人</dt><dd>{{ item.executorName }}</dd></div></dl><ul v-if="item.targets?.length"><li v-for="target in item.targets" :key="target.attachmentId">{{ target.partNo || target.drawingNo }} · {{ target.name }}</li></ul><p v-if="!item.submissions?.length && item.actualChanges">{{ item.actualChanges }}</p><template v-if="!item.submissions?.length"><p v-for="(diff, index) in item.diffs" :key="index">{{ diff.field }}：{{ diff.oldValue || '空' }} → {{ diff.newValue || '空' }}</p></template></div>
          <div v-if="expandedId === item.id" class="record"><p>{{ item.reason }}</p><p v-if="historyLoading" role="status">加载记录…</p><article v-for="entry in item.submissions" :key="entry.id"><b>{{ entry.legacyHistory ? '历史提交' : `第 ${entry.round} 轮` }}</b><p>{{ entry.actualChanges }}</p><p v-for="(diff, index) in entry.diffs" :key="index">{{ diff.field }}：{{ diff.oldValue || '空' }} → {{ diff.newValue || '空' }}</p></article><p v-for="action in item.actions" :key="action.id" class="muted">{{ action.actorName || '系统' }} · {{ action.opinion }}</p></div>
        </article>
        <div v-if="!filteredHistory.length" class="empty-state"><DemoIcon name="history" :size="32" /><h3>{{ history.length ? '没有匹配的工单' : '暂无历史工单' }}</h3><p>{{ history.length ? '试试其他关键词或状态。' : '已完成、已驳回和已终止的工单将在这里保留。' }}</p></div>
      </section>
    </template>
    <p v-else-if="!loading" class="message">请从所属总图打开变更工单。</p>
  </main>
</template>

<style scoped>
.changes-page { width: min(100%, 1120px); margin: 0 auto; padding: 28px 24px 48px; box-sizing: border-box; color: var(--text); }
.changes-heading, .section-heading, .form-footer, .file-toolbar { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.changes-heading { margin-bottom: 28px; }
.heading-title { display: flex; align-items: center; gap: 14px; }
.heading-icon { display: grid; place-items: center; width: 48px; height: 48px; flex-shrink: 0; border: 1px solid var(--border); border-radius: 15px; background: var(--bg-card); color: var(--accent); }
.workorder-tabs { display: flex; gap: 8px; border-bottom: 1px solid var(--border); margin-bottom: 24px; }
.workorder-tabs button { display: flex; gap: 10px; align-items: center; padding: 14px 20px; background: transparent; color: var(--text-muted); border: 0; border-bottom: 2px solid transparent; font: inherit; cursor: pointer; }
.workorder-tabs button.selected { color: var(--accent); border-bottom-color: var(--accent); font-weight: 600; }
.workorder-tabs button span { font-size: 12px; background: var(--bg-card); border: 1px solid var(--border); border-radius: 12px; min-width: 24px; text-align: center; }
.eyebrow { display: block; font-size: 11px; letter-spacing: .08em; color: var(--text-muted); margin-bottom: 8px; }
.progress-track { list-style: none; display: grid; grid-template-columns: repeat(4, 1fr); padding: 18px; margin: 24px 0; border-radius: 10px; background: var(--bg); gap: 12px; }
.progress-track li { display: flex; align-items: center; gap: 9px; font-size: 12px; color: var(--text-muted); }
.progress-track li > span { display: grid; place-items: center; width: 26px; height: 26px; border: 1px solid var(--border); border-radius: 50%; flex-shrink: 0; }
.progress-track li.reached { color: var(--accent); }.progress-track li.now > span { background: var(--accent); color: white; border-color: var(--accent); }.progress-track li.now { font-weight: 600; }
.request-meta { display: flex; flex-wrap: wrap; gap: 20px; font-size: 12px; color: var(--text-muted); padding-bottom: 16px; border-bottom: 1px solid var(--border); }.request-meta b { color: var(--text); font-weight: 500; }
.history-filters { display: flex; gap: 12px; margin: 20px 0; }.history-filters select { width: 140px; flex-shrink: 0; }
.history-title { display: flex; flex-direction: column; gap: 7px; min-width: 0; flex: 1; }.history-title b { overflow-wrap: anywhere; }.history-title small { color: var(--text-muted); font-weight: 400; }
.history-summary { padding: 4px 20px; background: var(--bg); border-radius: 8px; font-size: 13px; }
.empty-state { text-align: center; padding: 54px 24px; color: var(--text-muted); }.empty-state > svg { color: var(--accent); margin-bottom: 18px; }.empty-state h3 { color: var(--text); margin-bottom: 10px; }.empty-state p { font-size: 13px; margin-bottom: 22px; }.empty-state .btn + .btn { margin-left: 10px; }
.status[data-status=completed] { color: var(--success, #16815c); }.status[data-status=rejected], .status[data-status=cancelled] { color: var(--text-muted); }
.heading-actions { display: flex; flex-wrap: wrap; gap: 8px; }
h2, h3, h4, h5, p { margin: 0; }
h2 { font-size: 21px; } h3 { font-size: 17px; } h4 { margin: 20px 0 12px; font-size: 14px; }
.changes-heading p, .muted, dt { color: var(--text-muted, #718096); font-size: 13px; }
.changes-heading p { margin-top: 6px; }
.sheet { padding: 28px; background: var(--bg-card); border: 1px solid var(--border); border-radius: 14px; margin-bottom: 16px; box-shadow: 0 8px 28px rgb(20 50 80 / 4%); }
fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
.section-heading h4 { margin-top: 0; }.section-heading { margin-bottom: 12px; }.sheet > h3 { margin-bottom: 20px; }
.section-heading > span { font-size: 12px; }
.field { display: flex; flex-direction: column; gap: 8px; margin: 16px 0; font-size: 13px; font-weight: 600; }
input:not([type=checkbox]), textarea, select { width: 100%; min-width: 0; box-sizing: border-box; padding: 10px 12px; border: 1px solid var(--border); background: var(--bg); color: var(--text); border-radius: 7px; font: inherit; }
textarea { resize: vertical; }
input:focus-visible, textarea:focus-visible, select:focus-visible, button:focus-visible, summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
input[type=checkbox] { accent-color: var(--accent); width: 16px; height: 16px; flex-shrink: 0; }
.file-toolbar { margin-bottom: 12px; }.file-toolbar .btn { flex-shrink: 0; }
.target-list { max-height: 310px; overflow: auto; border: 1px solid var(--border); border-radius: 8px; padding: 8px 12px; scrollbar-width: thin; }
.target-group h5 { font-size: 12px; padding: 10px 0; }.target-group h5 span { font-weight: 400; color: var(--text-muted); }
.file-option { display: flex; gap: 10px; align-items: center; padding: 9px 4px; font-size: 13px; overflow-wrap: anywhere; cursor: pointer; }
.file-option:hover, .file-option:has(input:checked) { background: var(--bg); }.file-option { border-radius: 6px; }.file-kind { font-size: 11px; color: var(--accent); flex-shrink: 0; }
.optional { border-top: 1px solid var(--border); margin-top: 16px; padding-top: 14px; font-size: 13px; }
summary { cursor: pointer; padding: 4px 0; }li { margin: 8px 0; overflow-wrap: anywhere; }
.form-footer { padding-top: 18px; }.status { border-radius: 6px; padding: 6px 10px; background: var(--bg); color: var(--accent); }
.request-info > div { display: grid; grid-template-columns: 80px 1fr; gap: 12px; margin: 12px 0; }
dd { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14px; }
.message { padding: 14px; background: var(--bg); border-radius: 8px; margin: 16px 0; font-size: 13px; line-height: 1.7; }.error { border: 1px solid var(--danger, #dc5656); }
.attribute-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 18px; }
.history { padding: 18px 24px; }.history-item { border-top: 1px solid var(--border); margin-top: 12px; }
.history-button { width: 100%; display: flex; gap: 16px; align-items: center; text-align: left; color: inherit; background: transparent; border: 0; padding: 16px 0; cursor: pointer; }.history-button time { margin-left: auto; font-size: 12px; color: var(--text-muted); }
.record { padding: 12px 0; line-height: 1.8; font-size: 13px; overflow-wrap: anywhere; }.record p { white-space: pre-wrap; }
@media (max-width: 700px) { .changes-page { padding: 12px; }.sheet { padding: 16px; }.changes-heading, .form-footer { align-items: flex-start; flex-direction: column; }.attribute-grid { grid-template-columns: 1fr; }.history-button { flex-wrap: wrap; }.file-toolbar { flex-wrap: wrap; } }
@media (max-width: 700px) { .progress-track { grid-template-columns: 1fr 1fr; }.history-title { flex-basis: 100%; }.history-filters { flex-direction: column; }.history-filters select { width: 100%; }.heading-actions { margin-top: 12px; } }
</style>
