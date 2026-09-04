import { defineStore } from 'pinia'
import { ref } from 'vue'
import { adminService } from '@/app/container'
import type { UserAccount } from '@/types/domain.types'
import type { AdminDrawingPage, AdminDrawingSummary, AdminPartSummary, SystemLogFile, SystemLogLine, UpdateRecord } from '@/modules/admin'

type AdminListItem = AdminDrawingSummary | AdminPartSummary

/** 管理后台查询缓存；页面状态与管理员应用服务解耦。 */
export const useAdminStore = defineStore('admin', () => {
  const users = ref<UserAccount[]>([])
  const systemLogs = ref<SystemLogLine[]>([])
  const systemLogFiles = ref<SystemLogFile[]>([])
  const updates = ref<UpdateRecord[]>([])
  const drawings = ref<AdminListItem[]>([])
  const drawingsTotal = ref(0)
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadUsers(): Promise<void> { users.value = await adminService.listUsers() }
  async function loadSystemLogs(lines?: number, keyword?: string): Promise<void> {
    systemLogs.value = await adminService.fetchSystemLogs(lines, keyword)
  }
  async function loadSystemLogFiles(): Promise<void> { systemLogFiles.value = await adminService.fetchSystemLogFiles() }
  async function loadUpdates(): Promise<void> { updates.value = await adminService.fetchUpdateList() }

  async function loadDrawings(options: Parameters<typeof adminService.listDrawings>[0] = {}): Promise<AdminDrawingPage> {
    loading.value = true
    error.value = null
    try {
      const page = await adminService.listDrawings(options)
      drawings.value = page.list
      drawingsTotal.value = page.total
      return page
    } catch (loadError: unknown) {
      error.value = loadError instanceof Error ? loadError.message : String(loadError)
      throw loadError
    } finally {
      loading.value = false
    }
  }

  function invalidate() {
    users.value = []
    systemLogs.value = []
    systemLogFiles.value = []
    updates.value = []
    drawings.value = []
    drawingsTotal.value = 0
  }

  return {
    users,
    systemLogs,
    systemLogFiles,
    updates,
    drawings,
    drawingsTotal,
    loading,
    error,
    loadUsers,
    loadSystemLogs,
    loadSystemLogFiles,
    loadUpdates,
    loadDrawings,
    invalidate,
  }
})
