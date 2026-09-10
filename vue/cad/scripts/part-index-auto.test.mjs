import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createAutoPartIndexer } from '../src/features/part-index/part-index.auto.ts'
import { uniquePartIndexProjects, partIndexProjectLabel } from '../src/features/part-index/part-index.helpers.ts'

const item = (id, extra = {}) => ({ attachmentId: id, versionId: 'v1', canWrite: true, extractionStatus: 'pending', ...extra })
const page = (list, number = 1, total = list.length, pageSize = list.length || 100) => ({ list, page: number, total, pageSize })
function fixture(overrides = {}) {
  const reads = [], updates = [], progress = []
  const worker = createAutoPartIndexer({ list: async () => page([]), extract: async value => reads.push(value.attachmentId),
    stopped: () => false, updated: value => updates.push(value), progress: value => progress.push(value), ...overrides })
  return { worker, reads, updates, progress }
}
test('自动补齐先收集全部分页，跨状态去重，跳过只读及已识别文件', async () => {
  const events = []
  const { worker } = fixture({
    list: async (status, number) => {
      events.push(`${status}-${number}`)
      if (status === 'failed') return page([item('b', { extractionStatus: 'failed' })])
      return page(number === 1 ? [item('a'), item('r', { canWrite: false })] : [item('b'), item('c', { extractionStatus: 'extracted' })], number, 4, 2)
    }, extract: async value => events.push(value.attachmentId),
  })
  await worker.run()
  assert.deepEqual(events, ['pending-1', 'pending-2', 'failed-1', 'a', 'b'])
})
test('并发触发共用任务，成功文件不会随旧列表反复提取', async () => {
  let release
  const pending = new Promise(resolve => { release = resolve })
  const { worker, reads } = fixture({ list: async status => { await pending; return page(status === 'pending' ? [item('a')] : []) } })
  const first = worker.run(), second = worker.run()
  assert.equal(first, second)
  release()
  await Promise.all([first, second])
  await worker.run()
  assert.deepEqual(reads, ['a'])
})
test('失败继续处理下一文件，退避后自动重试；新版本不受旧版本退避影响', async () => {
  let time = 0, versionId = 'v1', failures = 0
  const calls = []
  const { worker } = fixture({
    now: () => time,
    list: async status => page(status === 'pending' ? [item('a', { versionId }), item('b')] : []),
    extract: async value => { calls.push(`${value.attachmentId}:${value.versionId}`); if (value.attachmentId === 'a') { failures++; throw Error('转换失败') } },
  })
  await worker.run(); await worker.run()
  assert.deepEqual(calls, ['a:v1', 'b:v1'])
  time = 60_000; await worker.run()
  assert.equal(failures, 2)
  time += 60_000; await worker.run()
  assert.equal(failures, 2)
  versionId = 'v2'; await worker.run()
  assert.equal(failures, 3)
})
test('退出会话后丢弃收集结果，不再开始提取', async () => {
  let stopped = false
  const { worker, reads } = fixture({ stopped: () => stopped, list: async () => { stopped = true; return page([item('a')]) } })
  await worker.run()
  assert.deepEqual(reads, [])
})
test('处理中退出会话，不更新页面也不继续下一文件', async () => {
  let stopped = false
  const { worker, updates, progress } = fixture({ stopped: () => stopped,
    list: async status => page(status === 'pending' ? [item('a'), item('b')] : []), extract: async () => { stopped = true },
  })
  await worker.run()
  assert.deepEqual(updates, [])
  assert.equal(progress.length, 1)
})
test('分页失败时不启动部分任务，下次扫描可恢复', async () => {
  let failed = true
  const { worker, reads } = fixture({ list: async (status, number) => {
    if (status === 'failed') return page([])
    if (number === 2 && failed) throw Error('网络中断')
    return page([item(String(number))], number, 2, 1)
  } })
  await assert.rejects(worker.run(), /网络中断/)
  assert.deepEqual(reads, [])
  failed = false; await worker.run()
  assert.deepEqual(reads, ['1', '2'])
})
test('项目按身份去重，重复编号和名称只展示一次，不合并同名不同项目', () => {
  const project = { drawingId: 'a', drawingNo: '2000W', projectCode: '2000W', projectName: '2000W' }
  assert.equal(uniquePartIndexProjects([project, project, { ...project, drawingId: 'b' }]).length, 2)
  assert.equal(partIndexProjectLabel(project), '2000W')
  assert.equal(partIndexProjectLabel({ ...project, projectName: '油缸' }), '2000W · 油缸')
  assert.deepEqual(uniquePartIndexProjects(null), [])
})
