import * as XLSX from 'xlsx'
import type { BomItem } from '@/types/domain.types'

function formatNumber(value: unknown): number {
  if (value === undefined || value === null) return 0
  if (typeof value === 'number') return Number.isFinite(value) ? value : 0
  const normalized = String(value).replace(/,/g, '').trim()
  if (!normalized || normalized === '/') return 0
  const parsed = Number(normalized)
  return Number.isFinite(parsed) ? parsed : 0
}

function cleanCell(value: unknown): string {
  if (value === undefined || value === null) return ''
  return String(value).replace(/\r?\n/g, ' ').trim()
}

async function decodeText(content: Blob): Promise<string> {
  const buffer = await content.arrayBuffer()
  const bytes = new Uint8Array(buffer)

  // 1. 检查 UTF-8 BOM
  if (bytes.length >= 3 && bytes[0] === 0xEF && bytes[1] === 0xBB && bytes[2] === 0xBF) {
    return new TextDecoder('utf-8').decode(bytes.subarray(3))
  }

  // 2. 尝试使用 strict utf-8 解码，如果失败则回退到 gbk
  try {
    const utf8Decoder = new TextDecoder('utf-8', { fatal: true })
    const text = utf8Decoder.decode(bytes)
    // 如果包含过多乱码字符，尝试 GBK
    if (!text.includes('\uFFFD')) {
      return text
    }
  } catch {
    // 忽略异常，尝试 GBK
  }

  try {
    const gbkDecoder = new TextDecoder('gbk')
    return gbkDecoder.decode(bytes)
  } catch {
    return new TextDecoder('utf-8').decode(bytes)
  }
}

export function parseRowsToBom(rows: string[][], drawingNo: string, sourceFileId: string): BomItem[] {
  if (!rows.length) return []

  // 寻找表头所在行
  let headerRowIndex = -1
  let idCol = -1
  let nameCol = -1
  let materialCol = -1
  let specCol = -1
  let qtyCol = -1
  let weightCol = -1
  let remarkCol = -1
  let outerDiaCol = -1
  let innerDiaCol = -1
  let lengthCol = -1

  for (let r = 0; r < Math.min(rows.length, 10); r += 1) {
    const row = rows[r] ?? []
    const normalized = row.map((cell) => cell.replace(/\s+/g, ''))
    const findIndex = (keywords: string[]) =>
      normalized.findIndex((cell) => keywords.some((kw) => cell.includes(kw)))

    const testId = findIndex(['零件代号', '图号', '代号', '零件号', '物料代号', '标准号'])
    const testName = findIndex(['零件名称', '物料名称', '名称', '品名'])

    if (testId >= 0 && testName >= 0) {
      headerRowIndex = r
      idCol = testId
      nameCol = testName
      materialCol = findIndex(['材质', '材料'])
      specCol = findIndex(['规格', '型材规格'])
      qtyCol = findIndex(['单支数量', '单件数量', '数量', '计划数量', '总数', 'qty'])
      weightCol = findIndex(['毛坯重量', '单重', '重量', '总重', '净重', '单件重量'])
      remarkCol = findIndex(['备注', '说明'])
      const dimensionCol = findIndex(['下料尺寸'])
      outerDiaCol = findIndex(['外径', '长'])
      innerDiaCol = findIndex(['内径', '宽'])
      lengthCol = findIndex(['长度', '高'])
      if (dimensionCol >= 0) {
        outerDiaCol = dimensionCol
        innerDiaCol = dimensionCol + 1
        lengthCol = dimensionCol + 2
      }
      break
    }
  }

  // 如果没有找到精确包含代号+名称的表头行，退化为第一行做匹配
  if (headerRowIndex === -1) {
    const firstRow = (rows[0] ?? []).map((cell) => cell.replace(/\s+/g, ''))
    const findIndex = (keywords: string[]) =>
      firstRow.findIndex((cell) => keywords.some((kw) => cell.includes(kw)))
    headerRowIndex = 0
    idCol = findIndex(['零件代号', '图号', '代号', '零件号', 'id', 'no', '编号'])
    nameCol = findIndex(['零件名称', '物料名称', '名称', 'name'])
    materialCol = findIndex(['材质', '材料'])
    specCol = findIndex(['规格', 'spec'])
    qtyCol = findIndex(['数量', 'qty', 'count'])
    weightCol = findIndex(['单重', '毛坯重量', '重量', 'weight'])
    remarkCol = findIndex(['备注', 'remark'])
  }

  const items: BomItem[] = []
  let rowNumber = 1

  for (let r = headerRowIndex + 1; r < rows.length; r += 1) {
    const row = rows[r] ?? []
    // 跳过全空行
    if (row.every((cell) => !cell)) continue

    const idVal = idCol >= 0 ? cleanCell(row[idCol]) : ''
    const nameVal = nameCol >= 0 ? cleanCell(row[nameCol]) : ''
    const materialVal = materialCol >= 0 ? cleanCell(row[materialCol]) : ''
    let specVal = specCol >= 0 ? cleanCell(row[specCol]) : ''
    const qtyRaw = qtyCol >= 0 ? row[qtyCol] : undefined
    const weightRaw = weightCol >= 0 ? row[weightCol] : undefined
    const remarkVal = remarkCol >= 0 ? cleanCell(row[remarkCol]) : ''

    // 过滤落款/签名/合计行
    const rowConcat = row.join('').replace(/\s+/g, '')
    const ignoredRowKeywords = [
      '编制：',
      '审核：',
      '批准：',
      '标准化：',
      '要求入库时间',
      '外径',
      '内径',
      '发放：',
      '要求下料完成时间',
      '下料组',
      '半成品库：',
      '物流：',
      '总装：',
    ]
    if (ignoredRowKeywords.some((keyword) => rowConcat.includes(keyword))) {
      continue
    }

    if (!idVal && !nameVal) {
      continue
    }

    // 组合规格信息：材质 + 规格 (如 27SiMn 圆钢 / 180x193)
    const specParts: string[] = []
    if (materialVal && materialVal !== '/') specParts.push(materialVal)
    if (specVal && specVal !== '/') specParts.push(specVal)

    // 如果下料尺寸存在，拼接到规格
    const outer = outerDiaCol >= 0 ? cleanCell(row[outerDiaCol]) : ''
    const inner = innerDiaCol >= 0 ? cleanCell(row[innerDiaCol]) : ''
    const len = lengthCol >= 0 ? cleanCell(row[lengthCol]) : ''
    const dimensions = [outer, inner, len].filter((v) => v && v !== '/' && v !== '0')
    if (dimensions.length > 0) {
      specParts.push(dimensions.join('×'))
    }

    const finalSpec = specParts.join(' ') || '—'
    const qty = formatNumber(qtyRaw) || (qtyRaw === '1' ? 1 : 1)
    const weight = formatNumber(weightRaw)

    items.push({
      no: rowNumber,
      id: idVal || `${drawingNo}-BOM-${String(rowNumber).padStart(3, '0')}`,
      drawingNo,
      sourceFileId,
      name: nameVal || idVal || '未命名物料',
      spec: finalSpec,
      qty: qty,
      weight: weight,
      remark: remarkVal === '/' ? '' : remarkVal,
    })

    rowNumber += 1
  }

  return items
}

export function extractMaterialAuthor(rows: string[][]): string | undefined {
  const authorPattern = /编制[:：]\s*([^\s\r\n\t审核批准]+)/
  for (const row of rows) {
    const joined = row.join(' ')
    const match = authorPattern.exec(joined)
    if (match && match[1]) {
      const name = match[1].trim()
      if (name && !['朱', '张', '李'].includes(name) || name.length >= 2) {
        return name
      }
    }
  }
  return undefined
}

export async function parseMaterialFileContent(
  file: Blob,
  fileName: string,
  drawingNo: string,
  sourceFileId: string,
): Promise<{ items: BomItem[]; author?: string }> {
  const ext = fileName.toLowerCase().slice(fileName.lastIndexOf('.'))

  if (ext === '.csv') {
    const text = await decodeText(file)
    const workbook = XLSX.read(text, { type: 'string', FS: ',' })
    const sheetName = workbook.SheetNames[0]
    if (!sheetName) return { items: [] }
    const sheet = workbook.Sheets[sheetName]
    if (!sheet) return { items: [] }
    const rawRows = XLSX.utils.sheet_to_json<unknown[]>(sheet, { header: 1, defval: '' })
    const rows: string[][] = rawRows.map((row) =>
      Array.isArray(row) ? row.map((cell) => cleanCell(cell)) : [],
    )
    return {
      items: parseRowsToBom(rows, drawingNo, sourceFileId),
      author: extractMaterialAuthor(rows),
    }
  }

  if (ext === '.xlsx' || ext === '.xls') {
    const buffer = await file.arrayBuffer()
    const workbook = XLSX.read(buffer, { type: 'array' })
    const sheetName = workbook.SheetNames[0]
    if (!sheetName) return { items: [] }
    const sheet = workbook.Sheets[sheetName]
    if (!sheet) return { items: [] }
    const rawRows = XLSX.utils.sheet_to_json<unknown[]>(sheet, { header: 1, defval: '' })
    const rows: string[][] = rawRows.map((row) =>
      Array.isArray(row) ? row.map((cell) => cleanCell(cell)) : [],
    )
    return {
      items: parseRowsToBom(rows, drawingNo, sourceFileId),
      author: extractMaterialAuthor(rows),
    }
  }

  return { items: [] }
}
