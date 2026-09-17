import type { DrawingFileIdentity } from '@/types/application.types'
import { parseDrawingNumber } from '@/utils/drawing-number-parser'

/**
 * 上传零件图的领域决策（纯函数，无 Vue 状态、无 IO）。
 *
 * 原先这套判断混在 DrawingPreviewTab 的 onPartFilesChange 里：扩展名分类、
 * 图号解析、借用判断、结构件判断、父级是否存在、是否已有同名零件，
 * 以及最终走哪条持久化路径。现在只留下「决策」，执行仍由调用方负责。
 */

export type PartUploadRole = 'part' | 'other'

/** CAD 图纸扩展名：只有它们才走 CAD 识别与建档，其余一律归入「其他文件」。 */
export function isCadPartFile(fileName: string): boolean {
  const extension = fileName.toLowerCase().match(/\.[^.]+$/)?.[0] || ''
  return extension === '.exb' || extension === '.dwg' || extension === '.dxf'
}

/** 材料取值：优先识别结果，其次标题栏常见字段。 */
export function materialOf(identity: DrawingFileIdentity): string {
  return identity.material || identity.titleBlock?.['材料名称'] || identity.titleBlock?.['材料'] || identity.titleBlock?.['材质'] || '—'
}

export interface PartUploadPlanInput {
  /** 当前项目根图号。 */
  rootDrawingNo: string
  /** drawingFileService.identify 的结果。 */
  identity: DrawingFileIdentity
  /** 已有零件：判断父级是否存在、是否已有同名零件。 */
  parts: readonly { no: string; parentNo?: string }[]
  /** 已有图纸图号：判断父级是否存在。 */
  drawingNos: readonly string[]
}

export interface PartUploadPlanBase {
  /** 附件角色：只有规范的结构件才是 part，其余一律 other。 */
  role: PartUploadRole
  /** 规范结构件才带图号；非结构件不写 partNo。 */
  partNo?: string
  material: string
}

export type PartUploadPlan =
  | (PartUploadPlanBase & { kind: 'other-file'; reason: 'not-structured-part' | 'parent-missing' })
  | (PartUploadPlanBase & { kind: 'attach-to-existing-part'; partNo: string })
  | (PartUploadPlanBase & { kind: 'create-part'; partNo: string; parentNo: string; borrowFrom?: string })

/**
 * 判定一个已识别的 CAD 文件该怎么落库：
 * - other-file：非结构件或父级不存在 → 归入当前总图的「其他文件」；
 * - attach-to-existing-part：项目里已有同名零件 → 作为它的新附件上传；
 * - create-part：新建零件并挂到 parentNo 下（借用件记录 borrowFrom）。
 */
export function planPartUpload(input: PartUploadPlanInput): PartUploadPlan {
  const parsed = parseDrawingNumber(input.identity.partNo)
  const material = materialOf(input.identity)
  const isBorrowed = parsed.rootNo !== input.rootDrawingNo
  const isStructuredPart = parsed.no !== input.rootDrawingNo
  const parentNo = isBorrowed ? input.rootDrawingNo : parsed.parentNo ?? input.rootDrawingNo

  if (!isStructuredPart) return { kind: 'other-file', reason: 'not-structured-part', role: 'other', material }

  const parentExists = input.drawingNos.includes(parentNo) || input.parts.some((part) => part.no === parentNo)
  if (!parentExists) return { kind: 'other-file', reason: 'parent-missing', role: 'part', partNo: parsed.no, material }

  const existing = input.parts.find((part) => part.no === parsed.no && part.parentNo === input.rootDrawingNo)
  if (existing) return { kind: 'attach-to-existing-part', role: 'part', partNo: parsed.no, material }

  return {
    kind: 'create-part',
    role: 'part',
    partNo: parsed.no,
    parentNo,
    material,
    ...(isBorrowed ? { borrowFrom: parsed.rootNo ?? parsed.no } : {}),
  }
}
