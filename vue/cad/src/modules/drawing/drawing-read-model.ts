import type { BomItem, CraftFile, Drawing, DrawingFile, DrawingSigners, MaterialFile, StructurePart } from '@/types/domain.types'
import type { DrawingQuerySnapshot } from './drawing-query-service'
import type { StoredAttachment } from '@/types/application.types'
import { formatReadableDateTime } from '@/utils/date-time'

export type DrawingId = string
export type PartId = string

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export interface FileView {
  id: string
  name: string
  size: string
  uploadedAt: string
  version: string
  role: 'assembly' | 'part' | 'other'
  drawingNo: string
  partNo?: string
  uploadedBy: string
  previewable: boolean
  storageKey?: string
  currentStorageKey?: string
  replacedBy?: string
  replacedAt?: string
  replaceReason?: string
  mimeType?: string
  history: FileHistoryView[]
}

export interface FileHistoryView {
  id: string
  name: string
  size: string
  version: string
  uploadedBy: string
  uploadedAt: string
  replacedBy?: string
  replacedAt?: string
  replaceReason?: string
  storageKey?: string
  mimeType?: string
  previewable: boolean
}

export interface MaterialFileView {
  id: string
  drawingNo: string
  name: string
  size: string
  version: string
  uploadedBy: string
  uploadedAt: string
  storageKey?: string
  mimeType?: string
  author?: string
}

export interface CraftFileView {
  id: string
  drawingNo: string
  name: string
  op: string
  ver: string
  by: string
  date: string
  size: string
  storageKey?: string
  mimeType?: string
  previewable: boolean
  author?: string
  scanned: boolean
}

export interface DrawingSummaryView {
  id: DrawingId
  no: string
  name: string
  kind: Drawing['kind']
  project: string
  revision?: number
  vendor: string
  remark?: string
  attributeValues: Record<string, string>
  status: Drawing['status']
  version: string
  updatedAt: string
  createdBy?: string
  forkedFrom?: string
  designer?: string
  signers?: Partial<DrawingSigners>
  borrowed: boolean
  sourceDrawing?: string
  publishedVersion?: string
  fileCount: number
  files: FileView[]
  otherFiles: FileView[]
  materialFiles: MaterialFileView[]
  craftFiles: CraftFileView[]
}

export interface PartView {
	id: PartId
	relationId?: string
	relationRevision?: number
	relationType?: string
  no: string
  name: string
  parentNo: string
  project?: string
  material: string
  spec: string
  remark?: string
  qty: number
  revision?: number
  weight: number
  surfaceTreatment: string
  partType: StructurePart['partType']
  vendor?: string
  status: StructurePart['status']
  version: string
  updatedAt: string
  fileNames: string[]
  files: FileView[]
  otherFiles: FileView[]
  materialFiles: MaterialFileView[]
  craftFiles: CraftFileView[]
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
    const normalized = this.withAttachments(snapshot)
    const drawingsByNo = new Map(normalized.drawings.map((drawing) => [drawing.no, drawing]))
	    const partsByNo = new Map<string, StructurePart[]>()
	    for (const part of normalized.structure) {
	      const candidates = partsByNo.get(part.no) ?? []
	      candidates.push(part)
	      partsByNo.set(part.no, candidates)
	    }

    const drawings = normalized.drawings.map((drawing) => this.toDrawingSummary(drawing, drawingsByNo))
    const parts = normalized.structure.map((part) => this.toPartView(part, drawingsByNo, partsByNo))
	    const nodes = new Map<string, StructureNodeView>()
	    normalized.structure.forEach((part, index) => {
	      const view = parts[index]
	      if (view) nodes.set(this.partKey(part, index), { ...view, children: [] })
	    })
    const structureByDrawing: Record<string, StructureNodeView[]> = {}

    for (const part of normalized.structure) {
	      const node = nodes.get(this.partKey(part, normalized.structure.indexOf(part)))
	      if (!node) continue
	      const parent = (partsByNo.get(part.parentNo) ?? []).find((candidate) => candidate.drawingId === part.drawingId)
	      const parentNode = parent ? nodes.get(this.partKey(parent, normalized.structure.indexOf(parent))) : undefined
      if (parentNode) {
        parentNode.children.push(node)
        continue
      }
      const rootDrawingNo = drawingsByNo.has(part.parentNo) ? part.parentNo : this.findRootDrawingNo(part, partsByNo, drawingsByNo)
      if (rootDrawingNo) (structureByDrawing[rootDrawingNo] ??= []).push(node)
    }

    return { drawings, parts, structureByDrawing, bom: snapshot.bom.map((item) => ({ ...item })) }
  }

  private withAttachments(snapshot: DrawingQuerySnapshot): { drawings: Drawing[]; structure: StructurePart[] } {
    if (!snapshot.attachments.length) {
      return { drawings: snapshot.drawings, structure: snapshot.structure }
    }

    const drawings: Drawing[] = snapshot.drawings.map((drawing) => ({
      ...drawing,
      files: [],
      otherFiles: [],
      materialFiles: [],
      craftFiles: [],
    }))
    const structure: StructurePart[] = snapshot.structure.map((part) => ({
      ...part,
      files: [],
      otherFiles: [],
      materialFiles: [],
      craftFiles: [],
    }))
    const drawingByNo = new Map(drawings.map((drawing) => [drawing.no, drawing]))
	    const drawingIdsByNo = new Map(drawings.map((drawing) => [drawing.no, drawing.id]))
	    const partByNo = new Map<string, StructurePart[]>()
	    for (const part of structure) {
	      const candidates = partByNo.get(part.no) ?? []
	      candidates.push(part)
	      partByNo.set(part.no, candidates)
	    }

    for (const item of snapshot.attachments) {
	      const partCandidates = item.partNo ? partByNo.get(item.partNo) ?? [] : []
	      const owner = item.partNo
	        ? partCandidates.find((part) => part.drawingId === drawingIdsByNo.get(item.drawingNo)) ?? partCandidates[0]
	        : drawingByNo.get(item.drawingNo)
      if (!owner) continue
      const name = item.name || item.currentName || '未命名文件'
      const uploadedAt = formatReadableDateTime(item.createdAt, '历史记录')
      if (item.role === 'material') {
        const file: MaterialFile = {
          id: item.id,
          drawingNo: item.drawingNo,
          name,
          size: formatFileSize(item.currentSize ?? item.size),
          version: item.version,
          uploadedBy: item.uploadedBy || '未知用户',
          uploadedAt,
          storageKey: item.currentStorageKey || item.storageKey,
          mimeType: item.currentMimeType || item.mimeType,
        }
        owner.materialFiles = [...(owner.materialFiles ?? []).filter((candidate) => candidate.id !== file.id), file]
        continue
      }
      if (item.role === 'craft') {
        const file: CraftFile = {
          id: item.id,
          drawingNo: item.drawingNo,
          name,
          op: '未分类工艺',
          ver: item.version,
          by: item.uploadedBy || '未知用户',
          date: uploadedAt,
          size: formatFileSize(item.currentSize ?? item.size),
          storageKey: item.currentStorageKey || item.storageKey,
          mimeType: item.currentMimeType || item.mimeType,
          previewable: item.previewable,
          scanned: false,
        }
        owner.craftFiles = [...(owner.craftFiles ?? []).filter((candidate) => candidate.id !== file.id), file]
        continue
      }
      const file: DrawingFile = {
        id: item.id,
        name,
        rawName: item.currentName && item.currentName !== name ? item.currentName : undefined,
        size: formatFileSize(item.currentSize ?? item.size),
        role: item.role === 'assembly' || item.role === 'part' ? item.role : 'other',
        drawingNo: item.drawingNo,
        ...(item.partNo ? { partNo: item.partNo } : {}),
        version: item.version,
        uploadedBy: item.uploadedBy || '未知用户',
        uploadedAt,
        storageKey: item.currentStorageKey || item.storageKey,
        rawStorageKey: item.storageKey,
        currentStorageKey: item.currentStorageKey,
        mimeType: item.currentMimeType || item.mimeType,
        previewable: item.previewable,
        revision: item.revision,
      }
      const isMain = (item.role === 'assembly' && 'kind' in owner) || (item.role === 'part' && 'parentNo' in owner)
      if (isMain) owner.files = [...(owner.files ?? []).filter((candidate) => candidate.id !== file.id), file]
      else owner.otherFiles = [...(owner.otherFiles ?? []).filter((candidate) => candidate.id !== file.id), file]
      owner.hasFile = (owner.files ?? []).length > 0
    }
    return { drawings, structure }
  }

  private toDrawingSummary(drawing: Drawing, drawingsByNo: Map<string, Drawing>): DrawingSummaryView {
    const source = drawing.borrowFrom ? drawingsByNo.get(drawing.borrowFrom) : undefined
    return {
      id: drawing.id ?? drawing.no,
      no: drawing.no,
      name: drawing.name,
      kind: drawing.kind,
      project: drawing.project,
      ...(drawing.revision !== undefined ? { revision: drawing.revision } : {}),
      vendor: drawing.vendor,
      ...(drawing.remark ? { remark: drawing.remark } : {}),
      attributeValues: { ...(drawing.attributeValues ?? {}) },
      status: drawing.status,
      version: drawing.ver,
      updatedAt: drawing.updatedAt ?? drawing.updated,
      ...(drawing.createdBy ? { createdBy: drawing.createdBy } : {}),
      ...(drawing.forkedFrom ? { forkedFrom: drawing.forkedFrom } : {}),
      ...(drawing.designer ? { designer: drawing.designer } : {}),
      ...(drawing.signers ? { signers: { ...drawing.signers } } : {}),
      borrowed: Boolean(drawing.borrowFrom),
      ...(drawing.borrowFrom ? { sourceDrawing: drawing.borrowFrom } : {}),
      ...(source?.ver ? { publishedVersion: source.ver } : {}),
      fileCount: (drawing.files?.length ?? 0) + (drawing.otherFiles?.length ?? 0),
      files: (drawing.files ?? []).map((file) => this.toFileView(file)),
      otherFiles: (drawing.otherFiles ?? []).map((file) => this.toFileView(file)),
      materialFiles: (drawing.materialFiles ?? []).map((file) => this.toMaterialFileView(file)),
      craftFiles: (drawing.craftFiles ?? []).map((file) => this.toCraftFileView(file)),
    }
  }

	  private toPartView(part: StructurePart, drawingsByNo: Map<string, Drawing>, partsByNo: Map<string, StructurePart[]>): PartView {
	    const sourcePart = part.borrowFrom ? partsByNo.get(part.borrowFrom)?.[0] : undefined
	    const sourceDrawing = part.borrowFrom && drawingsByNo.has(part.borrowFrom)
	      ? part.borrowFrom
	      : sourcePart ? this.findRootDrawingNo(sourcePart, partsByNo, drawingsByNo) ?? part.borrowFrom : part.borrowFrom || undefined
	    return {
	      id: part.id ?? part.no,
	      ...(part.relationId ? { relationId: part.relationId } : {}),
	      ...(part.relationRevision !== undefined ? { relationRevision: part.relationRevision } : {}),
	      ...(part.relationType ? { relationType: part.relationType } : {}),
      no: part.no,
      name: part.name,
      parentNo: part.parentNo,
      ...(part.project ? { project: part.project } : {}),
      material: part.material,
      spec: part.spec,
      ...(part.remark ? { remark: part.remark } : {}),
      qty: part.qty,
      ...(part.revision !== undefined ? { revision: part.revision } : {}),
      weight: part.weight,
      surfaceTreatment: part.surfaceTreatment,
      partType: part.partType,
      ...(part.vendor ? { vendor: part.vendor } : {}),
      status: part.status,
      version: part.ver,
      updatedAt: part.updatedAt ?? part.createdAt ?? '',
      fileNames: [...new Set([...(part.files ?? []), ...(part.otherFiles ?? [])].map((file) => file.name).filter(Boolean))],
	      borrowed: part.relationType === 'borrowed' || Boolean(part.borrowFrom),
      ...(sourcePart ? { sourcePartId: sourcePart.id ?? sourcePart.no } : {}),
      ...(sourceDrawing ? { sourceDrawing } : {}),
      ...(sourcePart?.ver ? { publishedVersion: sourcePart.ver } : {}),
      fileCount: (part.files?.length ?? 0) + (part.otherFiles?.length ?? 0),
      files: (part.files ?? []).map((file) => this.toFileView(file)),
      otherFiles: (part.otherFiles ?? []).map((file) => this.toFileView(file)),
      materialFiles: (part.materialFiles ?? []).map((file) => this.toMaterialFileView(file)),
      craftFiles: (part.craftFiles ?? []).map((file) => this.toCraftFileView(file)),
    }
  }

  private toFileView(file: {
    id: string
    name: string
    size: string
    uploadedAt: string
    version: string
    role: 'assembly' | 'part' | 'other'
    drawingNo: string
    partNo?: string
    uploadedBy: string
    previewable: boolean
    storageKey?: string
    currentStorageKey?: string
    replacedBy?: string
    replacedAt?: string
    replaceReason?: string
    mimeType?: string
    history?: Array<{
      id: string
      name: string
      size: string
      version: string
      uploadedBy: string
      uploadedAt: string
      replacedBy?: string
      replacedAt?: string
      replaceReason?: string
      storageKey?: string
      mimeType?: string
      previewable: boolean
    }>
  }): FileView {
    return {
      id: file.id,
      name: file.name,
      size: file.size,
      uploadedAt: file.uploadedAt,
      version: file.version,
      role: file.role,
      drawingNo: file.drawingNo,
      ...(file.partNo ? { partNo: file.partNo } : {}),
      uploadedBy: file.uploadedBy,
      previewable: file.previewable,
      ...(file.storageKey ? { storageKey: file.storageKey } : {}),
      ...(file.currentStorageKey ? { currentStorageKey: file.currentStorageKey } : {}),
      ...(file.replacedBy ? { replacedBy: file.replacedBy } : {}),
      ...(file.replacedAt ? { replacedAt: file.replacedAt } : {}),
      ...(file.replaceReason ? { replaceReason: file.replaceReason } : {}),
      ...(file.mimeType ? { mimeType: file.mimeType } : {}),
      history: (file.history ?? []).map((item) => ({ ...item })),
    }
  }

  private toMaterialFileView(file: MaterialFile): MaterialFileView {
    return {
      id: file.id,
      drawingNo: file.drawingNo,
      name: file.name,
      size: file.size,
      version: file.version,
      uploadedBy: file.uploadedBy,
      uploadedAt: file.uploadedAt,
      ...(file.storageKey ? { storageKey: file.storageKey } : {}),
      ...(file.mimeType ? { mimeType: file.mimeType } : {}),
      ...(file.author ? { author: file.author } : {}),
    }
  }

  private toCraftFileView(file: CraftFile): CraftFileView {
    return {
      id: file.id,
      drawingNo: file.drawingNo,
      name: file.name,
      op: file.op,
      ver: file.ver,
      by: file.by,
      date: file.date,
      size: file.size,
      ...(file.storageKey ? { storageKey: file.storageKey } : {}),
      ...(file.mimeType ? { mimeType: file.mimeType } : {}),
      previewable: file.previewable,
      ...(file.author ? { author: file.author } : {}),
      scanned: file.scanned,
    }
  }

	  private partKey(part: StructurePart, index: number): string {
	    return part.relationId ?? part.id ?? `${part.no}:${part.drawingId ?? ''}:${index}`
	  }

	  private findRootDrawingNo(
	    part: StructurePart,
	    partsByNo: Map<string, StructurePart[]>,
    drawingsByNo: Map<string, Drawing>,
  ): string | undefined {
    const visited = new Set<string>()
    let parentNo = part.parentNo
    while (parentNo && !visited.has(parentNo)) {
      if (drawingsByNo.has(parentNo)) return parentNo
      visited.add(parentNo)
	      const parent = (partsByNo.get(parentNo) ?? []).find((candidate) => candidate.drawingId === part.drawingId) ?? partsByNo.get(parentNo)?.[0]
      if (!parent) return undefined
      parentNo = parent.parentNo
    }
    return undefined
  }
}
