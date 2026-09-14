import { Line, LineBasicMaterial, type Object3D } from 'three'
import { LineMaterial } from 'three/examples/jsm/lines/LineMaterial.js'
import { LineSegments2 } from 'three/examples/jsm/lines/LineSegments2.js'
import { LineSegmentsGeometry } from 'three/examples/jsm/lines/LineSegmentsGeometry.js'

type BatchLine = Line & {
  geometryCount: number
  getGeometryRangeAt(id: number): {
    flags: number
    indexStart: number
    indexCount: number
    vertexStart: number
    vertexCount: number
  }
}

/** SHX 笔画使用原生 WebGL 线，linewidth 无法加粗；打印时临时改为宽线几何。 */
export function installCadPdfStrokes(root: Object3D, pixelsPerMm: number): () => void {
  const undo: Array<() => void> = []
  const lines: Line[] = []
  root.traverseVisible(object => {
    const line = object as Line
    if (line.isLine && line.parent && line.material instanceof LineBasicMaterial
      && line.material.visible && line.material.opacity > 0) lines.push(line)
  })
  try {
    for (const line of lines) {
      const source = line.material as LineBasicMaterial
      // 保留专用虚线着色器的语义，不把虚线转为实线。
      if ('isLineDashedMaterial' in source) continue
      const geometry = line.geometry
      const position = geometry.getAttribute('position')
      if (!position) continue
      const index = geometry.index
      const count = index?.count ?? position.count
      const ranges: Array<{ start: number; count: number }> = []
      const batch = line as BatchLine
      if (typeof batch.getGeometryRangeAt === 'function') {
        for (let id = 0; id < batch.geometryCount; id++) {
          let slot: ReturnType<BatchLine['getGeometryRangeAt']>
          // SDK 对软删除槽位抛错；geometryCount 仍包含这些历史槽位。
          try { slot = batch.getGeometryRangeAt(id) } catch { continue }
          // Active + Visible；不导出删除或隐藏的批次槽位及预分配空区。
          if ((slot.flags & 3) !== 3) continue
          ranges.push(index
            ? { start: slot.indexStart, count: slot.indexCount }
            : { start: slot.vertexStart, count: slot.vertexCount })
        }
      } else ranges.push(geometry.drawRange)
      const points: number[] = []
      const step = 'isLineSegments' in line ? 2 : 1
      for (const range of ranges) {
        const end = Math.min(count, range.start + range.count)
        for (let i = range.start; i + 1 < end; i += step) {
          for (const offset of [i, i + 1]) {
            const vertex = index ? index.getX(offset) : offset
            points.push(position.getX(vertex), position.getY(vertex), position.getZ(vertex))
          }
        }
        if ('isLineLoop' in line && end - range.start > 2) {
          for (const offset of [end - 1, range.start]) {
            const vertex = index ? index.getX(offset) : offset
            points.push(position.getX(vertex), position.getY(vertex), position.getZ(vertex))
          }
        }
      }
      if (!points.length) continue
      const wideGeometry = new LineSegmentsGeometry().setPositions(points)
      const material = new LineMaterial({
        color: source.color.getHex(),
        linewidth: Math.max(1, 0.13 * pixelsPerMm),
        depthTest: source.depthTest,
        depthWrite: source.depthWrite,
        clippingPlanes: source.clippingPlanes ?? undefined,
      })
      const replacement = new LineSegments2(wideGeometry, material)
      material.userData = { ...source.userData }
      replacement.position.copy(line.position)
      replacement.quaternion.copy(line.quaternion)
      replacement.scale.copy(line.scale)
      replacement.matrix.copy(line.matrix)
      replacement.matrixAutoUpdate = line.matrixAutoUpdate
      replacement.layers.mask = line.layers.mask
      replacement.renderOrder = line.renderOrder
      replacement.frustumCulled = false
      const visible = line.visible
      line.parent!.add(replacement)
      line.visible = false
      undo.push(() => {
        replacement.removeFromParent()
        wideGeometry.dispose()
        material.dispose()
        line.visible = visible
      })
    }
  } catch (error) {
    for (const restore of undo.reverse()) restore()
    throw error
  }
  return () => { for (const restore of undo.splice(0).reverse()) restore() }
}
