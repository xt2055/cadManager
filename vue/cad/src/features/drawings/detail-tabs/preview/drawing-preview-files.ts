import type { DrawingSummaryView, FileView, PartView, StructureNodeView } from '@/modules/drawing'
import type { DrawingFile } from '@/types/domain.types'
import { isModelFile } from '@/utils/model-formats'

/** 项目级文件清单里的一条：附件本身，外加它归属的图纸/零件。 */
export type ProjectDrawingFile = DrawingFile & {
  ownerNo: string
  ownerName: string
  /**
   * 借用图：附件属于别的项目（本项目的零件视图是借用关系）。
   * 借用方只读展示，改文件要回来源图号发起。
   */
  borrowed?: boolean
}

/** 文件视图 → 领域文件：补齐原始名称与存储键，并复制历史，避免下游改到视图对象。 */
export function toDrawingFile(file: FileView): DrawingFile {
  return {
    ...file,
    rawName: file.name,
    rawStorageKey: file.storageKey,
    history: file.history.map((item) => ({ ...item })),
  }
}

/** 结构树拍平成零件列表（含各层子件）。 */
export function flattenStructure(nodes: StructureNodeView[]): PartView[] {
  return nodes.flatMap((node) => [node, ...flattenStructure(node.children)])
}

/** 去重后按「总图 → 零件图 → 其他」排序，同级按归属方与文件名稳定排列。 */
export function sortProjectFiles(files: ProjectDrawingFile[]): ProjectDrawingFile[] {
  const uniqueFiles = files.filter((file, index, sourceFiles) => sourceFiles.findIndex((candidate) => candidate.id === file.id) === index)
  return uniqueFiles.sort((left, right) => {
    const roleRank = (file: ProjectDrawingFile) => file.role === 'assembly' ? 0 : file.role === 'part' ? 1 : 2
    return roleRank(left) - roleRank(right) || left.ownerNo.localeCompare(right.ownerNo) || left.name.localeCompare(right.name)
  })
}

export interface CollectProjectFilesInput {
  /** 当前打开的是总图还是零件图；为空时返回空清单。 */
  currentItem: DrawingSummaryView | PartView | null
  /** 当前项是否为总图（无 parentNo）。 */
  isAssembly: boolean
  /** 项目根图号，用于回填零件附件的 drawingNo。 */
  rootDrawingNo: string
  /** 全部零件：旧快照没有结构树时，靠父级链回退判断归属。 */
  parts: readonly PartView[]
  /** 读取某张总图的服务端结构树。 */
  structureOf: (drawingNo: string) => StructureNodeView[]
}

// 总图页是项目级文件清单：总图与结构树内全部零件图都必须在这里出现。
// 文件的归属从结构关系取得，而不依赖 CAD 标题栏或文件角色的历史数据。
export function collectProjectFiles(input: CollectProjectFilesInput): ProjectDrawingFile[] {
  const { currentItem, isAssembly, rootDrawingNo, parts, structureOf } = input
  if (!currentItem) return []

  const files: ProjectDrawingFile[] = []
  const appendOwnerFiles = (owner: DrawingSummaryView | PartView) => {
    for (const file of [...(owner.files ?? []), ...(owner.otherFiles ?? [])]) {
      files.push({
        ...toDrawingFile(file),
        ownerNo: owner.no,
        ownerName: owner.name,
        ...('parentNo' in owner && owner.borrowed ? { borrowed: true } : {}),
        ...(file.partNo ? { partNo: file.partNo } : {}),
        drawingNo: file.drawingNo || ('parentNo' in owner ? rootDrawingNo : owner.no),
      })
    }
  }

  // 如果是总图，递归汇总总图和所有后代零件。
  if (isAssembly) {
    const mainDrawing = currentItem as DrawingSummaryView
    appendOwnerFiles(mainDrawing)
    const belongsToDrawing = (part: PartView): boolean => {
      if (part.parentNo === mainDrawing.no || part.no.startsWith(`${mainDrawing.no}-`)) return true

      const visited = new Set<string>()
      let parentNo = part.parentNo
      while (parentNo && !visited.has(parentNo)) {
        if (parentNo === mainDrawing.no) return true
        visited.add(parentNo)
        const parent = parts.find((candidate) => candidate.no === parentNo)
        if (!parent) return part.no.startsWith(`${mainDrawing.no}-`)
        parentNo = parent.parentNo
      }
      return false
    }

    // 优先使用服务端归属到当前总图的结构树；外部借用件、图号前缀不规范的零件
    // 与多层子件都能被递归汇总。旧快照没有结构树时才回退为父级链判断。
    const structureParts = flattenStructure(structureOf(mainDrawing.no))
    const projectParts = structureParts.length
      ? structureParts
      : parts.filter(belongsToDrawing)
    projectParts.forEach(appendOwnerFiles)
  } else {
    // 如果当前选中的就是零件图
    appendOwnerFiles(currentItem as PartView)
  }

  return sortProjectFiles(files.filter((file) => !isModelFile(file)))
}
