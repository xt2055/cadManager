<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'

import { useAuthStore } from '@/stores/auth.store'
import { useSystemStatusStore } from '@/stores/system-status.store'

defineOptions({
  name: 'DesktopStatusBar',
})

const authStore = useAuthStore()
const systemStore = useSystemStatusStore()

const currentUserLabel = computed(() => authStore.currentUser?.displayName || '未登录')

onMounted(() => {
  systemStore.startPolling(30000)
})

onUnmounted(() => {
  systemStore.stopPolling()
})
</script>

<template>
  <footer id="statusbar">
    <span>
      <span class="status-dot" :style="{ background: systemStore.isHealthy ? 'var(--ok)' : systemStore.isOnline ? 'var(--warn)' : 'var(--danger)' }"></span>
      服务状态 · {{ systemStore.isHealthy ? '已连接 (在线)' : systemStore.isOnline ? '部分降级' : '已离线' }}
    </span>
    <span>在线用户 <b>{{ systemStore.onlineCount }}</b> 人</span>
    <span class="sp"></span>
    <span class="zoom-ind">缩放 100%</span>
    <span>当前用户 · {{ currentUserLabel }}</span>
    <span>v0.1.0</span>
  </footer>
</template>
