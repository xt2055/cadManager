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
</script>

<template>
  <nav class="subnav" aria-label="图纸详情导航">
    <RouterLink
      v-for="tab in tabs"
      :key="tab.key"
      class="subtab"
      :to="{ name: tab.routeName, params: { drawingId } }"
    >
      <DemoIcon :name="tab.icon" :size="15" />
      {{ tab.title }}
    </RouterLink>
  </nav>
</template>
