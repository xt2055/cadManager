import { defineStore } from 'pinia'
import { ref } from 'vue'
import { reviewService } from '@/app/container'
import { isAssignedReviewer, pickReviewCase } from '@/features/reviews/review-workspace'
import type { ApiCompletedAction, ApiReviewCase, ReviewFlowDto } from '@/modules/review'
import { useAuthStore } from '@/stores/auth.store'

export interface PendingReviewView {
  reviewCaseId: string
  no: string
  name: string
  node: string
  /** 负责人：后端暂未提供该字段，先占位为空串，待接口补齐后改为取其返回值。 */
  responsible: string
  time: string
}

/** 审核 Read Model 和轮询状态；审核事务仍由 ReviewService 执行。 */
export const useReviewStore = defineStore('review', () => {
  const authStore = useAuthStore()
  const cases = ref<ApiReviewCase[]>([])
  const completed = ref<ApiCompletedAction[]>([])
  const flows = ref<ReviewFlowDto[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  let pollTimer: number | null = null

  const myPendingReviews = () => {
    const user = authStore.currentUser
    if (!user) return [] as PendingReviewView[]
    return cases.value.flatMap((reviewCase) => {
      if (reviewCase.status !== 'reviewing') return []
      const activeNode = [...reviewCase.nodes]
        .filter((node) => node.status === 'pending')
        .sort((a, b) => a.order - b.order)[0]
      if (!activeNode || !isAssignedReviewer(activeNode, user)) return []
      return [{
        reviewCaseId: reviewCase.id,
        no: reviewCase.drawingNo,
        name: reviewCase.drawingName || reviewCase.drawingNo,
        node: activeNode.name,
        // 负责人字段待后端提供；占位期间列表显示“—”，不误用其他字段顶替。
        responsible: '',
        time: reviewCase.startedAt,
      }]
    })
  }

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const [nextCases, nextCompleted] = await Promise.all([reviewService.listCases(), reviewService.listCompleted()])
      cases.value = nextCases
      completed.value = nextCompleted
    } catch (loadError: unknown) {
      error.value = loadError instanceof Error ? loadError.message : String(loadError)
      throw loadError
    } finally {
      loading.value = false
    }
  }

/**
 * 读取审核流程模板：整数组替换是既有约定，页面必须通过 computed 跟随。
 * 这里补齐 loading（页面据此区分「加载中」与「确实没有」），并对异常响应兜底成空列表，
 * 避免后端返回空体时把 flows 写成 undefined 让列表页崩掉。
 */
async function loadFlows(): Promise<void> {
  loading.value = true
  error.value = null
  try {
    flows.value = (await reviewService.listFlows()) ?? []
  } catch (loadError: unknown) {
    error.value = loadError instanceof Error ? loadError.message : String(loadError)
    throw loadError
  } finally {
    loading.value = false
  }
}

  // 轮次选择统一走 pickReviewCase：进行中的轮次优先，绝不按分钟级时间戳猜轮次，
  // 否则驳回后重新提交会挑到上一轮，下一个审核员就会看到上一轮的批注。
  function getCase(drawingNo: string, caseId?: string): ApiReviewCase | null {
    return pickReviewCase(cases.value.filter((item) => item.drawingNo === drawingNo), caseId)
  }

  function getFlow(id: string): ReviewFlowDto | null {
    return flows.value.find((flow) => flow.id === id) ?? null
  }

  async function startCase(drawingNo: string): Promise<ApiReviewCase> {
    const reviewCase = await reviewService.startCase(drawingNo)
    cases.value = [...cases.value.filter((item) => item.id !== reviewCase.id), reviewCase]
    return reviewCase
  }

  async function submitNode(caseId: string, nodeName: string, action: 'pass' | 'rejected', opinion: string): Promise<ApiReviewCase> {
    const reviewCase = await reviewService.submitNode(caseId, nodeName, action, opinion)
    cases.value = cases.value.map((item) => item.id === reviewCase.id ? reviewCase : item)
    return reviewCase
  }

  async function createFlow(input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto> {
    const flow = await reviewService.createFlow(input)
    flows.value = [...flows.value, flow]
    return flow
  }

  async function updateFlow(id: string, input: Omit<ReviewFlowDto, 'id' | 'createdBy' | 'createdAt' | 'updatedAt'>): Promise<ReviewFlowDto> {
    const flow = await reviewService.updateFlow(id, input)
    flows.value = flows.value.map((item) => item.id === id ? flow : item)
    return flow
  }

  async function toggleFlow(id: string, enabled: boolean): Promise<ReviewFlowDto> {
    const flow = await reviewService.toggleFlow(id, enabled)
    flows.value = flows.value.map((item) => item.id === id ? flow : item)
    return flow
  }

  function startPolling(intervalMs = 30_000) {
    if (pollTimer !== null || typeof window === 'undefined') return
    pollTimer = window.setInterval(() => { void load().catch(() => undefined) }, intervalMs)
  }

  function stopPolling() {
    if (pollTimer === null || typeof window === 'undefined') return
    window.clearInterval(pollTimer)
    pollTimer = null
  }

  return {
    cases,
    completed,
    flows,
    myPendingReviews,
    loading,
    error,
    load,
    loadFlows,
    getCase,
    getFlow,
    startCase,
    submitNode,
    createFlow,
    updateFlow,
    toggleFlow,
    startPolling,
    stopPolling,
  }
})
