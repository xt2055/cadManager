<script setup lang="ts">
import { windowService } from '@/services/tauri/window.service'

defineOptions({
  name: 'WindowCloseButton',
})

async function handleClick() {
  await windowService.close()
}
</script>

<template>
  <button class="window-control window-control--close" type="button" aria-label="关闭" @click="handleClick">
    <span class="window-control__cross"></span>
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

.window-control--close:hover {
  background-color: var(--color-danger);
  color: #ffffff;
}

.window-control__cross {
  position: relative;
  width: 10px;
  height: 10px;
}

.window-control__cross::before,
.window-control__cross::after {
  content: '';
  position: absolute;
  top: 50%;
  left: 0;
  width: 100%;
  height: 1px;
  background-color: currentcolor;
}

.window-control__cross::before {
  transform: rotate(45deg);
}

.window-control__cross::after {
  transform: rotate(-45deg);
}
</style>
