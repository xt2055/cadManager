<script setup lang="ts">
defineOptions({
  name: 'AuthLayout',
})
</script>

<template>
  <div class="auth-window-root">
    <RouterView />
  </div>
</template>

<style scoped>
.auth-window-root {
  width: 100%;
  height: 100%;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
