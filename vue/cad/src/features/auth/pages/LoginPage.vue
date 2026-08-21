<script setup lang="ts">
defineOptions({
  name: 'LoginPage',
})
</script>

<template>
  <main class="login-page">
    <div class="login-page__content">
      <div class="login-page__brand"></div>
      <RouterView />
    </div>
  </main>
</template>

<style scoped>
.login-page {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.login-page__content {
  width: min(100% - 32px, 420px);
}

.login-page__brand {
  width: 48px;
  height: 48px;
  margin: 0 auto 24px;
  border-radius: var(--radius-lg);
  background-color: var(--color-accent);
}
</style>
