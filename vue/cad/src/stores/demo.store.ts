import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { dataManager } from '@/services/data-manager'
import type { DataDocument } from '@/services/data-manager'
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
  const initialized = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const reviewCount = computed(() => myReviews.value.length)

  let initializationPromise: Promise<void> | null = null
  let saveQueue: Promise<void> = Promise.resolve()

  function toDocument(): DataDocument {
    return {
      version: 1,
      drawings: drawings.value,
      structure: structure.value,
      versions: versions.value,
      branches: branches.value,
      borrows: borrows.value,
      bom: bom.value,
      crafts: crafts.value,
      logs: logs.value,
      reviewNodes: reviewNodes.value,
      myReviews: myReviews.value,
      users: users.value,
      flows: flows.value,
      hiddenList: hiddenList.value,
      adminLogs: adminLogs.value,
    }
  }

  function persist(): Promise<void> {
    const saveOperation = saveQueue
      .catch(() => undefined)
      .then(() => dataManager.save(toDocument()))
    saveQueue = saveOperation.catch(() => undefined)

    return saveOperation.catch((saveError: unknown) => {
      error.value = saveError instanceof Error ? saveError.message : String(saveError)
      console.error('保存业务数据失败', saveError)
      throw saveError
    })
  }

  function initialize(): Promise<void> {
    if (initialized.value) return Promise.resolve()
    if (initializationPromise) return initializationPromise

    loading.value = true
    error.value = null
    initializationPromise = (async () => {
      try {
        const document = await dataManager.load()
        drawings.value = document.drawings
        structure.value = document.structure
        versions.value = document.versions
        branches.value = document.branches
        borrows.value = document.borrows
        bom.value = document.bom
        crafts.value = document.crafts
        logs.value = document.logs
        reviewNodes.value = document.reviewNodes
        myReviews.value = document.myReviews
        users.value = document.users
        flows.value = document.flows
        hiddenList.value = document.hiddenList
        adminLogs.value = document.adminLogs
        initialized.value = true
      } catch (loadError: unknown) {
        error.value = loadError instanceof Error ? loadError.message : String(loadError)
        console.error('加载业务数据失败', loadError)
        throw loadError
      } finally {
        loading.value = false
        initializationPromise = null
      }
    })()

    return initializationPromise
  }

  function openDrawing(no: string) {
    currentDrawing.value = drawings.value.find((drawing) => drawing.no === no) ?? null
    selectedStructureIndex.value = 0
  }

  function clearCurrentDrawing() {
    currentDrawing.value = null
    selectedStructureIndex.value = 0
  }

  async function approveReview(index: number): Promise<void> {
    await initialize()
    myReviews.value.splice(index, 1)
    await persist()
  }

  async function setReviewNodeStatus(name: string, status: 'pass' | 'pending', opinion = ''): Promise<void> {
    await initialize()
    const node = reviewNodes.value.find((item) => item.name === name)
    if (node) {
      node.status = status
      node.time = status === 'pass' ? '刚刚' : '—'
      node.opinion = opinion
      await persist()
    }
  }

  async function toggleUser(index: number): Promise<void> {
    await initialize()
    const user = users.value[index]
    if (user) {
      user.status = user.status === '正常' ? '已禁用' : '正常'
      await persist()
    }
  }

  async function toggleFlow(index: number): Promise<void> {
    await initialize()
    const flow = flows.value[index]
    if (flow) {
      flow.on = !flow.on
      await persist()
    }
  }

  async function restoreHidden(index: number): Promise<void> {
    await initialize()
    hiddenList.value.splice(index, 1)
    await persist()
  }

  async function addDrawing(drawing: Drawing, structureParts?: StructurePart[]): Promise<void> {
    await initialize()
    drawings.value.unshift(drawing)
    if (structureParts && structureParts.length) {
      structure.value.push(...structureParts)
    }
    await persist()
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
    initialized,
    loading,
    error,
    reviewCount,
    initialize,
    openDrawing,
    clearCurrentDrawing,
    addDrawing,
    approveReview,
    setReviewNodeStatus,
    toggleUser,
    toggleFlow,
    restoreHidden,
  }
})
