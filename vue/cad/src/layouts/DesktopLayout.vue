<script setup lang="ts">
import { onMounted } from 'vue'
import DesktopTitleBar from './components/DesktopTitleBar/DesktopTitleBar.vue'
import DesktopSidebar from './components/DesktopSidebar/DesktopSidebar.vue'
import DesktopStatusBar from './components/DesktopStatusBar/DesktopStatusBar.vue'
import DemoModal from '@/components/feedback/DemoModal.vue'
import DemoToast from '@/components/feedback/DemoToast.vue'
import { windowService } from '@/services/tauri/window.service'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({
  name: 'DesktopLayout',
})

const demoStore = useDemoStore()
const uiStore = useUiStore()

onMounted(async () => {
  const initializePromise = demoStore.initialize().catch((error: unknown) => {
    console.error('初始化业务数据失败', error)
    uiStore.toast('业务数据加载失败，请检查数据服务配置', 'warn')
  })

  try {
    await windowService.setSizeConstraints(1100, 700)
    await windowService.setResizable(true)
    await windowService.setSize(1440, 900)
    await windowService.center()
  } catch (error) {
    console.error('初始化工作台窗口失败', error)
  }

  await initializePromise
})
</script>

<template>
  <div class="bg-fx">
    <div class="bg-grid"></div>
    <div class="orb o1"></div>
    <div class="orb o2"></div>
  </div>

  <section class="desktop-layout app-shell layout-expand-in">
    <DesktopTitleBar />

    <div id="layout">
      <DesktopSidebar />
      <main id="main">
        <div id="content">
          <RouterView />
        </div>
      </main>
    </div>

    <DesktopStatusBar />
    <DemoToast />
    <DemoModal />
  </section>
</template>

<style scoped>
.desktop-layout {
  width: 100%;
}

.layout-expand-in {
  animation: layout-burst 0.5s cubic-bezier(0.16, 1, 0.3, 1) forwards;
}

@keyframes layout-burst {
  0% {
    opacity: 0;
    transform: scale(0.96);
    filter: blur(4px);
  }
  100% {
    opacity: 1;
    transform: scale(1);
    filter: blur(0);
  }
}
</style>
