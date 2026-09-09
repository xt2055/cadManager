import { test } from 'node:test'
import assert from 'node:assert/strict'
import { extractTitleBlockRecord, extractTitleBlockBatch } from '../src/services/title-block-workflow.ts'

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
test('权限拒绝与版本冲突不能作为保存成功', async () => {
  const { io, record, writes } = fixture()
  record.canWrite = false
  await assert.rejects(extractTitleBlockRecord('a', false, io), /权限/)
  assert.equal(writes.length, 0)
  record.canWrite = true
  io.save = async () => { throw new Error('文件版本已变化') }
  await assert.rejects(extractTitleBlockRecord('a', false, io), /版本已变化/)
  assert.equal(record.payload, null)
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
