<script setup lang="ts">
defineOptions({
  name: 'AuthLayout',
})
</script>

<template>
  <section class="auth-layout">
    <RouterView />
  </section>
</template>

<style scoped>
.auth-layout {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--color-bg-base);
}
</style>
