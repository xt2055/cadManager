import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  activeReviewNode,
  canSignReviewNode,
  canStartRegularReview,
  isAssignedReviewer,
  pickReviewCase,
  reviewNodeStatusLabel,
} from '../src/features/reviews/review-workspace.ts'

const user = { id: 'u1', account: 'checker', displayName: '校对员', roles: ['reviewer'], status: 'active' }

test('存在变更工单时创建者、管理员及修改人均不能绕过工单走普通送审', () => {
  const drawing = { status: 'draft', createdBy: user.displayName }
  for (const current of [user, { ...user, roles: ['admin'] }, { ...user, displayName: '指定修改人' }]) {
    assert.equal(canStartRegularReview(drawing, current, true), false)
  }
  assert.equal(canStartRegularReview(drawing, user, false), true)
})
const nodes = [
  { name: '设计自检', assignedUserId: 'd1', assignedName: '设计员', status: 'pass', opinion: '自检通过', required: true, order: 1 },
  { name: '校对复核', assignedUserId: 'u1', assignedName: '校对员', status: 'pending', opinion: '', required: true, order: 2 },
  { name: '专业审核', assignedUserId: 'u2', assignedName: '审核员', status: 'pending', opinion: '', required: true, order: 3 },
]

test('当前节点取第一个待处理节点，未发起时不显示当前节点', () => {
  assert.equal(activeReviewNode(nodes, true)?.name, '校对复核')
  assert.equal(activeReviewNode(nodes, false), null)
})

test('只有当前节点责任人可以签署，用户ID优先于姓名', () => {
  const current = activeReviewNode(nodes, true)
  assert.equal(canSignReviewNode(current, current?.name, user, true), true)
  assert.equal(canSignReviewNode(nodes[2], current?.name, user, true), false)
  assert.equal(isAssignedReviewer({ assignedUserId: 'u9', assignedName: '校对员' }, user), false)
  assert.equal(isAssignedReviewer({ assignedName: 'checker' }, user), true)
})

test('节点状态文案区分当前节点、已通过和还未到你', () => {
  assert.equal(reviewNodeStatusLabel(nodes[1], '校对复核', true, 'u1'), '当前节点')
  assert.equal(reviewNodeStatusLabel(nodes[1], '校对复核', true, 'u2'), '还未到你')
  assert.equal(reviewNodeStatusLabel(nodes[0], '校对复核', true), '已同意')
  assert.equal(reviewNodeStatusLabel(nodes[2], '校对复核', true, 'u1'), '还未到你')
})

test('流程驳回后，驳回节点之后的节点显示还未到你', () => {
  const rejected = { ...nodes[1], status: 'rejected' }
  assert.equal(reviewNodeStatusLabel(rejected, null, false, 'u1', true), '已驳回')
  assert.equal(reviewNodeStatusLabel(nodes[2], null, false, 'u2', true), '还未到你')
})

test('驳回后重新提交：工作台必须选进行中的新一轮，不能按分钟级时间戳挑到被驳回的上一轮', () => {
  // 驳回 → 修改 → 重新提交通常在同一分钟内完成，两个轮次的 startedAt 完全相同。
  const rounds = [
    { id: 'r1', status: 'rejected', startedAt: '2026-01-02 09:00' },
    { id: 'r2', status: 'reviewing', startedAt: '2026-01-02 09:00' },
  ]
  assert.equal(pickReviewCase(rounds)?.id, 'r2')
  // 规则不依赖后端返回的数组顺序：反过来给同样选新版。
  assert.equal(pickReviewCase([...rounds].reverse())?.id, 'r2')
  // 待处理状态同样属于进行中轮次。
  assert.equal(pickReviewCase([rounds[0], { id: 'r3', status: 'pending', startedAt: '2026-01-02 08:00' }])?.id, 'r3')
})

test('没有进行中轮次时才回退到最近一轮，且不就地重排调用方的数组', () => {
  const cases = [
    { id: 'r1', status: 'published', startedAt: '2026-01-01 09:00' },
    { id: 'r2', status: 'rejected', startedAt: '2026-01-02 09:00' },
  ]
  assert.equal(pickReviewCase(cases)?.id, 'r2')
  assert.deepEqual(cases.map((item) => item.id), ['r1', 'r2'], '选择轮次不得就地排序调用方的数组')
  // 同一时间戳再回退到 id 兜底，保证结果稳定。
  assert.equal(pickReviewCase([{ id: 'a', status: 'rejected', startedAt: '2026-01-02 09:00' }, { id: 'b', status: 'rejected', startedAt: '2026-01-02 09:00' }])?.id, 'b')
  // 明确指定案例时按 id 命中，查不到就返回 null，不回退到别的轮次。
  assert.equal(pickReviewCase(cases, 'r1')?.id, 'r1')
  assert.equal(pickReviewCase(cases, 'missing'), null)
  assert.equal(pickReviewCase([]), null)
})
