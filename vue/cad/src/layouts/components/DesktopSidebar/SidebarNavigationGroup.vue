<script setup lang="ts">
import type { MainNavigationItem } from '@/constants/route.constants'
import SidebarNavigationItem from './SidebarNavigationItem.vue'

defineOptions({
  name: 'SidebarNavigationGroup',
})

defineProps<{
  title: string
  items: MainNavigationItem[]
}>()
</script>

<template>
  <div class="sidebar-navigation-group">
    <p v-if="title" class="sidebar-navigation-group__title">{{ title }}</p>
    <ul class="sidebar-navigation-group__list">
      <li v-for="item in items" :key="item.key">
        <SidebarNavigationItem :item="item" />
      </li>
    </ul>
  </div>
</template>

<style scoped>
.sidebar-navigation-group__title {
  padding: 8px 10px 4px;
  font-size: var(--font-size-xs);
  color: var(--color-text-tertiary);
}

.sidebar-navigation-group__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
</style>
