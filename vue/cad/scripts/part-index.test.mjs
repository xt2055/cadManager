import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  appendPartIndexBackfillErrors,
  applyTitleCandidate,
  canBatchExtract,
  emptyPartIndexFields,
  extractAndRebuildPartIndex,
  fieldsFromTitleSpace,
  isValidPartIndexDrawingDate,
  partIndexQueryString,
  partIndexStatusLabels,
  runSerialPartIndexBatch,
} from '../src/features/part-index/part-index.helpers.ts'

test('列表请求参数使用固定 snake_case 合同', () => {
  const query = partIndexQueryString({
    page: 2, pageSize: 20, keyword: '轴', projectId: 'project-id',
    material: '40Cr', designer: '张三', dateFrom: '2026-09-01', dateTo: '2026-09-09', status: 'confirmed',
  })
  assert.equal(query, '?page=2&page_size=20&keyword=%E8%BD%B4&project_id=project-id&material=40Cr&designer=%E5%BC%A0%E4%B8%89&date_from=2026-09-01&date_to=2026-09-09&status=confirmed')
})

test('标题栏空间映射完整 12 字段，图幅保留人工值', () => {
  const fields = fieldsFromTitleSpace({
    id: 'layout1', name: '布局1', textCount: 3, fields: [
      { key: 'number', value: ' J1233-03 ', candidates: [] },
      { key: 'name', value: '主动轴', candidates: [] },
      { key: 'material', value: '40Cr', candidates: [] },
      { key: 'checker', value: '李四', candidates: [] },
      { key: 'approver', value: '王五', candidates: [] },
      { key: 'process', value: '调质', candidates: [] },
    ],
  }, 'A3')
  assert.deepEqual(fields, {
    ...emptyPartIndexFields(), drawingNo: 'J1233-03', partName: '主动轴',
    material: '40Cr', checker: '李四', approver: '王五', process: '调质', sheetSize: 'A3',
  })
})

test('候选值只能填入对应字段，不能覆盖其他人工字段', () => {
  const current = { ...emptyPartIndexFields(), partName: '人工名称', sheetSize: 'A3' }
  const updated = applyTitleCandidate(current, 'material', ' 42CrMo ')
  assert.deepEqual(updated, { ...current, material: '42CrMo' })
  assert.equal(applyTitleCandidate(updated, 'unknown', 'ignored'), updated)
})

test('批量提取仅允许可写 pending 或 failed 条目，失败继续', async () => {
  const pending = { attachmentId: 'a', fileName: 'A.dwg', canWrite: true, extractionStatus: 'pending' }
  const failed = { attachmentId: 'b', fileName: 'B.dwg', canWrite: true, extractionStatus: 'failed' }
  assert.equal(canBatchExtract({ ...pending }), true)
  assert.equal(canBatchExtract({ ...pending, canWrite: false }), false)
  assert.equal(canBatchExtract({ ...pending, extractionStatus: 'extracted' }), false)
  const calls = []
  const result = await runSerialPartIndexBatch([pending, failed], () => false, async item => {
    calls.push(item.attachmentId)
    if (item.attachmentId === 'a') throw new Error('转换未完成')
  }, () => undefined)
  assert.deepEqual(calls, ['a', 'b'])
  assert.equal(result.completed, 2)
  assert.equal(result.succeeded, 1)
  assert.deepEqual(result.failures, ['A.dwg：转换未完成'])
})

test('批量提取后按标题栏快照重建索引', async () => {
  const calls = []
  await extractAndRebuildPartIndex(
    async () => ({ versionId: 'version-1', snapshotRevision: 3 }),
    async (versionId, snapshotRevision) => { calls.push({ versionId, snapshotRevision }) },
  )
  assert.deepEqual(calls, [{ versionId: 'version-1', snapshotRevision: 3 }])
})

test('没有标题栏快照时报告失败，不能把批量提取记为成功', async () => {
  let rebuilt = false
  await assert.rejects(extractAndRebuildPartIndex(
    async () => ({ versionId: 'version-1', snapshotRevision: 0 }),
    async () => { rebuilt = true },
  ), /标题栏快照尚未保存/)
  assert.equal(rebuilt, false)
})

test('补建失败详情保留可重试附件并限制显示数量', () => {
  const first = appendPartIndexBackfillErrors([], [
    { attachmentId: ' a ', message: ' 写入失败 ' },
    { attachmentId: '', message: '无附件应忽略' },
  ], 2)
  assert.deepEqual(first, {
    items: [{ attachmentId: 'a', message: '写入失败' }],
    omitted: 0,
  })
  const second = appendPartIndexBackfillErrors(first.items, [
    { attachmentId: 'b', message: '快照格式错误' },
    { attachmentId: 'c', message: '事务失败' },
  ], 2)
  assert.deepEqual(second, {
    items: [
      { attachmentId: 'a', message: '写入失败' },
      { attachmentId: 'b', message: '快照格式错误' },
    ],
    omitted: 1,
  })
})

test('日期校验与服务端一致，拒绝 0000 年', () => {
  assert.equal(isValidPartIndexDrawingDate('0000-01-01'), false)
  assert.equal(isValidPartIndexDrawingDate('0001-01-01'), true)
  assert.equal(isValidPartIndexDrawingDate('2024-02-29'), true)
  assert.equal(isValidPartIndexDrawingDate('2025-02-29'), false)
  assert.equal(isValidPartIndexDrawingDate('2026-13-01'), false)
})

test('批量停止后不开始下一条', async () => {
  const items = [
    { attachmentId: 'a', fileName: 'A.dwg', canWrite: true, extractionStatus: 'pending' },
    { attachmentId: 'b', fileName: 'B.dwg', canWrite: true, extractionStatus: 'pending' },
  ]
  let stop = false
  const calls = []
  const result = await runSerialPartIndexBatch(items, () => stop, async item => {
    calls.push(item.attachmentId)
    stop = true
  }, () => undefined)
  assert.deepEqual(calls, ['a'])
  assert.equal(result.stopped, true)
  assert.equal(result.completed, 1)
})

test('状态标签覆盖所有后端状态', () => {
  assert.deepEqual(Object.keys(partIndexStatusLabels).sort(), ['confirmed', 'failed', 'needs_confirmation', 'pending', 'recheck'])
})
