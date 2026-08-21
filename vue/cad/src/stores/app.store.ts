import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAppStore = defineStore('app', () => {
  const ready = ref(false)
  const sidebarCollapsed = ref(false)
  const currentPage = ref('dashboard')

  function setReady(value: boolean) {
    ready.value = value
  }

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }

  function setSidebarCollapsed(value: boolean) {
    sidebarCollapsed.value = value
  }

  function setCurrentPage(value: string) {
    currentPage.value = value
  }

  return {
    ready,
    sidebarCollapsed,
    currentPage,
    setReady,
    toggleSidebar,
    setSidebarCollapsed,
    setCurrentPage,
  }
})
