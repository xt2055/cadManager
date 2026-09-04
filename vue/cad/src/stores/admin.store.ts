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
  async function createUser(input: Parameters<typeof adminService.createUser>[0]): Promise<UserAccount> {
    const user = await adminService.createUser(input)
    users.value = [user, ...users.value]
    return user
  }
  async function updateUser(userId: string, input: Parameters<typeof adminService.updateUser>[1]): Promise<UserAccount> {
    const user = await adminService.updateUser(userId, input)
    users.value = users.value.map((item) => item.id === userId ? user : item)
    return user
  }
  async function toggleUser(userId: string): Promise<UserAccount> {
    const user = users.value.find((item) => item.id === userId)
    if (!user) throw new Error('未找到目标账号')
    if (user.status === 'active' && user.roles.includes('admin') && users.value.filter((item) => item.status === 'active' && item.roles.includes('admin')).length <= 1) {
      throw new Error('不能禁用最后一个管理员账号')
    }
    return updateUser(userId, {
      account: user.account,
      displayName: user.displayName,
      roles: user.roles,
      status: user.status === 'active' ? 'disabled' : 'active',
    })
  }
  async function loadSystemLogs(lines?: number, keyword?: string): Promise<void> {
    systemLogs.value = await adminService.fetchSystemLogs(lines, keyword)
  }
  async function loadSystemLogFiles(): Promise<void> { systemLogFiles.value = await adminService.fetchSystemLogFiles() }
  async function loadUpdates(): Promise<void> { updates.value = await adminService.fetchUpdateList() }
  function systemLogDownloadUrl(file: string): string { return adminService.systemLogDownloadUrl(file) }
  function updatePackageDownloadUrl(record: UpdateRecord): string { return adminService.updatePackageDownloadUrl(record) }
  async function uploadUpdatePackage(input: Parameters<typeof adminService.uploadUpdatePackage>[0]): Promise<UpdateRecord> {
    const record = await adminService.uploadUpdatePackage(input)
    updates.value = [record, ...updates.value]
    return record
  }
  async function deleteUpdatePackage(id: string): Promise<void> {
    await adminService.deleteUpdatePackage(id)
    updates.value = updates.value.filter((item) => item.id !== id)
  }

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
    createUser,
    updateUser,
    toggleUser,
    loadSystemLogs,
    loadSystemLogFiles,
    loadUpdates,
    systemLogDownloadUrl,
    updatePackageDownloadUrl,
    uploadUpdatePackage,
    deleteUpdatePackage,
    loadDrawings,
    invalidate,
  }
})
