import type { BomItem, Drawing, StructurePart } from '@/types/domain.types'
import type { StoredAttachment } from '@/services/data-manager/data-provider'

export interface DrawingQueryGateway {
  loadDrawings(): Promise<Drawing[]>
  loadStructure(): Promise<StructurePart[]>
  loadBom(): Promise<BomItem[]>
  loadAttachments(): Promise<StoredAttachment[]>
}

export interface DrawingQuerySnapshot {
  drawings: Drawing[]
  structure: StructurePart[]
  bom: BomItem[]
  attachments: StoredAttachment[]
}

/** 图纸查询应用服务：集中读取图纸、结构和 BOM，避免页面拼装旧数据源。 */
export class DrawingQueryService {
  public constructor(private readonly gateway: DrawingQueryGateway) {}

  listDrawings(): Promise<Drawing[]> {
    return this.gateway.loadDrawings()
  }

  async getDrawing(idOrNo: string): Promise<Drawing | null> {
    const drawings = await this.listDrawings()
    return drawings.find((drawing) => drawing.id === idOrNo || drawing.no === idOrNo) ?? null
  }

  async getStructure(drawingNo?: string): Promise<StructurePart[]> {
    const structure = await this.gateway.loadStructure()
    if (!drawingNo) return structure

    const result: StructurePart[] = []
    const pending = [drawingNo]
    while (pending.length) {
      const parentNo = pending.shift()
      if (!parentNo) continue
      for (const part of structure) {
        if (part.parentNo !== parentNo || result.some((item) => item.no === part.no)) continue
        result.push(part)
        pending.push(part.no)
      }
    }
    return result
  }

  async getPart(partNoOrId: string): Promise<StructurePart | null> {
    const structure = await this.gateway.loadStructure()
    return structure.find((part) => part.id === partNoOrId || part.no === partNoOrId) ?? null
  }

  async getBom(drawingNo?: string): Promise<BomItem[]> {
    const bom = await this.gateway.loadBom()
    return drawingNo ? bom.filter((item) => item.drawingNo === drawingNo) : bom
  }

  async loadSnapshot(): Promise<DrawingQuerySnapshot> {
    const [drawings, structure, bom, attachments] = await Promise.all([
      this.gateway.loadDrawings(),
      this.gateway.loadStructure(),
      this.gateway.loadBom(),
      this.gateway.loadAttachments(),
    ])
    return { drawings, structure, bom, attachments }
  }
}
