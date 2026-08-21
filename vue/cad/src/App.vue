<script setup lang="ts">
import { onMounted } from 'vue'
import { useThemeStore } from './stores/theme.store'

defineOptions({
  name: 'App',
})

const themeStore = useThemeStore()

onMounted(() => {
  themeStore.applyTheme()
})
</script>

<template>
  <RouterView />
</template>

<style>
#app {
  width: 100%;
  height: 100%;
}
</style>
