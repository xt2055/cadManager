export interface ParsedDrawingFileName {
  no: string
  name: string
  rootNo: string | null
  parentNo: string | null
  level: number
  isStandard: boolean
}

function stripExtension(fileName: string): string {
  return fileName.trim().replace(/\.[^./\\]+$/, '')
}

export function normalizeDrawingDisplayName(value: string): string {
  return value.trim().replace(/[）)】\]]+$/g, '').trim()
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function invalidResult(): ParsedDrawingFileName {
  return {
    no: '',
    name: '',
    rootNo: null,
    parentNo: null,
    level: -1,
    isStandard: false,
  }
}

function extractDrawingName(baseName: string, drawingNo: string): string {
  let suffix = baseName.slice(drawingNo.length).trim().replace(/^[\s_-]+/, '')
  const openingIndex = suffix.search(/[（(【\[]/)
  if (openingIndex === 0) {
    const closingIndex = suffix.search(/[）)】\]]/)
    if (closingIndex > 0) {
      const bracketName = suffix.slice(1, closingIndex).trim()
      const trailingName = suffix.slice(closingIndex + 1).replace(/[）)】\]]+$/g, '').trim()
      suffix = `${bracketName}${trailingName ? ` ${trailingName}` : ''}`
    } else {
      suffix = suffix.slice(1)
    }
  }

  return normalizeDrawingDisplayName(suffix) || drawingNo
}

export function drawingNumberRoot(drawingNo: string): string {
  const firstLevelSeparator = drawingNo.indexOf('-')
  return firstLevelSeparator > 0 ? drawingNo.slice(0, firstLevelSeparator) : drawingNo
}

export function isSameDrawingFamily(partNo: string, drawingNo: string): boolean {
  const part = partNo.trim()
  const drawing = drawingNo.trim()
  if (!part || !drawing) return false
  return drawingNumberRoot(part).toLowerCase() === drawingNumberRoot(drawing).toLowerCase()
}

// 图号比较键：标题栏真值可能含 /（如 JG9055e-50/32-00），文件名变体会丢失斜杠
// （如 JG9055e-5032-00）；比较时统一去除斜杠并忽略大小写与空白。
function drawingNoCompareKey(value: string): string {
  return value.trim().toUpperCase().replace(/[\/\\\s]/g, '')
}

// 只有已知总图完整编号与其首段简称等价，不能把同族零件号当成总图简称。
export function isEquivalentAssemblyNo(candidateValue: string, assemblyValue: string, knownAssemblyNos: string[] = []): boolean {
  const candidate = drawingNoCompareKey(candidateValue)
  const assembly = drawingNoCompareKey(assemblyValue)
  if (!candidate || !assembly) return false
  if (candidate === assembly) return true
  const knownAssembly = knownAssemblyNos
    .map((value) => drawingNoCompareKey(value))
    .find((value) => value && value.includes('-') && (value === candidate || value === assembly))
  if (!knownAssembly) return false
  const shortNo = drawingNumberRoot(knownAssembly)
  return candidate === shortNo && assembly === knownAssembly || assembly === shortNo && candidate === knownAssembly
}

function parseDrawingNumberPrefix(baseName: string): string | null {
  const match = baseName.match(/^([A-Za-z0-9][A-Za-z0-9./-]*[A-Za-z0-9])(?=$|[\s_().（）【】\[\]]|[^\x00-\x7F])/)
  const drawingNo = match?.[1] ?? ''
  if (!drawingNo || !/\d/.test(drawingNo)) return null
  return drawingNo
}

export function directParentDrawingNo(no: string): string | null {
  const match = no.match(/^(.+)-\d+$/)
  return match?.[1] ?? null
}

export function parseDrawingNumber(noValue: string): ParsedDrawingFileName {
  const no = noValue.trim()
  if (!no) return invalidResult()
  const rootNo = drawingNumberRoot(no)
  const segments = no.slice(rootNo.length).split('-').filter(Boolean)
  return {
    no,
    name: '',
    rootNo,
    parentNo: directParentDrawingNo(no),
    level: segments.length,
    isStandard: true,
  }
}

export function parseDrawingFileName(fileName: string, rootDrawingNo: string): ParsedDrawingFileName {
  const rootNo = rootDrawingNo.trim()
  const baseName = stripExtension(fileName)
  if (!rootNo || !baseName) return invalidResult()

  const match = baseName.match(
    new RegExp(`^(${escapeRegExp(rootNo)}((?:-\\d+)*))(?=$|[\\s_().（）【】\\[\\]]|[^\\x00-\\x7F])`),
  )
  if (!match) return invalidResult()

  const no = match[1] ?? ''
  const suffix = match[2] ?? ''
  const segments = suffix.split('-').filter(Boolean)
  const name = extractDrawingName(baseName, no)

  return {
    no,
    name,
    rootNo,
    parentNo: directParentDrawingNo(no),
    level: segments.length,
    isStandard: Boolean(no),
  }
}

export function parseStandaloneDrawingFileName(fileName: string): ParsedDrawingFileName {
  const baseName = stripExtension(fileName)
  const no = parseDrawingNumberPrefix(baseName)
  if (!no) return invalidResult()

  return {
    ...parseDrawingNumber(no),
    name: extractDrawingName(baseName, no),
  }
}

export function parseAgainstRoots(fileName: string, rootDrawingNos: string[]): ParsedDrawingFileName {
  const roots = [...rootDrawingNos].sort((left, right) => right.length - left.length)
  for (const rootNo of roots) {
    const parsed = parseDrawingFileName(fileName, rootNo)
    if (parsed.isStandard) return parsed

    // 总图可能使用完整编号，零件文件名则只保留同族简称，例如 JG9063d-90-50-01。
    const standaloneParsed = parseStandaloneDrawingFileName(fileName)
    if (standaloneParsed.isStandard && (isEquivalentAssemblyNo(standaloneParsed.no, rootNo) || isSameDrawingFamily(standaloneParsed.no, rootNo))) {
      return standaloneParsed
    }
  }
  return invalidResult()
}

export function parseFileNameForRoot(fileName: string, rootDrawingNo: string): ParsedDrawingFileName {
  return parseDrawingFileName(fileName, rootDrawingNo)
}
