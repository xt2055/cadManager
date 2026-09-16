<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useAppStore } from '@/stores/app.store'
import { useUiStore } from '@/stores/ui.store'
import { useAuthStore } from '@/stores/auth.store'
import SidebarNavigation from './SidebarNavigation.vue'
import PatentReminder from '@/features/patents/PatentReminder.vue'

defineOptions({
  name: 'DesktopSidebar',
})

const router = useRouter()
const appStore = useAppStore()
const uiStore = useUiStore()
const authStore = useAuthStore()
const currentUser = computed(() => authStore.currentUser)

function roleLabel(): string {
  if (authStore.hasRole('admin')) return '系统管理员'
  if (authStore.hasRole('reviewer')) return '审核人员'
  return '设计人员'
}

const userAvatar = computed(() => {
  if (!currentUser.value?.displayName) return '未'
  return currentUser.value.displayName.slice(0, 1)
})

async function logout() {
  if (!await uiStore.confirmNavigation()) return
  authStore.logout()
  uiStore.toast('已退出登录', 'info')
  router.push({ name: 'login' })
}
</script>

<template>
  <aside id="sidebar" :class="{ collapsed: appStore.sidebarCollapsed }">
    <SidebarNavigation />
    <PatentReminder />

    <div class="side-foot">
      <div class="side-user">
        <div class="avatar">{{ userAvatar }}</div>
        <div class="user-info">
          <b>{{ currentUser?.displayName || '未登录' }}</b>
          <span>{{ roleLabel() }}</span>
        </div>
        <button class="icon-btn account-switch" type="button" title="退出登录" @click="logout">
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
