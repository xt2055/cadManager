import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { useAuthStore } from '@/stores/auth.store'

export interface SystemStatusData {
  service: {
    status: string
    version: string
    platform: string
    checkedAt: string
  }
  database: {
    status: string
    totalConns: number
    acquiredConns: number
    idleConns: number
    maxConns: number
    minConns: number
  }
  storage: {
    status: string
    fileCount: number
    usedBytes: number
    formattedUsed: string
    formattedDisk: string
    backupStatus: string
  }
  onlineUsers: {
    count: number
  }
}

export const useSystemStatusStore = defineStore('systemStatus', () => {
  const authStore = useAuthStore()
  const status = ref<SystemStatusData | null>(null)
  const isOnline = ref(false)
  const loading = ref(false)
  const lastChecked = ref<Date | null>(null)

  const isHealthy = computed(() => status.value?.service?.status === 'ok' && status.value?.database?.status === 'ok')
  const onlineCount = computed(() => status.value?.onlineUsers?.count ?? (isOnline.value ? 1 : 0))
  const storageSummary = computed(() => ({
    fileCount: status.value?.storage?.fileCount ?? 0,
    formattedUsed: status.value?.storage?.formattedUsed || '0 B',
    backupStatus: status.value?.storage?.backupStatus || '未配置',
  }))

  async function fetchStatus(): Promise<void> {
    loading.value = true
    const baseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
    let fetched = false
    try {
      const token = authStore.token || (typeof window !== 'undefined'
        ? window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token')
        : null)
      const headers: Record<string, string> = { Accept: 'application/json' }
      if (token) headers.Authorization = `Bearer ${token}`

      const response = await fetch(`${baseUrl}/system/status`, {
        method: 'GET',
        headers,
        credentials: 'include',
      })
      if (response.ok) {
        const body = await response.json()
        const data = body && typeof body === 'object' && 'data' in body ? body.data : body
        status.value = data as SystemStatusData
        isOnline.value = true
        fetched = true
        lastChecked.value = new Date()
        return
      }
    } catch {
      // 忽略
    } finally {
      if (!fetched) {
        try {
          const ping = await fetch(`${baseUrl}/health`, { method: 'GET' })
          isOnline.value = ping.ok
          lastChecked.value = new Date()
          if (!ping.ok) status.value = null
        } catch {
          isOnline.value = false
          status.value = null
        }
      }
      loading.value = false
    }
  }

  async function heartbeat(): Promise<void> {
    const baseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
    const token = authStore.token || (typeof window !== 'undefined'
      ? window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token')
      : null)
    if (!token) return
    try {
      await fetch(`${baseUrl}/auth/heartbeat`, {
        method: 'POST',
        headers: { Accept: 'application/json', Authorization: `Bearer ${token}` },
        credentials: 'include',
      })
    } catch {
      // 状态轮询会负责显示服务是否可用。
    }
  }

  let timer: ReturnType<typeof setInterval> | null = null

  function startPolling(intervalMs = 30000) {
    if (timer) clearInterval(timer)
    fetchStatus()
    heartbeat()
    timer = setInterval(() => {
      fetchStatus()
      heartbeat()
    }, intervalMs)
  }

  function stopPolling() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  return {
    status,
    isOnline,
    isHealthy,
    onlineCount,
    storageSummary,
    lastChecked,
    fetchStatus,
    startPolling,
    stopPolling,
  }
})
