import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  appendPartIndexBackfillErrors,
  applyTitleCandidate,
  canBatchExtract,
  collectPendingPartIndexes,
  togglePartIndexPageSelection,
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

test('状态标签区分自动可用、人工修订和真正需要检查的结果', () => {
  assert.deepEqual(Object.keys(partIndexStatusLabels).sort(), ['confirmed', 'edited', 'failed', 'needs_confirmation', 'pending', 'recheck', 'recognized'])
  assert.equal(partIndexStatusLabels.recognized, '已识别')
  assert.equal(partIndexStatusLabels.edited, '已人工修订')
  assert.equal(partIndexStatusLabels.needs_confirmation, '需检查')
  assert.equal(partIndexStatusLabels.recheck, '换版待检查')
})

function batchItem(id, overrides = {}) {
  return { attachmentId: id, fileName: `${id}.dwg`, canWrite: true, extractionStatus: 'pending', ...overrides }
}

test('本页全选包含已识别文件，但排除无写权限文件', () => {
  const items = [batchItem('a'), batchItem('b', { extractionStatus: 'extracted' }), batchItem('c', { canWrite: false })]
  assert.deepEqual(togglePartIndexPageSelection(items, []), ['a', 'b'])
  assert.deepEqual(togglePartIndexPageSelection(items, ['a']), ['a', 'b'])
  assert.deepEqual(togglePartIndexPageSelection(items, ['a', 'b']), [])
  assert.deepEqual(togglePartIndexPageSelection([], []), [])
})

test('跨页先收集队列再执行，按附件去重并排除无权限和已识别文件', async () => {
  const calls = []
  const pages = [
    [batchItem('a'), batchItem('b', { extractionStatus: 'failed' }), batchItem('c', { canWrite: false })],
    [batchItem('a'), batchItem('d'), batchItem('e', { extractionStatus: 'extracted' })],
  ]
  const queue = await collectPendingPartIndexes(async page => {
    calls.push(`page-${page}`)
    return { list: pages[page - 1], page, pageSize: 3, total: 6 }
  }, () => false, () => undefined)
  assert.equal(queue.stopped, false)
  assert.deepEqual(queue.items.map(item => item.attachmentId), ['a', 'b', 'd'])
  await runSerialPartIndexBatch(queue.items, () => false, async item => calls.push(item.attachmentId), () => undefined)
  assert.deepEqual(calls, ['page-1', 'page-2', 'a', 'b', 'd'])
})

test('收集被停止时丢弃部分队列，不启动读取', async () => {
  let stop = false
  let pages = 0
  const result = await collectPendingPartIndexes(async page => {
    pages++
    stop = true
    return { list: [batchItem('a')], page, pageSize: 1, total: 2 }
  }, () => stop, () => undefined)
  assert.equal(pages, 1)
  assert.deepEqual(result, { items: [], stopped: true })
})

test('收集任意一页失败或异常空页时不返回部分成功队列', async () => {
  await assert.rejects(collectPendingPartIndexes(async page => {
    if (page === 2) throw new Error('网络中断')
    return { list: [batchItem('a')], page, pageSize: 1, total: 2 }
  }, () => false, () => undefined), /网络中断/)
  await assert.rejects(collectPendingPartIndexes(async page => ({ list: [], page, pageSize: 1, total: 2 }), () => false, () => undefined), /列表发生变化/)
})

test('进度总数固定，显示当前文件，失败项可以单独重试', async () => {
  const queue = [batchItem('a'), batchItem('b')]
  const progress = []
  const result = await runSerialPartIndexBatch(queue, () => false, async item => {
    queue.pop()
    if (item.attachmentId === 'b') throw new Error('CAD 解析失败')
  }, value => progress.push(value))
  assert.equal(result.total, 2)
  assert.equal(result.completed, 2)
  assert.equal(result.succeeded, 1)
  assert.equal(result.currentFile, '')
  assert.ok(progress.every(value => value.total === 2))
  assert.equal(progress[0].currentFile, 'a.dwg')
  assert.deepEqual(result.failedItems.map(item => item.attachmentId), ['b'])
  const retried = []
  const retry = await runSerialPartIndexBatch(result.failedItems, () => false, async item => retried.push(item.attachmentId), () => undefined)
  assert.deepEqual(retried, ['b'])
  assert.equal(retry.succeeded, 1)
  assert.deepEqual(retry.failedItems, [])
})

test('读取完成但索引重建失败时整条任务计为失败', async () => {
  const result = await runSerialPartIndexBatch([batchItem('a')], () => false, () => extractAndRebuildPartIndex(
    async () => ({ versionId: 'v1', snapshotRevision: 1 }),
    async () => { throw new Error('索引版本冲突') },
  ), () => undefined)
  assert.equal(result.succeeded, 0)
  assert.equal(result.failedItems[0].attachmentId, 'a')
  assert.match(result.failures[0], /索引版本冲突/)
})
