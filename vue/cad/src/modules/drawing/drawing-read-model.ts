import type { BomItem, Drawing, StructurePart } from '@/types/domain.types'
import type { DrawingQuerySnapshot } from './drawing-query-service'

export type DrawingId = string
export type PartId = string

export interface DrawingSummaryView {
  id: DrawingId
  no: string
  name: string
  kind: Drawing['kind']
  project: string
  status: string
  version: string
  updatedAt: string
  designer?: string
  borrowed: boolean
  sourceDrawing?: string
  publishedVersion?: string
  fileCount: number
}

export interface PartView {
  id: PartId
  no: string
  name: string
  parentNo: string
  project?: string
  material: string
  spec: string
  qty: number
  status: string
  version: string
  updatedAt: string
  borrowed: boolean
  sourcePartId?: PartId
  sourceDrawing?: string
  publishedVersion?: string
  fileCount: number
}

export interface StructureNodeView extends PartView {
  children: StructureNodeView[]
}

export interface DrawingReadModelSnapshot {
  drawings: DrawingSummaryView[]
  parts: PartView[]
  structureByDrawing: Record<string, StructureNodeView[]>
  bom: BomItem[]
}

/** 将查询 DTO 映射为页面可读模型；页面不再持有 Drawing/StructurePart 实体引用。 */
export class DrawingReadModelMapper {
  map(snapshot: DrawingQuerySnapshot): DrawingReadModelSnapshot {
    const drawingsByNo = new Map(snapshot.drawings.map((drawing) => [drawing.no, drawing]))
    const partsByNo = new Map(snapshot.structure.map((part) => [part.no, part]))

    const drawings = snapshot.drawings.map((drawing) => this.toDrawingSummary(drawing, drawingsByNo))
    const parts = snapshot.structure.map((part) => this.toPartView(part, drawingsByNo, partsByNo))
    const nodes = new Map(parts.map((part) => [part.no, { ...part, children: [] as StructureNodeView[] }]))
    const structureByDrawing: Record<string, StructureNodeView[]> = {}

    for (const part of snapshot.structure) {
      const node = nodes.get(part.no)
      if (!node) continue
      const parentNode = nodes.get(part.parentNo)
      if (parentNode) {
        parentNode.children.push(node)
        continue
      }
      const rootDrawingNo = drawingsByNo.has(part.parentNo) ? part.parentNo : this.findRootDrawingNo(part, partsByNo, drawingsByNo)
      if (rootDrawingNo) (structureByDrawing[rootDrawingNo] ??= []).push(node)
    }

    return { drawings, parts, structureByDrawing, bom: snapshot.bom.map((item) => ({ ...item })) }
  }

  private toDrawingSummary(drawing: Drawing, drawingsByNo: Map<string, Drawing>): DrawingSummaryView {
    const source = drawing.borrowFrom ? drawingsByNo.get(drawing.borrowFrom) : undefined
    return {
      id: drawing.id ?? drawing.no,
      no: drawing.no,
      name: drawing.name,
      kind: drawing.kind,
      project: drawing.project,
      status: drawing.status,
      version: drawing.ver,
      updatedAt: drawing.updatedAt ?? drawing.updated,
      ...(drawing.designer ? { designer: drawing.designer } : {}),
      borrowed: Boolean(drawing.borrowFrom),
      ...(drawing.borrowFrom ? { sourceDrawing: drawing.borrowFrom } : {}),
      ...(source?.ver ? { publishedVersion: source.ver } : {}),
      fileCount: (drawing.files?.length ?? 0) + (drawing.otherFiles?.length ?? 0),
    }
  }

  private toPartView(part: StructurePart, drawingsByNo: Map<string, Drawing>, partsByNo: Map<string, StructurePart>): PartView {
    const sourcePart = part.borrowFrom ? partsByNo.get(part.borrowFrom) : undefined
    const sourceDrawing = part.borrowFrom ? this.findRootDrawingNo(sourcePart ?? part, partsByNo, drawingsByNo) : undefined
    return {
      id: part.id ?? part.no,
      no: part.no,
      name: part.name,
      parentNo: part.parentNo,
      ...(part.project ? { project: part.project } : {}),
      material: part.material,
      spec: part.spec,
      qty: part.qty,
      status: part.status,
      version: part.ver,
      updatedAt: part.updatedAt ?? part.createdAt ?? '',
      borrowed: Boolean(part.borrowFrom),
      ...(sourcePart ? { sourcePartId: sourcePart.id ?? sourcePart.no } : {}),
      ...(sourceDrawing ? { sourceDrawing } : {}),
      ...(sourcePart?.ver ? { publishedVersion: sourcePart.ver } : {}),
      fileCount: (part.files?.length ?? 0) + (part.otherFiles?.length ?? 0),
    }
  }

  private findRootDrawingNo(
    part: StructurePart,
    partsByNo: Map<string, StructurePart>,
    drawingsByNo: Map<string, Drawing>,
  ): string | undefined {
    const visited = new Set<string>()
    let parentNo = part.parentNo
    while (parentNo && !visited.has(parentNo)) {
      if (drawingsByNo.has(parentNo)) return parentNo
      visited.add(parentNo)
      const parent = partsByNo.get(parentNo)
      if (!parent) return undefined
      parentNo = parent.parentNo
    }
    return undefined
  }
}
