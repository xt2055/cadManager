<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName, type RouteNameKey } from '@/router/route-names'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useReviewStore } from '@/stores/review.store'
import { useSystemStatusStore } from '@/stores/system-status.store'
import { listAdminOperationLogs } from '@/services/drawing-operation-log.service'
import { ACTIVITY_LABELS, type ActivityLog, type DrawingStatus } from '@/types/domain.types'
import { STATUS } from '@/constants/drawing-status'
import { useTaskStore } from '@/stores/task.store'
import { availableWorkspaces, workspaceDataSources, workspaceDrawings, workspaceLabels, pendingWorkspaceReviews, completedWorkspaceReviews, type WorkspaceRole } from '../dashboard.helpers'
import { defaultTaskBoardFilter, isOverdue, progressTone } from '@/features/tasks/task.helpers'
import MyTaskPanel from '../components/MyTaskPanel.vue'
import '../dashboard.css'

defineOptions({ name: 'DashboardPage' })
const router = useRouter()
const auth = useAuthStore()
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const systemStore = useSystemStatusStore()
const taskStore = useTaskStore()
const selectedWorkspace = ref<WorkspaceRole | null>(null)
const activePanel = ref<'documents' | 'activity'>('documents')
const workspaces = computed(() => availableWorkspaces(auth.currentUser?.roles ?? []))
const workspace = computed(() => selectedWorkspace.value && workspaces.value.includes(selectedWorkspace.value) ? selectedWorkspace.value : workspaces.value[0] ?? null)
const keyword = ref('')
const drawingFilter = ref<DrawingStatus | ''>('')
const reviewFilter = ref<'pending' | 'completed'>('pending')
const reviewResult = ref<'' | 'pass' | 'rejected'>('')
const page = ref(1)
const pageSize = computed(() => workspace.value === 'admin' ? 4 : 5)
const refreshing = ref(false)
const dataError = ref('')
const auditError = ref('')
const taskError = ref('')
const activity = ref<ActivityLog[]>([])
let requestGeneration = 0
let disposed = false

const identity = computed(() => auth.currentUser?.displayName || auth.currentUser?.account || '')
// 顶栏问候卡：时段问候取代「工作台」，姓名取自当前账号，右侧一枚微型英文章戳。
// 工作台是常驻窗口，每分钟自检一次时钟，跨时段后问候语与日期跟着换，避免晚上还写着「晨安」。
const clock = ref(new Date())
const clockTimer = window.setInterval(() => { clock.value = new Date() }, 60_000)
const greeting = computed(() => { const hour = clock.value.getHours(); return hour >= 5 && hour < 12 ? '晨安' : hour < 18 ? '午安' : '夜安' })
const salutation = computed(() => identity.value ? `${greeting.value}，${identity.value}。` : `${greeting.value}。`)
// 章戳跟着工作视图走，只作身份标注，不抢问候语。
const ROLE_STAMPS: Record<WorkspaceRole, string> = { planner: 'CHIEF DESIGNER', designer: 'STRUCTURAL ENG.', reviewer: 'REVIEW OFFICE', admin: 'SYSTEM ADMIN' }
const stamp = computed(() => workspace.value ? ROLE_STAMPS[workspace.value] : '')
// 日期做成腕表副表盘：上一行等宽大写（09 / 17 · THU），下一行小字标注地点与年份。
const dateStamp = computed(() => {
  const now = clock.value
  const pad = (value: number) => String(value).padStart(2, '0')
  const weekday = new Intl.DateTimeFormat('en-US', { weekday: 'short' }).format(now).toUpperCase()
  return { main: `${pad(now.getMonth() + 1)} / ${pad(now.getDate())} · ${weekday}`, sub: `SHANGHAI · ${now.getFullYear()}` }
})
// 英文副表盘对读屏不友好，补一条完整的中文日期。
const dateLabel = computed(() => new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' }).format(clock.value))
const drawings = computed(() => workspaceDrawings(drawingStore.drawings, auth.currentUser, workspace.value))
const pending = computed(() => pendingWorkspaceReviews(reviewStore.cases, auth.currentUser))
const completed = computed(() => completedWorkspaceReviews(reviewStore.completed, auth.currentUser))
const sourceLoading = computed(() => workspace.value === 'reviewer' ? reviewStore.loading : drawingStore.loading)
const draftCount = computed(() => drawings.value.filter(item => item.status === 'draft').length)
const reviewingCount = computed(() => drawings.value.filter(item => item.status === 'reviewing').length)
const publishedCount = computed(() => drawings.value.filter(item => item.status === 'published').length)
const primaryAction = computed(() => {
  if (workspace.value === 'admin') return { label: '后台管理', icon: 'shield', route: RouteName.AdminAccounts }
  if (workspace.value === 'planner') return { label: '任务管理台', icon: 'clipboard-list', route: RouteName.TaskBoard }
  if (workspace.value === 'reviewer') return { label: '进入审核中心', icon: 'clipboard-check', route: RouteName.ReviewPending }
  // 设计人员没有建档权，主操作指向图纸库（那里能找到自己被指派和参与过的图纸）。
  return { label: '调阅图纸资产', icon: 'folder-open', route: RouteName.DrawingLibrary }
})
const shortcuts = computed<Array<{ label: string; description: string; icon: string; route: RouteNameKey }>>(() => {
  if (workspace.value === 'admin') return [
    { label: '账号与权限', description: '成员与角色', icon: 'users', route: RouteName.AdminAccounts },
    { label: '审核流程', description: '节点与签署规则', icon: 'workflow', route: RouteName.AdminReviewFlows },
    { label: '图纸管控', description: '发布与生命周期', icon: 'folder-lock', route: RouteName.AdminDrawings },
    { label: '变更审批', description: '工单批准与验收', icon: 'clipboard-check', route: RouteName.AdminChangeApprovals },
    { label: '变更记录', description: '工单留痕与追溯', icon: 'history', route: RouteName.AdminChangeRecords },
    { label: '格式转换', description: '转换任务与异常', icon: 'refresh-cw', route: RouteName.AdminConversions },
    { label: '操作审计', description: '操作记录追溯', icon: 'history', route: RouteName.AdminLogs },
    { label: '系统日志', description: '服务诊断', icon: 'file-terminal', route: RouteName.AdminSystemLogs },
    { label: '零件索引', description: '按图号与材料检索', icon: 'box', route: RouteName.PartIndexLibrary },
  ]
  if (workspace.value === 'planner') return [
    { label: '任务管理台', description: '指派与改派负责人', icon: 'clipboard-list', route: RouteName.TaskBoard },
    { label: '创建图纸', description: '建档、导入老图、分叉', icon: 'plus', route: RouteName.DrawingCreate },
    { label: '图纸库', description: '项目与工程图纸', icon: 'folder-open', route: RouteName.DrawingLibrary },
    { label: '零件索引', description: '按图号与材料检索', icon: 'box', route: RouteName.PartIndexLibrary },
    { label: '操作记录', description: '指派与编制留痕', icon: 'history', route: RouteName.OperationLogs },
  ]
  return [
    { label: '图纸库', description: '项目与工程图纸', icon: 'folder-open', route: RouteName.DrawingLibrary },
    { label: '零件索引', description: '按图号与材料检索', icon: 'box', route: RouteName.PartIndexLibrary },
    workspace.value === 'reviewer'
      ? { label: '已办审核', description: '签署意见与结论', icon: 'stamp', route: RouteName.ReviewCompleted }
      : { label: '操作记录', description: '图纸操作追溯', icon: 'history', route: RouteName.OperationLogs },
  ]
})
// 状态筛选：卡片本身就是 Tab。
// 原先「上方 4 张统计卡片 + 列表上方一行 Tab」是同一批信息的两次呈现，合并成一行后
// 既省掉整条 72px 卡片带，也不再出现「看数字在一处、点筛选在另一处」的割裂。
type StatusTabTone = 'draft' | 'reviewing' | 'published' | 'danger' | 'muted'
interface StatusTab { key: string; label: string; icon: string; tone: StatusTabTone; count: number }
const archivedCount = computed(() => drawings.value.filter(item => item.status === 'archived').length)
const disabledCount = computed(() => drawings.value.filter(item => item.status === 'disabled').length)
const passCount = computed(() => completed.value.filter(item => item.result === 'pass').length)
const rejectedCount = computed(() => completed.value.filter(item => item.result === 'rejected').length)

// 今日待办摘要：先一句定调，再报能落地的数字；为 0 的项不写出来（「待完善图稿 0 份」是噪声）。
// 加载中一律显示「—」，不拿 0 冒充数据：加载中／加载失败／无匹配／确实没有数据是四种状态。
interface BriefItem { label: string; count: number; unit: string }
const BRIEF_LEADS: Record<WorkspaceRole, string> = { planner: '今日排程已就绪。', designer: '今日构想已就绪。', reviewer: '今日校审已就绪。', admin: '今日总览已就绪。' }
const briefItems = computed<BriefItem[]>(() => {
  const items: BriefItem[] = []
  const push = (label: string, count: number, unit: string) => { if (count > 0) items.push({ label, count, unit }) }
  if (workspace.value === 'planner') {
    push('待指派', taskStore.boardSummary.unassigned, '张')
    push('进行中', taskStore.boardSummary.active, '张')
    push('已逾期', taskStore.boardSummary.overdue, '张')
  } else if (workspace.value === 'reviewer') {
    push('待我审核', pending.value.length, '项')
    push('已签署', completed.value.length, '项')
  } else if (workspace.value === 'admin') {
    push('审核中', reviewingCount.value, '份')
    push('生产中', publishedCount.value, '份')
    push('已停用', disabledCount.value, '份')
  } else if (workspace.value === 'designer') {
    push('待完善图稿', draftCount.value, '份')
    push('审核中', reviewingCount.value, '份')
    push('校审待批', pending.value.length, '项')
  }
  return items
})
const briefLead = computed(() => workspace.value ? BRIEF_LEADS[workspace.value] : '当前账号尚未分配工作角色。')
const briefLoading = computed(() => sourceLoading.value && !dataError.value)
// 没有工作视图时「尚未分配角色」本身就是全部信息，不再叠一句「今日无待办」。
const briefFallback = computed(() => !workspace.value ? '' : briefLoading.value ? '正在核对今日待办…' : '当前没有待办，图稿状态均已就绪。')
// 计划工作台只看「待指派」时用本地筛选；页脚会说明这是当前页，完整筛选在任务管理台。
const plannedOnly = ref<'all' | 'unassigned'>('all')
// 计划工作台的主列表只是任务总表的第一页预览，因此卡片计数只统计这一页真实存在的行，
// 保证「卡片上的数字 = 点下去看到的行数」；全局的待指派/已指派/进行中/已完成仍在右侧「指派进度」里。
const statusTabs = computed<StatusTab[]>(() => {
  if (workspace.value === 'reviewer') return [
    { key: 'pending', label: '待我审核', icon: 'clipboard-check', tone: 'reviewing', count: pending.value.length },
    { key: 'completed', label: '我的已办', icon: 'stamp', tone: 'muted', count: completed.value.length },
    { key: 'pass', label: '已通过', icon: 'check-circle-2', tone: 'published', count: passCount.value },
    { key: 'rejected', label: '已驳回', icon: 'corner-up-left', tone: 'danger', count: rejectedCount.value },
  ]
  if (workspace.value === 'planner') {
    const rows = taskStore.boardRows
    const byStatus = (status: DrawingStatus) => rows.filter(row => row.drawing.status === status).length
    return [
      { key: 'unassigned', label: '待指派', icon: 'user-plus', tone: 'draft', count: rows.filter(row => !row.assignment).length },
      { key: 'all', label: '全部图纸', icon: 'folder-tree', tone: 'muted', count: rows.length },
      { key: 'draft', label: '草稿', icon: 'pencil', tone: 'draft', count: byStatus('draft') },
      { key: 'reviewing', label: '审核中', icon: 'workflow', tone: 'reviewing', count: byStatus('reviewing') },
      { key: 'published', label: '生产中', icon: 'layers', tone: 'published', count: byStatus('published') },
    ]
  }
  if (workspace.value === 'admin') return [
    { key: 'all', label: '全部图纸', icon: 'folder-tree', tone: 'muted', count: drawings.value.length },
    { key: 'reviewing', label: '审核中', icon: 'workflow', tone: 'reviewing', count: reviewingCount.value },
    { key: 'published', label: '生产中', icon: 'layers', tone: 'published', count: publishedCount.value },
    { key: 'archived', label: '已存档', icon: 'archive', tone: 'muted', count: archivedCount.value },
    { key: 'disabled', label: '已停用', icon: 'circle-slash', tone: 'danger', count: disabledCount.value },
  ]
  return [
    { key: 'all', label: '全部图纸', icon: 'folder-tree', tone: 'muted', count: drawings.value.length },
    { key: 'draft', label: '待完善草稿', icon: 'pencil', tone: 'draft', count: draftCount.value },
    { key: 'reviewing', label: '审核中', icon: 'workflow', tone: 'reviewing', count: reviewingCount.value },
    { key: 'published', label: '生产中', icon: 'layers', tone: 'published', count: publishedCount.value },
    { key: 'archived', label: '已存档', icon: 'archive', tone: 'muted', count: archivedCount.value },
    { key: 'disabled', label: '已停用', icon: 'circle-slash', tone: 'danger', count: disabledCount.value },
  ]
})
const activeStatusKey = computed(() => {
  if (workspace.value === 'reviewer') return reviewResult.value || reviewFilter.value
  if (workspace.value === 'planner') return plannedOnly.value === 'unassigned' ? 'unassigned' : drawingFilter.value || 'all'
  return drawingFilter.value || 'all'
})
function selectStatusTab(key: string) {
  if (workspace.value === 'reviewer') {
    if (key === 'pending') { reviewFilter.value = 'pending'; reviewResult.value = ''; return }
    if (key === 'completed') { reviewFilter.value = 'completed'; reviewResult.value = ''; return }
    reviewFilter.value = 'completed'
    reviewResult.value = key === 'pass' ? 'pass' : 'rejected'
    return
  }
  if (workspace.value === 'planner') {
    plannedOnly.value = key === 'unassigned' ? 'unassigned' : 'all'
    drawingFilter.value = key === 'unassigned' || key === 'all' ? '' : key as DrawingStatus
    return
  }
  drawingFilter.value = key === 'all' ? '' : key as DrawingStatus
}
const filteredDrawings = computed(() => {
  const search = keyword.value.trim().toLowerCase()
  return drawings.value.filter(item => (!drawingFilter.value || item.status === drawingFilter.value) && (!search || [item.no, item.name, item.project].some(value => value.toLowerCase().includes(search))))
})
// 计划工作台的主列表不是「图纸库」，而是「谁在负责哪张图」：
// 未指派排在前面，因为那才是计划员当下要处理的事。
const plannedRows = computed(() => {
  const search = keyword.value.trim().toLowerCase()
  return taskStore.boardRows.filter(row =>
    (plannedOnly.value !== 'unassigned' || !row.assignment) &&
    (!drawingFilter.value || row.drawing.status === drawingFilter.value) &&
    (!search || [row.drawing.no, row.drawing.name, row.drawing.project, row.assignment?.assignee ?? ''].some(value => value.toLowerCase().includes(search))))
})
const reviewRows = computed(() => {
  const search = keyword.value.trim().toLowerCase()
  const items = reviewFilter.value === 'pending'
    ? pending.value.map(item => ({ ...item, result: '', opinion: '' }))
    : completed.value.map(item => ({ ...item, initiator: item.by }))
  return items.filter(item => (reviewFilter.value === 'pending' || !reviewResult.value || item.result === reviewResult.value) && (!search || [item.no, item.name, item.node, item.initiator].some(value => value?.toLowerCase().includes(search))))
})
const total = computed(() => workspace.value === 'reviewer' ? reviewRows.value.length : workspace.value === 'planner' ? plannedRows.value.length : filteredDrawings.value.length)
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const visibleDrawings = computed(() => filteredDrawings.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const visiblePlanRows = computed(() => plannedRows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const visibleReviews = computed(() => reviewRows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
// 主面板的标题与出口按工作视图集中判定：模板里堆多层三元表达式最容易改错一处。
const panelTitle = computed(() => workspace.value === 'reviewer' ? '审核任务' : workspace.value === 'planner' ? '图纸任务' : workspace.value === 'designer' ? '我的设计图纸' : '图纸资产')
const panelIcon = computed(() => workspace.value === 'reviewer' ? 'clipboard-check' : workspace.value === 'planner' ? 'clipboard-list' : 'folder-tree')
const panelRoute = computed<RouteNameKey>(() => workspace.value === 'reviewer' ? RouteName.ReviewPending : workspace.value === 'planner' ? RouteName.TaskBoard : RouteName.DrawingLibrary)
const panelLinkLabel = computed(() => workspace.value === 'reviewer' ? '审核中心' : workspace.value === 'planner' ? '任务管理台' : '打开图纸库')
const storage = computed(() => systemStore.status?.storage)
const serviceReady = computed(() => systemStore.isOnline && systemStore.status?.service.status === 'ok')

function go(route: RouteNameKey) { void router.push({ name: route }) }
function openDrawing(no: string) { void router.push({ name: RouteName.DrawingPreview, params: { drawingId: no } }) }
function openReview(no: string) { void router.push({ name: RouteName.ReviewWorkspace, params: { drawingNo: no } }) }
function clearSearch() { keyword.value = ''; drawingFilter.value = ''; reviewFilter.value = 'pending'; reviewResult.value = '' }
function dateText(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(date)
}
async function loadData(force = false) {
  const sources = workspaceDataSources(workspace.value)
  const current = ++requestGeneration
  refreshing.value = true
  dataError.value = ''; auditError.value = ''; taskError.value = ''
  const tasks: Promise<void>[] = []
  if (sources.drawings) tasks.push((force ? drawingStore.refresh() : drawingStore.load()).catch(cause => {
    if (!disposed && current === requestGeneration) dataError.value = cause instanceof Error ? cause.message : '图纸加载失败'
  }))
  if (sources.reviews) tasks.push(reviewStore.load().catch(cause => {
    if (!disposed && current === requestGeneration) dataError.value = cause instanceof Error ? cause.message : '审核任务加载失败'
  }))
  if (sources.tasks) tasks.push(taskStore.loadBoard(defaultTaskBoardFilter(), 1).catch(cause => {
    if (!disposed && current === requestGeneration) dataError.value = cause instanceof Error ? cause.message : '任务总表加载失败'
  }))
  if (sources.system) tasks.push(systemStore.fetchStatus())
  if (sources.audit) tasks.push(listAdminOperationLogs({ page: 1, pageSize: 5 }).then(result => {
    if (!disposed && current === requestGeneration && workspace.value === 'admin') activity.value = result.list
  }).catch(() => { if (!disposed && current === requestGeneration) auditError.value = '操作记录暂时不可用' }))
  tasks.push(taskStore.loadMine().catch(cause => {
    if (!disposed && current === requestGeneration) taskError.value = cause instanceof Error ? cause.message : '我的任务加载失败'
  }))
  await Promise.allSettled(tasks)
  if (!disposed && current === requestGeneration) refreshing.value = false
}
watch([workspace, () => auth.currentUser?.id], () => {
  activePanel.value = 'documents'
  keyword.value = ''; drawingFilter.value = ''; reviewFilter.value = 'pending'; reviewResult.value = ''; page.value = 1; activity.value = []
  void loadData()
}, { immediate: true })
watch([keyword, drawingFilter, reviewFilter, reviewResult], () => { page.value = 1 })
watch([pageCount, plannedOnly], () => { if (page.value > pageCount.value) page.value = pageCount.value })
onBeforeUnmount(() => { disposed = true; requestGeneration++; window.clearInterval(clockTimer) })
</script>

<template>
  <div class="page cad-workbench">
    <header class="wb-header">
      <div class="wb-heading"><div class="wb-heading-title"><h1>{{ salutation }}</h1><span v-if="stamp" class="wb-stamp">{{ stamp }}</span></div><p class="wb-brief"><span>{{ briefLead }}</span><template v-if="briefItems.length"><span v-for="(item, index) in briefItems" :key="item.label"><span v-if="index" class="wb-brief-sep" aria-hidden="true">·</span>{{ item.label }} <b class="wb-brief-num">{{ briefLoading ? '—' : item.count }}</b> {{ item.unit }}</span></template><span v-else>{{ briefFallback }}</span></p></div>
      <div v-if="workspaces.length > 1" class="wb-role-switch" role="group" aria-label="切换工作视图"><button v-for="role in workspaces" :key="role" type="button" :aria-pressed="workspace === role" :class="{ active: workspace === role }" @click="selectedWorkspace = role">{{ workspaceLabels[role] }}</button></div>
      <div class="wb-header-actions"><span class="wb-date"><span class="wb-sr-only">{{ dateLabel }}</span><span class="wb-date-main" aria-hidden="true">{{ dateStamp.main }}</span><small aria-hidden="true">{{ dateStamp.sub }}</small></span><button class="btn wb-refresh" type="button" :disabled="refreshing" aria-label="刷新工作台" @click="loadData(true)"><DemoIcon name="refresh-cw" :size="16" :class="{ 'wb-spinning': refreshing }" /></button><button v-if="workspace" class="btn wb-primary" type="button" @click="go(primaryAction.route)"><DemoIcon :name="primaryAction.icon" :size="16" />{{ primaryAction.label }}</button></div>
    </header>
    <template v-if="workspace">
      <div class="wb-layout">
        <main class="wb-main">
          <div v-if="workspace === 'admin'" class="wb-view-switch" role="group" aria-label="切换工作内容"><button type="button" :class="{ active: activePanel === 'documents' }" :aria-pressed="activePanel === 'documents'" @click="activePanel = 'documents'"><DemoIcon name="folder-tree" :size="16" />图纸资产</button><button type="button" :class="{ active: activePanel === 'activity' }" :aria-pressed="activePanel === 'activity'" @click="activePanel = 'activity'"><DemoIcon name="history" :size="16" />最近操作</button></div>
          <section v-show="activePanel === 'documents'" class="wb-panel wb-documents" :aria-busy="sourceLoading">
            <div class="wb-panel-heading"><h2><DemoIcon :name="panelIcon" :size="17" />{{ panelTitle }}</h2><button class="wb-text-button" type="button" @click="go(panelRoute)">{{ panelLinkLabel }}<DemoIcon name="arrow-up-right" :size="14" /></button></div>
            <div class="wb-document-tools">
              <div class="wb-status-tabs" role="group" aria-label="按状态筛选图纸">
                <button v-for="tab in statusTabs" :key="tab.key" type="button" class="wb-status-tab" :class="[tab.tone, { active: activeStatusKey === tab.key }]" :aria-pressed="activeStatusKey === tab.key" @click="selectStatusTab(tab.key)"><DemoIcon :name="tab.icon" :size="15" /><span class="wb-status-tab-label">{{ tab.label }}</span><b class="wb-status-tab-count">{{ sourceLoading || dataError ? '—' : tab.count }}</b></button>
              </div>
              <label class="wb-search"><DemoIcon name="search" :size="15" /><input v-model="keyword" :placeholder="workspace === 'reviewer' ? '图号、名称、审核节点' : workspace === 'planner' ? '图号、名称、项目号或负责人' : '图号、名称、项目编号'" aria-label="搜索工作台图纸" /><button v-if="keyword" type="button" aria-label="清空搜索" @click="keyword = ''"><DemoIcon name="x" :size="13" /></button></label>
            </div>
            <div v-if="dataError" class="wb-message error" role="alert"><DemoIcon name="alert-circle" :size="20" /><strong>工作数据加载失败</strong><p>{{ dataError }}</p><button class="btn" type="button" :disabled="refreshing" @click="loadData(true)">重新加载</button></div>
            <div v-else-if="sourceLoading && !total" class="wb-message" role="status"><DemoIcon name="loader" :size="24" class="wb-spinning" /><p>正在加载工作数据…</p></div>
            <div v-else-if="!total" class="wb-message"><div class="wb-empty-icon"><DemoIcon :name="keyword ? 'search-x' : workspace === 'reviewer' ? 'clipboard-check' : 'folder-open'" :size="28" /></div><strong>{{ keyword || drawingFilter ? '没有匹配的图纸' : workspace === 'reviewer' ? reviewFilter === 'pending' ? '当前没有分配给你的待审任务' : '还没有签署记录' : workspace === 'designer' ? '还没有我的设计图纸' : '图纸库暂无项目' }}</strong><p>{{ keyword || drawingFilter ? '调整关键词或切换筛选条件。' : workspace === 'reviewer' ? '分配到你的审核任务将在这里显示。' : workspace === 'designer' ? '从新建图纸开始，或在图纸库中查找项目。' : '团队创建的项目与图纸会汇总到这里。' }}</p><button v-if="keyword || drawingFilter" class="btn" type="button" @click="clearSearch">清空筛选</button><button v-else-if="workspace === 'planner'" class="btn primary" type="button" @click="go(RouteName.DrawingCreate)">创建图纸</button><button v-else-if="workspace === 'designer'" class="btn" type="button" @click="go(RouteName.DrawingLibrary)">打开图纸库</button></div>
            <div v-else class="wb-table-area">
              <table v-if="workspace === 'planner'" class="wb-table">
                <thead><tr><th>图纸 / 项目</th><th>状态</th><th>进度</th><th>负责人</th><th>截止日期</th><th><span class="wb-sr-only">操作</span></th></tr></thead>
                <tbody>
                  <tr v-for="row in visiblePlanRows" :key="row.drawing.id">
                    <td><button class="wb-drawing-title" type="button" @click="openDrawing(row.drawing.no)"><span class="wb-file-icon"><DemoIcon name="file" :size="19" /></span><span><strong>{{ row.drawing.name || row.drawing.no }}</strong><small>{{ row.drawing.no }}<template v-if="row.drawing.project && row.drawing.project !== row.drawing.no"> · {{ row.drawing.project }}</template></small></span></button></td>
                    <td><span class="wb-status" :class="row.drawing.status"><i />{{ STATUS[row.drawing.status].t }}</span></td>
                    <td><div class="wb-task-progress" :class="progressTone(row)"><span>{{ row.progress.stage }}</span><b>{{ row.progress.percent }}%</b></div></td>
                    <td><span v-if="row.assignment">{{ row.assignment.assignee }}</span><span v-else class="wb-node">待指派</span></td>
                    <td class="wb-time"><span v-if="row.assignment?.dueDate" :class="{ 'wb-overdue': isOverdue(row.assignment.dueDate) }">{{ row.assignment.dueDate }}</span><span v-else>—</span></td>
                    <td><button class="wb-open" type="button" :aria-label="`打开图纸 ${row.drawing.no}`" @click="openDrawing(row.drawing.no)"><DemoIcon name="chevron-right" :size="17" /></button></td>
                  </tr>
                </tbody>
              </table>
              <table v-else-if="workspace !== 'reviewer'" class="wb-table"><thead><tr><th>图纸 / 项目</th><th>版本</th><th>状态</th><th>文件</th><th>更新于</th><th><span class="wb-sr-only">操作</span></th></tr></thead><tbody><tr v-for="drawing in visibleDrawings" :key="drawing.id || drawing.no"><td><button class="wb-drawing-title" type="button" @click="openDrawing(drawing.no)"><span class="wb-file-icon"><DemoIcon name="file" :size="19" /></span><span><strong>{{ drawing.name || drawing.no }}</strong><small>{{ drawing.no }}<template v-if="drawing.project && drawing.project !== drawing.no"> · {{ drawing.project }}</template></small></span></button></td><td class="wb-mono">{{ drawing.version || '—' }}</td><td><span class="wb-status" :class="drawing.status"><i />{{ STATUS[drawing.status].t }}</span></td><td class="wb-mono">{{ drawing.fileCount }}</td><td class="wb-time">{{ dateText(drawing.updatedAt) }}</td><td><button class="wb-open" type="button" :aria-label="`打开图纸 ${drawing.no}`" @click="openDrawing(drawing.no)"><DemoIcon name="chevron-right" :size="17" /></button></td></tr></tbody></table>
              <table v-else class="wb-table wb-review-table"><thead><tr><th>图纸 / 图号</th><th>审核节点</th><th>{{ reviewFilter === 'pending' ? '发起人' : '结论' }}</th><th>{{ reviewFilter === 'pending' ? '发起时间' : '签署时间' }}</th><th><span class="wb-sr-only">操作</span></th></tr></thead><tbody><tr v-for="review in visibleReviews" :key="review.id"><td><button class="wb-drawing-title" type="button" @click="reviewFilter === 'pending' ? openReview(review.no) : openDrawing(review.no)"><span class="wb-file-icon"><DemoIcon name="file" :size="19" /></span><span><strong>{{ review.name }}</strong><small>{{ review.no }}</small></span></button></td><td><span class="wb-node">{{ review.node }}</span></td><td><span v-if="review.result" class="wb-status" :class="review.result === 'pass' ? 'published' : 'rejected'"><i />{{ review.result === 'pass' ? '通过' : '驳回' }}</span><span v-else>{{ review.initiator || '—' }}</span></td><td class="wb-time">{{ dateText(review.time) }}</td><td><button class="btn sm" :class="{ primary: reviewFilter === 'pending' }" type="button" @click="reviewFilter === 'pending' ? openReview(review.no) : openDrawing(review.no)">{{ reviewFilter === 'pending' ? '进入审核' : '查看' }}</button></td></tr></tbody></table>
            </div>
            <footer v-if="total && !dataError" class="wb-pagination"><span>{{ workspace === 'planner' ? `本页 ${total} 项，未指派优先；完整筛选与分页在任务管理台` : workspace === 'reviewer' && reviewFilter === 'pending' ? '按发起时间排序，优先处理较早任务' : '按最近更新时间排序' }}<template v-if="workspace !== 'planner'"> · 共 {{ total }} 项</template></span><div><button type="button" :disabled="page <= 1" aria-label="上一页" @click="page--"><DemoIcon name="chevron-left" :size="16" /></button><span>{{ page }} / {{ pageCount }}</span><button type="button" :disabled="page >= pageCount" aria-label="下一页" @click="page++"><DemoIcon name="chevron-right" :size="16" /></button></div></footer>
          </section>
          <section v-if="workspace === 'admin'" v-show="activePanel === 'activity'" class="wb-panel wb-audit"><div class="wb-panel-heading"><h2><DemoIcon name="history" :size="17" />最近操作</h2><button class="wb-text-button" type="button" @click="go(RouteName.AdminLogs)">全部记录<DemoIcon name="arrow-up-right" :size="14" /></button></div><p v-if="auditError" class="wb-inline-message" role="alert">{{ auditError }}</p><p v-else-if="!activity.length" class="wb-inline-message">{{ refreshing ? '正在加载操作记录…' : '暂无操作记录' }}</p><ul v-else class="wb-activity"><li v-for="item in activity" :key="item.id"><span class="wb-activity-dot" :class="{ failed: item.result === 'failed' }" /><span><b>{{ item.user }}</b> {{ ACTIVITY_LABELS[item.act] || '操作图纸' }} <button v-if="item.drawingNo" type="button" :title="item.drawingName || item.drawingNo" @click="openDrawing(item.drawingNo)">{{ item.drawingName || item.drawingNo }}</button><em v-if="item.result === 'failed'">未成功</em></span><time>{{ dateText(item.occurredAt || item.time) }}</time></li></ul><footer class="wb-audit-footer">展示最近 5 条操作，完整记录可在操作审计中查看。</footer></section>
        </main>
        <aside class="wb-aside">
          <MyTaskPanel />
          <section class="wb-panel"><div class="wb-panel-heading"><h2><DemoIcon :name="workspace === 'admin' ? 'sliders-horizontal' : 'layers'" :size="17" />{{ workspace === 'admin' ? '管理工具' : '常用工具' }}</h2></div><div class="wb-shortcuts" :class="{ 'admin-tools': workspace === 'admin' }"><button v-for="item in shortcuts" :key="item.route" type="button" @click="go(item.route)"><span class="wb-tool-icon"><DemoIcon :name="item.icon" :size="19" /></span><span><strong>{{ item.label }}</strong><small>{{ item.description }}</small></span><DemoIcon name="chevron-right" :size="14" /></button></div></section>
          <section v-if="workspace === 'admin'" class="wb-panel wb-runtime"><div class="wb-panel-heading"><h2><DemoIcon name="server" :size="17" />运行与存储</h2><span class="wb-status" :class="serviceReady ? 'published' : 'draft'"><i />{{ systemStore.loading ? '检测中' : serviceReady ? '服务在线' : '待检查' }}</span></div><dl class="wb-system"><div><dt>数据库</dt><dd>{{ !systemStore.status ? '未获取' : systemStore.status.database.status === 'ok' ? '连接正常' : '连接异常' }}</dd></div><div><dt>在线用户</dt><dd>{{ systemStore.status ? systemStore.onlineCount : '—' }}<small v-if="systemStore.status"> 人</small></dd></div><div><dt>最近检测</dt><dd>{{ dateText(systemStore.lastChecked?.toISOString()) }}</dd></div></dl><div class="wb-storage"><DemoIcon name="hard-drive" :size="20" /><div><span>数据库文件容量</span><strong>{{ storage ? storage.formattedUsed : '—' }}</strong></div><p>{{ storage ? `${storage.fileCount} 份文件记录` : '暂未获取' }}</p></div><dl class="wb-system wb-storage-facts"><div><dt>统计状态</dt><dd>{{ !storage ? '未获取' : storage.status === 'ok' ? '正常' : '读取异常' }}</dd></div><div><dt>统计来源</dt><dd>数据库记录</dd></div></dl></section>
          <section v-else-if="workspace === 'planner'" class="wb-panel"><div class="wb-panel-heading"><h2><DemoIcon name="workflow" :size="17" />指派进度</h2><button class="wb-text-button" type="button" @click="go(RouteName.TaskBoard)">去处理<DemoIcon name="arrow-up-right" :size="14" /></button></div><dl class="wb-system"><div><dt>待指派</dt><dd>{{ taskStore.boardSummary.unassigned }}<small> 张</small></dd></div><div><dt>已指派</dt><dd>{{ taskStore.boardSummary.assigned }}<small> 张</small></dd></div><div><dt>进行中</dt><dd>{{ taskStore.boardSummary.active }}<small> 张</small></dd></div><div><dt>已完成</dt><dd>{{ taskStore.boardSummary.done }}<small> 张</small></dd></div><div><dt>已逾期</dt><dd :class="{ 'wb-overdue': taskStore.boardSummary.overdue }">{{ taskStore.boardSummary.overdue }}<small> 张</small></dd></div></dl></section>
          <section v-else-if="workspace === 'reviewer'" class="wb-panel"><div class="wb-panel-heading"><h2><DemoIcon name="stamp" :size="17" />最近签署</h2></div><p v-if="dataError" class="wb-inline-message">审核记录暂时不可用</p><p v-else-if="!completed.length" class="wb-inline-message">{{ sourceLoading ? '正在加载…' : '暂无个人签署记录' }}</p><ul v-else class="wb-signatures"><li v-for="item in completed.slice(0, 3)" :key="item.id"><button type="button" @click="openDrawing(item.no)">{{ item.name || item.no }}</button><div><span class="wb-status" :class="item.result === 'pass' ? 'published' : 'rejected'">{{ item.result === 'pass' ? '通过' : '驳回' }} · {{ item.node }}</span><time>{{ dateText(item.time) }}</time></div><p v-if="item.opinion">{{ item.opinion }}</p></li></ul></section>
        </aside>
      </div>
    </template>
  </div>
</template>
