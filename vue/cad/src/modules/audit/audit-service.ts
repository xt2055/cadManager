import type { ActivityLog, ActivityResult, ActivityTargetType, ActivityType } from '@/types/domain.types'
import type { AdminOperationLogQuery, OperationLogPage } from '@/services/drawing-operation-log.service'

export interface CreateOperationLogInput {
  drawingNo: string
  drawingName?: string
  targetType: ActivityTargetType
  act: ActivityType
  txt: string
  result?: ActivityResult
  detail?: Record<string, unknown>
}

export interface AuditGateway {
  create(input: CreateOperationLogInput): Promise<ActivityLog>
  listDrawing(options?: { page?: number; pageSize?: number; action?: ActivityType; drawingNo?: string }): Promise<OperationLogPage>
  listAdmin(options?: AdminOperationLogQuery): Promise<OperationLogPage>
}

/** 审计应用服务：页面与 Store 不再直接依赖日志 HTTP 实现。 */
export class AuditService {
  public constructor(private readonly gateway: AuditGateway) {}

  record(input: CreateOperationLogInput): Promise<ActivityLog> { return this.gateway.create(input) }
  listDrawing(options?: Parameters<AuditGateway['listDrawing']>[0]): Promise<OperationLogPage> { return this.gateway.listDrawing(options) }
  listAdmin(options?: AdminOperationLogQuery): Promise<OperationLogPage> { return this.gateway.listAdmin(options) }
}
