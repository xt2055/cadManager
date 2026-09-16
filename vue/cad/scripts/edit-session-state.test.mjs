import { test } from 'node:test'
import assert from 'node:assert/strict'
import { editSessionStorageKey, sessionsForFiles, isExpiredEditSession } from '../src/modules/editing/session-state.ts'

test('同一客户端切换账号或服务器不会读取另一方的编辑会话', () => {
  const first = editSessionStorageKey('http://server-a/api', 'creator')
  assert.notEqual(first, editSessionStorageKey('http://server-a/api', 'admin'))
  assert.notEqual(first, editSessionStorageKey('http://server-b/api', 'creator'))
  assert.notEqual(first, 'cad:active-edit-sessions:v1')
})

test('项目面板仅展示本项目文件的会话，保留其他项目的后台会话', () => {
  const sessions = [{ fileId: 'assembly' }, { fileId: 'part' }, { fileId: 'other-project' }]
  assert.deepEqual(sessionsForFiles(sessions, [{ id: 'assembly' }, { id: 'part' }]), sessions.slice(0, 2))
  assert.equal(sessions.length, 3)
  assert.deepEqual(sessionsForFiles(sessions, []), [])
})

test('会话失效可以清理，但断网、保存失败和打开票据过期不能清掉编辑会话', () => {
  assert.equal(isExpiredEditSession(new Error('编辑会话已失效，请刷新占用状态后重新打开')), true)
  assert.equal(isExpiredEditSession(new Error('编辑票据或会话已失效，请重新打开')), true)
  for (const message of ['Failed to fetch', '保存编辑版本失败，请重试', '打开票据已失效，请重新点击本地编辑']) {
    assert.equal(isExpiredEditSession(new Error(message)), false)
  }
})
