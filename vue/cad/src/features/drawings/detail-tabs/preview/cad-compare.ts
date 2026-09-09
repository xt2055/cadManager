import type { AcDbDxfFiler, AcDbDatabase } from '@mlightcad/data-model'

export interface CompareBounds { minX: number; minY: number; maxX: number; maxY: number }
export interface CompareEntity {
  id: string
  type: string
  layer: string
  signature: string
  bounds?: CompareBounds
}
export interface DrawingDifference {
  id: number
  kind: 'added' | 'deleted' | 'modified'
  before?: CompareEntity
  after?: CompareEntity
}

// Handles and ownership describe file storage, not drawing content. Preserve all
// geometry, text, hatch and style fields, including repeated DXF group codes.
export function canonicalDxf(text: string, resolvedBlockName?: string, contentOnlyText = false): string {
  const lines = text.replace(/\r/g, '').trimEnd().split('\n')
  const pairs: Array<[number, string | number]> = []
  for (let i = 0; i + 1 < lines.length; i += 2) {
    const code = Number(lines[i])
    if ([6, 48, 370].includes(code)) continue
    if (code === 5 || code === 105 || code === 102 || (code >= 320 && code <= 369)) continue
    const raw = lines[i + 1]!
    // A resolved block is identified by its contents, not its generated name.
    if (code === 2 && raw === resolvedBlockName) { pairs.push([code, '<resolved-block>']); continue }
    const numeric = (code >= 10 && code <= 99) || (code >= 110 && code <= 149) || (code >= 210 && code <= 239) || (code >= 270 && code <= 299) || (code >= 370 && code <= 459)
    pairs.push([code, numeric && Number.isFinite(Number(raw)) ? Number(Number(raw).toFixed(6)) : raw])
  }
  // DXF text alignment uses point 11; point 10 is recalculated by the CAD font engine.
  for (let start = 0; start < pairs.length; start++) {
    if (pairs[start]![0] !== 100) continue
    let end = start + 1
    while (end < pairs.length && pairs[end]![0] !== 100 && pairs[end]![0] !== 0) end++
    const section = pairs.slice(start, end)
    if (pairs[start]![1] === 'AcDbDimension') {
      // 标注文字也支持 MTEXT 排版码；直径、公差、堆叠及转义内容必须保留。
      for (const pair of section) if (pair[0] === 1) {
        pair[1] = String(pair[1]).replace(/\\[\\{}]|\\[WHTQACFcf][^;]*;|\\[LlOoKk]|[{}]/g, token => /^\\[\\{}]$/.test(token) ? token : '')
      }
    }
    if (pairs[start]![1] === 'AcDbText') {
      const h = Number(section.find(pair => pair[0] === 72)?.[1] ?? 0)
      const v = Number(section.find(pair => pair[0] === 73)?.[1] ?? 0)
      if ((h !== 0 || v !== 0) && h !== 3 && h !== 5 && section.some(pair => pair[0] === 11)) {
        for (const pair of section) if ([10, 20, 30].includes(pair[0])) pair[1] = 0
      }
    }
    if (['AcDbText', 'AcDbMText'].includes(String(pairs[start]![1]))) {
      // 块内文字按设计内容比较；保留堆叠公差、换行及转义字符，不把它们当排版删除。
      const content = section.filter(pair => pair[0] === 1 || pair[0] === 3).map(pair => String(pair[1])).join('')
      const plain = pairs[start]![1] === 'AcDbMText'
        ? content.replace(/\\[\\{}]|\\[WHTQACFcf][^;]*;|\\[LlOoKk]|[{}]/g, token => /^\\[\\{}]$/.test(token) ? token : '')
        : content
      const location = contentOnlyText ? [] : section.filter(pair => [10, 20, 30, 11, 21, 31, 50, 210, 220, 230].includes(pair[0]))
      pairs.splice(start + 1, end - start - 1, ...location, [1, plain])
      continue
    }
    if (pairs[start]![1] === 'AcDbSpline') {
      const knots = section.filter(pair => pair[0] === 40)
      const first = Number(knots[0]?.[1])
      const span = Number(knots.at(-1)?.[1]) - first
      if (span > 0) for (const pair of knots) pair[1] = Number(((Number(pair[1]) - first) / span).toFixed(6))
    }
  }
  for (const pair of pairs) {
    if (pair[0] >= 50 && pair[0] <= 53 && typeof pair[1] === 'number') pair[1] = Number(((pair[1] % 360 + 360) % 360).toFixed(6))
  }
  return JSON.stringify(pairs)
}

export function snapshotDrawing(database: AcDbDatabase, createFiler: () => AcDbDxfFiler): CompareEntity[] {
  const includeEntity = (entity: any) => !['HATCH', 'WIPEOUT'].includes(entity.dxfTypeName || entity.type)
  const blocks = new Map<string, any>()
  for (const block of database.tables.blockTable.newIterator(true)) blocks.set(block.name, block)
  const blockCache = new Map<string, unknown[]>()
  function signature(entity: any, ancestors: Set<string>): unknown[] {
    const filer = createFiler()
    entity.dxfOut(filer)
    const text = filer.toString()
    if (!text.trim()) throw new Error(`暂不支持对比实体 ${entity.dxfTypeName || entity.type}`)
    // Include referenced block contents: a changed symbol may keep the same
    // INSERT handle, name, position and outer bounds.
    const name = entity.blockName ?? (entity.dxfTypeName === 'DIMENSION' ? entity.dimBlockId : undefined)
    const resolved = name && blocks.has(name) && !ancestors.has(name)
    const result: unknown[] = [JSON.parse(canonicalDxf(text, resolved ? name : undefined, ancestors.size > 0))]
    if (resolved) {
      let contents = blockCache.get(name)
      if (contents === undefined) {
        const next = new Set(ancestors).add(name)
        const block = blocks.get(name)
        const origin = block.origin
        contents = [
          [origin.x, origin.y, origin.z].map(value => Number(value.toFixed(6))),
          Array.from(block.newIterator()).filter(includeEntity).map((child: any) => signature(child, next)).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b))),
        ]
        blockCache.set(name, contents)
      }
      result.push(contents)
    }
    const layer = database.tables.layerTable.getAt(entity.layer)
    // 仅比较实体实际继承的图层属性；显式线宽不受图层线宽变化影响。
    const pairs = result[0] as Array<[number, unknown]>
    const property = (code: number) => pairs.find(pair => pair[0] === code)?.[1]
    result.push([
      property(62) === 256 && property(420) === undefined ? layer?.color?.toString() : null,
    ])
    return result
  }
  return Array.from(database.tables.blockTable.modelSpace.newIterator()).filter(includeEntity).map((entity: any) => {
    let bounds: CompareBounds | undefined
    try {
      const box = entity.geometricExtents
      const values = [box?.min.x, box?.min.y, box?.max.x, box?.max.y]
      if (values.every(Number.isFinite) && values[0] <= values[2] && values[1] <= values[3]) {
        bounds = { minX: values[0], minY: values[1], maxX: values[2], maxY: values[3] }
      }
    } catch { /* Some proxy entities have no extents; retain them in the list. */ }
    return { id: String(entity.objectId), type: entity.dxfTypeName || entity.type, layer: entity.layer, signature: JSON.stringify(signature(entity, new Set())), bounds }
  })
}

export function compareEntities(before: CompareEntity[], after: CompareEntity[]): DrawingDifference[] {
  // Match identical content first so regenerated handles and entity ordering
  // do not produce false changes. Buckets preserve duplicate multiplicity.
  const buckets = new Map<string, number[]>()
  after.forEach((entity, index) => {
    const bucket = buckets.get(entity.signature) ?? []
    bucket.push(index)
    buckets.set(entity.signature, bucket)
  })
  const matched = new Set<number>()
  const removed = before.filter(entity => {
    const index = buckets.get(entity.signature)?.pop()
    if (index === undefined) return true
    matched.add(index)
    return false
  })
  const remaining = new Map(after.flatMap((entity, index) => matched.has(index) ? [] : [[entity.id, entity] as const]))
  // Check tiny numeric serialization noise after exact matching; don't use rounded
  // buckets alone, because two near-identical values can straddle a rounding edge.
  const changed = removed.filter(entity => {
    for (const [id, candidate] of remaining) {
      if (entity.type === candidate.type && entity.layer === candidate.layer && equivalentSignature(entity.signature, candidate.signature)) {
        remaining.delete(id)
        return false
      }
    }
    return true
  })
  const result: DrawingDifference[] = []
  for (const entity of changed) {
    let candidate: CompareEntity | undefined
    if (entity.bounds) {
      // Renumbered entities at a unique unchanged extent are modifications, not
      // a deletion plus an addition. Never use extents to declare equality.
      const sameLocation = (other: CompareEntity) => other.type === entity.type && other.layer === entity.layer && other.bounds && equivalentValues(Object.values(entity.bounds!), Object.values(other.bounds))
      const candidates = [...remaining.values()].filter(sameLocation)
      if (candidates.length === 1 && changed.filter(sameLocation).length === 1) candidate = candidates[0]
    }
    // 不先按句柄抢占其他图元的对应项，转换可能给全部实体重新编号。
    if (!candidate) {
      const sameId = remaining.get(entity.id)
      if (sameId && (!entity.bounds || !sameId.bounds ||
        (entity.bounds.minX <= sameId.bounds.maxX && entity.bounds.maxX >= sameId.bounds.minX &&
         entity.bounds.minY <= sameId.bounds.maxY && entity.bounds.maxY >= sameId.bounds.minY))) candidate = sameId
    }
    if (candidate?.type === entity.type) {
      result.push({ id: result.length + 1, kind: 'modified', before: entity, after: candidate })
      remaining.delete(candidate.id)
    } else result.push({ id: result.length + 1, kind: 'deleted', before: entity })
  }
  for (const entity of remaining.values()) result.push({ id: result.length + 1, kind: 'added', after: entity })
  return result
}

function equivalentSignature(a: unknown, b: unknown): boolean {
  if (a === b) return true
  try { return equivalentValues(JSON.parse(String(a)), JSON.parse(String(b))) } catch { return false }
}

function equivalentValues(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (typeof a === 'number' && typeof b === 'number') return Math.abs(a - b) <= 0.000002
  if (Array.isArray(a) && Array.isArray(b)) return a.length === b.length && a.every((value, index) => equivalentValues(value, b[index]))
  return false
}
