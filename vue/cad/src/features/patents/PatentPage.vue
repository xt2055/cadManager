<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { downloadEvidence, lifecycleApi, patentAlerts, type PatentRecord, type EvidenceDocument } from '@/services/lifecycle.service'
import PatentReceiptUpload from '@/features/patents/PatentReceiptUpload.vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import { changeRequestService, type ChangeUserOption } from '@/services/change-request.service'

const auth = useAuthStore()
const drawingStore = useDrawingStore()
const uiStore = useUiStore()
const users = ref<ChangeUserOption[]>([])
const patents = ref<PatentRecord[]>([])
const selected = ref<PatentRecord | null>(null)
const search = ref('')
const onlyAlerts = ref(false)
const busy = ref(false)
const loading = ref(false)
const error = ref('')
const message = ref('')
const editing = ref(false)

const empty = () => ({
  number: '', title: '', patentType: '发明', jurisdiction: '中国', ownerName: '',
  responsibleId: auth.currentUser?.id || '', drawingId: '', startDate: '', feeCycleMonths: 12, expiresOn: '',
  reminderDays: 90, notes: '', revision: 0,
})
const form = reactive(empty())
const receipts = ref<EvidenceDocument[]>([])
const events = ref<{ id: string; action: string; actor: string; createdAt: string; detail: Record<string, unknown> }[]>([])
const eventLabels: Record<string, string> = { create: '新增登记', update: '修改登记', payment: '缴费登记', reminder: '期限提醒' }
const eventIcons: Record<string, string> = { create: 'plus', update: 'pencil', payment: 'check-circle-2', reminder: 'bell' }

const activeTab = ref<'overview' | 'payment' | 'history'>('overview')
const uploadOpen = ref(false)
const tabs = [
  { key: 'overview', label: '概览', icon: 'shield' },
  { key: 'payment', label: '缴费管理', icon: 'save' },
  { key: 'history', label: '办理历史', icon: 'history' },
] as const
const paymentReceipts = computed(() => receipts.value.filter(d => d.category === '缴费凭证'))

function eventLines(event: (typeof events.value)[number]) {
  if (event.action === 'reminder') return [`${event.detail.kind === 'fee' ? '缴费截止' : '权利到期'}：${event.detail.deadline}`, `触发时距登记期限 ${event.detail.days} 天`]
  const next = event.detail.submitted as Record<string, unknown> | undefined
  const before = event.detail.before as Record<string, unknown> | null | undefined
  if (!next) return ['原记录未包含详细字段。']
  if (event.action === 'payment') return [
    `本期缴费截止：${before?.fee_due || '原记录未登记'}`,
    `实际缴费：${next.paidOn || ''}`,
    `缴费凭证：${receipts.value.find(d => d.id === next.receiptId)?.title || '已归档原件'}`,
    `下一缴费截止：${next.feeDue || ''}`,
  ]
  const fields = [['number', 'number', '专利编号'], ['title', 'title', '名称'], ['patentType', 'patent_type', '类型'], ['jurisdiction', 'jurisdiction', '国家 / 地区'], ['ownerName', 'owner_name', '权利人'], ['startDate', 'start_date', '申请日'], ['feeCycleMonths', 'fee_cycle_months', '缴费周期(月)'], ['feeDue', 'fee_due', '本期缴费截止'], ['expiresOn', 'expires_on', '权利到期'], ['reminderDays', 'reminder_days', '提前提醒天数'], ['notes', 'notes', '备注']] as const
  return fields.filter(([key, oldKey]) => event.action === 'create' || String(next[key] ?? '') !== String(before?.[oldKey] ?? '')).map(([key, oldKey, label]) => event.action === 'create' ? `${label}：${next[key] || '未登记'}` : `${label}：${before?.[oldKey] || '未登记'} → ${next[key] || '未登记'}`)
}

const canEdit = computed(() => !selected.value || selected.value.responsibleId === auth.currentUser?.id || auth.hasRole('admin'))
const visible = computed(() => patents.value.filter(p => `${p.number} ${p.title} ${p.ownerName}`.toLowerCase().includes(search.value.toLowerCase()) && (!onlyAlerts.value || patentAlerts(p).length)))
const alertCount = computed(() => patents.value.filter(p => patentAlerts(p).length).length)
const stats = computed(() => {
  let overdue = 0
  let normal = 0
  for (const p of patents.value) {
    const isOverdue = (p.feeDays !== null && p.feeDays < 0) || (p.expiryDays !== null && p.expiryDays < 0)
    if (isOverdue) overdue += 1
    else if (!patentAlerts(p).length) normal += 1
  }
  return { total: patents.value.length, alert: alertCount.value, overdue, normal }
})
const linkedDrawing = computed(() => {
  const id = selected.value?.drawingId
  if (!id) return ''
  const target = drawingStore.drawings.find(d => (d.id || d.no) === id)
  return target ? `${target.no} · ${target.name}` : id
})

function patentStatus(p: PatentRecord) {
  const overdue = (p.feeDays !== null && p.feeDays < 0) || (p.expiryDays !== null && p.expiryDays < 0)
  if (overdue) return { key: 'danger', label: '已逾期' }
  const soon = (p.feeDays !== null && p.feeDays <= p.reminderDays) || (p.expiryDays !== null && p.expiryDays <= p.reminderDays)
  if (soon) return { key: 'warn', label: '临近期限' }
  if (!p.feeDue || !p.expiresOn) return { key: 'info', label: '资料待完善' }
  return { key: 'ok', label: '期限正常' }
}
function dueHint(days: number | null, missing: boolean) {
  if (missing) return '未登记日期'
  if (days === null) return '暂无期限数据'
  if (days < 0) return `已逾期 ${-days} 天`
  return `剩余 ${days} 天`
}
function dueTone(days: number | null, reminder: number) {
  if (days === null) return ''
  if (days < 0) return 'danger'
  return days <= reminder ? 'warn' : ''
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    patents.value = await lifecycleApi<PatentRecord[]>('/patents')
    if (selected.value) selected.value = patents.value.find(p => p.id === selected.value?.id) || null
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

let selectionGeneration = 0
async function select(p: PatentRecord) {
  selected.value = p
  editing.value = false
  activeTab.value = 'overview'
  uploadOpen.value = false
  events.value = []
  receipts.value = []
  error.value = ''
  message.value = ''
  const seq = ++selectionGeneration
  try {
    const [docs, history] = await Promise.all([
      lifecycleApi<EvidenceDocument[]>(`/lifecycle-documents?patentId=${p.id}`),
      lifecycleApi<typeof events.value>(`/patents/${p.id}/events`),
    ])
    if (seq === selectionGeneration) {
      receipts.value = docs
      events.value = history
    }
  } catch (e) {
    if (seq === selectionGeneration) error.value = (e as Error).message
  }
}

function create() {
  ++selectionGeneration
  selected.value = null
  Object.assign(form, empty())
  editing.value = true
  message.value = ''
  error.value = ''
}
function edit() {
  const p = selected.value
  if (!p) return
  Object.assign(form, {
    number: p.number, title: p.title, patentType: p.patentType, jurisdiction: p.jurisdiction,
    ownerName: p.ownerName, responsibleId: p.responsibleId, drawingId: p.drawingId || '',
    startDate: p.startDate || '', feeCycleMonths: p.feeCycleMonths || 12, expiresOn: p.expiresOn || '',
    reminderDays: p.reminderDays, notes: p.notes, revision: p.revision,
  })
  editing.value = true
}
async function save() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const result = await lifecycleApi<{ id: string }>(`/patents${selected.value ? `/${selected.value.id}` : ''}`, { method: selected.value ? 'PUT' : 'POST', body: JSON.stringify(form) })
    await load()
    const target = patents.value.find(p => p.id === result.id)
    if (target) await select(target)
    message.value = '专利记录已保存，提醒日期已更新。'
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
async function refreshReceipts() {
  if (!selected.value) return
  try {
    receipts.value = await lifecycleApi<EvidenceDocument[]>(`/lifecycle-documents?patentId=${selected.value.id}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}
function todayDate() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
}

async function recordPayment(doc: EvidenceDocument) {
  if (!selected.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await lifecycleApi(`/patents/${selected.value.id}/payment`, { method: 'POST', body: JSON.stringify({ receiptId: doc.id, paidOn: todayDate(), revision: selected.value.revision }) })
    await load()
    if (selected.value) await select(selected.value)
    activeTab.value = 'payment'
    message.value = '已登记缴费，下一期缴费截止按周期自动顺延。'
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function openReceiptUpload() {
  uploadOpen.value = true
}

function onReceiptUploaded() {
  uploadOpen.value = false
  void refreshReceipts()
}

async function downloadReceipt(doc: EvidenceDocument) {
  try {
    await downloadEvidence(`/lifecycle-documents/${doc.id}`, doc.fileName)
    uiStore.toast(`已开始下载：${doc.fileName}`, 'ok')
  } catch (e) {
    error.value = (e as Error).message
  }
}

onMounted(() => {
  void load()
  void drawingStore.load().catch(() => { error.value = '关联图号列表暂不可用，请刷新' })
  if (auth.hasRole('admin')) void changeRequestService.listUsers().then(result => { users.value = result }).catch(() => { error.value = '负责人列表暂不可用，请刷新' })
})
</script>

<template>
  <div class="page patent-page">
    <header class="patent-hero">
      <div class="hero-id">
        <span class="hero-mark"><DemoIcon name="shield-check" :size="22" /></span>
        <div>
          <h2>专利管理</h2>
          <p>缴费、期限、证明材料及办理记录统一归档</p>
        </div>
      </div>
      <div class="hero-actions">
        <button class="btn" :disabled="busy || loading" type="button" @click="load">
          <DemoIcon name="refresh-cw" :size="14" />刷新
        </button>
        <button class="btn primary" :disabled="busy" type="button" @click="create">
          <DemoIcon name="plus" :size="14" />新增专利
        </button>
      </div>
    </header>

    <section class="patent-stats">
      <article class="stat-card">
        <span class="stat-icon tone-accent"><DemoIcon name="shield" :size="16" /></span>
        <div class="stat-body"><b>{{ stats.total }}</b><span>专利总数</span></div>
      </article>
      <article class="stat-card">
        <span class="stat-icon tone-warn"><DemoIcon name="alert-triangle" :size="16" /></span>
        <div class="stat-body"><b>{{ stats.alert }}</b><span>需要关注</span></div>
      </article>
      <article class="stat-card">
        <span class="stat-icon tone-danger"><DemoIcon name="clock" :size="16" /></span>
        <div class="stat-body"><b>{{ stats.overdue }}</b><span>已逾期</span></div>
      </article>
      <article class="stat-card">
        <span class="stat-icon tone-ok"><DemoIcon name="check-circle-2" :size="16" /></span>
        <div class="stat-body"><b>{{ stats.normal }}</b><span>期限正常</span></div>
      </article>
    </section>

    <p v-if="error" class="patent-alert error" role="alert"><DemoIcon name="alert-triangle" :size="14" />{{ error }}</p>
    <p v-if="message" class="patent-alert success" role="status"><DemoIcon name="check-circle-2" :size="14" />{{ message }}</p>

    <div class="patent-toolbar">
      <label class="search-box">
        <DemoIcon name="search" :size="15" />
        <input v-model="search" placeholder="搜索专利编号、名称、权利人" aria-label="搜索专利" />
        <button v-if="search" class="search-clear" type="button" aria-label="清除搜索" @click="search = ''"><DemoIcon name="x" :size="13" /></button>
      </label>
      <button class="filter-toggle" :class="{ active: onlyAlerts }" type="button" :aria-pressed="onlyAlerts" @click="onlyAlerts = !onlyAlerts">
        <DemoIcon name="filter" :size="13" />仅需关注<span class="filter-count">{{ alertCount }}</span>
      </button>
      <span class="result-count">显示 {{ visible.length }} / {{ patents.length }} 项</span>
    </div>

    <div class="patent-layout">
      <aside class="patent-list" aria-label="专利列表">
        <div v-if="loading" class="list-placeholder"><span class="spinner" aria-hidden="true"></span>正在加载专利…</div>
        <div v-else-if="!visible.length" class="list-placeholder">
          <DemoIcon name="search-x" :size="26" />
          <span>暂无符合条件的专利</span>
        </div>
        <button
          v-for="p in visible"
          :key="p.id"
          class="patent-card"
          :class="{ active: selected?.id === p.id, alerting: patentAlerts(p).length }"
          type="button"
          :disabled="busy"
          @click="select(p)"
        >
          <div class="pc-top">
            <span class="tag plain pc-type">{{ p.patentType }}</span>
            <span class="tag" :class="patentStatus(p).key">{{ patentStatus(p).label }}</span>
          </div>
          <strong class="pc-title">{{ p.title }}</strong>
          <div class="pc-meta"><DemoIcon name="hash" :size="12" /><span>{{ p.number }}</span></div>
          <div class="pc-foot">
            <span><DemoIcon name="user-check" :size="12" />{{ p.responsibleName || '未指派' }}</span>
            <span><DemoIcon name="calendar" :size="12" />{{ p.feeDue || '待完善' }}</span>
          </div>
          <div v-if="patentAlerts(p).length" class="pc-alert"><DemoIcon name="alert-triangle" :size="12" />{{ patentAlerts(p)[0] }}</div>
        </button>
      </aside>

      <main class="patent-detail">
        <form v-if="editing" class="card patent-form" @submit.prevent="save">
          <div class="card-title">
            <DemoIcon :name="selected ? 'pencil' : 'plus'" :size="16" />
            {{ selected ? '修改专利登记' : '新增专利' }}
          </div>
          <div class="form-body">
            <section class="form-section">
              <h4>基本信息</h4>
              <div class="form-grid">
                <label class="field"><span>专利 / 申请编号</span><input v-model="form.number" required maxlength="100" /></label>
                <label class="field"><span>专利名称</span><input v-model="form.title" required maxlength="300" /></label>
                <label class="field"><span>类型</span><select v-model="form.patentType"><option>发明</option><option>实用新型</option><option>外观设计</option><option>其他</option></select></label>
                <label class="field"><span>国家 / 地区</span><input v-model="form.jurisdiction" required /></label>
                <label class="field"><span>权利人</span><input v-model="form.ownerName" /></label>
                <label class="field"><span>提前提醒天数</span><input v-model.number="form.reminderDays" type="number" min="1" max="365" required /></label>
              </div>
            </section>

            <section class="form-section">
              <h4>期限信息</h4>
              <div class="form-grid">
                <label class="field"><span>申请日（起始日期）</span><input v-model="form.startDate" type="date" required /></label>
                <label class="field"><span>缴费周期（月）</span><input v-model.number="form.feeCycleMonths" type="number" min="1" max="120" required /></label>
                <label class="field"><span>权利到期日期</span><input v-model="form.expiresOn" type="date" /></label>
              </div>
            </section>

            <section class="form-section">
              <h4>关联与责任</h4>
              <div class="form-grid">
                <label class="field"><span>关联图号</span>
                  <select v-model="form.drawingId">
                    <option value="">独立专利，不关联图号</option>
                    <option v-for="d in drawingStore.drawings" :key="d.id || d.no" :value="d.id">{{ d.no }} · {{ d.name }}</option>
                  </select>
                </label>
                <label v-if="auth.hasRole('admin')" class="field"><span>负责人</span>
                  <select v-model="form.responsibleId" required>
                    <option :value="auth.currentUser?.id">我本人</option>
                    <option v-for="u in users.filter(u => u.id !== auth.currentUser?.id)" :key="u.id" :value="u.id">{{ u.displayName }}</option>
                  </select>
                </label>
                <label class="field wide"><span>备注</span><textarea v-model="form.notes" rows="3" /></label>
              </div>
            </section>

            <p class="form-note"><DemoIcon name="info" :size="14" />本期缴费截止日期由申请日与缴费周期自动推算，缴费后按周期顺延；权利到期日单独登记。缺少日期会显示资料待完善提醒。</p>
          </div>
          <div class="form-actions">
            <button class="btn primary" :disabled="busy" type="submit">{{ busy ? '正在保存…' : '保存专利' }}</button>
            <button class="btn" type="button" :disabled="busy" @click="editing = false">取消编辑</button>
          </div>
        </form>

        <template v-else-if="selected">
          <div class="patent-detail-head">
            <nav class="patent-tabs" role="tablist" aria-label="专利详情分区">
              <button
                v-for="tab in tabs"
                :key="tab.key"
                class="pt-tab"
                :class="{ active: activeTab === tab.key }"
                type="button"
                role="tab"
                :aria-selected="activeTab === tab.key"
                @click="activeTab = tab.key"
              >
                <DemoIcon :name="tab.icon" :size="14" />
                {{ tab.label }}
                <span v-if="tab.key === 'history'" class="pt-count">{{ events.length }}</span>
              </button>
            </nav>
            <button v-if="canEdit" class="btn sm" type="button" :disabled="busy" @click="edit"><DemoIcon name="pencil" :size="13" />编辑登记</button>
          </div>

          <section v-show="activeTab === 'overview'" class="tab-panel">
          <article class="card patent-overview">
            <div class="ov-head">
              <div class="ov-tags">
                <span class="tag plain">{{ selected.patentType }}</span>
                <span class="tag mute"><DemoIcon name="stamp" :size="11" />{{ selected.jurisdiction }}</span>
                <span class="tag" :class="patentStatus(selected).key">{{ patentStatus(selected).label }}</span>
              </div>
            </div>
            <h3 class="ov-title">{{ selected.title }}</h3>
            <p class="ov-number"><DemoIcon name="hash" :size="13" />{{ selected.number }}</p>

            <div class="ov-grid">
              <div class="ov-item">
                <span class="ov-label"><DemoIcon name="calendar" :size="13" />申请日</span>
                <b>{{ selected.startDate || '待完善' }}</b>
                <small>起始日期</small>
              </div>
              <div class="ov-item">
                <span class="ov-label"><DemoIcon name="refresh-cw" :size="13" />缴费周期</span>
                <b>每 {{ selected.feeCycleMonths }} 个月</b>
                <small>缴费后自动顺延</small>
              </div>
              <div class="ov-item">
                <span class="ov-label"><DemoIcon name="calendar" :size="13" />本期缴费截止</span>
                <b :class="dueTone(selected.feeDays, selected.reminderDays)">{{ selected.feeDue || '待完善' }}</b>
                <small>{{ dueHint(selected.feeDays, !selected.feeDue) }}</small>
              </div>
              <div class="ov-item">
                <span class="ov-label"><DemoIcon name="clock" :size="13" />权利到期</span>
                <b :class="dueTone(selected.expiryDays, selected.reminderDays)">{{ selected.expiresOn || '待完善' }}</b>
                <small>{{ dueHint(selected.expiryDays, !selected.expiresOn) }}</small>
              </div>
              <div class="ov-item">
                <span class="ov-label"><DemoIcon name="user-check" :size="13" />负责人</span>
                <b>{{ selected.responsibleName || '未指派' }}</b>
                <small>提前 {{ selected.reminderDays }} 天提醒</small>
              </div>
              <div class="ov-item">
                <span class="ov-label"><DemoIcon name="users" :size="13" />权利人</span>
                <b>{{ selected.ownerName || '未登记' }}</b>
                <small>{{ selected.jurisdiction }}</small>
              </div>
              <div v-if="linkedDrawing" class="ov-item wide">
                <span class="ov-label"><DemoIcon name="layers" :size="13" />关联图号</span>
                <b class="ov-text">{{ linkedDrawing }}</b>
              </div>
            </div>

            <div v-if="patentAlerts(selected).length" class="ov-alerts">
              <p v-for="alert in patentAlerts(selected)" :key="alert"><DemoIcon name="alert-triangle" :size="13" />{{ alert }}</p>
            </div>
            <div v-if="selected.notes" class="ov-notes">
              <span class="ov-label"><DemoIcon name="scroll-text" :size="13" />备注</span>
              <p>{{ selected.notes }}</p>
            </div>
          </article>
          </section>

          <section v-show="activeTab === 'payment'" class="tab-panel">
            <article class="card pay-summary">
              <div class="card-title"><DemoIcon name="save" :size="16" />缴费周期与状态</div>
              <div class="ov-grid">
                <div class="ov-item">
                  <span class="ov-label"><DemoIcon name="calendar" :size="13" />申请日</span>
                  <b>{{ selected.startDate || '待完善' }}</b>
                  <small>起始日期</small>
                </div>
                <div class="ov-item">
                  <span class="ov-label"><DemoIcon name="refresh-cw" :size="13" />缴费周期</span>
                  <b>每 {{ selected.feeCycleMonths }} 个月</b>
                  <small>缴费后自动顺延</small>
                </div>
                <div class="ov-item">
                  <span class="ov-label"><DemoIcon name="calendar" :size="13" />本期缴费截止</span>
                  <b :class="dueTone(selected.feeDays, selected.reminderDays)">{{ selected.feeDue || '待完善' }}</b>
                  <small>{{ dueHint(selected.feeDays, !selected.feeDue) }}</small>
                </div>
                <div class="ov-item">
                  <span class="ov-label"><DemoIcon name="clock" :size="13" />权利到期</span>
                  <b :class="dueTone(selected.expiryDays, selected.reminderDays)">{{ selected.expiresOn || '待完善' }}</b>
                  <small>{{ dueHint(selected.expiryDays, !selected.expiresOn) }}</small>
                </div>
              </div>
            </article>

            <article class="card pay-receipts">
              <div class="card-title">
                <DemoIcon name="file-up" :size="16" />缴费凭证
                <span class="hint">共 {{ paymentReceipts.length }} 份</span>
                <button v-if="canEdit" class="btn sm primary title-action" type="button" @click="openReceiptUpload"><DemoIcon name="upload" :size="13" />上传凭证</button>
              </div>
              <div v-if="!paymentReceipts.length" class="receipt-empty">
                <DemoIcon name="file-text" :size="24" />
                <span>上传缴费凭证后，即可一键登记缴费（默认使用今天日期）</span>
              </div>
              <ul v-else class="receipt-list">
                <li v-for="doc in paymentReceipts" :key="doc.id" class="receipt-item">
                  <span class="ri-icon"><DemoIcon name="file-text" :size="15" /></span>
                  <div class="ri-main">
                    <b>{{ doc.title }}</b>
                    <small>{{ doc.fileName }} · {{ new Date(doc.createdAt).toLocaleDateString() }}</small>
                  </div>
                  <div class="ri-actions">
                    <button class="btn sm" type="button" @click="downloadReceipt(doc)"><DemoIcon name="download" :size="12" />下载</button>
                    <button v-if="canEdit" class="btn sm primary" type="button" :disabled="busy" @click="recordPayment(doc)"><DemoIcon name="check-circle-2" :size="12" />登记缴费</button>
                  </div>
                </li>
              </ul>
            </article>
          </section>

          <section v-show="activeTab === 'history'" class="tab-panel">
            <section class="card patent-history">
              <div class="card-title"><DemoIcon name="history" :size="16" />办理历史<span class="hint">共 {{ events.length }} 条</span></div>
              <div v-if="!events.length" class="history-empty"><DemoIcon name="history" :size="26" />暂无办理记录</div>
              <div v-else class="timeline">
                <details v-for="event in events" :key="event.id" class="tl-item">
                  <summary>
                    <span class="tl-dot" :class="event.action"></span>
                    <span class="tl-icon"><DemoIcon :name="eventIcons[event.action] || 'activity'" :size="13" /></span>
                    <span class="tl-title">{{ eventLabels[event.action] || event.action }}</span>
                    <span class="tl-meta">{{ new Date(event.createdAt).toLocaleString() }} · {{ event.actor }}</span>
                  </summary>
                  <div class="tl-body">
                    <p v-for="(line, index) in eventLines(event)" :key="index">{{ line }}</p>
                  </div>
                </details>
              </div>
            </section>
          </section>

          <PatentReceiptUpload :patent-id="selected.id" :open="uploadOpen" @close="uploadOpen = false" @uploaded="onReceiptUploaded" />
        </template>

        <div v-else class="card detail-empty">
          <span class="empty-mark"><DemoIcon name="shield" :size="34" /></span>
          <div class="t">未选择专利</div>
          <p>从左侧选择一项专利查看资料与办理历史，或新增专利登记。</p>
          <button class="btn primary" type="button" @click="create"><DemoIcon name="plus" :size="14" />新增专利</button>
        </div>
      </main>
    </div>
  </div>
</template>

<style scoped>
.patent-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 页头 */
.patent-hero {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.hero-id {
  display: flex;
  align-items: center;
  gap: 14px;
  flex: 1;
  min-width: 0;
}

.hero-mark {
  display: grid;
  width: 46px;
  height: 46px;
  flex: none;
  place-items: center;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--accent-soft);
  color: var(--accent);
}

.patent-hero h2 {
  margin: 0;
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 900;
}

.patent-hero p {
  margin: 4px 0 0;
  color: var(--text-2);
  font-size: 12.5px;
}

.hero-actions {
  display: flex;
  gap: 10px;
  flex: none;
}

/* 统计 */
.patent-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--panel);
  box-shadow: var(--shadow);
  transition: border-color 0.25s, transform 0.25s;
}

.stat-card:hover {
  border-color: var(--line-strong);
  transform: translateY(-2px);
}

.stat-icon {
  display: grid;
  width: 36px;
  height: 36px;
  flex: none;
  place-items: center;
  border-radius: 11px;
}

.stat-icon.tone-accent { background: var(--accent-soft); color: var(--accent); }
.stat-icon.tone-warn { background: rgb(251 191 36 / 12%); color: var(--warn); }
.stat-icon.tone-danger { background: rgb(248 113 113 / 12%); color: var(--danger); }
.stat-icon.tone-ok { background: rgb(52 211 153 / 12%); color: var(--ok); }

.stat-body {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.stat-body b {
  font-family: 'JetBrains Mono', monospace;
  font-size: 20px;
  line-height: 1.1;
}

.stat-body span {
  margin-top: 3px;
  color: var(--text-3);
  font-size: 11.5px;
}

/* 提示条 */
.patent-alert {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  padding: 10px 14px;
  border: 1px solid;
  border-radius: 11px;
  font-size: 12.5px;
}

.patent-alert.error { border-color: rgb(248 113 113 / 45%); background: rgb(248 113 113 / 8%); color: var(--danger); }
.patent-alert.success { border-color: rgb(52 211 153 / 45%); background: rgb(52 211 153 / 8%); color: var(--ok); }

/* 工具栏 */
.patent-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 220px;
  height: 36px;
  padding: 0 12px;
  border: 1px solid var(--line);
  border-radius: 11px;
  background: var(--panel);
  color: var(--text-3);
  transition: border-color 0.25s, box-shadow 0.25s;
}

.search-box:focus-within {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.search-box input {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  color: var(--text-1);
  font: inherit;
  font-size: 13px;
  outline: none;
}

.search-clear {
  display: grid;
  width: 20px;
  height: 20px;
  place-items: center;
  border-radius: 6px;
  color: var(--text-3);
}

.search-clear:hover {
  background: var(--hover);
  color: var(--text-1);
}

.filter-toggle {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  height: 36px;
  padding: 0 13px;
  border: 1px solid var(--line);
  border-radius: 11px;
  background: var(--panel);
  color: var(--text-2);
  font-size: 12.5px;
  transition: all 0.25s;
}

.filter-toggle:hover {
  border-color: var(--line-strong);
  color: var(--text-1);
}

.filter-toggle.active {
  border-color: transparent;
  background: var(--accent);
  color: var(--accent-ink);
}

.filter-count {
  display: grid;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  place-items: center;
  border-radius: 99px;
  background: var(--accent-soft);
  color: var(--accent);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  font-weight: 700;
}

.filter-toggle.active .filter-count {
  background: rgb(255 255 255 / 26%);
  color: var(--accent-ink);
}

.result-count {
  margin-left: auto;
  color: var(--text-3);
  font-size: 11.5px;
  white-space: nowrap;
}

/* 主布局 */
.patent-layout {
  display: grid;
  grid-template-columns: minmax(280px, 340px) minmax(0, 1fr);
  gap: 18px;
  align-items: start;
}

.patent-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}

.list-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 42px 16px;
  border: 1px dashed var(--line);
  border-radius: var(--radius);
  color: var(--text-3);
  font-size: 12.5px;
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--line-strong);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: patent-spin 0.8s linear infinite;
}

@keyframes patent-spin {
  to { transform: rotate(360deg); }
}

.patent-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  padding: 14px 16px;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--panel);
  color: inherit;
  cursor: pointer;
  text-align: left;
  transition: border-color 0.22s, background 0.22s, transform 0.22s, box-shadow 0.22s;
}

.patent-card:hover {
  border-color: var(--line-strong);
  transform: translateX(2px);
}

.patent-card.active {
  border-color: var(--accent);
  background: var(--panel-2);
  box-shadow: 0 0 0 1px var(--accent-soft), var(--shadow);
}

.patent-card.active::before {
  position: absolute;
  top: 12px;
  bottom: 12px;
  left: 0;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: var(--accent);
  content: '';
}

.patent-card.alerting:not(.active) {
  border-left-color: var(--warn);
}

.pc-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pc-type {
  font-size: 10.5px;
}

.pc-top .tag:last-child {
  margin-left: auto;
}

.pc-title {
  font-size: 13.5px;
  line-height: 1.45;
}

.pc-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-2);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
}

.pc-foot {
  display: flex;
  align-items: center;
  gap: 14px;
  color: var(--text-3);
  font-size: 11.5px;
}

.pc-foot span {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}

.pc-alert {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 9px;
  border-radius: 8px;
  background: rgb(251 191 36 / 10%);
  color: var(--warn);
  font-size: 11.5px;
  line-height: 1.4;
}

/* 详情 */
.patent-detail {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.patent-overview {
  padding: 20px 22px;
}

.ov-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  justify-content: space-between;
}

.ov-tags {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.ov-title {
  margin: 14px 0 0;
  font-family: var(--font-display);
  font-size: 19px;
  font-weight: 900;
  line-height: 1.4;
}

.ov-number {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 7px 0 0;
  color: var(--text-2);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12.5px;
}

.ov-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 18px;
}

.ov-item {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: 13px 15px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--panel-2);
}

.ov-item.wide {
  grid-column: 1 / -1;
}

.ov-label {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-3);
  font-size: 11.5px;
}

.ov-label svg {
  color: var(--accent);
}

.ov-item b {
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
}

.ov-item b.ov-text {
  font-family: 'Noto Sans SC', sans-serif;
  font-size: 13px;
  font-weight: 500;
  line-height: 1.6;
}

.ov-item b.warn { color: var(--warn); }
.ov-item b.danger { color: var(--danger); }

.ov-item small {
  color: var(--text-3);
  font-size: 11px;
}

.ov-alerts {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 16px;
  padding: 12px 14px;
  border: 1px solid rgb(251 191 36 / 35%);
  border-radius: 12px;
  background: rgb(251 191 36 / 8%);
}

.ov-alerts p {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0;
  color: var(--warn);
  font-size: 12.5px;
}

.ov-notes {
  margin-top: 16px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--panel-2);
}

.ov-notes p {
  margin: 7px 0 0;
  color: var(--text-2);
  font-size: 12.5px;
  line-height: 1.7;
  white-space: pre-line;
}

/* 表单 */
.patent-form {
  padding: 4px 0 18px;
}

.form-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 16px 22px 4px;
}

.form-section h4 {
  margin: 0 0 12px;
  color: var(--text-2);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.4px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 7px;
  min-width: 0;
}

.field.wide {
  grid-column: 1 / -1;
}

.field > span {
  color: var(--text-2);
  font-size: 12px;
}

.field input,
.field select,
.field textarea {
  width: 100%;
  padding: 9px 11px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  color: var(--text-1);
  font: inherit;
  font-size: 12.5px;
  min-width: 0;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.field textarea {
  resize: vertical;
  line-height: 1.6;
}

.field input:focus,
.field select:focus,
.field textarea:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
  outline: none;
}

.form-note {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 0;
  padding: 11px 13px;
  border: 1px dashed var(--line-strong);
  border-radius: 11px;
  color: var(--text-3);
  font-size: 12px;
  line-height: 1.65;
}

.form-note svg {
  flex: none;
  margin-top: 2px;
  color: var(--accent);
}

.form-actions {
  display: flex;
  gap: 10px;
  padding: 4px 22px 0;
}

/* 历史时间线 */
.patent-history {
  padding: 4px 0 16px;
}

.timeline {
  padding: 12px 22px 0;
}

.tl-item {
  position: relative;
  padding: 0 0 16px 26px;
}

.tl-item::before {
  position: absolute;
  top: 20px;
  bottom: 0;
  left: 6px;
  width: 1.5px;
  background: var(--line-strong);
  content: '';
}

.tl-item:last-child {
  padding-bottom: 0;
}

.tl-item:last-child::before {
  display: none;
}

.tl-item > summary {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  list-style: none;
}

.tl-item > summary::-webkit-details-marker {
  display: none;
}

.tl-dot {
  position: absolute;
  top: 4px;
  left: 0;
  width: 13px;
  height: 13px;
  border: 2.5px solid var(--line-strong);
  border-radius: 50%;
  background: var(--panel);
}

.tl-dot.create { border-color: var(--accent); }
.tl-dot.update { border-color: var(--info); }
.tl-dot.payment { border-color: var(--ok); }
.tl-dot.reminder { border-color: var(--warn); }

.tl-icon {
  display: grid;
  width: 24px;
  height: 24px;
  flex: none;
  place-items: center;
  border-radius: 8px;
  background: var(--panel-2);
  color: var(--text-2);
}

.tl-title {
  font-size: 12.5px;
  font-weight: 600;
}

.tl-meta {
  margin-left: auto;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  white-space: nowrap;
}

.tl-body {
  margin: 8px 0 0 34px;
  padding: 11px 14px;
  border: 1px solid var(--line);
  border-radius: 11px;
  background: var(--panel-2);
}

.tl-body p {
  margin: 0 0 5px;
  color: var(--text-2);
  font-size: 12px;
  line-height: 1.65;
}

.tl-body p:last-child {
  margin-bottom: 0;
}

.history-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 34px 16px;
  color: var(--text-3);
  font-size: 12.5px;
}

/* 空状态 */
.detail-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 60px 24px;
  text-align: center;
}

.empty-mark {
  display: grid;
  width: 68px;
  height: 68px;
  place-items: center;
  border-radius: 20px;
  background: var(--accent-soft);
  color: var(--accent);
}

.detail-empty .t {
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 900;
}

.detail-empty p {
  max-width: 320px;
  margin: 0 0 6px;
  color: var(--text-3);
  font-size: 12.5px;
  line-height: 1.7;
}

@media (max-width: 1080px) {
  .patent-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .patent-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .form-grid,
  .ov-grid {
    grid-template-columns: 1fr;
  }

  .result-count {
    margin-left: 0;
  }
}

/* 详情标签页 */
.patent-detail-head {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: space-between;
  flex-wrap: wrap;
}

.patent-tabs {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px;
  overflow-x: auto;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--panel);
  scrollbar-width: thin;
}

.pt-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 14px;
  border-radius: 9px;
  background: transparent;
  color: var(--text-2);
  font-size: 12.5px;
  white-space: nowrap;
  transition: background 0.2s, color 0.2s;
}

.pt-tab:hover {
  color: var(--text-1);
}

.pt-tab.active {
  background: var(--accent);
  color: var(--accent-ink);
  font-weight: 600;
}

.pt-count {
  display: grid;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  place-items: center;
  border-radius: 99px;
  background: var(--panel-2);
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  font-weight: 700;
}

.pt-tab.active .pt-count {
  background: rgb(255 255 255 / 26%);
  color: var(--accent-ink);
}

.tab-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

/* 缴费管理 */
.pay-summary,
.pay-receipts {
  padding: 18px 22px;
}

.pay-summary .ov-grid {
  margin-top: 12px;
}

.pay-receipts .card-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title-action {
  margin-left: auto;
}

.receipt-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 22px 16px;
  border: 1px dashed var(--line);
  border-radius: 12px;
  color: var(--text-3);
  font-size: 12.5px;
}

.receipt-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.receipt-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 11px;
  background: var(--panel-2);
}

.ri-icon {
  display: grid;
  width: 34px;
  height: 34px;
  flex: none;
  place-items: center;
  border-radius: 9px;
  background: var(--accent-soft);
  color: var(--accent);
}

.ri-main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.ri-main b {
  overflow: hidden;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ri-main small {
  overflow: hidden;
  color: var(--text-3);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ri-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

@media (prefers-reduced-motion: reduce) {
  .stat-card,
  .patent-card,
  .spinner {
    transition: none;
    animation: none;
  }
}
</style>
