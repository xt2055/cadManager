import type { Branch, BorrowRecord } from '@/types/domain.types'

export interface DrawingRelationQueryGateway {
  loadBranches(): Promise<Branch[]>
  loadBorrows(): Promise<BorrowRecord[]>
}

/** 图纸分支与借用关系的只读查询入口。 */
export class DrawingRelationQueryService {
  public constructor(private readonly gateway: DrawingRelationQueryGateway) {}

  listBranches(): Promise<Branch[]> { return this.gateway.loadBranches() }
  listBorrows(): Promise<BorrowRecord[]> { return this.gateway.loadBorrows() }
}
