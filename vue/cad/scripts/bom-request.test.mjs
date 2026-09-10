import { test } from 'node:test'
import assert from 'node:assert/strict'
import { drawingBomRequestBody } from '../src/modules/drawing/bom-request.ts'
import { normalizeBom, normalizeDrawings, normalizeMaterialFile, normalizeStructure } from '../src/services/api/data.types.ts'
import { hasGeneratedBomIds, originalMaterialWorkbook } from '../src/features/drawings/detail-tabs/material/material-print.ts'

test('BOM 替换请求使用后端严格 JSON 合同', () => {
  const body = drawingBomRequestBody({
    expectedRevision: 3,
    items: [{
      no: 1,
      id: 'local-row-id',
      drawingNo: '2000W.02.03d',
      sourceFileId: '11111111-1111-4111-8111-111111111111',
      name: '缸筒',
      spec: '27SiMn Φ180×193',
      qty: 2,
      weight: 15.5,
      remark: '',
    }],
  })

  assert.deepEqual(body, {
    expectedRevision: 3,
    items: [{
      no: 1,
      id: 'local-row-id',
      name: '缸筒',
      spec: '27SiMn Φ180×193',
      qty: 2,
      weight: 15.5,
      remark: '',
      sourceAttachmentVersionId: '11111111-1111-4111-8111-111111111111',
    }],
  })
  assert.equal('quantity' in body.items[0], false)
  assert.equal('sourceAttachmentVersion' in body.items[0], false)
})

test('BOM 响应保留附件版本来源标识', () => {
  const [item] = normalizeBom([{
    no: 1,
    id: 'bom-row-id',
    sourceAttachmentVersionId: '11111111-1111-4111-8111-111111111111',
    name: '缸筒',
    spec: '27SiMn',
    qty: 2,
    weight: 15.5,
    remark: '',
  }])

  assert.equal(item.id, 'bom-row-id')
  assert.equal(item.sourceFileId, '11111111-1111-4111-8111-111111111111')
})

test('图纸和零件接口响应保留创建人', () => {
  assert.equal(normalizeDrawings([{ no: 'D-01', name: '总图', createdBy: '张工' }])[0].createdBy, '张工')
  assert.equal(normalizeStructure([{ no: 'P-01', name: '零件', parentNo: 'D-01', createdBy: '李工' }])[0].createdBy, '李工')
})

test('备料附件响应保留编制人和当前版本标识', () => {
  const file = normalizeMaterialFile({
    id: 'attachment-id',
    name: '下料明细表.xlsx',
    author: '朱春蓉',
    currentVersionId: 'version-id',
  }, '2000W.02.03d')

  assert.equal(file.author, '朱春蓉')
  assert.equal(file.currentVersionId, 'version-id')
})

test('打印优先选择原始 xlsx 附件而不是重新生成工作簿', () => {
  const source = originalMaterialWorkbook([
    { name: '备料.csv', storageKey: 'csv-key' },
    { name: '原始下料明细表.xlsx', storageKey: 'xlsx-key' },
  ])

  assert.deepEqual(source, { name: '原始下料明细表.xlsx', storageKey: 'xlsx-key' })
})

test('识别历史 BOM 中被误当作图号的数据库 UUID', () => {
  assert.equal(hasGeneratedBomIds([{ id: '2c5df30c-d51f-43d8-8d31-123456789abc' }]), true)
  assert.equal(hasGeneratedBomIds([{ id: '2000W.02.03d-01' }]), false)
})
