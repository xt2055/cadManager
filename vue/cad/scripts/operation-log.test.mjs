import assert from 'node:assert/strict'
import test from 'node:test'

import {
  mergeSelectOptions,
  paginationPages,
  paginationRange,
} from '../src/features/operation-logs/operation-log.helpers.ts'

test('分页页码围绕当前页展示并受总页数约束', () => {
  assert.deepEqual(paginationPages(1, 10), [1, 2, 3, 4, 5])
  assert.deepEqual(paginationPages(5, 10), [3, 4, 5, 6, 7])
  assert.deepEqual(paginationPages(10, 10), [6, 7, 8, 9, 10])
  assert.deepEqual(paginationPages(4, 3), [1, 2, 3])
})

test('远程候选去重并保留当前已选择值', () => {
  assert.deepEqual(
    mergeSelectOptions([
      { value: 'JG-001', label: 'JG-001 · 总图' },
      { value: 'JG-002', label: 'JG-002 · 零件图' },
    ], 'JG-001'),
    [
      { value: 'JG-001', label: 'JG-001 · 总图' },
      { value: 'JG-002', label: 'JG-002 · 零件图' },
    ],
  )
  assert.deepEqual(
    mergeSelectOptions([{ value: 'JG-001', label: 'JG-001 · 总图' }], 'JG-003'),
    [
      { value: 'JG-001', label: 'JG-001 · 总图' },
      { value: 'JG-003', label: 'JG-003' },
    ],
  )
})

test('分页范围使用服务端总数并处理空结果', () => {
  assert.equal(paginationRange(1, 20, 53), '1–20')
  assert.equal(paginationRange(3, 20, 53), '41–53')
  assert.equal(paginationRange(1, 20, 0), '0')
})
