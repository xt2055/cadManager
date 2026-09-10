import { test } from 'node:test'
import assert from 'node:assert/strict'
import { canDeleteDrawingFiles } from '../src/features/drawings/detail-tabs/preview/drawing-file-delete.ts'

test('仅未存档图纸的创建者或管理员可删除图纸文件', () => {
  assert.equal(canDeleteDrawingFiles({ status: 'published', creator: '张工', userName: '张工', admin: false }), true)
  assert.equal(canDeleteDrawingFiles({ status: 'reviewing', creator: '张工', userName: '管理员', admin: true }), true)
  assert.equal(canDeleteDrawingFiles({ status: 'published', creator: '张工', userName: '李工', admin: false }), false)
  assert.equal(canDeleteDrawingFiles({ status: 'reviewing', creator: '张工', userName: '审核人', admin: false }), false)
  assert.equal(canDeleteDrawingFiles({ status: 'archived', creator: '张工', userName: '张工', admin: false }), false)
  assert.equal(canDeleteDrawingFiles({ status: 'archived', creator: '张工', userName: '管理员', admin: true }), false)
})
