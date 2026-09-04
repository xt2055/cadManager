import { defineStore } from 'pinia'
import { ref } from 'vue'
import { auditService } from '@/app/container'
import type { OperationLogPage } from '@/services/drawing-operation-log.service'

/** 操作日志查询缓存；筛选和分页由 AuditService 对应的查询接口完成。 */
export const useAuditStore = defineStore('audit', () => {
  const drawingLogs = ref<OperationLogPage>({ list: [], total: 0, page: 1, pageSize: 20 })
  const adminLogs = ref<OperationLogPage>({ list: [], total: 0, page: 1, pageSize: 20 })
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function loadDrawing(options: Parameters<typeof auditService.listDrawing>[0] = {}): Promise<OperationLogPage> {
    loading.value = true
    error.value = null
    try {
      drawingLogs.value = await auditService.listDrawing(options)
      return drawingLogs.value
    } catch (loadError: unknown) {
      error.value = loadError instanceof Error ? loadError.message : String(loadError)
      throw loadError
    } finally {
      loading.value = false
    }
  }

  async function loadAdmin(options: Parameters<typeof auditService.listAdmin>[0] = {}): Promise<OperationLogPage> {
    loading.value = true
    error.value = null
    try {
      adminLogs.value = await auditService.listAdmin(options)
      return adminLogs.value
    } catch (loadError: unknown) {
      error.value = loadError instanceof Error ? loadError.message : String(loadError)
      throw loadError
    } finally {
      loading.value = false
    }
  }

  function invalidate() {
    drawingLogs.value = { list: [], total: 0, page: 1, pageSize: 20 }
    adminLogs.value = { list: [], total: 0, page: 1, pageSize: 20 }
    error.value = null
  }

  return { drawingLogs, adminLogs, loading, error, loadDrawing, loadAdmin, invalidate }
})
