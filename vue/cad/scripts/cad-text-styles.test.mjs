import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import { LibreDwg, Dwg_File_Type } from '@mlightcad/libredwg-web'
import { normalizeCadDwgTextStyles } from '../src/services/cad-text-styles.ts'

const require = createRequire(import.meta.url)
const { MText, FontManager } = require('@mlightcad/mtext-renderer')
const { AcDbDatabase, AcDbNativeDxfConverter, AcDbHostApplicationServices } = require('@mlightcad/data-model')
const { Box3, MeshBasicMaterial, LineBasicMaterial } = require('three')
const buffer = bytes => bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)

test('仅为空字体的 CAXA 样式提供回退，重复执行不改变数据', () => {
  const model = { tables: { APPID: { entries: [] }, STYLE: { entries: [
    { font: '', bigFont: '' }, { font: 'txt' }, { font: '', extendedFont: 'Arial' }, { font: '', bigFont: 'hztxt' },
  ] } } }
  assert.equal(normalizeCadDwgTextStyles(model), 0)
  model.tables.APPID.entries.push({ name: 'CAXA_DRAFT_TXTSTYLE' })
  assert.equal(normalizeCadDwgTextStyles(model), 1)
  assert.equal(model.tables.STYLE.entries[0].font, 'simsun')
  assert.equal(model.tables.STYLE.entries[1].font, 'txt')
  assert.equal(model.tables.STYLE.entries[2].extendedFont, 'Arial')
  assert.equal(normalizeCadDwgTextStyles(model), 0)
})

test('导向套公差不再与主尺寸重叠，保留字体以外的全部模型数据', async () => {
  const lib = await LibreDwg.create('./public/assets/')
  const ptr = lib.dwg_read_data(buffer(readFileSync('../../test/cs/JG9055d-5032-04(导向套).dwg')), Dwg_File_Type.DWG)
  try {
    const model = lib.convert(ptr)
    const original = structuredClone(model)
    const fm = FontManager.instance
    fm.enableFontCache = false
    fm.setDefaultFonts('modern')
    const fonts = JSON.parse(readFileSync('./public/cad-data/fonts/fonts.json', 'utf8'))
    for (const name of ['hztxt', 'simsun', 'CXGDT']) {
      const font = fonts.find(f => f.name.some(n => n.toLowerCase() === name.toLowerCase()))
      assert.ok(font)
      await fm.cacheFont(buffer(readFileSync('./public/cad-data/fonts/' + font.file)), font.file, font.name)
    }
    const materials = { getMeshBasicMaterial: () => new MeshBasicMaterial(), getLineBasicMaterial: () => new LineBasicMaterial() }
    const bounds = (entity, style) => {
      const text = new MText({ text: entity.text, height: entity.textHeight, width: entity.rectWidth,
        position: entity.insertionPoint, attachmentPoint: entity.attachmentPoint,
        directionVector: entity.direction, lineSpaceFactor: entity.lineSpacing }, style, materials, fm)
      text.syncDraw()
      const box = new Box3().setFromObject(text)
      text.dispose()
      return box
    }
    const detail = model.tables.BLOCK_RECORD.entries.find(b => b.name === '*X60')
    const texts = detail.entities.filter(e => e.type === 'MTEXT')
    assert.deepEqual(texts.map(e => e.text), ['\\T1.1;2-6.2', '\\T1.1;{\\H0.707107x;+0.2}', '\\T1.1;{\\H0.707107x;0}'])
    const style = model.tables.STYLE.entries.find(s => s.name === texts[0].styleName)
    assert.ok(bounds(texts[0], style).intersectsBox(bounds(texts[2], style)), '复现旧字体导致的主尺寸/下偏差重叠')
    assert.equal(normalizeCadDwgTextStyles(model), 4)
    const boxes = texts.map(e => bounds(e, style))
    assert.ok(!boxes[0].intersectsBox(boxes[1]))
    assert.ok(!boxes[0].intersectsBox(boxes[2]))
    assert.ok(!boxes[1].intersectsBox(boxes[2]))
    const frame = model.tables.BLOCK_RECORD.entries.find(b => b.name === '*X104')
    const frameTexts = frame.entities.filter(e => e.type === 'MTEXT')
    assert.ok(!bounds(frameTexts[1], style).intersectsBox(bounds(frameTexts[2], style)))
    assert.equal(frameTexts[0].text, '{\\fcxgdt;\\W1;\\T1;r}')
    // 唯一变化是原本为空的 STYLE.font，不修改数值、位置、字号、宽度和块结构。
    for (let i = 0; i < model.tables.STYLE.entries.length; i++) original.tables.STYLE.entries[i].font = model.tables.STYLE.entries[i].font
    assert.deepEqual(model, original)

    const db = new AcDbDatabase()
    AcDbHostApplicationServices.instance.workingDatabase = db
    await new AcDbNativeDxfConverter().read(buffer(readFileSync('../../test/cs/JG9055d-5032-04(导向套).dxf')), db)
    const dxfStyle = db.tables.textStyleTable.getAt('标准').textStyle
    assert.ok(/宋体|simsun/i.test(dxfStyle.font), 'DXF 已保留原图指定的宋体')
    const reopened = new AcDbDatabase()
    AcDbHostApplicationServices.instance.workingDatabase = reopened
    await new AcDbNativeDxfConverter().read(buffer(Buffer.from(db.dxfOut())), reopened)
    assert.equal(reopened.tables.textStyleTable.getAt('标准').textStyle.font, dxfStyle.font)
  } finally {
    lib.dwg_free(ptr)
  }
})
