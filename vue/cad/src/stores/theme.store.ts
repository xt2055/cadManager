import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ThemeMode, ThemeSkin } from '@/types/theme.types'

export function getTimeBasedMode(): ThemeMode {
  const hour = new Date().getHours()
  return hour >= 6 && hour < 18 ? 'light' : 'dark'
}

export const useThemeStore = defineStore('theme', () => {
  const skin = ref<ThemeSkin>(localStorage.getItem('cad:juli-theme') === 'true' ? 'juli' : 'classic')
  const mode = ref<ThemeMode>(getTimeBasedMode())
  const hydraulicPhase = ref<'idle' | 'withdraw' | 'charge' | 'impact' | 'reveal'>('idle')
  const hydraulicStartedAt = ref(0)
  let transitionTimers: number[] = []

  function stopHydraulicTransition() {
    transitionTimers.forEach(window.clearTimeout)
    transitionTimers = []
    hydraulicPhase.value = 'idle'
    hydraulicStartedAt.value = 0
  }

  function setSkin(value: ThemeSkin) {
    stopHydraulicTransition()
    skin.value = value
    localStorage.setItem('cad:juli-theme', String(value === 'juli'))
    applyTheme()
  }

  function setMode(value: ThemeMode) {
    if (value === mode.value || hydraulicPhase.value !== 'idle') return
    if (skin.value !== 'juli' || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      mode.value = value
      applyTheme()
      return
    }
    hydraulicStartedAt.value = performance.now()
    hydraulicPhase.value = 'withdraw'
    const schedule = (delay: number, action: () => void) => transitionTimers.push(window.setTimeout(action, delay))
    schedule(420, () => { hydraulicPhase.value = 'charge' })
    schedule(1500, () => {
      hydraulicPhase.value = 'impact'
      mode.value = value
      applyTheme()
    })
    schedule(1690, () => { hydraulicPhase.value = 'reveal' })
    schedule(2900, stopHydraulicTransition)
  }

  function toggleMode() {
    setMode(mode.value === 'dark' ? 'light' : 'dark')
  }

  function applyTheme() {
    document.documentElement.dataset.skin = skin.value
    document.documentElement.dataset.theme = mode.value
  }

  return {
    skin,
    mode,
    hydraulicPhase,
    hydraulicStartedAt,
    setSkin,
    setMode,
    toggleMode,
    applyTheme,
  }
})
