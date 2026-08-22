import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ThemeMode, ThemeSkin } from '@/types/theme.types'

export function getTimeBasedMode(): ThemeMode {
  const hour = new Date().getHours()
  return hour >= 6 && hour < 18 ? 'light' : 'dark'
}

export const useThemeStore = defineStore('theme', () => {
  const skin = ref<ThemeSkin>('classic')
  const mode = ref<ThemeMode>(getTimeBasedMode())

  function setSkin(value: ThemeSkin) {
    skin.value = value
    applyTheme()
  }

  function setMode(value: ThemeMode) {
    mode.value = value
    applyTheme()
  }

  function toggleMode() {
    mode.value = mode.value === 'dark' ? 'light' : 'dark'
    applyTheme()
  }

  function applyTheme() {
    document.documentElement.dataset.skin = skin.value
    document.documentElement.dataset.theme = mode.value
  }

  return {
    skin,
    mode,
    setSkin,
    setMode,
    toggleMode,
    applyTheme,
  }
})
