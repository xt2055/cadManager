import { defineStore } from 'pinia'
import { ref } from 'vue'
import { reviewService } from '@/app/container'
import { isAssignedReviewer } from '@/features/reviews/review-workspace'
import type { ApiCompletedAction, ApiReviewCase, ReviewFlowDto } from '@/modules/review'
import { useAuthStore } from '@/stores/auth.store'

export interface PendingReviewView {
  reviewCaseId: string
  no: string
  name: string
  node: string
  by: string
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
        by: activeNode.assignedName,
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

  async function loadFlows(): Promise<void> {
    flows.value = await reviewService.listFlows()
  }

  function getCase(drawingNo: string, caseId?: string): ApiReviewCase | null {
    const candidates = cases.value.filter((item) => item.drawingNo === drawingNo)
    if (caseId) return candidates.find((item) => item.id === caseId) ?? null
    return candidates.sort((a, b) => (b.startedAt || '').localeCompare(a.startedAt || ''))[0] ?? null
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
