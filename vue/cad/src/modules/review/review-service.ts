import type { ApiCompletedAction, ApiReviewCase } from '@/services/review-case.service'
import type { ReviewAccountRole, ReviewFlowDto, ReviewFlowNodeDto } from '@/services/review-flow.service'

export type { ApiCompletedAction, ApiReviewCase, ReviewAccountRole, ReviewFlowDto, ReviewFlowNodeDto }

export interface ReviewCaseGateway {
  start(drawingNo: string): Promise<ApiReviewCase>
  list(): Promise<ApiReviewCase[]>
  submit(caseId: string, nodeName: string, action: 'pass' | 'rejected', opinion: string): Promise<ApiReviewCase>
  completed(): Promise<ApiCompletedAction[]>
}

export interface ReviewFlowGateway {
  list(): Promise<ReviewFlowDto[]>
  get(id: string): Promise<ReviewFlowDto>
  create(input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto>
  update(id: string, input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto>
  toggle(id: string, enabled: boolean): Promise<ReviewFlowDto>
}

/** 审核应用服务：统一审核案例与审核流程的 API 边界。 */
export class ReviewService {
  public constructor(
    private readonly cases: ReviewCaseGateway,
    private readonly flows: ReviewFlowGateway,
  ) {}

  startCase(drawingNo: string): Promise<ApiReviewCase> { return this.cases.start(drawingNo) }
  listCases(): Promise<ApiReviewCase[]> { return this.cases.list() }
  submitNode(caseId: string, nodeName: string, action: 'pass' | 'rejected', opinion: string): Promise<ApiReviewCase> {
    return this.cases.submit(caseId, nodeName, action, opinion)
  }
  listCompleted(): Promise<ApiCompletedAction[]> { return this.cases.completed() }

  listFlows(): Promise<ReviewFlowDto[]> { return this.flows.list() }
  getFlow(id: string): Promise<ReviewFlowDto> { return this.flows.get(id) }
  createFlow(input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto> {
    return this.flows.create(input)
  }
  updateFlow(id: string, input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto> {
    return this.flows.update(id, input)
  }
  toggleFlow(id: string, enabled: boolean): Promise<ReviewFlowDto> { return this.flows.toggle(id, enabled) }
}

const NODE_NAME_TO_SIGNER_ROLE: Record<string, string> = {
  设计自检: '设计',
  校对复核: '校对',
  专业审核: '审核',
  工艺会签: '工艺',
  标准化审查: '标准化',
  主管批准: '批准',
}

export function signerRoleForNode(name: string, signerRole?: string): string {
  const trimmedRole = signerRole?.trim()
  return trimmedRole || NODE_NAME_TO_SIGNER_ROLE[name?.trim() ?? ''] || ''
}
