import assert from 'node:assert/strict'
import test from 'node:test'

import { withReviewContext } from '../src/features/drawings/review-context.ts'

/**
 * 回归点：从审核工作台「查阅图纸」进入图纸详情后，再点某个文件的「浏览 / 历史」，
 * 跳转必须保留 from=review。查看页据此启用批注工作区，丢了它就会出现
 * 「从审核打开却没有标注按钮」。
 */
test('普通预览跳转不注入任何审核参数', () => {
  assert.deepEqual(withReviewContext({ fileId: 'f1' }, {}), { fileId: 'f1' })
  assert.deepEqual(withReviewContext({ fileId: 'f1' }, { from: 'library' }), { fileId: 'f1' })
})

test('审核链路跳转保留 from=review 与审核上下文', () => {
  assert.deepEqual(
    withReviewContext({ fileId: 'f1' }, { from: 'review', reviewNo: 'JG6539', reviewCaseId: 'case-1' }),
    { fileId: 'f1', from: 'review', reviewNo: 'JG6539', reviewCaseId: 'case-1' },
  )
})

test('审核上下文缺省时只带 from=review，不写入空串参数', () => {
  const query = withReviewContext({ fileId: 'f1' }, { from: 'review' })
  assert.deepEqual(query, { fileId: 'f1', from: 'review' })
  assert.equal('reviewNo' in query, false)
  assert.equal('reviewCaseId' in query, false)
})

test('基础参数里的空值被剔除，版本参数缺失也不会串成空串', () => {
  const query = withReviewContext(
    { fileId: 'f1', versionId: undefined, versionKey: '' },
    { from: 'review', reviewNo: 'JG6539' },
  )
  assert.deepEqual(query, { fileId: 'f1', from: 'review', reviewNo: 'JG6539' })
})

test('数组或非字符串的审核参数不写入查询串', () => {
  const query = withReviewContext({ fileId: 'f1' }, { from: 'review', reviewNo: ['a', 'b'], reviewCaseId: null })
  assert.deepEqual(query, { fileId: 'f1', from: 'review' })
})
