import { test } from 'node:test'
import assert from 'node:assert/strict'
import { extractTitleBlockRecord, extractTitleBlockBatch, normalizeTitlePayload } from '../src/services/title-block-workflow.ts'

function fixture() {
  const record = { attachmentId: 'a', versionId: 'v1', canWrite: true, payload: null }
  const writes = []
  const io = {
    load: async () => ({ ...record }),
    parse: async () => ({ spaces: [{ id: 'model', textCount: 2, fields: [{ key: 'designer', value: '张定', candidates: ['张定'] }, { key: 'name', value: '', candidates: ['甲', '乙'] }] }] }),
    save: async (id, version, payload) => { writes.push({ id, version, payload }); record.payload = payload },
  }
  return { record, writes, io }
}
test('提取后按开始时的文件版本保存，并读回数据库结果', async () => {
  const { io, writes } = fixture()
  const result = await extractTitleBlockRecord('a', false, io)
  assert.equal(writes[0].version, 'v1')
  assert.equal(result.payload.spaces[0].fields[0].value, '张定')
  assert.equal(result.payload.spaces[0].fields[1].value, '')
  assert.deepEqual(result.payload.spaces[0].fields[1].candidates, ['甲', '乙'])
})
test('重复创建恢复不重复提取，手动重新提取可更新', async () => {
  const { io, writes } = fixture()
  await extractTitleBlockRecord('a', false, io)
  await extractTitleBlockRecord('a', false, io)
  assert.equal(writes.length, 1)
  await extractTitleBlockRecord('a', true, io)
  assert.equal(writes.length, 2)
})
test('强制读取已有有效快照时仍调用 CAD 解析，普通补齐可复用快照', async () => {
  const { io, record, writes } = fixture()
  record.payload = { spaces: [{ id: 'model', fields: [{ key: 'name', value: '旧名称' }] }] }
  let reads = 0
  io.parse = async () => {
    reads++
    return { spaces: [{ id: 'model', fields: [{ key: 'name', value: '新名称' }] }] }
  }
  const cached = await extractTitleBlockRecord('a', false, io)
  assert.equal(reads, 0)
  assert.equal(cached.payload.spaces[0].fields[0].value, '旧名称')
  const refreshed = await extractTitleBlockRecord('a', true, io)
  assert.equal(reads, 1)
  assert.equal(writes.length, 1)
  assert.equal(refreshed.payload.spaces[0].fields[0].value, '新名称')
})

test('解析失败保存失败状态，下一次重试可以恢复', async () => {
  const { io, record } = fixture()
  const parse = io.parse
  io.parse = async () => { throw new Error('转换未完成') }
  await assert.rejects(extractTitleBlockRecord('a', false, io), /转换未完成/)
  assert.equal(record.payload.error, '转换未完成')
  io.parse = parse
  await extractTitleBlockRecord('a', false, io)
  assert.equal(record.payload.error, undefined)
})
test('权限拒绝与保存冲突不能作为保存成功', async () => {
  const { io, record, writes } = fixture()
  record.canWrite = false
  await assert.rejects(extractTitleBlockRecord('a', false, io), /权限/)
  assert.equal(writes.length, 0)
  record.canWrite = true
  io.save = async () => { throw new Error('文件版本已变化') }
  await assert.rejects(extractTitleBlockRecord('a', false, io), /版本已变化/)
  assert.equal(record.payload, null)
})
test('CAD 源版本冲突不保存失败快照', async () => {
  const { io, record, writes } = fixture()
  const conflict = Object.assign(new Error('文件版本已变化，请重新加载'), { status: 409 })
  io.parse = async () => { throw conflict }
  await assert.rejects(extractTitleBlockRecord('a', false, io), /文件版本已变化/)
  assert.equal(writes.length, 0)
  assert.equal(record.payload, null)
})
test('EXB 转换中不请求 CAD 源，不写失败快照；转换后可继续提取', async () => {
  const { io, record, writes } = fixture()
  record.fileName = '空白.exb'
  let parses = 0
  const parse = io.parse
  io.parse = async (...args) => { parses++; return parse(...args) }
  await assert.rejects(extractTitleBlockRecord('a', false, io), /正在转换/)
  assert.equal(parses, 0)
  assert.equal(writes.length, 0)
  record.fileName = '空白.dwg'
  record.versionId = 'v2'
  await extractTitleBlockRecord('a', false, io)
  assert.equal(writes[0].version, 'v2')
})
test('源文件版本发生切换后重新读取并解析新版，不将旧结果写入新版', async () => {
  const { io, record, writes } = fixture()
  const versions = []
  io.parse = async (id, version) => {
    versions.push(version)
    if (version === 'v1') {
      record.versionId = 'v2'
      throw Object.assign(new Error('版本变化'), { status: 409 })
    }
    return { spaces: [] }
  }
  await extractTitleBlockRecord('a', false, io)
  assert.deepEqual(versions, ['v1', 'v2'])
  assert.equal(writes.length, 1)
  assert.equal(writes[0].version, 'v2')
})
test('保存后读回发现版本变化时必须报告冲突，不能当作成功', async () => {
  const { io } = fixture()
  let loads = 0
  io.load = async () => ({
    attachmentId: 'a',
    versionId: ++loads === 1 ? 'v1' : 'v2',
    canWrite: true,
    payload: null,
    snapshotRevision: 1,
  })
  await assert.rejects(extractTitleBlockRecord('a', true, io), (error) => error.status === 409)
})
test('历史标题栏 null 集合会标准化为可迭代数组', () => {
  const payload = normalizeTitlePayload({
    spaces: [{ id: 'model', name: '模型', fields: [{ key: 'name', candidates: null }], warnings: null }],
  })
  assert.deepEqual(payload.spaces[0].fields[0].candidates, [])
  assert.deepEqual(payload.spaces[0].warnings, [])
})
test('批量创建去重和过滤非 CAD，单个失败不影响其他文件保存', async () => {
  const called = [], progress = []
  const failures = await extractTitleBlockBatch([
    { id: 'a', name: '总图.exb' }, { id: 'a', name: '总图.exb' }, { id: 'b', name: '零件.dwg' }, { id: 'c', name: '说明.pdf' },
  ], async id => { called.push(id); if (id === 'a') throw new Error('读取失败') }, (done, total) => progress.push([done, total]))
  assert.deepEqual(called, ['a', 'b'])
  assert.deepEqual(failures, ['总图.exb：读取失败'])
  assert.deepEqual(progress.at(-1), [2, 2])
})
