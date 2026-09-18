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

test('图纸和零件读模型保留真实创建人', () => {
  const result = new DrawingReadModelMapper().map({
    drawings: [{ ...target, createdBy: '张工' }],
    structure: [{ ...borrowed, createdBy: '李工' }],
    bom: [], attachments: [],
  })
  assert.equal(result.drawings[0].createdBy, '张工')
  assert.equal(result.parts[0].createdBy, '李工')
})

test('工艺附件读模型保留服务端持久化的编制人员', () => {
  const result = new DrawingReadModelMapper().map({
    drawings: [target],
    structure: [],
    bom: [],
    attachments: [{
      id: 'craft-id', drawingNo: target.no, role: 'craft', name: '工艺卡.docx',
      storageKey: 'blob-key', size: 1024, mimeType: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      version: 'v1.0', previewable: true, uploadedBy: '上传用户', author: '熊焱林',
    }],
  })

  assert.equal(result.drawings[0].craftFiles[0].author, '熊焱林')
  assert.equal(result.drawings[0].craftFiles[0].scanned, true)
  assert.equal(result.drawings[0].craftFiles[0].by, '上传用户')
})

// 借用关系是「一份附件、多张图纸共享」：借用方必须能看到这张借用图（截图里的「已关联图纸文件清单」）。
const modelFormatsModule = moduleUrl(await readFile(new URL('../src/utils/model-formats.ts', import.meta.url), 'utf8'))
const previewSource = await readFile(new URL('../src/features/drawings/detail-tabs/preview/drawing-preview-files.ts', import.meta.url), 'utf8')
const { collectProjectFiles } = await import(moduleUrl(previewSource.replace("'@/utils/model-formats'", JSON.stringify(modelFormatsModule))))

const borrowedFile = {
  id: 'pipe-file', drawingNo: '2000W.02.03c', partNo: borrowed.no, role: 'part',
  name: '2000W.02.03c-01-3(油管).exb', storageKey: 'blob-key', size: 2048,
  version: 'v1.0', previewable: true, uploadedBy: '测试',
}
const sourceView = { id: 'src-pipe', no: borrowed.no, name: borrowed.name, parentNo: '2000W.02.03c', drawingId: 'c' }
function mapBorrowedView() {
  return new DrawingReadModelMapper().map({
    drawings: [target, { id: 'c', no: '2000W.02.03c' }],
    structure: [borrowed, sourceView],
    attachments: [borrowedFile],
    bom: [],
  })
}

test('借用件在借用方也拿得到借用图，来源方视图不丢', () => {
  const result = mapBorrowedView()
  assert.equal(result.parts[0].files.length, 1)
  assert.equal(result.parts[1].files.length, 1)
  assert.equal(result.structureByDrawing[target.no][0].files[0].name, borrowedFile.name)
  assert.equal(result.structureByDrawing['2000W.02.03c'][0].files[0].name, borrowedFile.name)
})

test('已关联图纸文件清单列出借用图，并标记为借用（借用方只读）', () => {
  const result = mapBorrowedView()
  const files = collectProjectFiles({
    currentItem: { id: 'd', no: target.no, name: '斜撑油缸', files: [], otherFiles: [] },
    isAssembly: true,
    rootDrawingNo: target.no,
    parts: result.parts,
    structureOf: () => [{ ...result.parts[0], children: [] }],
  })
  assert.equal(files.length, 1)
  assert.equal(files[0].partNo, borrowed.no)
  assert.equal(files[0].borrowed, true)
})

test('来源项目自己的清单不把同一张图标成借用', () => {
  const result = mapBorrowedView()
  const files = collectProjectFiles({
    currentItem: { id: 'c', no: '2000W.02.03c', name: '油管', files: [], otherFiles: [] },
    isAssembly: true,
    rootDrawingNo: '2000W.02.03c',
    parts: result.parts,
    structureOf: () => [{ ...result.parts[1], children: [] }],
  })
  assert.equal(files.length, 1)
  assert.equal(files[0].borrowed, undefined)
})
