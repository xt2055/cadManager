<script setup lang="ts">
import { windowService } from '@/services/tauri/window.service'
import { useWindowStore } from '@/stores/window.store'

defineOptions({
  name: 'WindowMaximizeButton',
})

const windowStore = useWindowStore()

async function handleClick() {
  await windowService.toggleMaximize()
  windowStore.setMaximized(await windowService.isMaximized())
}
</script>

<template>
  <button class="window-control" type="button" aria-label="最大化" @click="handleClick">
    <span class="window-control__box"></span>
  </button>
</template>

<style scoped>
.window-control {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 26px;
  border-radius: var(--radius-sm);
  color: var(--color-text-secondary);
}

.window-control:hover {
  background-color: var(--color-bg-elevated);
}

.window-control__box {
  width: 10px;
  height: 10px;
  border: 1px solid currentcolor;
}
</style>
