import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildUserRoleUpdate, USER_ROLE_OPTIONS } from '../src/features/admin/account-role-editor.ts'

const user = {
  account: 'zhang',
  displayName: '张工',
  status: 'active',
}

test('账号身份支持设计、审核和管理员多选', () => {
  assert.deepEqual(USER_ROLE_OPTIONS.map((item) => item.value), ['designer', 'reviewer', 'admin'])
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
