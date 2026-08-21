<script setup lang="ts">
import { appConfig } from '@/app/app.config'

defineOptions({
  name: 'AppBrand',
})
</script>

<template>
  <div class="app-brand">
    <span class="app-brand__icon"></span>
    <span class="app-brand__title">{{ appConfig.appName }}</span>
  </div>
</template>

<style scoped>
.app-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 160px;
  height: 100%;
}

.app-brand__icon {
  width: 20px;
  height: 20px;
  border-radius: var(--radius-sm);
  background-color: var(--color-accent);
}

.app-brand__title {
  font-size: var(--font-size-md);
  font-weight: var(--font-weight-semibold);
  color: var(--color-text-primary);
}
</style>
