import { DRAWING_2D_EXTENSIONS, MODEL_EXTENSIONS, fileFormat } from '../../../utils/model-formats.ts'

/**
 * 图纸创建的三种方式。
 * - new：创建新图纸，只输入图纸名称与图号，图纸材料可选
 * - legacy：上传老图纸，导入既有 2D 总图/零件图并保留原图号
 * - fork：从老图纸分叉，继承源图纸结构与文件生成新图号
 */
export type DrawingCreateMode = 'new' | 'legacy' | 'fork'

/** 无 mode 参数时保持历史行为：进入上传老图纸向导。 */
export const DEFAULT_CREATE_MODE: DrawingCreateMode = 'legacy'

export interface CreateModeOption {
  value: DrawingCreateMode
  title: string
  sub: string
  icon: string
}

export const CREATE_MODE_OPTIONS: CreateModeOption[] = [
  { value: 'new', title: '创建新图纸', sub: '只填图纸名称与图号，材料可选', icon: 'file-plus' },
  { value: 'legacy', title: '上传老图纸', sub: '导入既有 2D 总图与零件图', icon: 'upload' },
  { value: 'fork', title: '从老图纸分叉', sub: '继承老图纸结构与文件', icon: 'git-branch' },
]

export function resolveCreateMode(value: unknown): DrawingCreateMode {
  const candidate = Array.isArray(value) ? value[0] : value
  return candidate === 'new' || candidate === 'fork' || candidate === 'legacy' ? candidate : DEFAULT_CREATE_MODE
}

export function createModeTitle(mode: DrawingCreateMode): string {
  return CREATE_MODE_OPTIONS.find((option) => option.value === mode)?.title ?? '创建图纸'
}

/** 图纸材料统一进「资料档案」，这里只按扩展名给列表一个可读标签。 */
export type MaterialLabel = '图纸文件' | '3D 模型' | '图片' | '文档' | '其他文件'

const IMAGE_EXTENSIONS = ['png', 'jpg', 'jpeg', 'gif', 'bmp', 'webp', 'svg', 'tif', 'tiff', 'heic', 'avif']
const DOCUMENT_EXTENSIONS = ['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'txt', 'csv', 'md', 'rtf', 'odt', 'ods', 'odp', 'wps', 'et']

/** 材料列表与汇总里显示的标签：只做展示，归档去向统一是资料档案。 */
export function materialLabel(name: string): MaterialLabel {
  const extension = fileFormat(name)
  if (DRAWING_2D_EXTENSIONS.includes(extension)) return '图纸文件'
  if (MODEL_EXTENSIONS.includes(extension)) return '3D 模型'
  if (IMAGE_EXTENSIONS.includes(extension)) return '图片'
  if (DOCUMENT_EXTENSIONS.includes(extension)) return '文档'
  return '其他文件'
}

/**
 * 新图纸的图纸材料不限制扩展名：现场照片、检验报告、压缩包等都要能直接收，
 * 所以 accept 放开为全部文件；材料在创建成功后统一归入该图纸的资料档案。
 */
export const NEW_DRAWING_MATERIAL_ACCEPT = '*/*'
