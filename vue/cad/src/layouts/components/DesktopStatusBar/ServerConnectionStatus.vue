<script setup lang="ts">
defineOptions({
  name: 'ServerConnectionStatus',
})
</script>

<template>
  <span class="status-item">
    <span class="status-item__dot status-item__dot--success"></span>
    服务已连接
  </span>
</template>

<style scoped>
.status-item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  white-space: nowrap;
}

.status-item__dot {
  width: 6px;
  height: 6px;
  border-radius: var(--radius-full);
}

.status-item__dot--success {
  background-color: var(--color-success);
}
</style>
