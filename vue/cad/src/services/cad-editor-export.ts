import type { AcDbDatabase } from '@mlightcad/data-model'

// DWG 导入器可能未填充 dimensionType；CAXA 按组码 70 而非子类名识别标注。
export function repairDimensionTypes(dxf: string): string {
  const lines = dxf.replace(/\r/g, '').split('\n')
  for (let start = 0; start + 1 < lines.length;) {
    let end = start + 2
    while (end + 1 < lines.length && Number(lines[end]) !== 0) end += 2
    if (Number(lines[start]) === 0 && lines[start + 1]?.trim() === 'DIMENSION') {
      const subclasses = new Set<string>()
      let typeIndex = -1
      for (let i = start + 2; i + 1 < end; i += 2) {
        if (Number(lines[i]) === 100) subclasses.add(lines[i + 1]!.trim())
        if (Number(lines[i]) === 70 && typeIndex === -1) typeIndex = i + 1
      }
      const type = subclasses.has('AcDbRotatedDimension') ? 0
        : subclasses.has('AcDbDiametricDimension') ? 3
        : subclasses.has('AcDbRadialDimension') ? 4
        : subclasses.has('AcDb3PointAngularDimension') ? 5
        : subclasses.has('AcDb2LineAngularDimension') ? 2
        : subclasses.has('AcDbOrdinateDimension') ? 6
        : subclasses.has('AcDbAlignedDimension') ? 1 : undefined
      if (typeIndex !== -1 && type !== undefined) {
        lines[typeIndex] = String((Number(lines[typeIndex]) & ~7) | type)
      }
    }
    start = end
  }
  return lines.join('\n')
}

export function exportEditorDxf(database: AcDbDatabase): string {
  const result = database.dxfOut(undefined, 12, 'AC1027', { format: 'ascii' })
  if (typeof result !== 'string') throw new Error('CAD 导出未返回 ASCII DXF，未提交保存')
  return repairDimensionTypes(result)
}
