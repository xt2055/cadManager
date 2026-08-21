<script setup lang="ts">
defineOptions({
  name: 'SidebarBadge',
})
</script>

<template>
  <span class="sidebar-badge"></span>
</template>

<style scoped>
.sidebar-badge {
  display: inline-block;
  min-width: 8px;
  height: 8px;
  border-radius: var(--radius-full);
  background-color: var(--color-warning);
}
</style>
