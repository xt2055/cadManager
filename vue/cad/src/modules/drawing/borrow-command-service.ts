import type { BorrowPartInput, DrawingBorrowResult } from '@/services/data-manager/data-provider'

export interface BorrowCommandGateway {
  borrow(drawingId: string, input: BorrowPartInput): Promise<DrawingBorrowResult>
}

/** 借用命令入口：借用关系由后端事务创建，不再克隆零件快照。 */
export class BorrowCommandService {
  public constructor(private readonly gateway: BorrowCommandGateway) {}

  borrow(drawingId: string, sourcePartId: string, borrowReason?: string): Promise<DrawingBorrowResult> {
    return this.gateway.borrow(drawingId, {
      sourcePartId,
      qty: 1,
      borrowReason,
    })
  }
}
