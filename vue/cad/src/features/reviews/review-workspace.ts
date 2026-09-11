import { matchesUser } from '../dashboard/dashboard.helpers.ts'
import type { AuthUser } from '../auth/types/auth.types.ts'
import type { ApiReviewCase, ApiReviewCaseNode } from '../../services/review-case.service.ts'

export type ReviewNodeStatus = ApiReviewCaseNode['status']

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
