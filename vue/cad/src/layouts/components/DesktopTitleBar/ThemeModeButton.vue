<script setup lang="ts">
import { useThemeStore } from '@/stores/theme.store'

defineOptions({
  name: 'ThemeModeButton',
})

const themeStore = useThemeStore()

function handleClick() {
  themeStore.toggleMode()
  themeStore.applyTheme()
}
</script>

<template>
  <button
    class="theme-mode-button"
    type="button"
    :aria-label="themeStore.mode === 'dark' ? '切换到白天模式' : '切换到黑夜模式'"
    @click="handleClick"
  >
    <span class="theme-mode-button__icon"></span>
    <span class="theme-mode-button__label">
      {{ themeStore.mode === 'dark' ? '黑夜' : '白天' }}
    </span>
  </button>
</template>

<style scoped>
.theme-mode-button {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 26px;
  padding: 0 8px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-surface);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.theme-mode-button__icon {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background-color: var(--color-accent);
}
</style>
