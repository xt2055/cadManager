<script setup lang="ts">
import DesktopTitleBar from './components/DesktopTitleBar/DesktopTitleBar.vue'
import DesktopSidebar from './components/DesktopSidebar/DesktopSidebar.vue'
import DesktopStatusBar from './components/DesktopStatusBar/DesktopStatusBar.vue'
import DemoModal from '@/components/feedback/DemoModal.vue'
import DemoToast from '@/components/feedback/DemoToast.vue'

defineOptions({
  name: 'DesktopLayout',
})
</script>

<template>
  <div class="bg-fx">
    <div class="bg-grid"></div>
    <div class="orb o1"></div>
    <div class="orb o2"></div>
  </div>

  <section class="desktop-layout app-shell">
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
</style>
