<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useAppStore } from '@/stores/app.store'
import SidebarNavigation from './SidebarNavigation.vue'

defineOptions({
  name: 'DesktopSidebar',
})

const appStore = useAppStore()
</script>

<template>
  <aside id="sidebar" :class="{ collapsed: appStore.sidebarCollapsed }">
    <SidebarNavigation />

    <div class="side-foot">
      <div class="side-user">
        <div class="avatar">未</div>
        <div class="user-info">
          <b>未登录</b>
          <span>用户信息待获取</span>
        </div>
        <button class="icon-btn account-switch" type="button" title="切换账号">
          <DemoIcon name="log-out" :size="14" />
        </button>
      </div>

      <button class="collapse-btn" type="button" @click="appStore.toggleSidebar()">
        <DemoIcon name="chevrons-left" :size="15" />
        <span class="collapse-txt">收起导航</span>
      </button>
    </div>
  </aside>
</template>
