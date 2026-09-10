<script setup lang="ts">
import { defineAsyncComponent, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute } from 'vue-router'
const HydraulicBackdrop = defineAsyncComponent(() => import('./components/common/HydraulicBackdrop.vue'))
import './styles/themes/juli.css'
import { useThemeStore } from './stores/theme.store'
import { useAuthStore } from './stores/auth.store'
import { startAutomaticPartIndex } from './services/part-index-auto.service'

defineOptions({
  name: 'App',
})

const themeStore = useThemeStore()
const route = useRoute()
const authStore = useAuthStore()
let stopAutomaticIndex: (() => void) | undefined
watch(() => authStore.isAuthenticated ? authStore.currentUser?.id : null, (userId) => {
  stopAutomaticIndex?.()
  stopAutomaticIndex = userId ? startAutomaticPartIndex() : undefined
}, { immediate: true })
onBeforeUnmount(() => stopAutomaticIndex?.())

onMounted(() => {
  themeStore.applyTheme()
})
</script>

<template>
  <HydraulicBackdrop v-if="themeStore.skin === 'juli' && route.path !== '/login' && route.path !== '/login/'" />
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
