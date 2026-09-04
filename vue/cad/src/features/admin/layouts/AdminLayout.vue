<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted } from 'vue'

import DemoModal from '@/components/feedback/DemoModal.vue'
import DemoToast from '@/components/feedback/DemoToast.vue'
import DesktopStatusBar from '@/layouts/components/DesktopStatusBar/DesktopStatusBar.vue'
import DesktopTitleBar from '@/layouts/components/DesktopTitleBar/DesktopTitleBar.vue'
import { windowService } from '@/services/tauri/window.service'
import { useAdminStore } from '@/stores/admin.store'
import { useReviewStore } from '@/stores/review.store'
import { useThemeStore } from '@/stores/theme.store'
import { useUiStore } from '@/stores/ui.store'
import AdminSidebar from '../components/AdminSidebar.vue'

defineOptions({ name: 'AdminLayout' })

const adminStore = useAdminStore()
const reviewStore = useReviewStore()
const uiStore = useUiStore()
const themeStore = useThemeStore()
const isStarrySkin = computed(() => themeStore.skin === 'starry')
const StarryGalaxy = defineAsyncComponent(() => import('@/components/common/StarryGalaxy.vue'))

onMounted(async () => {
  const initializePromise = Promise.all([
    adminStore.loadUsers(),
    reviewStore.loadFlows(),
  ]).catch((error: unknown) => {
    console.error('初始化后台数据失败', error)
    uiStore.toast('后台数据加载失败，请检查服务配置', 'warn')
  })
  try {
    await windowService.setWorkspaceWindowSize()
  } catch (error) {
    console.error('初始化后台窗口失败', error)
  }
  await initializePromise
})
</script>

<template>
  <div class="bg-fx"><StarryGalaxy v-if="isStarrySkin" /><div class="bg-grid"></div><div class="orb o1"></div><div class="orb o2"></div></div>
  <section class="admin-workspace app-shell layout-expand-in">
    <DesktopTitleBar />
    <div id="layout">
      <AdminSidebar />
      <main id="main"><div id="content"><RouterView /></div></main>
    </div>
    <DesktopStatusBar />
    <DemoToast />
    <DemoModal />
  </section>
</template>

<style scoped>
.admin-workspace { width: 100%; }
</style>
