import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'
import { LibreDwg, Dwg_File_Type } from '@mlightcad/libredwg-web'
import { snapshotDrawing, compareEntities } from '../src/features/drawings/detail-tabs/preview/cad-compare.ts'
import { exportEditorDxf } from '../src/services/cad-editor-export.ts'
const require = createRequire(import.meta.url)
const { AcDbDatabase, AcDbDatabaseConverterManager, AcDbFileType, AcDbDxfFiler, AcDbNativeDxfConverter } = require('@mlightcad/data-model')
const { AcDbLibreDwgConverter } = require('@mlightcad/libredwg-converter')
if (!process.argv[2] || !process.argv[3]) throw new Error('用法：node --experimental-strip-types scripts/cad-compare-files.mjs 旧图.dwg 新图.dwg [--expect=修改数,删除数,新增数]')
const parser = await LibreDwg.create('./node_modules/@mlightcad/libredwg-web/wasm/')
class Converter extends AcDbLibreDwgConverter {
  async parse(data) {
    const dwg = parser.dwg_read_data(data, Dwg_File_Type.DWG)
    if (!dwg) throw new Error('DWG 解析失败')
    try { return { model: parser.convert(dwg) } } finally { parser.dwg_free(dwg) }
  }
}
AcDbDatabaseConverterManager.instance.register(AcDbFileType.DWG, new Converter({ convertByEntityType: false }))
async function load(path) {
  const bytes = await readFile(path)
  console.log('source', path, bytes.subarray(0, 6).toString(), bytes.length)
  const database = new AcDbDatabase()
  try {
    await database.read(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength), { readOnly: true }, AcDbFileType.DWG)
  } catch (error) { console.error(error.message); process.exit(1) }
  if (database.lastOpenError) { console.error(database.lastOpenError.message); process.exit(1) }
  if (process.argv.includes('--roundtrip')) {
    const dxf = exportEditorDxf(database)
    const lines = String(dxf).replace(/\r/g, '').split('\n')
    const records = []
    for (let i = 0; i + 1 < lines.length; i += 2) {
      if (Number(lines[i]) === 0 || records.length === 0) records.push('')
      records[records.length - 1] += `${lines[i].trim()}\n${lines[i + 1]}\n`
    }
    const dimensions = records.filter(record => record.startsWith('0\nDIMENSION\n'))
    console.log('exported dimensions', dimensions.map(record => ({ type: record.match(/\n70\n([^\n]*)/)?.[1], subclasses: [...record.matchAll(/AcDb\w*Dimension/g)].map(m => m[0]) })))
    console.log('exported wipeouts', records.filter(record => record.startsWith('0\nWIPEOUT\n')).length)
    AcDbDatabaseConverterManager.instance.register(AcDbFileType.DXF, new AcDbNativeDxfConverter())
    const roundtrip = new AcDbDatabase()
    const bytes = new TextEncoder().encode(dxf)
    await roundtrip.read(bytes.buffer, { readOnly: true }, AcDbFileType.DXF)
    const a = snapshotDrawing(database, () => new AcDbDxfFiler({ database, precision: 10 }))
    const b = snapshotDrawing(roundtrip, () => new AcDbDxfFiler({ database: roundtrip, precision: 10 }))
    console.log('roundtrip', a.length, b.length, compareEntities(a, b).map(d => `${d.kind}:${d.before?.type ?? d.after?.type}`))
    for (const diff of compareEntities(a, b)) {
      if (!diff.before || !diff.after) continue
      let index = 0
      while (index < diff.before.signature.length && diff.before.signature[index] === diff.after.signature[index]) index++
      console.log('roundtrip-detail', diff.before.type, diff.before.id,
        diff.before.signature.slice(Math.max(0, index - 40), index + 100),
        diff.after.signature.slice(Math.max(0, index - 40), index + 100))
    }
  }
  return snapshotDrawing(database, () => new AcDbDxfFiler({ database, precision: 10 }))
}
const before = await load(process.argv[2])
const after = await load(process.argv[3])
const differences = compareEntities(before, after)
const expected = process.argv.find(arg => arg.startsWith('--expect='))
if (expected) assert.deepEqual(['modified', 'deleted', 'added'].map(kind => differences.filter(diff => diff.kind === kind).length), expected.slice(9).split(',').map(Number))
console.log('counts', before.length, after.length, 'differences', differences.length)
console.log('by-kind', Object.fromEntries(['modified', 'deleted', 'added'].map(kind => [kind, differences.filter(diff => diff.kind === kind).length])))
if (process.argv.includes('--summary')) process.exit(0)
for (const diff of differences) {
  const a = diff.before
  const b = diff.after ?? (a?.bounds && after.filter(e => e.type === a.type && e.bounds).sort((x, y) => {
    const distance = e => Math.abs(e.bounds.minX - a.bounds.minX) + Math.abs(e.bounds.minY - a.bounds.minY)
    return distance(x) - distance(y)
  })[0])
  let i = 0
  if (a && b) while (i < a.signature.length && a.signature[i] === b.signature[i]) i++
  console.log(diff.kind, a?.type ?? b?.type, a?.id, b?.id, a && b ? [a.signature.slice(Math.max(0, i - 100), i + 180), b.signature.slice(Math.max(0, i - 100), i + 180)] : '')
}
