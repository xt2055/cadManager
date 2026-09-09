import test from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
const require = createRequire(import.meta.url)
const { AcDbDatabase, AcDbText, AcDbMText, AcDbBlockTableRecord, AcDbBlockReference, AcDbAttribute } = require('@mlightcad/data-model')
import { cleanCadText, collectTitleSpaces, extractTitleFields } from '../src/features/drawings/detail-tabs/preview/cad-title-block.ts'

const text = (value, x, y, extra = {}) => ({ text: value, x, y, height: 10, angle: 0, ...extra })
const fields = texts => Object.fromEntries(extractTitleFields(texts).map(field => [field.key, field]))

test('清理多行文字格式但保留图号、中文与转义字符', () => {
  assert.equal(cleanCadText('{\\fSimSun;整体\\P导向套}'), '整体 导向套')
  assert.equal(cleanCadText('\\U+8BBE\\U+8BA1'), '设计')
  assert.equal(cleanCadText('JG9055e-50/32-04'), 'JG9055e-50/32-04')
  assert.equal(cleanCadText('\\{A\\} %%d \\S1#2;'), '{A} ° 1/2')
})

test('按机械标题栏布局提取无标签的图号、名称和材料及人员', () => {
  const result = fields([
    text('设 计', 0, 0), text('张定', 55, 0), text('标准', 115, 0), text('宋某', 165, 0),
    text('校对', 0, -25), text('牛某', 55, -25), text('批准', 115, -25), text('王超', 165, -25),
    text('工艺', 0, -50), text('杨某', 55, -50), text('日期', 115, -50),
    text('整体导向套', 275, 50, { height: 24 }), text('27SiMn', 325, -40),
    text('JG9055e-50/32-04', 525, -50), text('泸州市巨力液压有限公司', 500, 80),
    text('比例', 620, 45), text('1.2:1', 620, 15), text('技术要求', 300, 500),
  ])
  assert.equal(result.number.value, 'JG9055e-50/32-04')
  assert.equal(result.name.value, '整体导向套')
  assert.equal(result.material.value, '27SiMn')
  assert.equal(result.designer.value, '张定')
  assert.equal(result.standard.value, '宋某')
  assert.equal(result.approver.value, '王超')
  assert.equal(result.company.value, '泸州市巨力液压有限公司')
  assert.equal(result.scale.value, '1.2:1')
  assert.equal(result.date.value, '')
})

test('空白签名不越过下一个标签，多个设计签名不猜测', () => {
  assert.equal(fields([text('设计', 0, 0), text('标准', 100, 0), text('张某', 140, 0)]).designer.value, '')
  const result = fields([text('设计', 0, 0), text('甲', 50, 0), text('设计', 0, 100), text('乙', 50, 100)])
  assert.equal(result.designer.value, '')
  assert.deepEqual(result.designer.candidates, ['甲', '乙'])
})

test('块属性优先，支持行内标签，纯几何图纸不编造字段', () => {
  const result = fields([text('设计', 0, 0), text('旧名', 50, 0), text('新名', 0, 100, { tag: 'DESIGNED_BY' }), text('图号：JG-01-02', 0, 200)])
  assert.equal(result.designer.value, '新名')
  assert.equal(result.designer.source, '块属性')
  assert.equal(result.number.value, 'JG-01-02')
  assert.ok(extractTitleFields([]).every(field => field.value === ''))
})

test('CAXA 实际字段名可提取材料、单位与签名，不将审核署名当成批准', () => {
  const result = fields([
    text('27SiMn', 0, 100, { tag: '材料名称' }),
    text('泸州市巨力液压有限公司', 0, 200, { tag: '单位名称' }),
    text('宋洪钊', 0, 300, { tag: '标准化_人员编号' }),
    text('张定', 0, 400, { tag: '设计_人员编号' }),
    text('1.2:1', 0, 500, { tag: '图纸比例' }),
    text('校对', 0, 0), text('牟太有', 50, 0, { tag: '审核_人员编号' }),
    text('王超', 0, 600, { tag: '批准_人员编号' }),
  ])
  assert.equal(result.material.value, '27SiMn')
  assert.equal(result.company.value, '泸州市巨力液压有限公司')
  assert.equal(result.standard.value, '宋洪钊')
  assert.equal(result.designer.source, '块属性')
  assert.equal(result.scale.value, '1.2:1')
  assert.equal(result.checker.value, '牟太有')
  assert.equal(result.approver.value, '王超')
})

test('旋转后的标签与值仍可配对', () => {
  assert.equal(fields([text('设计', 10, 10, { angle: Math.PI / 2 }), text('张某', 10, 60, { angle: Math.PI / 2 })]).designer.value, '张某')
})

test('真实 CAD 数据库能遍历文字、块和附属属性，且不修改实体', () => {
  const db = new AcDbDatabase()
  const title = new AcDbBlockTableRecord()
  title.name = 'title'
  db.tables.blockTable.add(title)
  const label = new AcDbText()
  label.textString = '设计'
  label.height = 10
  label.position = { x: 0, y: 0, z: 0 }
  title.appendEntity(label)
  const insert = new AcDbBlockReference('title')
  insert.position = { x: 100, y: 200, z: 0 }
  db.tables.blockTable.modelSpace.appendEntity(insert)
  const attribute = new AcDbAttribute()
  attribute.tag = '设计'
  attribute.textString = '张某'
  attribute.height = 10
  attribute.position = { x: 150, y: 200, z: 0 }
  insert.appendAttributes(attribute)
  const name = new AcDbMText()
  name.contents = '图纸名称：整体导向套'
  name.height = 10
  name.location = { x: 300, y: 250, z: 0 }
  db.tables.blockTable.modelSpace.appendEntity(name)
  const spaces = collectTitleSpaces(db)
  const model = spaces.find(space => space.id === String(db.tables.blockTable.modelSpace.objectId))
  assert.ok(model)
  assert.equal(fields(model.texts).designer.value, '张某')
  assert.equal(fields(model.texts).name.value, '整体导向套')
  assert.equal(model.texts.find(item => item.text === '设计').x, 100)
  assert.equal(label.position.x, 0)
  assert.equal(attribute.position.x, 150)
  assert.ok(!spaces.some(space => space.name === 'title'))
})

const matrix = (a, b, c, d, x, y) => ({ elements: [a, b, 0, 0, c, d, 0, 0, 0, 0, 1, 0, x, y, 0, 1] })
const entity = (value, x, y, extra = {}) => ({ dxfTypeName: 'TEXT', textString: value, position: { x, y, z: 0 }, height: 10, rotation: 0, ...extra })
const block = (name, entities, id = name) => ({ name, objectId: id, newIterator: () => entities })
const database = blocks => ({ tables: { blockTable: { newIterator: () => blocks } } })

test('嵌套块应用旋转、缩放和平移，属性不重复应用所属插入变换', () => {
  const leaf = block('leaf', [entity('设计', 1, 2), entity('默认提示', 0, 0, { dxfTypeName: 'ATTDEF', isConst: false })])
  const parent = block('parent', [{ dxfTypeName: 'INSERT', blockName: 'leaf', blockTransform: matrix(2, 0, 0, 2, 10, 0), attributeIterator: () => [entity('张某', 30, 0, { tag: '设计' })] }])
  const model = block('*Model_Space', [{ dxfTypeName: 'INSERT', blockName: 'parent', blockTransform: matrix(0, 1, -1, 0, 100, 200) }])
  const result = collectTitleSpaces(database([model, parent, leaf]))
  assert.equal(result.length, 1)
  const label = result[0].texts.find(item => item.text === '设计')
  assert.equal(label.x, 96)
  assert.equal(label.y, 212)
  assert.equal(label.height, 20)
  assert.equal(label.angle, Math.PI / 2)
  const attribute = result[0].texts.find(item => item.tag)
  assert.equal(attribute.x, 100)
  assert.equal(attribute.y, 230)
  assert.equal(result[0].texts.length, 2)
})

test('跳过非标准平面块时明确警告，循环块引用不会无限递归', () => {
  const child = block('child', [entity('图号：JG-01-02', 0, 0)])
  const model = block('*Model_Space', [{ dxfTypeName: 'INSERT', blockName: 'child', normal: { x: 0, y: 1, z: 0 } }])
  const result = collectTitleSpaces(database([model, child]))
  assert.equal(result[0].texts.length, 0)
  assert.equal(result[0].warnings.length, 1)
  const cyclic = block('cycle', [{ dxfTypeName: 'INSERT', blockName: 'cycle', blockTransform: matrix(1, 0, 0, 1, 0, 0) }])
  const root = block('*Model_Space', [{ dxfTypeName: 'INSERT', blockName: 'cycle', blockTransform: matrix(1, 0, 0, 1, 0, 0) }])
  assert.equal(collectTitleSpaces(database([root, cyclic]))[0].texts.length, 0)
})

test('模型与布局结果隔离，未插入的普通块不能产生候选', () => {
  const result = collectTitleSpaces(database([
    block('*Model_Space', [entity('甲', 0, 0, { tag: '设计' })]),
    block('*Paper_Space', [entity('乙', 0, 0, { tag: '设计' })]),
    block('unused', [entity('丙', 0, 0, { tag: '设计' })]),
  ]))
  assert.equal(result.length, 2)
  assert.equal(fields(result[0].texts).designer.value, '甲')
  assert.equal(fields(result[1].texts).designer.value, '乙')
})
