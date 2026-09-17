import type { DrawingFile } from '@/types/domain.types'

/**
 * 图号重新识别的纯判断（无 Vue 状态、无 IO）。
 * 原先这三段判断混在 reidentifyAllPartFiles 里，现按「表格类筛选 / 单文件资格 / 批量过滤」拆开。
 */

/** 明细表 / BOM 等表格类文件即使扩展名是 CAD，也不参与图号重新识别。 */
export function isNonPartCadFile(fileName: string): boolean {
  const lower = fileName.toLowerCase()
  return (
    lower.includes('明细表') ||
    lower.includes('外购件') ||
    lower.includes('标准件') ||
    lower.includes('密封件') ||
    lower.includes('汇总表') ||
    lower.includes('目录') ||
    lower.includes('bom')
  )
}

/** 是否具备重新识别资格：非总图、有物理存储、是零件 CAD，且不是表格类文件。 */
export function isCadFileEligibleForReidentify(file: DrawingFile): boolean {
  const extension = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] || ''
  return (
    file.role !== 'assembly' &&
    Boolean(file.storageKey) &&
    ['.exb', '.dwg', '.dxf'].includes(extension) &&
    !isNonPartCadFile(file.name)
  )
}

export function filterReidentifiableFiles(files: readonly DrawingFile[]): DrawingFile[] {
  return files.filter(isCadFileEligibleForReidentify)
}
