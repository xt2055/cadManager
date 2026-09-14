import assert from 'node:assert/strict'
import { test } from 'node:test'
import { LineSegments2 } from 'three/examples/jsm/lines/LineSegments2.js'
import { Line2 } from 'three/examples/jsm/lines/Line2.js'
import { installBatchHighlightRenderer } from '../node_modules/@mlightcad/three-renderer/lib/batch/highlight/AcTrBatchHighlightShaders.js'
import { installCadLineRenderCallback } from '../src/services/cad-line-render-callback.ts'

const state = { hasAnyHighlight: () => false, needsCompareUniforms: () => false }
const renderer = (width, height) => ({ getViewport: value => value.set(0, 0, width, height) })

test('复现 SDK 高亮包装丢失 this；修复后连续绘制和重建不再抛错', () => {
  const broken = new LineSegments2()
  installBatchHighlightRenderer(broken, state)
  assert.throws(() => broken.onBeforeRender(renderer(800, 600)), /material/)
  broken.geometry.dispose()
  broken.material.dispose()

  installCadLineRenderCallback()
  installCadLineRenderCallback()
  for (let session = 0; session < 3; session++) {
    const lines = [new LineSegments2(), new Line2()]
    for (const line of lines) installBatchHighlightRenderer(line, state)
    for (let frame = 0; frame < 1200; frame++) {
      lines[0].onBeforeRender(renderer(800, 600))
      lines[1].onBeforeRender(renderer(320, 240))
    }
    assert.deepEqual(lines[0].material.uniforms.resolution.value.toArray(), [800, 600])
    assert.deepEqual(lines[1].material.uniforms.resolution.value.toArray(), [320, 240])
    for (const line of lines) {
      line.geometry.dispose()
      line.material.dispose()
    }
  }
})

test('SDK 仍可替换回调，且后续重复安装不会覆盖包装器', () => {
  installCadLineRenderCallback()
  const line = new LineSegments2()
  const previous = line.onBeforeRender
  let calls = 0
  line.onBeforeRender = (...args) => { calls++; previous(...args) }
  installCadLineRenderCallback()
  line.onBeforeRender(renderer(1024, 768))
  assert.equal(calls, 1)
  assert.deepEqual(line.material.uniforms.resolution.value.toArray(), [1024, 768])
  line.geometry.dispose()
  line.material.dispose()
})
