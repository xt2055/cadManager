<script setup lang="ts">
import { appConfig } from '@/app/app.config'

defineOptions({
  name: 'AppVersionStatus',
})
</script>

<template>
  <span class="status-item">v{{ appConfig.version }}</span>
</template>

<style scoped>
.status-item {
  white-space: nowrap;
}
</style>
