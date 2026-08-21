<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import type { MainNavigationItem } from '@/constants/route.constants'
import SidebarBadge from './SidebarBadge.vue'

defineOptions({
  name: 'SidebarNavigationItem',
})

const props = defineProps<{
  item: MainNavigationItem
}>()

const route = useRoute()
const router = useRouter()

function isActive(): boolean {
  const name = props.item.routeName
  return route.name === name || route.matched.some((r) => r.name === name)
}

function handleClick() {
  router.push({ name: props.item.routeName })
}
</script>

<template>
  <button
    class="sidebar-navigation-item"
    :class="{ 'sidebar-navigation-item--active': isActive() }"
    type="button"
    @click="handleClick"
  >
    <span class="sidebar-navigation-item__icon"></span>
    <span class="sidebar-navigation-item__title">{{ item.title }}</span>
    <SidebarBadge class="sidebar-navigation-item__badge" />
  </button>
</template>

<style scoped>
.sidebar-navigation-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  height: 34px;
  padding: 0 10px;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-align: left;
}

.sidebar-navigation-item:hover {
  background-color: var(--color-bg-elevated);
  color: var(--color-text-primary);
}

.sidebar-navigation-item--active {
  background-color: var(--color-accent);
  color: #ffffff;
}

.sidebar-navigation-item__icon {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  border: 1.5px solid currentcolor;
  border-radius: 3px;
}

.sidebar-navigation-item__title {
  flex: 1;
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-navigation-item__badge {
  flex-shrink: 0;
}
</style>
