<script setup lang="ts">
defineOptions({
  name: 'NotificationButton',
})
</script>

<template>
  <button class="notification-button" type="button" aria-label="通知">
    <span class="notification-button__icon"></span>
    <span class="notification-button__dot"></span>
  </button>
</template>

<style scoped>
.notification-button {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 26px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-surface);
}

.notification-button__icon {
  width: 14px;
  height: 14px;
  border: 1.5px solid var(--color-text-secondary);
  border-radius: 4px;
}

.notification-button__dot {
  position: absolute;
  top: 5px;
  right: 6px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: var(--color-danger);
}
</style>
