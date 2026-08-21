<script setup lang="ts">
import { appConfig } from '@/app/app.config'

defineOptions({
  name: 'SidebarHeader',
})

defineProps<{
  isCollapsed?: boolean
}>()
</script>

<template>
  <div class="sidebar-header">
    <span class="sidebar-header__logo"></span>
    <span v-show="!isCollapsed" class="sidebar-header__title">{{ appConfig.appTitle }}</span>
  </div>
</template>

<style scoped>
.sidebar-header {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 48px;
  padding: 0 12px;
}

.sidebar-header__logo {
  width: 24px;
  height: 24px;
  flex-shrink: 0;
  border-radius: var(--radius-sm);
  background-color: var(--color-accent);
}

.sidebar-header__title {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
