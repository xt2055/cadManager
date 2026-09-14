<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDrawingStore } from '@/stores/drawing.store'
import { useReviewStore } from '@/stores/review.store'
import { useAuthStore } from '@/stores/auth.store'

defineOptions({
  name: 'SidebarNavigation',
})

const route = useRoute()
const router = useRouter()
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const authStore = useAuthStore()

const groups = [
  {
    title: '主导航',
    items: [
      { id: 'dashboard', routeName: 'dashboard', icon: 'layout-dashboard', label: '工作台' },
      { id: 'library', routeName: 'drawing-library', icon: 'search', label: '图纸库', badge: 'library' },
      { id: 'parts', routeName: 'part-index-list', icon: 'boxes', label: '零件索引' },
      { id: 'patents', routeName: 'patents', icon: 'shield', label: '专利管理' },
      { id: 'review', routeName: 'review-pending', icon: 'clipboard-check', label: '图纸审核', badge: 'review' },
    ],
  },
  {
    title: '追溯',
    items: [{ id: 'history', routeName: 'operation-logs', icon: 'history', label: '操作记录' }],
  },
  {
    title: '系统',
    items: [
      { id: 'settings', routeName: 'settings', icon: 'settings', label: '系统设置' },
      { id: 'admin', routeName: 'admin-accounts', icon: 'shield', label: '后台管理' },
    ],
  },
]

const visibleGroups = computed(() => groups.map((group) => ({
  ...group,
  items: group.items.filter((item) => item.id !== 'admin' || authStore.hasRole('admin')),
})).filter((group) => group.items.length > 0))

const activeId = computed(() => {
  if (route.name === 'patents') return 'patents'
  if (route.name === 'drawing-library' || route.name === 'drawing-create' || route.name === 'drawing-detail') return 'library'
  if (route.name === 'part-index-list' || route.name === 'part-index-detail') return 'parts'
  if (route.name === 'review-pending' || route.name === 'review-completed' || route.name === 'review-center' || route.name === 'review-workspace') return 'review'
  if (String(route.name || '').startsWith('admin-')) return 'admin'
  if (route.name === 'operation-logs') return 'history'
  if (route.name === 'settings') return 'settings'
  return 'dashboard'
})

function go(routeName: string) {
  router.push({ name: routeName })
}
</script>

<template>
  <nav id="navBox">
    <template v-for="group in visibleGroups" :key="group.title">
      <div class="nav-group">{{ group.title }}</div>
      <button
        v-for="item in group.items"
        :key="item.id"
        class="nav-item"
        :class="{ active: activeId === item.id }"
        type="button"
        @click="go(item.routeName)"
      >
        <DemoIcon :name="item.icon" :size="17" />
        <span class="nav-text">{{ item.label }}</span>
        <span
          v-if="item.badge === 'review' && reviewStore.myPendingReviews().length > 0"
          class="badge"
          data-badge="review"
        >{{ reviewStore.myPendingReviews().length }}</span>
        <span v-else-if="item.badge === 'library' && drawingStore.drawings.length" class="badge muted-badge">{{ drawingStore.drawings.length }}</span>
      </button>
    </template>
  </nav>
</template>
