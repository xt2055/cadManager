<script setup lang="ts">
import { windowService } from '@/services/tauri/window.service'

defineOptions({
  name: 'WindowHideButton',
})

async function handleClick() {
  await windowService.hide()
}
</script>

<template>
  <button class="window-control" type="button" aria-label="隐藏到托盘" @click="handleClick">
    <span class="window-control__bar window-control__bar--hide"></span>
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

.window-control__bar {
  display: block;
  width: 10px;
  border-top: 1px solid currentcolor;
}

.window-control__bar--hide {
  border-top-width: 1px;
}
</style>
