import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useUserPreferenceStore = defineStore('user-preference', () => {
  const showViewerZoomStatus = ref(true)

  function setShowViewerZoomStatus(value: boolean) {
    showViewerZoomStatus.value = value
  }

  return {
    showViewerZoomStatus,
    setShowViewerZoomStatus,
  }
})
