import type { AcDbDatabase } from '@mlightcad/data-model'

export interface TitleText {
  text: string
  tag?: string
  x: number
  y: number
  height: number
  angle: number
}
export interface TitleSpace { id: string; name: string; texts: TitleText[]; warnings?: string[] }
export interface TitleField {
  key: string
  label: string
  value: string
  source: string
  candidates: string[]
}
const definitions = [
  ['number', '图号', ['图号', '图纸编号', '零件图号', '代号', 'DRAWINGNO', 'DRAWINGNUMBER']],
  ['name', '图纸名称', ['图名', '名称', '图纸名称', '零件名称', 'TITLE', 'DRAWINGNAME']],
  ['designer', '设计', ['设计', '设计人', '设计人员', 'DESIGNEDBY', 'DESIGNER']],
  ['checker', '校对', ['校对', '校核', 'CHECKEDBY']],
  ['process', '工艺', ['工艺', '工艺员']],
  ['standard', '标准化', ['标准', '标准化']],
  ['approver', '批准', ['批准', 'APPROVEDBY']],
  ['date', '日期', ['日期', '设计日期', 'DATE']],
  ['material', '材料', ['材料', '材料名称', '材质', 'MATERIAL']],
  ['scale', '比例', ['比例', '图纸比例', 'SCALE']],
  ['company', '公司名称', ['公司', '公司名称', '单位', '单位名称', 'COMPANY']],
] as const

export function cleanCadText(value: string): string {
  return value.replace(/\\U\+([0-9a-f]{4})/gi, (_, code: string) => String.fromCharCode(parseInt(code, 16)))
    .replace(/\\([\\{}])|\\[Pp~]|\\[WHTQACFcf][^;]*;|\\[LlOoKk]|\\S([^;]*);|[{}]/g,
      (token, literal: string, stacked: string) => literal ?? (stacked ? stacked.replace(/[\^#]/g, '/') : /^\\[Pp~]$/.test(token) ? ' ' : ''))
    .replace(/%%d/gi, '°').replace(/%%p/gi, '±').replace(/%%c/gi, 'Φ').replace(/\s+/g, ' ').trim()
}
const normalize = (text: string) => text.replace(/[\s_:：\-]/g, '').toUpperCase()
function fieldKey(text: string): string | undefined {
  const key = normalize(text)
  return definitions.find(([, , aliases]) => aliases.some(alias => normalize(alias) === key))?.[0]
}

type Point = { x: number; y: number; z: number }
type Matrix = { elements: number[] }
function transform(point: Point, matrices: Matrix[]): Point {
  return matrices.reduce((p, matrix) => {
    const e = matrix.elements
    return { x: e[0]! * p.x + e[4]! * p.y + e[8]! * p.z + e[12]!, y: e[1]! * p.x + e[5]! * p.y + e[9]! * p.z + e[13]!, z: e[2]! * p.x + e[6]! * p.y + e[10]! * p.z + e[14]! }
  }, point)
}

export function collectTitleSpaces(database: AcDbDatabase): TitleSpace[] {
  const blocks = Array.from(database.tables.blockTable.newIterator(true))
  const byName = new Map(blocks.map(block => [block.name, block]))
  const spaces = blocks.filter(block => block.name === '*Model_Space' || block.name.startsWith('*Paper_Space') || block.layoutId)
  let visited = 0
  return spaces.map(block => {
    const texts: TitleText[] = []
    const warnings = new Set<string>()
    function read(entity: any, matrices: Matrix[]) {
      const text = cleanCadText(String(entity.contents ?? entity.textString ?? ''))
      if (!text) return
      const aligned = entity.horizontalMode || entity.verticalMode
      const location = entity.location ?? (aligned ? entity.alignmentPoint : entity.position)
      if (!location || !Number.isFinite(location.x) || !Number.isFinite(location.y)) return
      const angle = Number(entity.rotation ?? 0)
      const height = Number(entity.height) || 1
      const p = { x: location.x, y: location.y, z: location.z ?? 0 }
      const origin = transform(p, matrices)
      const axis = transform({ x: p.x + Math.cos(angle), y: p.y + Math.sin(angle), z: p.z }, matrices)
      const top = transform({ x: p.x - Math.sin(angle) * height, y: p.y + Math.cos(angle) * height, z: p.z }, matrices)
      texts.push({ text, tag: entity.tag, x: origin.x, y: origin.y, height: Math.max(0.001, Math.hypot(top.x - origin.x, top.y - origin.y)), angle: Math.atan2(axis.y - origin.y, axis.x - origin.x) })
    }
    function walk(entities: Iterable<any>, matrices: Matrix[], ancestors: Set<string>) {
      for (const entity of entities) {
        if (++visited > 200000) throw new Error('图纸实体过多，请使用较小的单张图纸提取信息')
        const type = entity.dxfTypeName
        if (['TEXT', 'MTEXT', 'ATTRIB'].includes(type) || (type === 'ATTDEF' && entity.isConst)) read(entity, matrices)
        if (type !== 'INSERT') continue
        // 附属属性坐标已在插入块所属空间中，不能再次应用本块变换。
        if (entity.attributeIterator) for (const attribute of entity.attributeIterator()) read(attribute, matrices)
        const child = byName.get(entity.blockName)
        if (!child || ancestors.has(child.name) || ancestors.size > 32) continue
        const normal = entity.normal
        if (normal && (Math.abs(normal.x) > 1e-6 || Math.abs(normal.y) > 1e-6 || Math.abs(normal.z - 1) > 1e-6)) {
          warnings.add('存在非标准平面方向的块，已跳过其内部文字；提取结果可能不完整。')
          continue
        }
        walk(child.newIterator(), [entity.blockTransform, ...matrices], new Set(ancestors).add(child.name))
      }
    }
    walk(block.newIterator(), [], new Set([block.name]))
    return { id: String(block.objectId), name: block.name === '*Model_Space' ? '模型空间' : block.name, texts, warnings: [...warnings] }
  })
}

export function extractTitleFields(texts: TitleText[]): TitleField[] {
  const hits = new Map<string, { value: string; source: string; priority: number }[]>()
  const add = (key: string, value: string, source: string, priority: number) => {
    if (!value || fieldKey(value)) return
    hits.set(key, [...(hits.get(key) ?? []), { value, source, priority }])
  }
  for (const text of texts) {
    const key = text.tag && fieldKey(text.tag.replace(/[_＿](?:人员编号|人员姓名|姓名)$/, ''))
    if (key) add(key, text.text, '块属性', 3)
    const inline = text.text.match(/^([^:：]{1,16})[:：]\s*(.+)$/)
    const inlineKey = inline && fieldKey(inline[1]!)
    if (inlineKey) add(inlineKey, inline![2]!.trim(), '标签文字', 2)
  }
  const labels = texts.filter(text => fieldKey(text.text))
  const relative = (origin: TitleText, text: TitleText) => {
    const x = text.x - origin.x, y = text.y - origin.y
    return { x: x * Math.cos(origin.angle) + y * Math.sin(origin.angle), y: -x * Math.sin(origin.angle) + y * Math.cos(origin.angle) }
  }
  for (const label of labels) {
    const key = fieldKey(label.text)!
    const row = texts.filter(text => {
      if (text === label) return false
      const p = relative(label, text)
      return p.x > label.height * 0.5 && p.x < label.height * 18 && Math.abs(p.y) <= label.height * 0.8
    }).sort((a, b) => relative(label, a).x - relative(label, b).x)
    // 下一个标签是单元格边界；空单元格不可越过边界借用其他签名。
    const next = row[0]
    if (next && !fieldKey(next.text)) add(key, next.text, '标签右侧（待核对）', 1)
    if (next && !fieldKey(next.text)) continue
    if (!['number', 'name', 'material', 'scale', 'company'].includes(key)) continue
    const below = texts.filter(text => {
      const p = relative(label, text)
      return p.y < -label.height && p.y > -label.height * 6 && Math.abs(p.x) < label.height * 3
    }).sort((a, b) => relative(label, b).y - relative(label, a).y)[0]
    if (below && !fieldKey(below.text)) add(key, below.text, '标签下方（待核对）', 1)
  }
  const anchors = labels.filter(label => ['designer', 'checker', 'process'].includes(fieldKey(label.text)!))
  const region = texts.filter(text => !fieldKey(text.text) && !text.tag && anchors.some(anchor => {
    const p = relative(anchor, text)
    return p.x > -anchor.height * 3 && p.x < anchor.height * 100 && p.y > -anchor.height * 10 && p.y < anchor.height * 16
  }))
  for (const text of region) {
    const value = text.text.replace(/\s/g, '')
    if (/^(?=.*\d)[A-Za-z0-9]+(?:[-/.][A-Za-z0-9]+){2,}$/.test(value) && !/^\d{4}[-/.]\d{1,2}[-/.]\d{1,2}$/.test(value)) add('number', value, '标题栏图号候选（待核对）', 0)
    if (/^(?:\d{1,3}(?:Si|Mn|Cr|Ni|Mo|Ti|Al|Cu|[钢号#]))[A-Za-z0-9]*$|^(?:Q\d{3}[A-Z]*|[HQ]T\d{2,3}|SUS\d{3})$/i.test(value)) add('material', value, '标题栏材料候选（待核对）', 0)
    if (/\d+(?:\.\d+)?[:：]\d+(?:\.\d+)?/.test(value) && /^\d+(?:\.\d+)?[:：]\d+(?:\.\d+)?$/.test(value)) add('scale', value, '标题栏比例候选（待核对）', 0)
    if (/[\u4e00-\u9fff].*(?:公司|研究院|设计院|制造厂)$/.test(value)) add('company', text.text, '标题栏单位候选（待核对）', 0)
  }
  const names = region.filter(text => {
    if (!/^[\u4e00-\u9fffA-Za-z0-9（）()·-]{2,24}$/.test(text.text.replace(/\s/g, '')) || !/[\u4e00-\u9fff]/.test(text.text)) return false
    if (/公司|日期|签字|文件|标记|处数|共.*张|第.*张|公差|技术|要求|重量|图样/.test(text.text)) return false
    return anchors.some(anchor => {
      const p = relative(anchor, text)
      return p.x > anchor.height * 20 && p.x < anchor.height * 65 && p.y > anchor.height * 2 && p.y < anchor.height * 14
    })
  })
  for (const text of names) add('name', text.text, '标题栏名称候选（待核对）', 0)
  return definitions.map(([key, label]) => {
    const all = hits.get(key) ?? []
    const priority = Math.max(-1, ...all.map(hit => hit.priority))
    const best = all.filter(hit => hit.priority === priority)
    const candidates = [...new Set(best.map(hit => hit.value))]
    return { key, label, value: candidates.length === 1 ? candidates[0]! : '', source: candidates.length > 1 ? '多个候选，请选择或填写' : best[0]?.source ?? '未识别', candidates }
  })
}
