<script setup lang="ts">
defineOptions({
  name: 'EmptyLayout',
})
</script>

<template>
  <section class="empty-layout">
    <RouterView />
  </section>
</template>

<style scoped>
.empty-layout {
  width: 100%;
  height: 100%;
}
</style>
