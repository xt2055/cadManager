import assert from 'node:assert/strict'
import { test } from 'node:test'
import { panViewBox, zoomViewBox } from '../src/features/drawings/detail-tabs/preview/cad-viewport-gestures.ts'

const box = { minX: 0, minY: 0, maxX: 100, maxY: 50 }
const near = (actual, expected) => assert.ok(Math.abs(actual - expected) < 1e-9, `${actual} !== ${expected}`)
const size = value => ({ width: value.maxX - value.minX, height: value.maxY - value.minY })

test('滚轮缩放围绕鼠标锚点，图纸上的锚点始终停在鼠标下方', () => {
  const pivot = { x: 25, y: 12.5 }
  for (const factor of [2, 0.5, 1.15, 1 / 1.15]) {
    const next = zoomViewBox(box, factor, pivot)
    near((pivot.x - next.minX) / (next.maxX - next.minX), 0.25)
    near((pivot.y - next.minY) / (next.maxY - next.minY), 0.25)
    near(size(next).width, 100 / factor)
    near(size(next).height, 50 / factor)
  }
})

test('缺省锚点围绕视图中心缩放，放大缩小可原路还原', () => {
  const zoomed = zoomViewBox(box, 2)
  near((zoomed.minX + zoomed.maxX) / 2, 50)
  near((zoomed.minY + zoomed.maxY) / 2, 25)
  const restored = zoomViewBox(zoomed, 0.5)
  for (const key of ['minX', 'minY', 'maxX', 'maxY']) near(restored[key], box[key])
})

test('缩放忽略非法倍率和退化视框', () => {
  assert.equal(zoomViewBox(box, 0), null)
  assert.equal(zoomViewBox(box, -1), null)
  assert.equal(zoomViewBox(box, Number.NaN), null)
  assert.equal(zoomViewBox({ minX: 0, minY: 0, maxX: 0, maxY: 50 }, 2), null)
})

test('中键拖动平移：图纸跟随手势移动且不改变缩放比例', () => {
  // 屏幕 x 向右对应图纸 x 增大，屏幕 y 向下对应图纸 y 减小。
  const xUnit = { x: 1, y: 0 }, yUnit = { x: 0, y: -1 }
  const right = panViewBox(box, 10, 0, xUnit, yUnit)
  near((right.minX + right.maxX) / 2, 40)
  near(size(right).width, 100)
  near(size(right).height, 50)
  const down = panViewBox(box, 0, 10, xUnit, yUnit)
  near((down.minY + down.maxY) / 2, 35)
  assert.equal(panViewBox(box, 0, 0, xUnit, yUnit), null)
  assert.equal(panViewBox({ minX: 5, minY: 5, maxX: 5, maxY: 5 }, 10, 0, xUnit, yUnit), null)
})
