import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildUserRoleUpdate, USER_ROLE_OPTIONS } from '../src/features/admin/account-role-editor.ts'

const user = {
  account: 'zhang',
  displayName: '张工',
  status: 'active',
}

test('账号身份支持设计、审核和管理员多选', () => {
  assert.deepEqual(USER_ROLE_OPTIONS.map((item) => item.value), ['planner', 'designer', 'reviewer', 'admin'])
  // 每个身份都要有一句说明：管理员在分配角色时需要知道它能做什么。
  assert.ok(USER_ROLE_OPTIONS.every((item) => item.label && item.description))
  assert.deepEqual(buildUserRoleUpdate(user, ['designer', 'reviewer']), {
    account: 'zhang',
    displayName: '张工',
    roles: ['designer', 'reviewer'],
    status: 'active',
  })
})

test('修改身份时至少保留一个身份', () => {
  assert.throws(() => buildUserRoleUpdate(user, []), /请至少选择一个身份/)
})
