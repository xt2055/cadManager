<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { drawingDetailTabs } from '../../constants/drawing-detail-tabs'
import { useDrawingStore } from '@/stores/drawing.store'

defineOptions({
  name: 'DrawingDetailSubnav',
})

const route = useRoute()
const drawingStore = useDrawingStore()
const drawingId = computed(() => String(route.params.drawingId ?? ''))
const tabs = computed(() => drawingDetailTabs.filter((tab) => tab.key !== 'changes' || Boolean(drawingStore.getDrawing(drawingId.value))))
const groups = computed(() => {
  const buckets = new Map<string, typeof tabs.value>()
  for (const tab of tabs.value) {
    const items = buckets.get(tab.group)
    if (items) items.push(tab)
    else buckets.set(tab.group, [tab])
  }
  return [...buckets].map(([title, items]) => ({ title, items }))
})
</script>

<template>
  <aside class="detail-nav" aria-label="图纸详情导航">
    <div v-for="group in groups" :key="group.title" class="detail-nav-group">
      <div class="detail-nav-title">{{ group.title }}</div>
      <RouterLink
        v-for="tab in group.items"
        :key="tab.key"
        class="detail-nav-item"
        :to="{ name: tab.routeName, params: { drawingId }, query: route.query.from === 'review' ? { from: 'review', reviewNo: route.query.reviewNo || drawingId } : {} }"
      >
        <DemoIcon :name="tab.icon" :size="16" />
        <span class="detail-nav-label">{{ tab.title }}</span>
      </RouterLink>
    </div>
  </aside>
</template>
