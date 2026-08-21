<script setup lang="ts">
defineOptions({
  name: 'SidebarUserCard',
})
</script>

<template>
  <div class="sidebar-user-card">
    <span class="sidebar-user-card__avatar"></span>
    <span class="sidebar-user-card__content">
      <span class="sidebar-user-card__name">当前用户</span>
      <span class="sidebar-user-card__role">设计管理员</span>
    </span>
  </div>
</template>

<style scoped>
.sidebar-user-card {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  padding: 8px;
  color: var(--color-text-secondary);
}

.sidebar-user-card__avatar {
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  border-radius: var(--radius-full);
  background-color: var(--color-accent);
}

.sidebar-user-card__content {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.sidebar-user-card__name,
.sidebar-user-card__role {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.sidebar-user-card__name {
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-medium);
}

.sidebar-user-card__role {
  font-size: var(--font-size-xs);
}
</style>
