import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import ts from 'typescript'

const moduleUrl = source => `data:text/javascript;base64,${Buffer.from(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText).toString('base64')}`
const dateModule = moduleUrl(await readFile(new URL('../src/utils/date-time.ts', import.meta.url), 'utf8'))
const source = await readFile(new URL('../src/modules/drawing/drawing-read-model.ts', import.meta.url), 'utf8')
const { DrawingReadModelMapper } = await import(moduleUrl(source.replace("'@/utils/date-time'", JSON.stringify(dateModule))))

const target = { id: 'd', no: '2000W.02.03d', name: '斜撑油缸' }
const borrowed = { id: 'pipe', no: '2000W.02.03c-01-3', name: '油管', parentNo: target.no, drawingId: target.id, borrowFrom: '2000W.02.03c', relationType: 'borrowed' }
function map(part = borrowed, drawings = [target], others = []) {
  return new DrawingReadModelMapper().map({ drawings, structure: [part, ...others], attachments: [], bom: [] })
}

test('来源总图未建档时保留 c，不回退为当前装配总图 d', () => {
  const result = map()
  assert.equal(result.parts[0].sourceDrawing, '2000W.02.03c')
  assert.equal(result.parts[0].parentNo, target.no)
  assert.equal(result.parts[0].borrowed, true)
  assert.equal(result.structureByDrawing[target.no][0].sourceDrawing, '2000W.02.03c')
})
test('来源总图已入库时保持明确来源', () => {
  assert.equal(map(borrowed, [target, { id: 'c', no: borrowed.borrowFrom }]).parts[0].sourceDrawing, borrowed.borrowFrom)
})
test('兼容来源为零件号的历史记录，只追溯实际来源零件', () => {
  const sourcePart = { id: 'origin', no: 'source-part', parentNo: 'origin-root', drawingId: 'origin-drawing' }
  const result = map({ ...borrowed, borrowFrom: sourcePart.no }, [target, { id: 'origin-drawing', no: 'origin-root' }], [sourcePart])
  assert.equal(result.parts[0].sourceDrawing, 'origin-root')
  assert.equal(result.parts[0].sourcePartId, 'origin')
})
test('来源零件的父链缺失时保留原始来源，不能用当前父链兜底', () => {
  const sourcePart = { id: 'origin', no: 'source-part', parentNo: 'missing', drawingId: 'origin-drawing' }
  assert.equal(map({ ...borrowed, borrowFrom: sourcePart.no }, [target], [sourcePart]).parts[0].sourceDrawing, 'source-part')
})
test('没有明确来源时不伪造借用来源', () => {
  const result = map({ ...borrowed, borrowFrom: undefined })
  assert.equal(result.parts[0].sourceDrawing, undefined)
  assert.equal(result.parts[0].borrowed, true)
})
