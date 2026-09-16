import type { DrawingTaskRow, DrawingTaskSummary } from '@/services/drawing-task.service'

/** 进度色调：让计划员在表格里一眼区分「还没派」「在做」「做完了」。 */
export type TaskProgressTone = 'idle' | 'attention' | 'positive' | 'muted'

export function progressTone(row: DrawingTaskRow): TaskProgressTone {
  if (row.progress.done) return 'positive'
  // 未指派用弱化色：它不是「出问题了」，而是「还没开始」。
  if (!row.assignment) return 'muted'
  // 已指派但还没有图纸文件属于需要跟进的状态，用警示色优先引起注意。
  if (row.progress.percent <= 0) return 'attention'
  return 'idle'
}

/** 每行必须能回答「下一步点哪里」：未指派就提示指派，已指派就说清编制阶段。 */
export function rowNextStep(row: DrawingTaskRow): string {
  if (!row.assignment) return '尚未指派负责人，请选择人员后指派。'
  return row.progress.detail
}

export function emptySummary(): DrawingTaskSummary {
  return { total: 0, assigned: 0, unassigned: 0, active: 0, done: 0, overdue: 0 }
}

/** 截止日期是否已过期（按本地日期比较，不比较时间，避免当天被误判为逾期）。 */
export function isOverdue(dueDate: string | undefined, now: Date = new Date()): boolean {
  if (!dueDate) return false
  const due = new Date(`${dueDate}T00:00:00`)
  if (Number.isNaN(due.getTime())) return false
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  return due.getTime() < today.getTime()
}

/** 逾期天数；未逾期或没有截止日期时返回 0。 */
export function overdueDays(dueDate: string | undefined, now: Date = new Date()): number {
  if (!isOverdue(dueDate, now) || !dueDate) return 0
  const due = new Date(`${dueDate}T00:00:00`)
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  return Math.round((today.getTime() - due.getTime()) / 86_400_000)
}

export function dueDateLabel(dueDate: string | undefined, now: Date = new Date()): string {
  if (!dueDate) return '未设截止日期'
  const days = overdueDays(dueDate, now)
  if (days > 0) return `已逾期 ${days} 天`
  const due = new Date(`${dueDate}T00:00:00`)
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const remaining = Math.round((due.getTime() - today.getTime()) / 86_400_000)
  if (remaining === 0) return '今天到期'
  if (remaining === 1) return '明天到期'
  return `还有 ${remaining} 天`
}

export interface TaskBoardFilter {
  keyword: string
  status: '' | 'draft' | 'reviewing' | 'published' | 'archived' | 'disabled'
  assigned: '' | 'assigned' | 'unassigned'
}

export function defaultTaskBoardFilter(): TaskBoardFilter {
  return { keyword: '', status: '', assigned: '' }
}

/** 情况摘要：管理台顶部一行字说清「总共多少、还有多少没派」。 */
export function summaryHeadline(summary: DrawingTaskSummary): string {
  if (!summary.total) return '还没有图纸需要指派负责人。'
  if (!summary.unassigned) return `共 ${summary.total} 张图纸，全部已指派负责人。`
  return `共 ${summary.total} 张图纸，还有 ${summary.unassigned} 张待指派负责人。`
}

/** 已指派任务的负责人显示名，未指派时给出显式占位而不是空单元格。 */
export function assigneeLabel(row: DrawingTaskRow): string {
  return row.assignment?.assignee || '待指派'
}

export function roleLabels(roles: readonly string[]): string {
  const labels: Record<string, string> = {
    admin: '管理员',
    planner: '计划员',
    designer: '设计人员',
    reviewer: '审核人员',
  }
  return roles.map((role) => labels[role] ?? role).join(' / ')
}
