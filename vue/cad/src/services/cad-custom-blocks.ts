import type { DwgDatabase, DwgProxyEntity, LibreDwgEx } from '@mlightcad/libredwg-web'

// 只替换实体类型行，原字节保持不变，避免破坏 ANSI_936 图层名和文字。
export function normalizeCadDxfBlockReferences(data: ArrayBuffer): ArrayBuffer {
  const text = new TextDecoder('latin1').decode(data)
  if (!text.includes('ACIDBLOCKREFERENCE')) return data
  const replacements: Array<{ start: number; end: number }> = []
  const records = [...text.matchAll(/^([^\r\n]*)\r?\n([^\r\n]*)(?:\r?\n|$)/gm)]
    .filter(pair => pair[1]?.trim() === '0')
  for (let i = 0; i < records.length; i++) {
    const record = records[i]!
    if (record[2]?.trim() !== 'ACIDBLOCKREFERENCE') continue
    const body = text.slice(record.index, records[i + 1]?.index ?? text.length)
    if (!/^[ \t]*100[ \t]*\r?\nAcDbBlockReference[ \t]*\r?$/m.test(body)) continue
    if (!/^[ \t]*2[ \t]*\r?\n[^\r\n]+/m.test(body)) continue
    const start = record.index! + record[0].indexOf('\n') + 1
    replacements.push({ start, end: start + record[2]!.length })
  }
  if (!replacements.length) return data
  const original = new Uint8Array(data)
  const insert = new TextEncoder().encode('INSERT')
  const output = new Uint8Array(data.byteLength + replacements.reduce((sum, r) => sum + insert.length - (r.end - r.start), 0))
  let source = 0
  let target = 0
  for (const { start, end } of replacements) {
    output.set(original.subarray(source, start), target)
    target += start - source
    output.set(insert, target)
    target += insert.length
    source = end
  }
  output.set(original.subarray(source), target)
  return output.buffer
}

// MLightCAD 的矩阵读取使用 Float64Array，要求每条命令的载荷按 8 字节对齐。
// CAXA 的命令只按 4 字节对齐；补齐记录尾部并更新长度，不改变任何绘图参数。
export function alignCadProxyGraphics(graphics: Uint8Array): Uint8Array {
  if (graphics.length < 8) return graphics
  const chunks: Uint8Array[] = [graphics.slice(0, 8)]
  let length = 8
  for (let offset = 8; offset < graphics.length;) {
    if (offset + 8 > graphics.length) return graphics
    const size = new DataView(graphics.buffer, graphics.byteOffset + offset, 4).getUint32(0, true)
    if (size < 8 || offset + size > graphics.length) return graphics
    const chunk = new Uint8Array(Math.ceil(size / 8) * 8)
    chunk.set(graphics.subarray(offset, offset + size))
    new DataView(chunk.buffer).setUint32(0, chunk.length, true)
    chunks.push(chunk)
    length += chunk.length
    offset += size
  }
  const output = new Uint8Array(length)
  let offset = 0
  for (const chunk of chunks) {
    output.set(chunk, offset)
    offset += chunk.length
  }
  // CAXA 流头的首个 DWORD 是总长度，第二个 DWORD 是命令数量。
  new DataView(output.buffer).setUint32(0, length, true)
  return output
}

// LibreDWG 不解码 ACIDBLOCKREFERENCE 的私有字段，但仍提供其代理绘图流。
// 放回原所属块，让 MLightCAD 处理嵌套变换、选择和编辑，而不是顶层叠画。
export function restoreCadDwgCustomBlocks(lib: LibreDwgEx, ptr: number, model: DwgDatabase): number {
  const id = (value: number | bigint) => value.toString(16).toUpperCase()
  const blocks = new Map(model.tables.BLOCK_RECORD.entries.map(block => [block.handle, block]))
  let restored = 0
  for (let i = 0; i < lib.dwg_get_num_objects(ptr); i++) {
    const object = lib.dwg_get_object(ptr, i)
    if (lib.dwg_object_get_fixedtype(object) !== 65534 || lib.dwg_object_get_dxfname(object) !== 'ACIDBLOCKREFERENCE') continue
    const entity = lib.dwg_object_to_entity(object)
    const owner = id(lib.dwg_object_entity_get_ownerhandle_object(entity).absolute_ref)
    const block = blocks.get(owner)
    const preview = lib.dwg_entity_get_preview(object)
    if (!block || !preview?.length) continue
    const graphics = alignCadProxyGraphics(preview)
    const handle = id(lib.dwg_object_get_handle_object(object).value)
    if (block.entities.some(item => item.handle === handle)) continue
    const color = lib.dwg_object_entity_get_color_object(entity)
    const layer = lib.dwg_object_entity_get_layer_object_ref(entity)
    const ltype = lib.dwg_object_entity_get_ltype_object_ref(entity)
    const proxy: DwgProxyEntity = {
      type: 'ACAD_PROXY_ENTITY',
      subclassMarker: 'AcDbProxyEntity',
      originalDxfName: 'ACIDBLOCKREFERENCE',
      proxyEntityClassId: 498,
      applicationEntityClassId: 0,
      handle,
      ownerBlockRecordSoftId: owner,
      ownerDictionaryHardId: id(lib.dwg_object_entity_get_xdicobjhandle_object(entity).absolute_ref),
      layer: model.tables.LAYER.entries.find(item => item.handle === id(layer.absolute_ref))?.name ?? '0',
      lineType: model.tables.LTYPE.entries.find(item => item.handle === id(ltype.absolute_ref))?.name ?? '',
      colorIndex: color.index,
      colorName: color.name,
      color: color.method === 0xc2 || (color.rgb >>> 24) === 0xc2 ? color.rgb & 0xffffff : undefined,
      lineweight: lib.dwg_object_entity_get_line_weight(entity),
      lineTypeScale: lib.dwg_object_entity_get_ltype_scale(entity),
      isVisible: !lib.dwg_object_entity_get_invisible(entity),
      transparency: color.alpha,
      transparencyType: color.alpha_type,
      xdata: lib.dwg_object_entity_get_xdata(entity),
      graphicsDataSize: graphics.length,
      graphicsData: Array.from(graphics, byte => byte.toString(16).padStart(2, '0')).join(''),
    }
    block.entities.push(proxy)
    if (/^\*(MODEL_SPACE|PAPER_SPACE\d*)$/i.test(block.name)) model.entities.push(proxy)
    restored++
  }
  return restored
}
