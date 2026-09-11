import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  activeReviewNode,
  canSignReviewNode,
  isAssignedReviewer,
  reviewNodeStatusLabel,
} from '../src/features/reviews/review-workspace.ts'

const user = { id: 'u1', account: 'checker', displayName: '校对员', roles: ['reviewer'], status: 'active' }
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
