import { LineSegments2 } from 'three/examples/jsm/lines/LineSegments2.js'

/** MLightCAD 1.6.3 的批量高亮包装器会脱离对象调用原回调。 */
export function installCadLineRenderCallback(): void {
  const prototype = LineSegments2.prototype
  const descriptor = Object.getOwnPropertyDescriptor(prototype, 'onBeforeRender')
  if (!descriptor || typeof descriptor.value !== 'function') return
  const original = descriptor.value as LineSegments2['onBeforeRender']
  const callbacks = new WeakMap<LineSegments2, LineSegments2['onBeforeRender']>()
  Object.defineProperty(prototype, 'onBeforeRender', {
    configurable: true,
    enumerable: descriptor.enumerable,
    get(this: LineSegments2) {
      let callback = callbacks.get(this)
      if (!callback) {
        callback = original.bind(this)
        callbacks.set(this, callback)
      }
      return callback
    },
    set(this: LineSegments2, callback: LineSegments2['onBeforeRender']) {
      // 允许 SDK 安装自己的高亮包装器，保留原来的可写属性语义。
      Object.defineProperty(this, 'onBeforeRender', {
        configurable: true, enumerable: true, writable: true, value: callback,
      })
    },
  })
}
