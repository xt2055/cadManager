import { test } from 'node:test'
import assert from 'node:assert/strict'
import { authorFromDocxRows } from '../src/utils/docx-metadata.ts'

test('读取标签下一行同列的编制人员', () => {
  assert.equal(authorFromDocxRows([
    ['编制', '校对', '审核'],
    ['熊焱林', '李四', '王五'],
  ]), '熊焱林')
})

test('读取标签右侧或同一单元格的编制人员', () => {
  assert.equal(authorFromDocxRows([['编制：', '龙学江', '日期', '2026-08-10']]), '龙学江')
  assert.equal(authorFromDocxRows([['编制人员：崔杰  审核：王五']]), '崔杰')
})

test('不能把其他签字标签或正文误当作编制人员', () => {
  assert.equal(authorFromDocxRows([['编制', '审核'], ['标准化', '王五']]), undefined)
  assert.equal(authorFromDocxRows([['编制工艺路线时应检查余量']]), undefined)
})
