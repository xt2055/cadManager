<script setup lang="ts">
import { useAppStore } from '@/stores/app.store'

defineOptions({
  name: 'SidebarCollapseButton',
})

const appStore = useAppStore()
</script>

<template>
  <button
    class="sidebar-collapse-button"
    type="button"
    :aria-label="appStore.sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
    @click="appStore.toggleSidebar()"
  >
    <span class="sidebar-collapse-button__icon"></span>
    <span v-if="!appStore.sidebarCollapsed">收起侧边栏</span>
  </button>
</template>

<style scoped>
.sidebar-collapse-button {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  width: 100%;
  min-height: 30px;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.sidebar-collapse-button:hover {
  background-color: var(--color-bg-elevated);
  color: var(--color-text-primary);
}

.sidebar-collapse-button__icon {
  width: 12px;
  height: 12px;
  border: 1px solid currentcolor;
  border-radius: 2px;
}
</style>
