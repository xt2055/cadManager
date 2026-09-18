import { matchesUser } from '../dashboard/dashboard.helpers.ts'
import { isDrawingDecider } from '../../modules/drawing/drawing-authority.ts'
import type { AuthUser } from '../auth/types/auth.types.ts'
import type { ApiReviewCase, ApiReviewCaseNode } from '../../services/review-case.service.ts'

export type ReviewNodeStatus = ApiReviewCaseNode['status']

// 变更工单存在或尚未确认时，负责人、创建者和管理员均不能走普通送审入口。
// 控制权判定统一走 isDrawingDecider：指派生效后送审权随负责人转移。
export function canStartRegularReview(
  drawing: { status: string; createdBy?: string; assignees?: Array<{ userId: string; name: string }> } | null | undefined,
  user: AuthUser | null | undefined,
  changeBlocked: boolean,
): boolean {
  if (!drawing || !user || changeBlocked || drawing.status === 'archived') return false
  return isDrawingDecider(drawing, user)
}

export interface ReviewWorkspaceNode {
  name: string
  assignedUserId?: string
  assignedName: string
  status: ReviewNodeStatus
  opinion: string
  required: boolean
  order: number
  time?: string
}

export function sortReviewNodes<T extends { order: number }>(nodes: readonly T[]): T[] {
  return [...nodes].sort((a, b) => a.order - b.order)
}

export function activeReviewNode<T extends { status: ReviewNodeStatus; order: number }>(
  nodes: readonly T[],
  reviewing: boolean,
): T | null {
  if (!reviewing) return null
  return sortReviewNodes(nodes).find((node) => node.status === 'pending') ?? null
}

export function isAssignedReviewer(
  node: { assignedUserId?: string; assignedName?: string } | null | undefined,
  user: AuthUser | null | undefined,
): boolean {
  if (!node || !user) return false
  if (node.assignedUserId) return node.assignedUserId === user.id
  return matchesUser(node.assignedName, user)
}

export function canSignReviewNode(
  node: ReviewWorkspaceNode | null | undefined,
  activeNodeName: string | null | undefined,
  user: AuthUser | null | undefined,
  reviewing: boolean,
): boolean {
  if (!reviewing || !node || node.name !== activeNodeName) return false
  return isAssignedReviewer(node, user)
}

export function reviewNodeStatusLabel(
  node: ReviewWorkspaceNode,
  activeNodeName: string | null,
  reviewing: boolean,
  currentUserId?: string,
  blockedByRejection = false,
): string {
  if (reviewing && node.name === activeNodeName) {
    if (currentUserId && node.assignedUserId && node.assignedUserId !== currentUserId) return '还未到你'
    return '当前节点'
  }
  if (node.status === 'pass') return '已同意'
  if (node.status === 'rejected') return '已驳回'
  return reviewing || blockedByRejection ? '还未到你' : '未开始'
}

export function toWorkspaceNodes(reviewCase: ApiReviewCase | null | undefined): ReviewWorkspaceNode[] {
  if (!reviewCase) return []
  return sortReviewNodes(reviewCase.nodes).map((node) => ({
    name: node.name,
    assignedUserId: node.assignedUserId,
    assignedName: node.assignedName,
    status: node.status,
    opinion: node.opinion,
    required: node.required,
    order: node.order,
    time: node.time,
  }))
}

export interface ReviewCaseLike {
  id: string
  status: string
  startedAt?: string
}

/**
 * 选择某个图纸当前要处理的审核轮次。
 *
 * 进行中的轮次在一张图纸上唯一（数据库部分唯一索引 uq_review_cases_active_drawing 保证），
 * 它才是当前轮次。绝不能按时间戳猜轮次：`started_at` 前后端都只精确到分钟，
 * 「驳回 → 修改 → 重新提交」通常在几分钟内完成，两个轮次的时间戳往往完全相同，
 * 一旦用「时间戳最大的那个」来选，就会挑到被驳回的上一轮——下一个审核员打开
 * 「图纸批注」看到的正是上一轮审核员留下的标注，而这对他毫无意义。
 *
 * 只有确实没有进行中轮次时（全部结束或退回待重提），才回退到最近结束的一轮，
 * 让工作台能显示「流程已驳回、等待重新发起」这类状态。
 */
export function pickReviewCase<T extends ReviewCaseLike>(cases: readonly T[], caseId?: string): T | null {
  if (caseId) return cases.find((item) => item.id === caseId) ?? null
  const active = cases.find((item) => item.status === 'reviewing' || item.status === 'pending')
  if (active) return active
  // 复制后再排序：调用方的 cases 数组是 store 状态，不得就地重排。
  return [...cases].sort((a, b) => {
    const byStartedAt = (b.startedAt || '').localeCompare(a.startedAt || '')
    return byStartedAt !== 0 ? byStartedAt : b.id.localeCompare(a.id)
  })[0] ?? null
}
