import type { AcApContext, AcTrView2d } from '@mlightcad/cad-simple-viewer'
import type { AcDbDatabase } from '@mlightcad/data-model'
import type { AcTrRenderer } from '@mlightcad/three-renderer'

/** 读取 HEADER 时的 LWDISPLAY 事件会触发未等待的清场重绘，必须等数据库完整后再绘制。 */
export async function readCadBeforeRendering<T>(context: AcApContext | undefined, read: () => Promise<T>): Promise<T> {
  if (!context?.isActive) return read()
  context.suspend()
  try {
    const result = await read()
    const view = context.view as AcTrView2d
    view.renderer.showLineWeight = !!context.doc.database.lwdisplay
    installCadLineWeights(view.renderer, context.doc.database)
    context.resume()
    await context.doc.database.regen()
    if (!await view.waitUntilIdle()) throw new Error('图纸重绘未完成，请重试')
    return result
  } finally {
    context.resume()
  }
}

export function resolveCadLineWeight(weight: number, layerWeight: number | undefined, defaultWeight: number): number {
  const resolved = weight === -1 ? layerWeight : weight
  return resolved != null && resolved >= 0 ? resolved : defaultWeight >= 0 ? defaultWeight : 25
}

const lineWeightRestorers = new WeakMap<AcTrRenderer, () => void>()

/** CAD 线宽单位为 1/100 mm；SDK 使用 max(1, weight / 40) 屏幕像素。 */
export function installCadLineWeights(renderer: AcTrRenderer, database: AcDbDatabase, pixelsPerMm = 96 / 25.4): () => void {
  lineWeightRestorers.get(renderer)?.()
  const styles = renderer.styleManager
  const original = styles.getLineMaterial
  const previousForce = renderer.forceShowLineWeight
  renderer.forceShowLineWeight = true
  styles.getLineMaterial = function (traits, basicMaterialOnly) {
    const layer = database.tables.layerTable.getAt(traits.layer)
    const weight = resolveCadLineWeight(traits.lineWeight, layer?.lineWeight, 25)
    return original.call(this, {
      ...traits,
      lineWeight: weight * pixelsPerMm * 40 / 100,
    }, basicMaterialOnly)
  }
  const restore = () => {
    styles.getLineMaterial = original
    renderer.forceShowLineWeight = previousForce
    lineWeightRestorers.delete(renderer)
  }
  lineWeightRestorers.set(renderer, restore)
  return restore
}
