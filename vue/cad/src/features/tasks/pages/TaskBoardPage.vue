<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { useTaskStore } from '@/stores/task.store'
import { useUiStore } from '@/stores/ui.store'
import { STATUS } from '@/constants/drawing-status'
import type { DrawingTaskHistoryEntry, DrawingTaskRow } from '@/services/drawing-task.service'
import type { DrawingStatus } from '@/types/domain.types'
import CreateDrawingDialog from '@/features/drawings/components/CreateDrawingDialog.vue'
import AssigneeDialog from '../components/AssigneeDialog.vue'
import {
  assigneeLabel,
  defaultTaskBoardFilter,
  dueDateLabel,
  isOverdue,
  progressTone,
  rowNextStep,
  summaryHeadline,
  type TaskBoardFilter,
} from '../task.helpers'

defineOptions({ name: 'TaskBoardPage' })

const router = useRouter()
const taskStore = useTaskStore()
const uiStore = useUiStore()

const filter = ref<TaskBoardFilter>(defaultTaskBoardFilter())
const page = ref(1)
const filterOpen = ref(false)
const createOpen = ref(false)
const activeRow = ref<DrawingTaskRow | null>(null)
const history = ref<DrawingTaskHistoryEntry[]>([])
const loadingHistory = ref(false)
const dialogError = ref('')
const busy = ref(false)
let disposed = false
let requestGeneration = 0

const rows = computed(() => taskStore.boardRows)
const summary = computed(() => taskStore.boardSummary)
const pageCount = computed(() => Math.max(1, Math.ceil((taskStore.boardPage?.total ?? 0) / taskStore.taskBoardPageSize)))
const headline = computed(() => summaryHeadline(summary.value))
const hasFilter = computed(() => Boolean(filter.value.keyword.trim() || filter.value.status || filter.value.assigned))

const stats = computed(() => [
  { key: 'total', label: '图纸总数', value: summary.value.total, icon: 'folder-tree', tone: '' },
  { key: 'unassigned', label: '待指派', value: summary.value.unassigned, icon: 'user-plus', tone: 'attention' },
  { key: 'assigned', label: '已指派', value: summary.value.assigned, icon: 'user-check', tone: '' },
  { key: 'active', label: '进行中', value: summary.value.active, icon: 'pencil', tone: '' },
  { key: 'done', label: '已完成', value: summary.value.done, icon: 'check-circle-2', tone: 'positive' },
  { key: 'overdue', label: '已逾期', value: summary.value.overdue, icon: 'alert-triangle', tone: 'attention' },
])

const statusOptions = computed(() => (Object.entries(STATUS) as Array<[DrawingStatus, { t: string }]>)
  .map(([value, meta]) => ({ value, label: meta.t })))

async function load(targetPage = page.value) {
  const current = ++requestGeneration
  try {
    await taskStore.loadBoard(filter.value, targetPage)
    if (disposed || current !== requestGeneration) return
    page.value = taskStore.boardPage?.page ?? targetPage
  } catch {
    // 错误信息由 store 保存，页面只负责展示与重试入口。
  }
}

function applyFilter() {
  filterOpen.value = false
  void load(1)
}

function showStat(key: string) {
  if (key === 'unassigned') filter.value.assigned = 'unassigned'
  else if (key === 'assigned') filter.value.assigned = 'assigned'
  else if (key === 'done') filter.value.status = 'archived'
  else if (key === 'active') filter.value.assigned = 'assigned'
  else { filter.value.assigned = ''; filter.value.status = '' }
  if (key === 'total') { filter.value.status = ''; filter.value.assigned = ''; filter.value.keyword = '' }
  void load(1)
}

function clearFilter() {
  filter.value = defaultTaskBoardFilter()
  void load(1)
}

async function openAssign(row: DrawingTaskRow) {
  activeRow.value = row
  dialogError.value = ''
  history.value = []
  loadingHistory.value = true
  try {
    await taskStore.loadCandidates()
  } catch (error) {
    dialogError.value = error instanceof Error ? error.message : '候选人员加载失败'
  }
  try {
    history.value = await taskStore.loadHistory(row.drawing.id)
  } catch {
    // 指派历史读取失败不阻塞指派本身，界面按「暂无记录」处理。
    history.value = []
  } finally {
    if (!disposed) loadingHistory.value = false
  }
}

function closeDialog() {
  if (busy.value) return
  activeRow.value = null
  dialogError.value = ''
  history.value = []
}

async function submitAssignment(payload: { assigneeId: string; note: string; dueDate: string; reason: string }) {
  const row = activeRow.value
  if (!row || busy.value) return
  busy.value = true
  dialogError.value = ''
  try {
    if (row.assignment) {
      await taskStore.reassign(row.assignment.taskId, payload)
      uiStore.toast(`已把「${row.drawing.no}」改派给新负责人`, 'ok')
    } else {
      await taskStore.assign({ drawingId: row.drawing.id, assigneeId: payload.assigneeId, note: payload.note, dueDate: payload.dueDate })
      uiStore.toast(`已指派「${row.drawing.no}」的负责人`, 'ok')
    }
    activeRow.value = null
    await taskStore.refreshAfterWrite(filter.value, page.value)
  } catch (error) {
    dialogError.value = error instanceof Error ? error.message : '指派未完成，请重试'
  } finally {
    busy.value = false
  }
}

function cancelAssignment(row: DrawingTaskRow) {
  const taskId = row.assignment?.taskId
  if (!taskId) return
  uiStore.askConfirm(
    '取消指派',
    `取消后「${row.drawing.no}」回到未指派状态，控制权回落给创建人，${row.assignment?.assignee} 会收到通知。`,
    '取消指派',
  ).then(async (accepted) => {
    if (!accepted) return
    try {
      await taskStore.cancel(taskId, '计划员取消指派')
      uiStore.toast('已取消指派', 'ok')
      await taskStore.refreshAfterWrite(filter.value, page.value)
    } catch (error) {
      uiStore.toast(error instanceof Error ? error.message : '取消指派失败', 'warn')
    }
  })
}

function openDrawing(row: DrawingTaskRow) {
  void router.push({ name: RouteName.DrawingPreview, params: { drawingId: row.drawing.no } })
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value) return
  void load(next)
}

function dateText(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

watch(pageCount, (value) => { if (page.value > value) void load(value) })
onMounted(() => { void load(1) })
onBeforeUnmount(() => { disposed = true; requestGeneration++ })
</script>

<template>
  <div class="page task-board-page">
    <header class="task-board-head">
      <div>
        <h1>任务管理台</h1>
        <p class="task-board-sub">{{ headline }}</p>
      </div>
      <div class="task-board-actions">
        <button class="btn" type="button" :disabled="taskStore.loadingBoard" @click="load(page)">
          <DemoIcon name="refresh-cw" :size="14" />刷新
        </button>
        <button class="btn" type="button" @click="createOpen = true">
          <DemoIcon name="plus" :size="14" />创建图纸
        </button>
      </div>
    </header>

    <section class="task-stats" aria-label="任务概览">
      <button
        v-for="item in stats"
        :key="item.key"
        class="task-stat"
        :class="item.tone"
        type="button"
        @click="showStat(item.key)"
      >
        <span class="task-stat-label"><DemoIcon :name="item.icon" :size="16" />{{ item.label }}</span>
        <span class="task-stat-value">{{ taskStore.loadingBoard && !summary.total ? '—' : item.value }}</span>
      </button>
    </section>

    <section class="card task-board-card">
      <div class="task-toolbar">
        <label class="task-search">
          <DemoIcon name="search" :size="15" />
          <input
            v-model="filter.keyword"
            placeholder="图号、图纸名称或项目号"
            aria-label="搜索图纸"
            @keyup.enter="applyFilter"
          />
          <button v-if="filter.keyword" type="button" aria-label="清空关键词" @click="filter.keyword = ''; applyFilter()">
            <DemoIcon name="x" :size="13" />
          </button>
        </label>
        <button class="btn" type="button" @click="filterOpen = !filterOpen">
          <DemoIcon name="filter" :size="14" />筛选
          <span v-if="filter.status || filter.assigned" class="task-filter-dot" aria-hidden="true"></span>
        </button>
        <button class="btn" type="button" @click="applyFilter">查询</button>
        <button v-if="hasFilter" class="btn" type="button" @click="clearFilter">重置</button>
      </div>

      <div v-if="filterOpen" class="task-filter-panel">
        <label>
          <span>图纸状态</span>
          <select v-model="filter.status" class="inp">
            <option value="">全部状态</option>
            <option v-for="option in statusOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
        </label>
        <label>
          <span>指派情况</span>
          <select v-model="filter.assigned" class="inp">
            <option value="">全部</option>
            <option value="unassigned">待指派</option>
            <option value="assigned">已指派</option>
          </select>
        </label>
        <div class="task-filter-actions">
          <button class="btn primary" type="button" @click="applyFilter">应用筛选</button>
        </div>
      </div>

      <div v-if="taskStore.errorBoard" class="task-state error" role="alert">
        <DemoIcon name="alert-circle" :size="24" />
        <strong>任务总表加载失败</strong>
        <p>{{ taskStore.errorBoard }}</p>
        <button class="btn" type="button" @click="load(page)">重新加载</button>
      </div>
      <div v-else-if="taskStore.loadingBoard && !rows.length" class="task-state" role="status">
        <DemoIcon name="loader" :size="24" class="task-spinning" />
        <p>正在加载任务总表…</p>
      </div>
      <div v-else-if="!rows.length" class="task-state">
        <DemoIcon :name="hasFilter ? 'search-x' : 'clipboard-list'" :size="30" />
        <strong>{{ hasFilter ? '没有符合条件的图纸' : '还没有需要指派的图纸' }}</strong>
        <p>{{ hasFilter ? '调整关键词或筛选条件后重新查询。' : '先用「创建图纸」建立图纸档案，再在这里指派负责人。' }}</p>
        <button v-if="hasFilter" class="btn" type="button" @click="clearFilter">清空筛选</button>
        <button v-else class="btn primary" type="button" @click="createOpen = true">创建图纸</button>
      </div>

      <div v-else class="task-table-wrap">
        <table class="tbl task-table">
          <thead>
            <tr>
              <th style="min-width: 220px;">图纸 / 项目</th>
              <th style="width: 96px;">状态</th>
              <th style="min-width: 190px;">进度</th>
              <th style="min-width: 200px;">负责人</th>
              <th style="width: 130px;">截止日期</th>
              <th style="width: 220px;">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in rows" :key="row.drawing.id">
              <td>
                <button class="task-drawing" type="button" @click="openDrawing(row)">
                  <strong>{{ row.drawing.name || row.drawing.no }}</strong>
                  <small>{{ row.drawing.no }}<template v-if="row.drawing.project && row.drawing.project !== row.drawing.no"> · {{ row.drawing.project }}</template></small>
                </button>
              </td>
              <td><span class="tag" :class="row.drawing.status === 'published' ? 'ok' : 'plain'">{{ STATUS[row.drawing.status].t }}</span></td>
              <td>
                <div class="task-progress" :class="progressTone(row)">
                  <div class="task-progress-bar" aria-hidden="true"><span :style="{ width: `${row.progress.percent}%` }" /></div>
                  <div class="task-progress-text">
                    <b>{{ row.progress.stage }}</b><span>{{ row.progress.percent }}%</span>
                  </div>
                  <small :title="rowNextStep(row)">{{ rowNextStep(row) }}</small>
                </div>
              </td>
              <td>
                <span v-if="row.assignment" class="task-assignee">
                  <b>{{ assigneeLabel(row) }}</b>
                  <small v-if="row.assignment.note" :title="row.assignment.note">{{ row.assignment.note }}</small>
                  <small v-if="row.assignment.assignedByName" class="task-assignee-meta">由 {{ row.assignment.assignedByName }} 指派 · {{ dateText(row.assignment.assignedAt) }}</small>
                </span>
                <span v-else class="task-unassigned">待指派</span>
              </td>
              <td>
                <span v-if="row.assignment?.dueDate" class="task-due" :class="{ overdue: isOverdue(row.assignment.dueDate) }">
                  {{ row.assignment.dueDate }}<small>{{ dueDateLabel(row.assignment.dueDate) }}</small>
                </span>
                <span v-else class="task-due muted">未设截止日期</span>
              </td>
              <td class="task-row-actions">
                <button class="btn sm" :class="{ primary: !row.assignment }" type="button" @click="openAssign(row)">
                  <DemoIcon :name="row.assignment ? 'rotate-cw' : 'user-plus'" :size="13" />{{ row.assignment ? '改派' : '指派' }}
                </button>
                <button v-if="row.assignment" class="btn sm" type="button" @click="cancelAssignment(row)">取消指派</button>
                <button class="btn sm" type="button" @click="openDrawing(row)">查看图纸</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <footer v-if="rows.length && !taskStore.errorBoard" class="task-pagination">
        <span>未指派优先展示，其余按截止日期与更新时间排序 · 共 {{ taskStore.boardPage?.total ?? 0 }} 张图纸</span>
        <div>
          <button type="button" :disabled="page <= 1" aria-label="上一页" @click="goPage(page - 1)"><DemoIcon name="chevron-left" :size="16" /></button>
          <span>{{ page }} / {{ pageCount }}</span>
          <button type="button" :disabled="page >= pageCount" aria-label="下一页" @click="goPage(page + 1)"><DemoIcon name="chevron-right" :size="16" /></button>
        </div>
      </footer>
    </section>

    <AssigneeDialog
      :row="activeRow"
      :candidates="taskStore.candidates"
      :history="history"
      :loading-candidates="taskStore.loadingCandidates"
      :loading-history="loadingHistory"
      :busy="busy"
      :error="dialogError"
      @submit="submitAssignment"
      @close="closeDialog"
    />

    <CreateDrawingDialog v-model:open="createOpen" />
  </div>
</template>

<style scoped>
.task-board-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
}
.task-board-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
.task-board-head h1 {
  font-family: var(--font-display);
  font-size: 18px;
  font-weight: 800;
}
.task-board-sub {
  color: var(--text-3);
  font-size: 12px;
  margin-top: 3px;
}
.task-board-actions {
  display: flex;
  gap: 8px;
}
.task-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 10px;
}
.task-stat {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 11px 13px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface-1);
  text-align: left;
}
.task-stat:hover {
  border-color: var(--accent, #2563eb);
}
.task-stat-label {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-3);
  font-size: 11.5px;
}
.task-stat-value {
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 800;
  color: var(--text-1);
}
.task-stat.attention .task-stat-value {
  color: var(--warn, #d97706);
}
.task-stat.positive .task-stat-value {
  color: var(--ok, #16a34a);
}
.task-board-card {
  padding: 0;
  overflow: hidden;
}
.task-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.task-search {
  flex: 1;
  min-width: 220px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-2, transparent);
}
.task-search input {
  flex: 1;
  border: none;
  background: transparent;
  outline: none;
  font-size: 12.5px;
  color: var(--text-1);
}
.task-filter-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent, #2563eb);
}
.task-filter-panel {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.task-filter-panel label {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 11.5px;
  color: var(--text-3);
}
.task-filter-actions {
  margin-left: auto;
}
.task-table-wrap {
  overflow-x: auto;
  padding: 0 14px;
}
.task-table td {
  vertical-align: top;
}
.task-drawing {
  display: flex;
  flex-direction: column;
  gap: 2px;
  text-align: left;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-1);
}
.task-drawing strong {
  font-size: 12.5px;
}
.task-drawing small {
  color: var(--text-3);
  font-size: 11.5px;
}
.task-progress {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.task-progress-bar {
  height: 6px;
  border-radius: 999px;
  background: var(--surface-3, rgba(148, 163, 184, 0.25));
  overflow: hidden;
}
.task-progress-bar span {
  display: block;
  height: 100%;
  background: var(--accent, #2563eb);
}
.task-progress.positive .task-progress-bar span {
  background: var(--ok, #16a34a);
}
.task-progress.attention .task-progress-bar span {
  background: var(--warn, #d97706);
}
.task-progress.muted .task-progress-bar span {
  background: var(--text-3);
}
.task-progress-text {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 11.5px;
  color: var(--text-2);
}
.task-progress small {
  color: var(--text-3);
  font-size: 11px;
  line-height: 1.5;
}
.task-assignee {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.task-assignee b {
  font-size: 12.5px;
}
.task-assignee small {
  color: var(--text-3);
  font-size: 11px;
}
.task-assignee-meta {
  font-size: 10.5px;
}
.task-unassigned {
  color: var(--warn, #d97706);
  font-size: 12px;
  font-weight: 700;
}
.task-due {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  color: var(--text-2);
}
.task-due.overdue {
  color: var(--danger, #dc2626);
  font-weight: 700;
}
.task-due.muted {
  color: var(--text-3);
}
.task-due small {
  font-size: 10.5px;
  font-weight: 400;
}
.task-row-actions {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.task-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 40px 16px;
  color: var(--text-3);
  text-align: center;
}
.task-state strong {
  font-size: 13px;
  color: var(--text-2);
}
.task-state p {
  font-size: 11.5px;
  max-width: 460px;
}
.task-state.error {
  color: var(--danger, #dc2626);
}
.task-spinning {
  animation: task-spin 1s linear infinite;
}
@keyframes task-spin {
  to { transform: rotate(360deg); }
}
.task-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 14px;
  border-top: 1px solid var(--border);
  color: var(--text-3);
  font-size: 11.5px;
  flex-wrap: wrap;
}
.task-pagination div {
  display: flex;
  align-items: center;
  gap: 6px;
}
.task-pagination button {
  border: 1px solid var(--border);
  border-radius: 6px;
  background: none;
  padding: 3px 6px;
  cursor: pointer;
  color: var(--text-2);
}
.task-pagination button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
</style>
