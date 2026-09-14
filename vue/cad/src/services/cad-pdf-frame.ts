type Point2d = { x: number; y: number }

/** 留出图框边缘，保持纵横比，并按约 430～520 DPI 输出工程图小字。 */
export function resolveCadPdfFrame(min: Point2d, max: Point2d, maxTextureSize: number) {
  const drawingWidth = max.x - min.x
  const drawingHeight = max.y - min.y
  if (![min.x, min.y, max.x, max.y, drawingWidth, drawingHeight].every(Number.isFinite)
    || drawingWidth < 0 || drawingHeight < 0 || Math.max(drawingWidth, drawingHeight) <= 0) {
    throw new Error('图纸没有有效的导出范围')
  }
  if (!Number.isFinite(maxTextureSize) || maxTextureSize < 1) throw new Error('无法获取 PDF 渲染尺寸限制')
  const padding = Math.max(drawingWidth, drawingHeight) * 0.01
  const worldWidth = drawingWidth + 2 * padding
  const worldHeight = drawingHeight + 2 * padding
  // A3 长边 420mm：7200px 约为 435 DPI。相比旧 6000px / 2400 万像素，
  // 细小尺寸、公差、标题栏签名会获得更多真实采样点，同时限制在约 3600 万
  // 像素，避免普通显卡导出时因帧缓冲过大而失败。
  const scale = Math.min(7200, Math.floor(maxTextureSize)) / Math.max(worldWidth, worldHeight)
  const memoryScale = Math.min(scale, Math.sqrt(36_000_000 / (worldWidth * worldHeight)))
  const width = Math.max(1, Math.floor(worldWidth * memoryScale))
  const height = Math.max(1, Math.floor(worldHeight * memoryScale))
  // 长边 420mm，采用相同的像素比例防止嵌入 PDF 时拉伸。
  const pageScale = 420 / Math.max(width, height)
  return {
    min: { x: min.x - padding, y: min.y - padding },
    max: { x: max.x + padding, y: max.y + padding },
    width, height,
    pageWidth: width * pageScale,
    pageHeight: height * pageScale,
  }
}
