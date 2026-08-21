import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import type {
  ActivityLog,
  AdminLog,
  AdminUser,
  BomItem,
  Branch,
  BorrowRecord,
  CraftFile,
  Drawing,
  DrawingVersion,
  HiddenObject,
  MyReview,
  ReviewFlow,
  ReviewNode,
  StructurePart,
} from '@/types/demo.types'

export const STATUS = {
  published: { t: '已发布', c: 'ok' },
  reviewing: { t: '审核中', c: 'info' },
  draft: { t: '草稿', c: 'mute' },
  hidden: { t: '已隐藏', c: 'danger' },
  disabled: { t: '已禁用', c: 'danger' },
} as const

export const useDemoStore = defineStore('demo', () => {
  const drawings = ref<Drawing[]>([])

  const structure = ref<StructurePart[]>([])

  const versions = ref<DrawingVersion[]>([])

  const branches = ref<Branch[]>([])

  const borrows = ref<BorrowRecord[]>([])

  const bom = ref<BomItem[]>([])

  const crafts = ref<CraftFile[]>([])

  const logs = ref<ActivityLog[]>([])

  const reviewNodes = ref<ReviewNode[]>([])

  const myReviews = ref<MyReview[]>([])

  const users = ref<AdminUser[]>([])

  const flows = ref<ReviewFlow[]>([])

  const hiddenList = ref<HiddenObject[]>([])

  const adminLogs = ref<AdminLog[]>([])

  const currentDrawing = ref<Drawing | null>(null)
  const selectedStructureIndex = ref(0)
  const treeOpen = ref(true)
  const reviewCount = computed(() => myReviews.value.length)

  function openDrawing(no: string) {
    currentDrawing.value = drawings.value.find((drawing) => drawing.no === no) ?? currentDrawing.value
    selectedStructureIndex.value = 0
  }

  function approveReview(index: number) {
    myReviews.value.splice(index, 1)
  }

  function setReviewNodeStatus(name: string, status: 'pass' | 'pending', opinion = '') {
    const node = reviewNodes.value.find((item) => item.name === name)
    if (node) {
      node.status = status
      node.time = status === 'pass' ? '刚刚' : '—'
      node.opinion = opinion
    }
  }

  function toggleUser(index: number) {
    const user = users.value[index]
    if (user) {
      user.status = user.status === '正常' ? '已禁用' : '正常'
    }
  }

  function toggleFlow(index: number) {
    const flow = flows.value[index]
    if (flow) {
      flow.on = !flow.on
    }
  }

  function restoreHidden(index: number) {
    hiddenList.value.splice(index, 1)
  }

  return {
    drawings,
    structure,
    versions,
    branches,
    borrows,
    bom,
    crafts,
    logs,
    reviewNodes,
    myReviews,
    users,
    flows,
    hiddenList,
    adminLogs,
    currentDrawing,
    selectedStructureIndex,
    treeOpen,
    reviewCount,
    openDrawing,
    approveReview,
    setReviewNodeStatus,
    toggleUser,
    toggleFlow,
    restoreHidden,
  }
})
