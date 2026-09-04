import type { DrawingAttribute } from '@/types/domain.types'

export interface AttributeQueryGateway {
  loadAttributes(): Promise<DrawingAttribute[]>
}

/** 属性查询应用服务；与纯属性规则 AttributeService 分离。 */
export class AttributeQueryService {
  public constructor(private readonly gateway: AttributeQueryGateway) {}

  list(): Promise<DrawingAttribute[]> {
    return this.gateway.loadAttributes()
  }
}
