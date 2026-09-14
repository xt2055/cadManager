/**
 * 背景变白，前景按与背景的最大通道差转成黑色墨迹。
 * 不用普通亮度灰度化，避免绿色、黄色及青色标注打印时过浅；
 * CAD 字体常用很浅的图层色。工程图打印不保留图层明度：只要像素与
 * 纸面存在有效差异，就可视为黑色墨迹；仅在最外侧亚像素保留抗锯齿。
 */
export function convertCadPixelsToMonochrome(pixels: Uint8ClampedArray, backgroundColor: number): void {
  const red = (backgroundColor >> 16) & 255
  const green = (backgroundColor >> 8) & 255
  const blue = backgroundColor & 255
  const redRange = Math.max(red, 255 - red)
  const greenRange = Math.max(green, 255 - green)
  const blueRange = Math.max(blue, 255 - blue)
  for (let i = 0; i < pixels.length; i += 4) {
    const coverage = Math.max(
      Math.abs(pixels[i]! - red) / redRange,
      Math.abs(pixels[i + 1]! - green) / greenRange,
      Math.abs(pixels[i + 2]! - blue) / blueRange,
    ) * (pixels[i + 3]! / 255)
    // 约 2/255 以内视为背景噪声，达到约 8/255 即输出纯黑；中间窄带
    // 使用 smoothstep 过渡。浅灰填写文字因此与标签一样为黑色，小字边缘
    // 仍有一条平滑过渡带，不会产生明显锯齿。
    const edgeStart = 2 / 255
    const solidInk = 8 / 255
    const normalized = Math.max(0, Math.min(1, (coverage - edgeStart) / (solidInk - edgeStart)))
    const printCoverage = normalized * normalized * (3 - 2 * normalized)
    const value = Math.round(255 * (1 - printCoverage))
    pixels[i] = pixels[i + 1] = pixels[i + 2] = value
    pixels[i + 3] = 255
  }
}
