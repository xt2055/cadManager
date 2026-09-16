import { test } from 'node:test'
import assert from 'node:assert/strict'

import { isDrawingAssignee, isDrawingDecider, isMyTaskDrawing, matchesUser } from '../src/modules/drawing/drawing-authority.ts'
import {
  dueDateLabel,
  isOverdue,
  overdueDays,
  progressTone,
  rowNextStep,
  summaryHeadline,
} from '../src/features/tasks/task.helpers.ts'
import { notificationActionLabel, opensTaskDrawing } from '../src/features/notifications/notification-targets.ts'

const user = { id: 'u1', account: 'engineer', displayName: '张工', roles: ['designer'], status: 'active' }
const admin = { id: 'u9', account: 'root', displayName: '管理员', roles: ['admin'], status: 'active' }
const planner = { id: 'u8', account: 'planner', displayName: '计划员', roles: ['planner'], status: 'active' }
const assigned = { createdBy: '李工', assignees: [{ userId: 'u1', name: '张工' }] }
const taskRow = (extra = {}, progress = {}, assignment = null) => ({
  drawing: { id: 'd1', no: 'D-1', name: '总图', project: 'P', status: 'draft', version: 'v1.0', fileCount: 0, ...extra },
  progress: { percent: 0, stage: '待上传图纸', detail: '还没有图纸文件', done: false, ...progress },
  ...(assignment ? { assignment: { taskId: 't1', assigneeId: 'u1', assignee: '张工', note: '', ...assignment } } : {}),
})
const MY_ASSIGNMENT = { assigneeId: 'u1', assignee: '张工' }

test('控制权：管理员始终拥有，有负责人时归负责人，无负责人时回落创建人', () => {
  assert.equal(isDrawingDecider(assigned, admin), true)
  assert.equal(isDrawingDecider(assigned, user), true)
  assert.equal(isDrawingDecider(assigned, planner), false)
  // 指派生效后创建人本人也不再拥有控制权。
  assert.equal(isDrawingDecider(assigned, { ...planner, displayName: '李工' }), false)
  assert.equal(isDrawingDecider({ createdBy: '李工' }, { ...planner, displayName: '李工' }), true)
  assert.equal(isDrawingDecider({ createdBy: '李工' }, planner), false)
  assert.equal(isDrawingDecider(null, user), false)
  assert.equal(isDrawingDecider(assigned, null), false)
})

test('负责人身份同时按用户 ID 与姓名匹配，兼容历史的姓名记录', () => {
  assert.equal(isDrawingAssignee({ assignees: [{ userId: 'u1', name: '张工' }] }, user), true)
  assert.equal(isDrawingAssignee({ assignees: [{ userId: 'u2', name: '张工' }] }, user), true)
  assert.equal(isDrawingAssignee({ assignees: [{ userId: 'u2', name: '李工' }] }, user), false)
  assert.equal(isDrawingAssignee({ createdBy: '张工' }, user), false)
})

test('「我的任务」不把管理员混进负责人的活里', () => {
  assert.equal(isMyTaskDrawing(assigned, user), true)
  assert.equal(isMyTaskDrawing(assigned, admin), false)
  assert.equal(isMyTaskDrawing({ createdBy: '李工' }, { ...planner, displayName: '李工' }), true)
})

test('身份匹配覆盖 ID、账号与姓名，空值不匹配任何人', () => {
  assert.equal(matchesUser('张工', user), true)
  assert.equal(matchesUser('engineer', user), true)
  assert.equal(matchesUser('u1', user), true)
  assert.equal(matchesUser('  ', user), false)
  assert.equal(matchesUser(undefined, user), false)
})

test('进度色调区分未指派、进行中与已完成', () => {
  assert.equal(progressTone(taskRow()), 'muted')
  assert.equal(progressTone(taskRow({}, { percent: 30, stage: '编制中', detail: '', done: false }, MY_ASSIGNMENT)), 'idle')
  assert.equal(progressTone(taskRow({}, { percent: 100, stage: '已存档', detail: '', done: true }, MY_ASSIGNMENT)), 'positive')
})

test('未指派的行给出指派指引，已指派的行给出编制说明', () => {
  assert.match(rowNextStep(taskRow()), /指派/)
  const assignedRow = taskRow({}, { percent: 30, stage: '编制中', detail: '图纸文件已就位', done: false }, MY_ASSIGNMENT)
  assert.equal(rowNextStep(assignedRow), '图纸文件已就位')
})

test('截止日期按天判定逾期，当天不算逾期', () => {
  const now = new Date('2026-10-10T15:30:00')
  assert.equal(isOverdue('2026-10-09', now), true)
  assert.equal(isOverdue('2026-10-10', now), false)
  assert.equal(isOverdue('2026-10-11', now), false)
  assert.equal(isOverdue(undefined, now), false)
  assert.equal(overdueDays('2026-10-07', now), 3)
  assert.equal(overdueDays('2026-10-10', now), 0)
})

test('截止日期文案给出可读的剩余时间', () => {
  const now = new Date('2026-10-10T09:00:00')
  assert.equal(dueDateLabel('2026-10-10', now), '今天到期')
  assert.equal(dueDateLabel('2026-10-11', now), '明天到期')
  assert.equal(dueDateLabel('2026-10-13', now), '还有 3 天')
  assert.equal(dueDateLabel('2026-10-08', now), '已逾期 2 天')
  assert.equal(dueDateLabel(undefined, now), '未设截止日期')
})

test('管理台摘要明确说出还有多少张待指派', () => {
  assert.match(summaryHeadline({ total: 0, assigned: 0, unassigned: 0, active: 0, done: 0, overdue: 0 }), /还没有图纸/)
  assert.match(summaryHeadline({ total: 5, assigned: 5, unassigned: 0, active: 3, done: 2, overdue: 0 }), /全部已指派/)
  assert.match(summaryHeadline({ total: 5, assigned: 2, unassigned: 3, active: 2, done: 0, overdue: 0 }), /还有 3 张待指派/)
})

test('任务通知跳转图纸预览，而不是审核或变更页签', () => {
  const item = { kind: 'task', drawingId: 'd1', drawingNo: 'D-1', target: '' }
  assert.equal(opensTaskDrawing(item), true)
  assert.equal(notificationActionLabel(item), '查看任务图纸')
  assert.equal(opensTaskDrawing({ kind: 'review', drawingNo: 'D-1' }), false)
  assert.equal(opensTaskDrawing({ kind: 'task' }), false)
  assert.equal(notificationActionLabel({ kind: 'change', drawingNo: 'D-1' }), '查看变更工单')
  assert.equal(notificationActionLabel({ kind: 'review', drawingNo: 'D-1' }), '查看图纸审批')
})
