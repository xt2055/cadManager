<script setup lang="ts">
import { defineAsyncComponent, onMounted } from 'vue'
const HydraulicBackdrop = defineAsyncComponent(() => import('./components/common/HydraulicBackdrop.vue'))
import './styles/themes/juli.css'
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
  <HydraulicBackdrop v-if="themeStore.skin === 'juli'" />
  <div class="theme-ui" :class="themeStore.skin === 'juli' ? `hydraulic-ui hydraulic-ui--${themeStore.hydraulicPhase}` : ''" :inert="themeStore.hydraulicPhase !== 'idle'">
    <RouterView />
  </div>
  <div v-if="themeStore.hydraulicPhase !== 'idle'" class="hydraulic-transition-shield" :class="`hydraulic-transition--${themeStore.hydraulicPhase}`" aria-label="正在切换主题" role="status">
    <div class="hydraulic-pressure-wave"></div>
    <div class="hydraulic-impact-light"></div>
  </div>
</template>

<style>
#app {
  width: 100%;
  height: 100%;
}
.theme-ui { display: contents; }
.theme-ui.hydraulic-ui { display: flex; flex: 1 1 auto; flex-direction: column; min-width: 0; min-height: 0; position: relative; z-index: 1; }
</style>
