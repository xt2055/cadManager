<script setup lang="ts">
defineOptions({
  name: 'GlobalSearchTrigger',
})
</script>

<template>
  <button class="global-search-trigger" type="button">
    <span class="global-search-trigger__icon"></span>
    <span class="global-search-trigger__hint">全局搜索</span>
    <kbd class="global-search-trigger__kbd">Ctrl K</kbd>
  </button>
</template>

<style scoped>
.global-search-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 320px;
  height: 28px;
  padding: 0 10px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background-color: var(--color-bg-surface);
  color: var(--color-text-tertiary);
  font-size: var(--font-size-sm);
}

.global-search-trigger__icon {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1.5px solid var(--color-text-tertiary);
}

.global-search-trigger__hint {
  flex: 1;
  text-align: left;
}

.global-search-trigger__kbd {
  padding: 1px 6px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-xs);
  background-color: var(--color-bg-elevated);
}
</style>
