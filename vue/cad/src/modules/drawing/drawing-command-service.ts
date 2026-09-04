import type { Drawing, StructurePart } from '@/types/domain.types'
import type { CreatePartInput, UpdateDrawingInput, UpdatePartInput } from '@/types/application.types'

export interface DrawingLifecycle {
  id: string
  no: string
  name: string
  status: string
}

export interface DrawingCommandGateway {
  updateDrawing(drawingId: string, input: UpdateDrawingInput): Promise<Drawing>
  updatePart(partId: string, input: UpdatePartInput): Promise<StructurePart>
  createPart(drawingId: string, input: CreatePartInput): Promise<StructurePart>
  archive(drawingNo: string): Promise<DrawingLifecycle>
  unarchive(drawingNo: string): Promise<DrawingLifecycle>
}

/** 图纸与零件原子命令的应用层入口；不依赖 Vue、Pinia 或具体 API 客户端。 */
export class DrawingCommandService {
  public constructor(private readonly gateway: DrawingCommandGateway) {}

  updateDrawing(drawingId: string, input: UpdateDrawingInput): Promise<Drawing> {
    return this.gateway.updateDrawing(drawingId, input)
  }

  updatePart(partId: string, input: UpdatePartInput): Promise<StructurePart> {
    return this.gateway.updatePart(partId, input)
  }

  createPart(drawingId: string, input: CreatePartInput): Promise<StructurePart> {
    return this.gateway.createPart(drawingId, input)
  }

  archive(drawingNo: string): Promise<DrawingLifecycle> {
    return this.gateway.archive(drawingNo)
  }

  unarchive(drawingNo: string): Promise<DrawingLifecycle> {
    return this.gateway.unarchive(drawingNo)
  }
}
