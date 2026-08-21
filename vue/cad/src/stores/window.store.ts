import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { WindowState } from '@/types/window.types'

export const useWindowStore = defineStore('window', () => {
  const state = ref<WindowState>({
    maximized: false,
    fullscreen: false,
    focused: true,
  })

  function setMaximized(value: boolean) {
    state.value.maximized = value
  }

  function setFullscreen(value: boolean) {
    state.value.fullscreen = value
  }

  function setFocused(value: boolean) {
    state.value.focused = value
  }

  return {
    state,
    setMaximized,
    setFullscreen,
    setFocused,
  }
})
