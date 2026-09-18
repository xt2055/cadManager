// 带 .ts 扩展名：本模块由 node --experimental-strip-types 直接跑测试，扩展名省略时 Node ESM 解析不到。
import { directParentDrawingNo, drawingNumberRoot, isSameDrawingFamily } from './drawing-number-parser.ts'

/**
 * 手工补录图号的纯逻辑（无 Vue 状态、无 IO）。
 *
 * 图号的主来源是**图幅（标题栏）**，文件名不可信（可能忘改）。只有在图幅里确实读不到图号时，
 * 才让用户手工补录：补录时用户只填后几位，项目号自动带出来；项目号允许修改 —— 借用件本来就跨项目号。
 */

function trimTailSeparators(value: string): string {
  return value.trim().replace(/[-\s]+$/, '')
}

function trimHeadSeparators(value: string): string {
  return value.trim().replace(/^[-\s]+/, '')
}

/**
 * 只有像「项目号」那样字母数字混排且够长的首段，才算自带根号。
 * 阈值取 4 是为了不把 01c 这类零件段误判成根号，否则会漏拼项目号。
 */
function looksLikeRoot(segment: string): boolean {
  return segment.length >= 4 && /[A-Za-z]/.test(segment) && /\d/.test(segment)
}

/**
 * 完整图号 = 项目号 + 后几位。
 * 已经自带项目号的（用户从图幅整段粘贴）以及自带根号的（借用件直接粘别族图号）原样接受，
 * 不重复拼接；后几位缺少前导 `-` 时自动补一个。
 */
export function composePartNo(projectNo: string, suffix: string): string {
  const prefix = trimTailSeparators(projectNo)
  const tail = trimHeadSeparators(suffix)
  if (!tail) return prefix
  if (!prefix) return tail
  if (tail.toLowerCase().startsWith(prefix.toLowerCase())) return tail
  const firstSegment = tail.split('-')[0]
  if (firstSegment && looksLikeRoot(firstSegment)) return tail
  return `${prefix}-${tail}`
}

/** 手工补录的项目号默认值：当前项目图号的族根。 */
export function defaultProjectNo(rootDrawingNo: string): string {
  const value = rootDrawingNo.trim()
  return value ? drawingNumberRoot(value) : ''
}

/** 借用件判定：图号不属于当前项目图号族。 */
export function isBorrowedNumber(partNo: string, projectFamilyNo: string): boolean {
  const value = partNo.trim()
  const family = projectFamilyNo.trim()
  if (!value || !family) return false
  return !isSameDrawingFamily(value, family)
}

/** 直接上级图号：图号去掉最后一段 -N；没有层级时返回 null。 */
export function parentPartNoOf(partNo: string): string | null {
  const value = partNo.trim()
  return value ? directParentDrawingNo(value) : null
}

/** 校验手工补录的图号，返回可直接展示给用户的原因；通过时返回空串。 */
export function validateManualPartNo(partNo: string): string {
  const value = partNo.trim()
  if (!value) return '请输入图号'
  if (/\s/.test(value)) return '图号不能包含空格'
  if (!/\d/.test(value)) return '图号至少需要包含一个数字'
  if (!/[A-Za-z]/.test(value) && !value.includes('-')) return '图号格式不正确，例如 2000W.02.03d-01-01c'
  return ''
}
