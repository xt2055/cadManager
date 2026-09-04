import type { BomItem } from '@/types/domain.types'
import type { DrawingBomSnapshot, ReplaceDrawingBomInput } from '@/types/application.types'

export interface BomCommandGateway {
  load(drawingId: string): Promise<DrawingBomSnapshot>
  replace(drawingId: string, input: ReplaceDrawingBomInput): Promise<DrawingBomSnapshot>
}

/** 图纸 BOM 的原子命令入口；调用方不再依赖旧整表写入接口。 */
export class BomCommandService {
  public constructor(private readonly gateway: BomCommandGateway) {}

  async replace(drawingId: string, items: BomItem[]): Promise<DrawingBomSnapshot> {
    const current = await this.gateway.load(drawingId)
    return this.gateway.replace(drawingId, {
      expectedRevision: current.revision,
      items,
    })
  }
}
