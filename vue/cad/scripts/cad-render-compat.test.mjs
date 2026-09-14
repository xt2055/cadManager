import assert from 'node:assert/strict'
import { test } from 'node:test'
import { installCadLineWeights, readCadBeforeRendering, resolveCadLineWeight } from '../src/services/cad-render-compat.ts'

function fixture() {
  const materials = new Map()
  const renderer = {
    forceShowLineWeight: false,
    styleManager: {
      getLineMaterial(traits) {
        const width = Math.max(1, traits.lineWeight / 40)
        const key = `${traits.layer}:${width}`
        if (!materials.has(key)) materials.set(key, { linewidth: width, pattern: traits.lineType.pattern })
        return materials.get(key)
      },
    },
  }
  const database = { tables: { layerTable: { getAt: name => ({ lineWeight: name === '粗实线层' ? 70 : 25 }) } } }
  return { renderer, database }
}

const traits = (lineWeight, layer = '0') => ({ lineWeight, layer, lineType: { pattern: [] } })

test('显式线宽、随图层、默认线宽分别解析，不按图层名称猜测粗细', () => {
  assert.equal(resolveCadLineWeight(18, 70, 25), 18)
  assert.equal(resolveCadLineWeight(-1, 70, 25), 70)
  assert.equal(resolveCadLineWeight(-1, -3, 25), 25)
  assert.equal(resolveCadLineWeight(-3, 70, 25), 25)
  assert.equal(resolveCadLineWeight(0, 70, 25), 0)
})

test('0.25 和 0.35 mm 不再被 SDK 的 1px 材质缓存合并，随图层 0.7 mm 为 7px', () => {
  const { renderer, database } = fixture()
  const original = renderer.styleManager.getLineMaterial
  const restore = installCadLineWeights(renderer, database, 10)
  const thin = renderer.styleManager.getLineMaterial(traits(25))
  const thick = renderer.styleManager.getLineMaterial(traits(35))
  assert.equal(thin.linewidth, 2.5)
  assert.equal(thick.linewidth, 3.5)
  assert.notEqual(thin, thick)
  assert.equal(renderer.styleManager.getLineMaterial(traits(-1, '粗实线层')).linewidth, 7)
  const dashed = { ...traits(25), lineType: { pattern: [4, -2, 0, -2] } }
  assert.equal(renderer.styleManager.getLineMaterial({ ...dashed, layer: '虚线层' }).pattern, dashed.lineType.pattern)
  restore()
  assert.equal(renderer.styleManager.getLineMaterial, original)
  assert.equal(renderer.forceShowLineWeight, false)
})

test('从预览切换到打印比例不会叠加两次缩放', () => {
  const { renderer, database } = fixture()
  installCadLineWeights(renderer, database)
  const restore = installCadLineWeights(renderer, database, 20)
  assert.equal(renderer.styleManager.getLineMaterial(traits(25)).linewidth, 5)
  restore()
})

test('HEADER 切换线宽期间暂停场景事件，完整读取后才重绘，并等待重绘完成', async () => {
  const { renderer, database } = fixture()
  const calls = []
  const context = {
    isActive: true,
    suspend() { this.isActive = false; calls.push('暂停') },
    resume() { this.isActive = true },
    doc: { database },
    view: { renderer, async waitUntilIdle() { calls.push('空闲'); return true } },
  }
  database.regen = async () => { assert.equal(context.isActive, true); calls.push('重绘') }
  const result = await readCadBeforeRendering(context, async () => {
    assert.equal(context.isActive, false)
    calls.push('完整读取')
    database.lwdisplay = true
    return true
  })
  assert.equal(result, true)
  assert.deepEqual(calls, ['暂停', '完整读取', '重绘', '空闲'])
  assert.equal(renderer.showLineWeight, true)
})

test('解析失败恢复上下文，独立数据库读取不触发主视图重绘', async () => {
  const context = { isActive: true, suspend() { this.isActive = false }, resume() { this.isActive = true } }
  await assert.rejects(readCadBeforeRendering(context, async () => { throw new Error('解析失败') }), /解析失败/)
  assert.equal(context.isActive, true)
  assert.equal(await readCadBeforeRendering(undefined, async () => 42), 42)
})
