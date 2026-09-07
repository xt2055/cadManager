import type { DwgDatabase } from '@mlightcad/libredwg-web'

// CAXA 将 TrueType 字体写入 STYLE 扩展数据，当前 LibreDWG 转换器未返回它。
// 仅为空字体的 CAXA 样式提供宋体回退，避免默认 hztxt 的字宽挤入公差位置。
// 不覆盖明确指定的字体、扩展字体或实体内部的 GDT 字体控制码。
export function normalizeCadDwgTextStyles(model: DwgDatabase): number {
  if (!model.tables.APPID.entries.some(entry => entry.name === 'CAXA_DRAFT_TXTSTYLE')) return 0
  let count = 0
  for (const style of model.tables.STYLE.entries) {
    if (style.font?.trim() || style.bigFont?.trim() || style.extendedFont?.trim()) continue
    style.font = 'simsun'
    count++
  }
  return count
}
