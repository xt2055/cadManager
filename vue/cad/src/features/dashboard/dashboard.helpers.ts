import type { AuthUser } from '../auth/types/auth.types'
import type { DrawingSummaryView } from '../../modules/drawing/drawing-read-model'
import type { ApiReviewCase, ApiCompletedAction } from '../../services/review-case.service'
import type { UserRole } from '../../types/domain.types'
import { isDrawingDecider, isMyTaskDrawing, matchesUser } from '../../modules/drawing/drawing-authority.ts'

export type WorkspaceRole = UserRole
export const workspaceLabels: Record<WorkspaceRole, string> = { admin: '管理工作台', planner: '计划工作台', designer: '设计工作台', reviewer: '审核工作台' }

export function availableWorkspaces(roles: readonly UserRole[]): WorkspaceRole[] {
  return (['admin', 'planner', 'designer', 'reviewer'] as const).filter(role => roles.includes(role))
}

export function workspaceDataSources(role: WorkspaceRole | null) {
  return {
    // 计划工作台的主列表来自任务总表（图纸 + 负责人），不需要再拉整个图纸库。
    drawings: role === 'admin' || role === 'designer',
    reviews: role === 'reviewer',
    // 计划工作台的「谁在负责哪些图纸」直接来自任务总表，不依赖图纸库的分页结果。
    tasks: role === 'admin' || role === 'planner',
    system: role === 'admin',
    audit: role === 'admin',
  }
}

// matchesUser 的单一实现在 modules/drawing/drawing-authority，这里只做转发，
// 避免再出现一份「谁算同一个人」的判断而与权限模块漂移。
export { matchesUser }

export function workspaceDrawings(drawings: readonly DrawingSummaryView[], user: AuthUser | null, role: WorkspaceRole | null): DrawingSummaryView[] {
  if (!user || (role !== 'admin' && role !== 'planner' && role !== 'designer') || !user.roles.includes(role)) return []
  const unique = [...new Map(drawings.map(drawing => [drawing.id || drawing.no, drawing])).values()]
  return unique.filter(drawing => {
    if (role === 'admin') return true
    if (role === 'planner') return isDrawingDecider(drawing, user)
    // 设计工作台只显示「我负责的图纸」：已指派看负责人，未指派才回落创建人。
    return isMyTaskDrawing(drawing, user) || matchesUser(drawing.designer, user) || matchesUser(drawing.signers?.设计, user)
  })
    .sort((a, b) => dashboardTimestamp(b.updatedAt) - dashboardTimestamp(a.updatedAt) || a.no.localeCompare(b.no))
}

export function dashboardTimestamp(value?: string): number {
  const timestamp = Date.parse(value ?? '')
  return Number.isFinite(timestamp) ? timestamp : 0
}

export function pendingWorkspaceReviews(cases: readonly ApiReviewCase[], user: AuthUser | null) {
  if (!user || !user.roles.includes('reviewer')) return []
  return [...new Map(cases.map(review => [review.id, review])).values()].flatMap(review => {
    if (review.status !== 'reviewing') return []
    const active = [...review.nodes].filter(node => node.status === 'pending').sort((a, b) => a.order - b.order)[0]
    if (!active || !(active.assignedUserId ? active.assignedUserId === user.id : matchesUser(active.assignedName, user))) return []
    return [{ id: review.id, no: review.drawingNo, name: review.drawingName || review.drawingNo, node: active.name, initiator: review.initiator, time: review.startedAt }]
  }).sort((a, b) => dashboardTimestamp(a.time) - dashboardTimestamp(b.time) || a.id.localeCompare(b.id))
}

export function completedWorkspaceReviews(actions: readonly ApiCompletedAction[], user: AuthUser | null) {
  if (!user || !user.roles.includes('reviewer')) return []
  return [...new Map(actions.map(action => [action.id, action])).values()]
    .filter(action => matchesUser(action.reviewer, user))
    .sort((a, b) => dashboardTimestamp(b.time) - dashboardTimestamp(a.time))
}
