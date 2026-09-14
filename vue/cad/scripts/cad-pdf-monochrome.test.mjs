import assert from 'node:assert/strict'
import { test } from 'node:test'
import { convertCadPixelsToMonochrome } from '../src/services/cad-pdf-monochrome.ts'

const convert = (colors, background) => {
  const data = new Uint8ClampedArray(colors.flat())
  convertCadPixelsToMonochrome(data, background)
  return Array.from(data)
}

test('黑底变白，白色图框及红绿蓝青黄品红标注均变黑', () => {
  const colors = [[0, 0, 0, 255], ...[
    [255, 255, 255], [255, 0, 0], [0, 255, 0], [0, 0, 255],
    [0, 255, 255], [255, 255, 0], [255, 0, 255],
  ].map(rgb => [...rgb, 255])]
  assert.deepEqual(convert(colors, 0), [255, 255, 255, 255, ...Array(7).fill([0, 0, 0, 255]).flat()])
})

test('白底图纸保留白纸，彩色线条转为黑色', () => {
  assert.deepEqual(convert([[255, 255, 255, 255], [0, 0, 0, 255], [0, 255, 255, 255], [255, 255, 0, 255]], 0xffffff),
    [255, 255, 255, 255, 0, 0, 0, 255, 0, 0, 0, 255, 0, 0, 0, 255])
})

test('灰色文字加深为纯黑，透明区域变为白纸', () => {
  assert.deepEqual(convert([[0, 128, 0, 255], [0, 255, 255, 0]], 0),
    [0, 0, 0, 255, 255, 255, 255, 255])
})

test('非纯黑的查看器背景也转换成纯白', () => {
  assert.deepEqual(convert([[32, 40, 48, 255], [255, 255, 255, 255]], 0x202830),
    [255, 255, 255, 255, 0, 0, 0, 255])
})

test('标题栏极浅灰填写文字仍按工程图墨色输出纯黑', () => {
  assert.deepEqual(convert([[224, 224, 224, 255], [247, 247, 247, 255], [254, 254, 254, 255]], 0xffffff),
    [0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 255, 255])
})
