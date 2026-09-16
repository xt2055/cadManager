import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createNotificationAlertTracker } from '../src/features/notifications/notification-alert-tracker.ts'

/** 通知弹窗只看未读新条目，因此这里统一构造最小可用的通知对象。 */
function item(id, createdAt, readAt = null) {
  return {
    id,
    kind: 'change',
    title: `通知 ${id}`,
    content: `内容 ${id}`,
    senderName: '系统',
    drawingId: 'd1',
    createdAt,
    readAt,
  }
}

const t = (hour) => `2026-09-16T0${hour}:00:00Z`

test('首次同步只建立基线：登录时已存在的历史未读不会逐条弹窗', () => {
  const tracker = createNotificationAlertTracker()
  const batch = tracker.collect([item('a', t(1)), item('b', t(2))])
  assert.equal(batch.seeded, true)
  assert.deepEqual(batch.alerts, [])
})

test('基线之后新到达的未读按由旧到新返回，已读的新条目不提醒', () => {
  const tracker = createNotificationAlertTracker()
  tracker.collect([item('a', t(1))])
  const batch = tracker.collect([
    item('c', t(4)),
    item('b', t(3)),
    item('seen', t(2), t(5)),
  ])
  assert.equal(batch.seeded, false)
  // 服务端按 created_at DESC 返回，提醒顺序必须翻转为旧→新。
  assert.deepEqual(batch.alerts.map((entry) => entry.id), ['b', 'c'])
})

test('同一批通知重复同步不会重复提醒', () => {
  const tracker = createNotificationAlertTracker()
  tracker.collect([])
  const once = tracker.collect([item('a', t(1)), item('b', t(2))])
  assert.equal(once.alerts.length, 2)
  assert.deepEqual(tracker.collect([item('a', t(1)), item('b', t(2))]).alerts, [])
})

test('已提醒过的通知后来变为已读，仍不会再次提醒', () => {
  const tracker = createNotificationAlertTracker()
  tracker.collect([])
  assert.equal(tracker.collect([item('a', t(1))]).alerts.length, 1)
  const after = tracker.collect([item('a', t(1), t(6)), item('b', t(2), t(7))])
  assert.deepEqual(after.alerts, [])
})

test('未读列表变化只影响页面数据，不产生提醒', () => {
  const tracker = createNotificationAlertTracker()
  tracker.collect([item('a', t(1))])
  // 未读过滤后同样的条目再次出现，不应被当成新通知。
  assert.deepEqual(tracker.collect([item('a', t(1))]).alerts, [])
})

test('reset 后重新建立基线，避免换账号时把历史未读当成新通知', () => {
  const tracker = createNotificationAlertTracker()
  tracker.collect([item('a', t(1))])
  assert.equal(tracker.collect([item('b', t(2))]).alerts.length, 1)
  tracker.reset()
  const afterReset = tracker.collect([item('b', t(2)), item('c', t(3))])
  assert.equal(afterReset.seeded, true)
  assert.deepEqual(afterReset.alerts, [])
  assert.equal(tracker.collect([item('d', t(4))]).alerts.length, 1)
})
