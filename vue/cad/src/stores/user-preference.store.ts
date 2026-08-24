import { defineStore } from 'pinia'
import { ref } from 'vue'

export type LoginAnimation = 'laser' | 'burst'

const LOGIN_ANIMATION_KEY = 'cad:login-animation:v1'

function readLoginAnimation(): LoginAnimation {
  if (typeof window === 'undefined') return 'laser'
  const value = window.localStorage.getItem(LOGIN_ANIMATION_KEY)
  return value === 'burst' ? 'burst' : 'laser'
}

export const useUserPreferenceStore = defineStore('user-preference', () => {
  const showViewerZoomStatus = ref(true)
  const loginAnimation = ref<LoginAnimation>(readLoginAnimation())

  function setShowViewerZoomStatus(value: boolean) {
    showViewerZoomStatus.value = value
  }

  function setLoginAnimation(value: LoginAnimation) {
    loginAnimation.value = value
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(LOGIN_ANIMATION_KEY, value)
    }
  }

  return {
    showViewerZoomStatus,
    setShowViewerZoomStatus,
    loginAnimation,
    setLoginAnimation,
  }
})
