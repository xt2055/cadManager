import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { canonicalDxf, compareEntities, snapshotDrawing as snapshot } from '../src/features/drawings/detail-tabs/preview/cad-compare.ts'
import { compareVersionOptions, compareSourcePath } from '../src/features/drawings/detail-tabs/preview/cad-compare-sources.ts'
const require = createRequire(import.meta.url)
const { AcDbDatabase, AcDbDxfFiler, AcDbLine, AcDbCircle, AcDbBlockTableRecord, AcDbBlockReference, AcDbLayerTableRecord } = require('@mlightcad/data-model')
const snapshotDrawing = database => snapshot(database, () => new AcDbDxfFiler({ database, precision: 10 }))

test('缸头直径公差标注仅排版变化不算修改，公差变化仍检出', () => {
  const dimension = text => `100\nAcDbDimension\n1\n${text}\n11\n-28.9161\n21\n-3.20132\n100\nAcDbAlignedDimension\n13\n-16.144475\n`
  const old = dimension('\\A1;%%C50{\\H1x;%%P0.012}')
  assert.equal(canonicalDxf(old), canonicalDxf(dimension('%%C50%%P0.012')))
  assert.notEqual(canonicalDxf(old), canonicalDxf(dimension('%%C50%%P0.013')))
  assert.notEqual(canonicalDxf(old), canonicalDxf(dimension('%%C51%%P0.012')))
  assert.notEqual(canonicalDxf(dimension('\\S+0.01^-0.02;')), canonicalDxf(dimension('\\S+0.01^-0.03;')))
})

test('字体排版和线型不参与比较，文字内容仍参与', () => {
  const a = '100\nAcDbEntity\n6\nContinuous\n370\n25\n100\nAcDbMText\n10\n1\n20\n2\n7\nFontA\n40\n3\n1\n{\\H2x;25±0.01}\n'
  const b = a.replace('Continuous', 'DASHED').replace('370\n25', '370\n50').replace('FontA', 'FontB').replace('40\n3', '40\n6').replace('\\H2x;', '\\H3x;')
  assert.equal(canonicalDxf(a), canonicalDxf(b))
  assert.notEqual(canonicalDxf(a), canonicalDxf(b.replace('25±0.01', '25±0.02')))
})

test('模型空间和块内填充不产生差异，几何线保留', () => {
  const ignored = { dxfTypeName: 'HATCH', dxfOut() { throw new Error('不应序列化填充') } }
  const block = { name: 'block', origin: { x: 0, y: 0, z: 0 }, newIterator: () => [ignored] }
  const insert = { objectId: '1', dxfTypeName: 'INSERT', blockName: 'block', layer: '0', dxfOut(filer) { filer.text = '100\nAcDbBlockReference\n2\nblock\n' } }
  const db = { tables: { blockTable: { newIterator: () => [block], modelSpace: { newIterator: () => [ignored, insert] } }, layerTable: { getAt: () => undefined } } }
  const filer = () => ({ text: '', toString() { return this.text } })
  const before = snapshot(db, filer)
  block.newIterator = () => []
  assert.equal(before.length, 1)
  assert.deepEqual(compareEntities(before, snapshot(db, filer)), [])
})

test('显式线宽不受未继承的图层线宽变化影响', () => {
  const a = line(); a.lineWeight = 25
  const b = line(); b.lineWeight = 25
  const oldDb = drawing(a); const newDb = drawing(b)
  oldDb.tables.layerTable.add(new AcDbLayerTableRecord({ name: '0', lineWeight: 9 }))
  newDb.tables.layerTable.add(new AcDbLayerTableRecord({ name: '0', lineWeight: 5 }))
  assert.deepEqual(compareEntities(snapshotDrawing(oldDb), snapshotDrawing(newDb)), [])
  b.lineWeight = 50
  assert.equal(compareEntities(snapshotDrawing(oldDb), snapshotDrawing(newDb)).length, 0)
})

test('设计内容对比忽略图层线宽变化', () => {
  const a = line(); a.lineWeight = -1
  const b = line(); b.lineWeight = -1
  const oldDb = drawing(a); const newDb = drawing(b)
  oldDb.tables.layerTable.add(new AcDbLayerTableRecord({ name: '0', lineWeight: 9 }))
  newDb.tables.layerTable.add(new AcDbLayerTableRecord({ name: '0', lineWeight: 5 }))
  assert.equal(compareEntities(snapshotDrawing(oldDb), snapshotDrawing(newDb)).length, 0)
})

const versionFile = { id: 'file-a', name: '总图.exb', version: '1.1', storageKey: 'a/original.exb', currentStorageKey: 'a/v1.1.exb', history: [] }
const versionRecords = [
  { id: 'a-1.0', version: '1.0', storageKey: 'a/v1.0.exb', createdAt: '2026-09-01' },
  { id: 'a-1.1', version: '1.1', storageKey: 'a/v1.1.exb', createdAt: '2026-09-08' },
]
test('同一图纸 1.0 与 1.1 按各自版本 ID 精确读取，当前版本去重', () => {
  const options = compareVersionOptions(versionFile, versionRecords)
  assert.equal(options.length, 2)
  assert.equal(options[0].label, '1.1（当前）')
  assert.equal(compareSourcePath('file-a', options[0]), '/file-versions/a-1.1/source')
  assert.equal(compareSourcePath('file-a', options[1]), '/file-versions/a-1.0/source')
})
test('跨图纸相同版本号保持各自独立的版本 ID', () => {
  const left = compareVersionOptions(versionFile, versionRecords)[1]
  const right = compareVersionOptions({ ...versionFile, id: 'file-b', currentStorageKey: 'b/current.exb' }, [{ id: 'b-1.0', version: '1.0', storageKey: 'b/v1.0.exb', createdAt: '2026-09-02' }])[1]
  assert.equal(compareSourcePath('file-a', left), '/file-versions/a-1.0/source')
  assert.equal(compareSourcePath('file-b', right), '/file-versions/b-1.0/source')
})
test('旧历史仅有存储键时精确读取，缺失时禁止回退当前图纸', () => {
  const options = compareVersionOptions({ ...versionFile, history: [{ name: '旧图.exb', version: '1.0', storageKey: '旧图/a 1.exb', uploadedAt: '2026-09-01' }] }, [])
  assert.equal(compareSourcePath('file-a', options[1]), `/file-versions/source?storageKey=${encodeURIComponent('旧图/a 1.exb')}`)
  assert.throws(() => compareSourcePath('file-a', { current: false }), /没有可读取/)
  assert.equal(compareSourcePath('file-a', options[0]), '/cad/source?attachmentId=file-a')
})
test('历史版本按时间排序，不按 1.9、1.10 的字符串大小排序', () => {
  const options = compareVersionOptions(versionFile, [
    { id: 'old', version: '1.9', storageKey: 'old', createdAt: '2026-08-01' },
    { id: 'new', version: '1.10', storageKey: 'new', createdAt: '2026-09-01' },
  ])
  assert.equal(options[1].versionId, 'new')
})

const line = (end = 10) => new AcDbLine({ x: 0, y: 0, z: 0 }, { x: end, y: 10, z: 0 })
function drawing(...entities) {
  const db = new AcDbDatabase()
  entities.forEach(entity => db.tables.blockTable.modelSpace.appendEntity(entity))
  return db
}

test('相同图纸、实体顺序和存储标识变化不会产生差异', () => {
  const before = snapshotDrawing(drawing(line(), line(20)))
  const after = snapshotDrawing(drawing(line(20), line()))
  after.forEach((entity, i) => { entity.id = `new-${i}` })
  assert.deepEqual(compareEntities(before, after), [])
})

test('重复图形按数量匹配，并识别新增、删除和修改', () => {
  const before = snapshotDrawing(drawing(line(), line(), line(20)))
  const after = before.map(entity => ({ ...entity }))
  after.pop()
  after[1].signature += 'changed'
  after.push({ ...before[0], id: 'new', signature: 'new' })
  const result = compareEntities(before, after)
  assert.deepEqual(result.map(diff => diff.kind), ['modified', 'deleted', 'added'])
  assert.equal(result[0].before.id, before[1].id)
})

test('几何坐标变化产生修改并提供定位边界', () => {
  const before = snapshotDrawing(drawing(line(10)))
  const after = snapshotDrawing(drawing(line(11)))
  after[0].id = before[0].id
  const [diff] = compareEntities(before, after)
  assert.equal(diff.kind, 'modified')
  assert.equal(diff.after.bounds.maxX, 11)
})

test('忽略文件句柄及浮点噪声，但保留文字、重复坐标和填充参数', () => {
  assert.equal(canonicalDxf('5\nA\n10\n1.00000001\n1\n文字\n'), canonicalDxf('5\nB\n10\n1\n1\n文字\n'))
  assert.notEqual(canonicalDxf('1\n文字A\n'), canonicalDxf('1\n文字B\n'))
  assert.notEqual(canonicalDxf('10\n1\n10\n2\n'), canonicalDxf('10\n1\n10\n3\n'))
})

test('同名块内部几何变化，即使 INSERT 未改变也能识别', () => {
  function blockDrawing(end) {
    const db = drawing()
    const block = new AcDbBlockTableRecord()
    block.name = 'symbol'
    db.tables.blockTable.add(block)
    block.appendEntity(line(end))
    const insert = new AcDbBlockReference('symbol')
    db.tables.blockTable.modelSpace.appendEntity(insert)
    return snapshotDrawing(db)
  }
  const before = blockDrawing(10)
  const after = blockDrawing(12)
  after[0].id = before[0].id
  assert.equal(compareEntities(before, after)[0]?.kind, 'modified')
})

function renamedBlockDrawing(name, end = 10, originX = 0) {
  const db = drawing()
  const block = new AcDbBlockTableRecord()
  block.name = name
  block.origin.x = originX
  db.tables.blockTable.add(block)
  block.appendEntity(line(end))
  db.tables.blockTable.modelSpace.appendEntity(new AcDbBlockReference(name))
  return db
}

test('块重命名且新增两个圆时，只报告两个新增圆', () => {
  const before = renamedBlockDrawing('*U1')
  const after = renamedBlockDrawing('*U27')
  for (const radius of [10, 20]) {
    after.tables.blockTable.modelSpace.appendEntity(new AcDbCircle({ x: 30, y: 40, z: 0 }, radius))
  }
  const result = compareEntities(snapshotDrawing(before), snapshotDrawing(after))
  assert.deepEqual(result.map(diff => [diff.kind, diff.after?.type]), [['added', 'CIRCLE'], ['added', 'CIRCLE']])
})

test('块重命名不能掩盖块内几何变化或基点变化', () => {
  const before = snapshotDrawing(renamedBlockDrawing('*U1'))
  for (const after of [renamedBlockDrawing('*U2', 12), renamedBlockDrawing('*U3', 10, 5)]) {
    assert.notEqual(before[0].signature, snapshotDrawing(after)[0].signature)
  }
})

test('未能解析的块名不能被忽略，文字内容不受块名归一化影响', () => {
  assert.notEqual(canonicalDxf('0\nINSERT\n2\nmissing-a\n'), canonicalDxf('0\nINSERT\n2\nmissing-b\n'))
  assert.notEqual(canonicalDxf('1\n*U1\n', '*U1'), canonicalDxf('1\n*U2\n', '*U2'))
})

test('嵌套块同时重命名不产生差异，插入位置改变仍产生差异', () => {
  function nested(name, offset = 0) {
    const db = renamedBlockDrawing(name)
    const outer = new AcDbBlockTableRecord()
    outer.name = `${name}-outer`
    db.tables.blockTable.add(outer)
    outer.appendEntity(new AcDbBlockReference(name))
    const insert = new AcDbBlockReference(outer.name)
    insert.position.x = offset
    db.tables.blockTable.modelSpace.appendEntity(insert)
    return snapshotDrawing(db)
  }
  assert.deepEqual(compareEntities(nested('*U1'), nested('*U9')), [])
  assert.notDeepEqual(compareEntities(nested('*U1'), nested('*U9', 5)), [])
})

const dxfEntity = (text, id = 'a', type = 'SPLINE') => ({ id, type, layer: '0', signature: canonicalDxf(text) })
test('块内文字内容比较忽略排版，但保留数字、公差和转义符', () => {
  const text = (value, x = 1, width = 10) => `100\nAcDbMText\n10\n${x}\n41\n${width}\n1\n${value}\n`
  const signature = value => canonicalDxf(value, undefined, true)
  assert.equal(signature(text('{\\W0.425298;数量}')), signature(text('{\\W0.422936;数量}', 2, 20)))
  assert.notEqual(signature(text('数量1')), signature(text('数量2')))
  assert.notEqual(signature(text('\\S+0.03^-0.05;')), signature(text('\\S+0.04^-0.05;')))
  assert.notEqual(signature(text('\\{数量\\}')), signature(text('数量')))
  assert.notEqual(signature(text('\\\\W1;数量')), signature(text('\\\\W2;数量')))
  assert.notEqual(canonicalDxf(text('数量', 1)), canonicalDxf(text('数量', 2)))
})
test('样条节点整体平移缩放不改变曲线，节点比例改变仍报告差异', () => {
  const spline = knots => '100\nAcDbSpline\n71\n3\n' + knots.map(n => `40\n${n}\n`).join('') + '10\n1\n20\n2\n'
  const before = [dxfEntity(spline([0, 0, 7.306691, 14.046357, 14.046357]))]
  assert.deepEqual(compareEntities(before, [dxfEntity(spline([0, 0, 4.738841, 9.109933, 9.109933]), 'b')]), [])
  assert.notDeepEqual(compareEntities(before, [dxfEntity(spline([0, 0, 4, 9.109933, 9.109933]), 'b')]), [])
})

test('对齐文字忽略重算的起点，但锚点、内容和拉伸文字起点仍须比较', () => {
  const text = (x, anchor = 30, h = 1, value = '图名') => `100\nAcDbText\n10\n${x}\n20\n0\n1\n${value}\n72\n${h}\n73\n2\n11\n${anchor}\n21\n0\n`
  assert.equal(canonicalDxf(text(139.999866)), canonicalDxf(text(139.980421)))
  assert.notEqual(canonicalDxf(text(139)), canonicalDxf(text(139, 31)))
  assert.notEqual(canonicalDxf(text(139, 30, 3)), canonicalDxf(text(140, 30, 3)))
  assert.notEqual(canonicalDxf(text(139, 30, 1, '[1]')), canonicalDxf(text(139, 30, 1, '[1.000001]')))
})

test('圆弧及填充角度整圈等价，舍入边界噪声不应变成增删', () => {
  assert.equal(canonicalDxf('50\n360\n'), canonicalDxf('50\n0\n'))
  const before = [dxfEntity('100\nAcDbArc\n51\n142.282794\n', 'a', 'ARC')]
  assert.deepEqual(compareEntities(before, [dxfEntity('100\nAcDbArc\n51\n142.282795\n', 'b', 'ARC')]), [])
  assert.notDeepEqual(compareEntities(before, [dxfEntity('100\nAcDbArc\n51\n142.283794\n', 'b', 'ARC')]), [])
})

test('相同包围框不代表内容相同，只用于唯一配对为修改', () => {
  const bounds = { minX: 0, minY: 0, maxX: 10, maxY: 10 }
  const before = [{ ...dxfEntity('1\n旧文字\n', 'a', 'INSERT'), bounds }]
  const after = [{ ...dxfEntity('1\n新文字\n', 'b', 'INSERT'), bounds }]
  assert.deepEqual(compareEntities(before, after).map(d => d.kind), ['modified'])
})
