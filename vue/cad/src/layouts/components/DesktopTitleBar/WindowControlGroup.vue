<script setup lang="ts">
import WindowMinimizeButton from './WindowMinimizeButton.vue'
import WindowMaximizeButton from './WindowMaximizeButton.vue'
import WindowHideButton from './WindowHideButton.vue'
import WindowCloseButton from './WindowCloseButton.vue'

defineOptions({
  name: 'WindowControlGroup',
})
</script>

<template>
  <div class="window-control-group">
    <WindowMinimizeButton />
    <WindowMaximizeButton />
    <WindowHideButton />
    <WindowCloseButton />
  </div>
</template>

<style scoped>
.window-control-group {
  display: flex;
  align-items: center;
  gap: 2px;
}
</style>
