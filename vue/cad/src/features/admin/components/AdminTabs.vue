<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'AdminTabs' })

defineProps<{
  active: 'users' | 'flows' | 'mgmt' | 'logs'
}>()
</script>

<template>
  <div class="admin-tabs">
    <RouterLink class="subtab" :class="{ 'router-link-active': active === 'users' }" :to="{ name: 'admin-accounts' }"><DemoIcon name="user-plus" :size="14" />账号管理</RouterLink>
    <RouterLink class="subtab" :class="{ 'router-link-active': active === 'flows' }" :to="{ name: 'admin-review-flows' }"><DemoIcon name="workflow" :size="14" />审核流程</RouterLink>
    <RouterLink class="subtab" :class="{ 'router-link-active': active === 'mgmt' }" :to="{ name: 'admin-drawings' }"><DemoIcon name="layers" :size="14" />图纸管理</RouterLink>
    <RouterLink class="subtab" :class="{ 'router-link-active': active === 'logs' }" :to="{ name: 'admin-logs' }"><DemoIcon name="scroll-text" :size="14" />操作日志</RouterLink>
  </div>
</template>

<style scoped>
.admin-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 18px;
}

.subtab {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 8px 16px;
  border: 1px solid var(--line);
  border-radius: 10px;
  color: var(--text-2);
  font-size: 13px;
}

.subtab:hover,
.subtab.router-link-active {
  border-color: var(--accent);
  color: var(--accent);
}
</style>
