import { test } from 'node:test'
import assert from 'node:assert/strict'
import { canDeleteDrawingFiles } from '../src/features/drawings/detail-tabs/preview/drawing-file-delete.ts'

// 删除权直接采用 isDrawingDecider 的结果（负责人优先、无负责人回落创建人），
// 这里只断言「已存档一律不可删」与「有控制权才可删」两件事，规则本身由权限模块测试覆盖。
test('仅未存档且拥有图纸控制权的账号可以删除图纸文件', () => {
  assert.equal(canDeleteDrawingFiles({ status: 'published', decides: true }), true)
  assert.equal(canDeleteDrawingFiles({ status: 'reviewing', decides: true }), true)
  assert.equal(canDeleteDrawingFiles({ status: 'published', decides: false }), false)
  assert.equal(canDeleteDrawingFiles({ status: 'archived', decides: true }), false)
  assert.equal(canDeleteDrawingFiles({ status: 'archived', decides: false }), false)
})
