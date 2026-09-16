import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import {
  drawingTaskService,
  type DrawingTaskCandidate,
  type DrawingTaskHistoryEntry,
  type DrawingTaskPage,
  type DrawingTaskRow,
  type DrawingTaskSummary,
} from '@/services/drawing-task.service'
import { emptySummary, type TaskBoardFilter } from '@/features/tasks/task.helpers'

const PAGE_SIZE = 20

/**
 * 任务数据源。
 * myPage 是首页任务面板的数据（scope=mine），boardPage 是任务管理台的图纸总表（scope=board）。
 * 两者分开保存：计划员自己也可能有任务，如果把两批数据混在一个 list 里，
 * 首页面板和管理台会互相覆盖对方的加载结果。
 */
export const useTaskStore = defineStore('task', () => {
  const myPage = ref<DrawingTaskPage | null>(null)
  const boardPage = ref<DrawingTaskPage | null>(null)
  const candidates = ref<DrawingTaskCandidate[]>([])
  const loadingMine = ref(false)
  const loadingBoard = ref(false)
  const loadingCandidates = ref(false)
  const errorMine = ref('')
  const errorBoard = ref('')

  const myTasks = computed<DrawingTaskRow[]>(() => myPage.value?.list ?? [])
  const mySummary = computed<DrawingTaskSummary>(() => myPage.value?.summary ?? emptySummary())
  const boardRows = computed<DrawingTaskRow[]>(() => boardPage.value?.list ?? [])
  const boardSummary = computed<DrawingTaskSummary>(() => boardPage.value?.summary ?? emptySummary())

  async function loadMine(pageSize = 50): Promise<void> {
    loadingMine.value = true
    errorMine.value = ''
    try {
      myPage.value = await drawingTaskService.list({ scope: 'mine', page: 1, pageSize })
    } catch (error) {
      errorMine.value = error instanceof Error ? error.message : '我的任务加载失败'
      throw error
    } finally {
      loadingMine.value = false
    }
  }

  async function loadBoard(filter: TaskBoardFilter, page = 1): Promise<void> {
    loadingBoard.value = true
    errorBoard.value = ''
    try {
      boardPage.value = await drawingTaskService.list({
        scope: 'board',
        page,
        pageSize: PAGE_SIZE,
        keyword: filter.keyword.trim(),
        status: filter.status,
        assigned: filter.assigned || undefined,
      })
    } catch (error) {
      errorBoard.value = error instanceof Error ? error.message : '任务总表加载失败'
      throw error
    } finally {
      loadingBoard.value = false
    }
  }

  async function loadCandidates(force = false): Promise<DrawingTaskCandidate[]> {
    if (!force && candidates.value.length) return candidates.value
    loadingCandidates.value = true
    try {
      candidates.value = await drawingTaskService.candidates()
      return candidates.value
    } finally {
      loadingCandidates.value = false
    }
  }

  async function assign(input: { drawingId: string; assigneeId: string; note?: string; dueDate?: string }): Promise<DrawingTaskRow> {
    return drawingTaskService.assign(input)
  }

  async function reassign(taskId: string, input: { assigneeId?: string; note?: string; dueDate?: string; reason?: string }): Promise<DrawingTaskRow> {
    return drawingTaskService.update(taskId, input)
  }

  async function cancel(taskId: string, reason: string): Promise<void> {
    return drawingTaskService.cancel(taskId, reason)
  }

  async function loadHistory(drawingId: string): Promise<DrawingTaskHistoryEntry[]> {
    return drawingTaskService.history(drawingId)
  }

  /** 写操作后统一刷新两侧数据：改派同时影响管理台与当事人的首页任务面板。 */
  async function refreshAfterWrite(filter: TaskBoardFilter, page: number): Promise<void> {
    await Promise.allSettled([loadBoard(filter, page), loadMine()])
  }

  function reset(): void {
    myPage.value = null
    boardPage.value = null
    errorMine.value = ''
    errorBoard.value = ''
  }

  return {
    myPage,
    boardPage,
    candidates,
    loadingMine,
    loadingBoard,
    loadingCandidates,
    errorMine,
    errorBoard,
    myTasks,
    mySummary,
    boardRows,
    boardSummary,
    taskBoardPageSize: PAGE_SIZE,
    loadMine,
    loadBoard,
    loadCandidates,
    loadHistory,
    assign,
    reassign,
    cancel,
    refreshAfterWrite,
    reset,
  }
})
