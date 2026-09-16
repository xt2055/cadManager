import type { AuthUser } from '@/features/auth/types/auth.types'

/**
 * 图纸的控制权判定，与后端 drawing.Drawing.Decides / drawing_decision_owner 同构。
 *
 * 规则：管理员始终拥有；图纸存在有效负责人时只归负责人（创建人不再拥有）；
 * 没有有效负责人时回落给创建人（保证未指派图纸的行为与旧版一致）。
 *
 * 前端只用于决定「按钮是否可见、是否有权限」，权威判定一律在后端。
 * 任何页面都不要再自己比较 createdBy，否则指派一改就会出现前后端不一致。
 */
export interface DrawingAuthorityTarget {
  createdBy?: string
  assignees?: DrawingAssigneeRef[]
}

export interface DrawingAssigneeRef {
  userId: string
  name: string
}

/**
 * 判断某个身份字段（账号、姓名、用户 ID）是否属于当前登录人。
 * 历史数据里创建人写的是姓名，节点责任人写的是用户 ID，因此三种都要能匹配。
 */
export function matchesUser(value: string | undefined, user: AuthUser | null | undefined): boolean {
  const identity = value?.trim()
  if (!identity || !user) return false
  return [user.id, user.account, user.displayName].filter(Boolean).includes(identity)
}

/** 判断当前登录人是否是这张图纸的负责人。 */
export function isDrawingAssignee(
  target: DrawingAuthorityTarget | null | undefined,
  user: AuthUser | null | undefined,
): boolean {
  const assignees = target?.assignees ?? []
  if (!assignees.length || !user) return false
  return assignees.some((assignee) => assignee.userId === user.id || matchesUser(assignee.name, user))
}

/** 判断当前登录人是否对图纸拥有决定控制权（编辑、存档、送审、文件与模型管理）。 */
export function isDrawingDecider(
  target: DrawingAuthorityTarget | null | undefined,
  user: AuthUser | null | undefined,
): boolean {
  if (!target || !user) return false
  if (user.roles?.includes('admin')) return true
  if (target.assignees?.length) return isDrawingAssignee(target, user)
  return matchesUser(target.createdBy, user)
}

/**
 * 「我负责的图纸」筛选：已指派时只看负责人，未指派时回落创建人。
 * 与 isDrawingDecider 不同的是，这里不把管理员算作负责人——
 * 管理员能看到全部图纸，混进「我的任务」只会让人误以为活在自己身上。
 */
export function isMyTaskDrawing(
  target: DrawingAuthorityTarget | null | undefined,
  user: AuthUser | null | undefined,
): boolean {
  if (!target || !user) return false
  if (target.assignees?.length) return isDrawingAssignee(target, user)
  return matchesUser(target.createdBy, user)
}
