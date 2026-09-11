export type FileCategory = 'drawing2d' | 'model3d' | 'other'

export const MODEL_EXTENSIONS = ['z3prt', 'z3asm', 'z3', 'step', 'stp', 'iges', 'igs', 'stl', 'obj', 'glb', 'gltf', 'sldprt', 'sldasm', 'prt', 'asm', 'catpart', 'catproduct', 'ipt', 'iam', 'x_t', 'x_b', 'sat', 'sab', 'jt', '3mf', '3dm', 'ifc', 'ply', 'fbx']
export const DRAWING_2D_EXTENSIONS = ['exb', 'dwg', 'dxf', 'pdf', 'slddrw', 'catdrawing', 'idw', 'drw', 'z3drw']
export const MODEL_FILE_ACCEPT = MODEL_EXTENSIONS.map((ext) => `.${ext}`).join(',')
export const MODEL_ACCEPT = `${MODEL_FILE_ACCEPT},.zip`
export const DRAWING_2D_ACCEPT = DRAWING_2D_EXTENSIONS.map((ext) => `.${ext}`).join(',')
export const DRAWING_ACCEPT = `${DRAWING_2D_ACCEPT},${MODEL_FILE_ACCEPT}`

export function fileFormat(name: string): string {
  const value = name.trim().toLowerCase()
  return value.match(/\.(prt|asm)\.\d+$/)?.[1] ?? value.match(/\.([^./\\]+)$/)?.[1] ?? ''
}

export function fileCategory(file: { name: string; fileCategory?: string }): FileCategory {
  if (file.fileCategory && file.fileCategory !== 'auto') return file.fileCategory as FileCategory
  const ext = fileFormat(file.name)
  if (MODEL_EXTENSIONS.includes(ext)) return 'model3d'
  if (DRAWING_2D_EXTENSIONS.includes(ext)) return 'drawing2d'
  return 'other'
}

export function isDrawing2DFile(file: { name: string; fileCategory?: string }): boolean {
  return fileCategory(file) === 'drawing2d'
}

export function isModelFile(file: { name: string; fileCategory?: string }): boolean {
  return fileCategory(file) === 'model3d'
}

export function acceptsModel(name: string): boolean {
  return MODEL_EXTENSIONS.includes(fileFormat(name)) || fileFormat(name) === 'zip'
}

export function drawingMediaLabel(owner: { files?: { name: string; fileCategory?: string }[]; otherFiles?: { name: string; fileCategory?: string }[] }): string {
  const categories = new Set([...(owner.files ?? []), ...(owner.otherFiles ?? [])].map(fileCategory))
  if (categories.has('drawing2d') && categories.has('model3d')) return '2D + 3D'
  if (categories.has('model3d')) return '3D'
  if (categories.has('drawing2d')) return '2D'
  return '未上传图纸'
}
