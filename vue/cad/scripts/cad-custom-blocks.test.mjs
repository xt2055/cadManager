import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import { LibreDwg, Dwg_File_Type } from '@mlightcad/libredwg-web'
import { normalizeCadDxfBlockReferences, restoreCadDwgCustomBlocks } from '../src/services/cad-custom-blocks.ts'

const require = createRequire(import.meta.url)
const { AcDbProxyGraphic, AcCmColor, AcDbNativeDxfConverter, AcDbDatabase, AcDbHostApplicationServices } = require('@mlightcad/data-model')
const { AcDbLibreDwgConverter } = require('@mlightcad/libredwg-converter')
const fixture = name => readFileSync(new URL(`../../../test/cs/${name}`, import.meta.url))
const buffer = bytes => bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)

test('DXF 仅兼容有标准块引用数据的自定义实体，保持原编码与普通实体', () => {
  const source = fixture('工程图文档1.dxf')
  const normalized = Buffer.from(normalizeCadDxfBlockReferences(buffer(source)))
  const expected = Buffer.from(source.toString('latin1').replace(/(\r?\n)ACIDBLOCKREFERENCE(\r?\n)(?=\s*5\s*\r?\n)/g, '$1INSERT$2'), 'latin1')
  assert.equal(Buffer.compare(normalized, expected), 0)
  assert.notEqual(Buffer.compare(normalized, source), 0)
  const once = buffer(normalized)
  assert.equal(normalizeCadDxfBlockReferences(once), once)
  for (const text of ['0\nLINE\n8\n0\n', '0\nACIDBLOCKREFERENCE\n100\nOtherClass\n2\n*X3\n']) {
    const original = buffer(Buffer.from(text))
    assert.equal(normalizeCadDxfBlockReferences(original), original)
  }
})

test('样例 DXF 经过原生转换器后保留嵌套引用和四条圆弧', async () => {
  const db = new AcDbDatabase()
  AcDbHostApplicationServices.instance.workingDatabase = db
  const converter = new AcDbNativeDxfConverter()
  await converter.read(normalizeCadDxfBlockReferences(buffer(fixture('工程图文档1.dxf'))), db)
  const blocks = db.tables.blockTable
  const detail = blocks.getAt('*X4')
  assert.ok(detail)
  const entities = [...detail.newIterator()]
  const reference = entities.find(entity => entity.dxfTypeName === 'INSERT')
  assert.equal(reference.blockName, '*X3')
  assert.equal([...blocks.getAt('*X3').newIterator()].filter(entity => entity.dxfTypeName === 'ARC').length, 4)
  const outer = [...blocks.modelSpace.newIterator()].find(entity => entity.blockName === '*X4')
  assert.equal(outer.scaleFactors.x, 2)
  assert.equal(outer.scaleFactors.y, 2)
  assert.ok(Math.abs(outer.rotation - 314.5060831013813 * Math.PI / 180) < 1e-9)
})

test('样例 DWG 恢复所属块内的代理并由实际绘图解码器生成四条圆弧', async () => {
  const lib = await LibreDwg.create('./public/assets/')
  const ptr = lib.dwg_read_data(buffer(fixture('工程图文档1.dwg')), Dwg_File_Type.DWG)
  assert.ok(ptr)
  try {
    const { database, stats } = lib.convertEx(ptr)
    assert.equal(stats.unknownEntityCount, 1)
    const detail = database.tables.BLOCK_RECORD.entries.find(block => block.name === '*X4')
    assert.deepEqual(detail.entities.map(entity => entity.type), ['CIRCLE'])
    const outer = structuredClone(database.entities)
    assert.equal(restoreCadDwgCustomBlocks(lib, ptr, database), 1)
    assert.equal(restoreCadDwgCustomBlocks(lib, ptr, database), 0)
    assert.deepEqual(database.entities, outer)
    const proxy = detail.entities.find(entity => entity.type === 'ACAD_PROXY_ENTITY')
    assert.equal(proxy.ownerBlockRecordSoftId, detail.handle)
    const arcs = []
    const renderer = {
      subEntityTraits: { color: new AcCmColor() },
      circularArc(arc) { arcs.push(arc); return arc },
      group(entities) { return entities },
    }
    const graphic = new AcDbProxyGraphic(Uint8Array.from(Buffer.from(proxy.graphicsData, 'hex')))
    assert.equal(graphic.worldDraw(renderer).length, 4)
    const sourceArcs = database.tables.BLOCK_RECORD.entries.find(block => block.name === '*X3').entities
    for (let i = 0; i < 4; i++) {
      assert.ok(Math.abs(arcs[i].radius - sourceArcs[i].radius) < 1e-9)
      assert.ok(Math.abs(arcs[i].center.x - sourceArcs[i].center.x) < 1e-9)
      assert.ok(Math.abs(arcs[i].center.y - sourceArcs[i].center.y) < 1e-9)
    }
    const db = new AcDbDatabase()
    AcDbHostApplicationServices.instance.workingDatabase = db
    const converter = new AcDbLibreDwgConverter({ convertByEntityType: false })
    converter.parse = async () => ({ model: database, data: { unknownEntityCount: 0 } })
    await converter.read(new ArrayBuffer(0), db)
    const outerReference = [...db.tables.blockTable.modelSpace.newIterator()].find(entity => entity.blockName === '*X4')
    const movedX = outerReference.position.x + 100
    outerReference.position = { x: movedX, y: outerReference.position.y, z: outerReference.position.z }
    const saved = db.dxfOut()
    assert.equal(typeof saved, 'string')
    const reopened = new AcDbDatabase()
    AcDbHostApplicationServices.instance.workingDatabase = reopened
    await new AcDbNativeDxfConverter().read(buffer(Buffer.from(saved)), reopened)
    const reopenedReference = [...reopened.tables.blockTable.modelSpace.newIterator()].find(entity => entity.blockName === '*X4')
    assert.ok(Math.abs(reopenedReference.position.x - movedX) < 1e-6)
    assert.equal(reopenedReference.scaleFactors.x, 2)
    assert.ok(Math.abs(reopenedReference.rotation - outerReference.rotation) < 1e-9)
    const reopenedProxy = [...reopened.tables.blockTable.getAt('*X4').newIterator()]
      .find(entity => entity.dxfTypeName === 'ACAD_PROXY_ENTITY')
    assert.ok(reopenedProxy)
    assert.equal(reopenedProxy.subWorldDraw(renderer).length, 4)
  } finally {
    lib.dwg_free(ptr)
  }
})
