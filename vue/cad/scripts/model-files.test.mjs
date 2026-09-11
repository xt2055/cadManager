import assert from 'node:assert/strict'
import { test } from 'node:test'
import { acceptsModel, fileCategory, fileFormat, drawingMediaLabel, isDrawing2DFile } from '../src/utils/model-formats.ts'
import { buildChangeTargetGroups } from '../src/features/drawings/components/detail/change-targets.ts'

test('识别中望、主流 CAD 和 Creo 版本后缀，拒绝伪装扩展名', () => {
  for (const name of ['零件.Z3PRT', 'a.z3', 'a.step', 'a.sldasm', 'a.CATPart', 'a.prt.12', 'a.ASM.1', 'a.glb']) {
    assert.equal(acceptsModel(name), true, name)
    assert.equal(fileCategory({ name }), 'model3d', name)
  }
  assert.equal(fileFormat('a.prt.12'), 'prt')
  assert.equal(acceptsModel('a.z3prt.exe'), false)
  assert.equal(acceptsModel('a.prt.backup'), false)
})
test('2D 工程图和 3D 模型严格分开识别', () => {
  for (const name of ['a.exb', 'a.dwg', 'a.dxf', 'a.pdf', 'a.slddrw']) assert.equal(isDrawing2DFile({ name }), true, name)
  for (const name of ['a.step', 'a.z3prt', 'a.zip', 'a.exe']) assert.equal(isDrawing2DFile({ name }), false, name)
})

test('普通 ZIP 不计为 3D，明确上传的装配包计为 3D', () => {
  assert.equal(fileCategory({ name: '资料.zip' }), 'other')
  assert.equal(fileCategory({ name: '装配.zip', fileCategory: 'model3d' }), 'model3d')
  assert.equal(drawingMediaLabel({ files: [{ name: 'a.dwg' }], otherFiles: [{ name: 'a.z3prt' }] }), '2D + 3D')
  assert.equal(drawingMediaLabel({ files: [{ name: 'a.step' }] }), '3D')
  assert.equal(drawingMediaLabel({ files: [{ name: 'notes.txt' }] }), '未上传图纸')
})

test('变更对象递归包含总图与多级零件的全部附件，并按附件 ID 去重', () => {
  const duplicated = { id: 'part-2d', name: 'P-01.dwg' }
  const groups = buildChangeTargetGroups(
    { no: 'A-00', name: '总装', files: [{ id: 'assembly-2d', name: 'A-00.dwg' }], otherFiles: [{ id: 'assembly-3d', name: 'A-00.step' }] },
    [{
      no: 'P-01', name: '一级零件', files: [duplicated], otherFiles: [duplicated], children: [{
        no: 'P-02', name: '二级零件', files: [], otherFiles: [{ id: 'part-3d', name: 'P-02.z3prt' }], children: [],
      }],
    }],
  )

  assert.deepEqual(groups.map((group) => [group.kind, group.no]), [['总图', 'A-00'], ['零件图', 'P-01'], ['零件图', 'P-02']])
  assert.deepEqual(groups.flatMap((group) => group.files.map((file) => file.id)), ['assembly-2d', 'assembly-3d', 'part-2d', 'part-3d'])
  assert.deepEqual(groups.flatMap((group) => group.files.map((file) => file.category)), ['drawing2d', 'model3d', 'drawing2d', 'model3d'])
})
