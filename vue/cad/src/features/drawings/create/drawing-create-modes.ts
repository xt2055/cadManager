import { DRAWING_2D_ACCEPT, DRAWING_2D_EXTENSIONS, MODEL_EXTENSIONS, fileFormat } from '../../../utils/model-formats.ts'

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
  { value: 'legacy', title: '上传老图纸', sub: '导入既有 2D 总图与零件图', icon: 'folder-up' },
  { value: 'fork', title: '从老图纸分叉', sub: '继承老图纸结构与文件', icon: 'git-branch' },
]

export function resolveCreateMode(value: unknown): DrawingCreateMode {
  const candidate = Array.isArray(value) ? value[0] : value
  return candidate === 'new' || candidate === 'fork' || candidate === 'legacy' ? candidate : DEFAULT_CREATE_MODE
}

export function createModeTitle(mode: DrawingCreateMode): string {
  return CREATE_MODE_OPTIONS.find((option) => option.value === mode)?.title ?? '创建图纸'
}

/** 图纸材料在图纸上的归属：2D 图纸进图纸文件，其余进其他文件。 */
export interface MaterialFileKind {
  role: 'assembly' | 'other'
  fileCategory: 'drawing2d' | 'model3d' | 'other'
  label: '图纸文件' | '3D 模型' | '其他文件'
  previewable: boolean
}

const MATERIAL_EXTRA_EXTENSIONS = ['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'txt', 'csv', 'zip', 'rar', '7z']

/**
 * 按扩展名判定图纸材料的归属：2D 工程图（含 PDF）作为总图文件，
 * 3D 模型与普通资料归入其他文件，避免污染图纸文件列表。
 */
export function materialFileKind(name: string): MaterialFileKind {
  const extension = fileFormat(name)
  if (DRAWING_2D_EXTENSIONS.includes(extension)) {
    return { role: 'assembly', fileCategory: 'drawing2d', label: '图纸文件', previewable: true }
  }
  if (MODEL_EXTENSIONS.includes(extension)) {
    return { role: 'other', fileCategory: 'model3d', label: '3D 模型', previewable: false }
  }
  return { role: 'other', fileCategory: 'other', label: '其他文件', previewable: false }
}

/** 新图纸可选的图纸材料范围：2D 图纸 + 3D 模型 + 常见办公与压缩格式。 */
export const NEW_DRAWING_MATERIAL_ACCEPT = [
  DRAWING_2D_ACCEPT,
  ...MODEL_EXTENSIONS.map((extension) => `.${extension}`),
  ...MATERIAL_EXTRA_EXTENSIONS.map((extension) => `.${extension}`),
].join(',')
