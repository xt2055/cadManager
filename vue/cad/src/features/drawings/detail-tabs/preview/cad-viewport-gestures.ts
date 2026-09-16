/**
 * 批注覆盖层挡住了画布自身的滚轮缩放与中键平移，视图手势由前端自己换算成目标视框。
 * 这里的换算保持纯函数，便于回归测试（见 scripts/cad-viewport-gestures.test.mjs）。
 */
export interface ViewBox { minX: number; minY: number; maxX: number; maxY: number }
export interface ViewPoint { x: number; y: number }

/**
 * 缩放视框：factor > 1 放大。
 * @param box 当前可见的图纸世界坐标范围
 * @param factor 缩放倍率
 * @param pivot 需要保持不动的图纸坐标（通常是鼠标位置），缺省围绕视框中心缩放
 * @returns 新的视框；倍率非法或视框退化时返回 null
 */
export function zoomViewBox(box: ViewBox, factor: number, pivot?: ViewPoint): ViewBox | null {
  if (!Number.isFinite(factor) || factor <= 0) return null
  const width = box.maxX - box.minX, height = box.maxY - box.minY
  if (!(width > 0) || !(height > 0)) return null
  const center = { x: (box.minX + box.maxX) / 2, y: (box.minY + box.maxY) / 2 }
  const anchor = pivot ?? center
  // 锚点在视框中的相对位置保持不变：新中心位于锚点与旧中心之间。
  const next = { x: anchor.x + (center.x - anchor.x) / factor, y: anchor.y + (center.y - anchor.y) / factor }
  const halfWidth = width / (2 * factor), halfHeight = height / (2 * factor)
  return { minX: next.x - halfWidth, minY: next.y - halfHeight, maxX: next.x + halfWidth, maxY: next.y + halfHeight }
}

/**
 * 平移视框：图纸内容跟随手势移动。
 * @param box 当前可见的图纸世界坐标范围
 * @param dx 手势的屏幕像素位移（向右为正）
 * @param dy 手势的屏幕像素位移（向下为正）
 * @param xUnit 每个屏幕像素对应的图纸坐标增量（x 方向）
 * @param yUnit 每个屏幕像素对应的图纸坐标增量（y 方向）
 * @returns 新的视框；位移非法或视框退化时返回 null
 */
export function panViewBox(box: ViewBox, dx: number, dy: number, xUnit: ViewPoint, yUnit: ViewPoint): ViewBox | null {
  if (!Number.isFinite(dx) || !Number.isFinite(dy) || (!dx && !dy)) return null
  const width = box.maxX - box.minX, height = box.maxY - box.minY
  if (!(width > 0) || !(height > 0)) return null
  const centerX = (box.minX + box.maxX) / 2 - (dx * xUnit.x + dy * yUnit.x)
  const centerY = (box.minY + box.maxY) / 2 - (dx * xUnit.y + dy * yUnit.y)
  return { minX: centerX - width / 2, minY: centerY - height / 2, maxX: centerX + width / 2, maxY: centerY + height / 2 }
}
