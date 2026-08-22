<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useAppStore } from '@/stores/app.store'
import { useUiStore } from '@/stores/ui.store'
import SidebarNavigation from './SidebarNavigation.vue'

defineOptions({
  name: 'DesktopSidebar',
})

const router = useRouter()
const appStore = useAppStore()
const uiStore = useUiStore()

const currentUser = ref<{ name: string; role: string; account: string } | null>(null)

function loadUser() {
  const userJson = localStorage.getItem('cad_current_user')
  if (userJson) {
    try {
      currentUser.value = JSON.parse(userJson)
    } catch {
      currentUser.value = null
    }
  } else {
    currentUser.value = { name: '张工', role: '设计工程师', account: 'zhang' }
  }
}

const userAvatar = computed(() => {
  if (!currentUser.value?.name) return '张'
  return currentUser.value.name.slice(0, 1)
})

function logout() {
  localStorage.removeItem('cad_access_token')
  localStorage.removeItem('cad_current_user')
  uiStore.toast('已退出登录', 'info')
  router.push({ name: 'login' })
}

onMounted(() => {
  loadUser()
})
</script>

<template>
  <aside id="sidebar" :class="{ collapsed: appStore.sidebarCollapsed }">
    <SidebarNavigation />

    <div class="side-foot">
      <div class="side-user">
        <div class="avatar">{{ userAvatar }}</div>
        <div class="user-info">
          <b>{{ currentUser?.name || '张工' }}</b>
          <span>{{ currentUser?.role || '设计工程师' }}</span>
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
