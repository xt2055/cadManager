<script setup lang="ts">
import { useThemeStore } from '@/stores/theme.store'
import type { ThemeSkin } from '@/types/theme.types'

defineOptions({
  name: 'ThemeSkinSwitch',
})

const themeStore = useThemeStore()

const skins: Array<{ key: ThemeSkin; label: string }> = [
  { key: 'juli', label: '巨力液压' },
  { key: 'classic', label: '经典' },
  { key: 'tech', label: '科技' },
  { key: 'elegant', label: '护眼' },
  { key: 'sage', label: '柔光' },
  { key: 'starry', label: '星空' },
  { key: 'bamboo', label: '墨竹' },
]

function handleClick(skin: ThemeSkin) {
  themeStore.setSkin(skin)
}
</script>

<template>
  <div class="theme-skin-switch" role="group" aria-label="主题皮肤">
    <button
      v-for="item in skins"
      :key="item.key"
      class="theme-skin-switch__item"
      :class="{ 'theme-skin-switch__item--active': themeStore.skin === item.key }"
      type="button"
      @click="handleClick(item.key)"
    >
      {{ item.label }}
    </button>
  </div>
</template>

<style scoped>
.theme-skin-switch {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-surface);
}

.theme-skin-switch__item {
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  text-transform: capitalize;
}

.theme-skin-switch__item--active {
  background-color: var(--color-accent);
  color: #ffffff;
}
</style>
