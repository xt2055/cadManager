import { test } from 'node:test'
import assert from 'node:assert/strict'
import { repairDimensionTypes, exportEditorDxf } from '../src/services/cad-editor-export.ts'

test('修正标注子类型并保留组码 70 高位标志', () => {
  for (const [subclass, type] of [['AcDbRadialDimension', 4], ['AcDbDiametricDimension', 3], ['AcDbAlignedDimension', 1], ['AcDb3PointAngularDimension', 5], ['AcDb2LineAngularDimension', 2], ['AcDbOrdinateDimension', 6]]) {
    const input = `0\nDIMENSION\n100\nAcDbDimension\n70\n160\n1\n25±0.01\n100\n${subclass}\n10\n12.123456789\n0\nEOF\n`
    const output = repairDimensionTypes(input)
    assert.equal(output, input.replace('70\n160', `70\n${160 | type}`))
    assert.equal(repairDimensionTypes(output), output)
  }
})

test('旋转标注不能因包含 Aligned 子类被改为对齐标注', () => {
  const input = '0\nDIMENSION\n100\nAcDbDimension\n70\n32\n100\nAcDbAlignedDimension\n100\nAcDbRotatedDimension\n0\nEOF\n'
  assert.equal(repairDimensionTypes(input), input)
})

test('其他实体、文字和重复组码不变', () => {
  const input = '0\nLWPOLYLINE\n70\n1\n10\n1\n20\n2\n10\n3\n20\n4\n0\nTEXT\n1\nDIMENSION\n70\n0\n0\nEOF\n'
  assert.equal(repairDimensionTypes(input), input)
})

test('在线导出使用十二位精度和明确的 ASCII 版本', () => {
  const database = { dxfOut(...args) {
    assert.deepEqual(args, [undefined, 12, 'AC1027', { format: 'ascii' }])
    return '0\nEOF\n'
  } }
  assert.equal(exportEditorDxf(database), '0\nEOF\n')
})
