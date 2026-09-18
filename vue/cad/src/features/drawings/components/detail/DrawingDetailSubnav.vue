<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { withReviewContext } from '@/features/drawings/review-context'
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

// 审核上下文整体继承：只带 from/reviewNo 会丢掉 reviewCaseId，
// 查看页在多轮审核的图纸上就无法确定该显示哪一轮的批注。
const reviewQuery = computed(() => withReviewContext(
  route.query.from === 'review' ? { reviewNo: String(route.query.reviewNo || drawingId.value) } : {},
  route.query,
))
</script>

<template>
  <aside class="detail-nav" aria-label="图纸详情导航">
    <div v-for="group in groups" :key="group.title" class="detail-nav-group">
      <div class="detail-nav-title">{{ group.title }}</div>
      <RouterLink
        v-for="tab in group.items"
        :key="tab.key"
        class="detail-nav-item"
        :to="{ name: tab.routeName, params: { drawingId }, query: reviewQuery }"
      >
        <DemoIcon :name="tab.icon" :size="16" />
        <span class="detail-nav-label">{{ tab.title }}</span>
      </RouterLink>
    </div>
  </aside>
</template>
