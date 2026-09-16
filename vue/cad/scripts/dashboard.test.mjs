import { test } from 'node:test'
import assert from 'node:assert/strict'
import { availableWorkspaces, workspaceDataSources, workspaceDrawings, pendingWorkspaceReviews, completedWorkspaceReviews } from '../src/features/dashboard/dashboard.helpers.ts'

const user = { id: 'u1', account: 'engineer', displayName: '张工', roles: ['designer', 'reviewer'], status: 'active' }
const drawing = (id, extra = {}) => ({ id, no: id, name: id, updatedAt: '2026-09-10T08:00:00Z', createdBy: 'u1', status: 'draft', ...extra })
const review = (id, extra = {}) => ({ id, drawingNo: id, drawingName: id, status: 'reviewing', initiator: '李工', startedAt: '2026-09-10T08:00:00Z', nodes: [{ name: '审核', order: 1, status: 'pending', assignedUserId: 'u1', assignedName: '张工' }], ...extra })

test('只能切换账号实际分配的工作角色，管理员不会自动混入其他角色视图', () => {
  assert.deepEqual(availableWorkspaces(['designer']), ['designer'])
  assert.deepEqual(availableWorkspaces(['reviewer']), ['reviewer'])
  assert.deepEqual(availableWorkspaces(['admin']), ['admin'])
  assert.deepEqual(availableWorkspaces(['reviewer', 'admin', 'designer', 'reviewer']), ['admin', 'designer', 'reviewer'])
  assert.deepEqual(availableWorkspaces(['planner']), ['planner'])
  assert.deepEqual(availableWorkspaces(['planner', 'designer']), ['planner', 'designer'])
  assert.deepEqual(availableWorkspaces([]), [])
})
test('普通工作台不请求管理日志与系统状态，设计工作台不请求无关审核数据', () => {
  assert.deepEqual(workspaceDataSources('designer'), { drawings: true, reviews: false, tasks: false, system: false, audit: false })
  assert.deepEqual(workspaceDataSources('reviewer'), { drawings: false, reviews: true, tasks: false, system: false, audit: false })
  assert.deepEqual(workspaceDataSources('admin'), { drawings: true, reviews: false, tasks: true, system: true, audit: true })
  assert.deepEqual(workspaceDataSources(null), { drawings: false, reviews: false, tasks: false, system: false, audit: false })
})

// 计划工作台的主列表是「谁在负责哪张图」，因此需要任务总表而不是整个图纸库：
// 拉整个图纸库既拿不到负责人，也把首页变成第二次全量加载。
test('计划工作台只请求任务总表，不重复加载图纸库', () => {
  assert.deepEqual(workspaceDataSources('planner'), { drawings: false, reviews: false, tasks: true, system: false, audit: false })
})

// 指派生效后控制权归负责人：设计工作台的「我的图纸」必须跟着换人，
// 否则被指派的设计人员在自己的首页看不到刚接手的图。
test('设计工作台按负责人（而不是创建人）认定我的图纸', () => {
  const rows = [
    drawing('assignee', { createdBy: 'u2', assignees: [{ userId: 'u1', name: '张工' }] }),
    drawing('other-assignee', { createdBy: 'u1', assignees: [{ userId: 'u2', name: '李工' }] }),
    drawing('unassigned-mine', { createdBy: 'u1' }),
  ]
  const mine = workspaceDrawings(rows, { ...user, roles: ['designer'] }, 'designer').map(item => item.id)
  assert.deepEqual(mine.sort(), ['assignee', 'unassigned-mine'])
})
test('设计图纸限定本人创建或负责，按项目身份去重并按更新时间排序', () => {
  const rows = [drawing('a'), drawing('a'), drawing('b', { createdBy: 'u2', designer: '张工', updatedAt: '2026-09-11T08:00:00Z' }), drawing('c', { createdBy: 'u2', designer: '李工' }), drawing('d', { createdBy: 'u2', signers: { 设计: 'engineer' } })]
  assert.deepEqual(workspaceDrawings(rows, user, 'designer').map(item => item.id), ['b', 'a', 'd'])
  assert.equal(workspaceDrawings(rows, { ...user, roles: ['admin'] }, 'admin').length, 4)
  assert.deepEqual(workspaceDrawings(rows, user, 'admin'), [])
  assert.deepEqual(workspaceDrawings(rows, null, 'designer'), [])
})
test('分配用户ID优先，不能将同名他人的审核显示为本人待办', () => {
  assert.deepEqual(pendingWorkspaceReviews([review('a', { nodes: [{ name: '审核', order: 1, status: 'pending', assignedUserId: 'u2', assignedName: '张工' }] })], user), [])
  const pending = pendingWorkspaceReviews([review('a', { nodes: [{ name: '审核', order: 1, status: 'pending', assignedUserId: 'u1', assignedName: '旧姓名' }] })], user)
  assert.equal(pending.length, 1)
  assert.equal(pending[0].initiator, '李工')
})
test('只显示当前最早待处理节点，后续分配和已经结束的流程不进入待办', () => {
  const later = review('a', { nodes: [{ order: 2, status: 'pending', assignedUserId: 'u1' }, { order: 1, status: 'pending', assignedUserId: 'u2' }] })
  assert.deepEqual(pendingWorkspaceReviews([later, review('done', { status: 'completed' })], user), [])
  assert.deepEqual(pendingWorkspaceReviews([review('a')], { ...user, roles: ['designer'] }), [])
})
test('兼容历史姓名分配，重复案例只显示一次，较早任务排在前面', () => {
  const legacy = review('old', { startedAt: '2026-09-09T08:00:00Z', nodes: [{ name: '校核', status: 'pending', order: 1, assignedName: 'engineer' }] })
  assert.deepEqual(pendingWorkspaceReviews([review('new'), legacy, legacy], user).map(item => item.id), ['old', 'new'])
})
test('已办签署只统计当前用户，按签署记录去重', () => {
  const action = { id: 'a', reviewer: '张工', time: '2026-09-10T08:00:00Z' }
  assert.deepEqual(completedWorkspaceReviews([action, action, { ...action, id: 'b', reviewer: '李工' }], user), [action])
})
