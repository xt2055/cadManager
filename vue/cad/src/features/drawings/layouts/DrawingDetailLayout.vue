<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import DrawingDetailHeader from '../components/detail/DrawingDetailHeader.vue'
import DrawingDetailSubnav from '../components/detail/DrawingDetailSubnav.vue'
import { useDemoStore } from '@/stores/demo.store'

defineOptions({
  name: 'DrawingDetailLayout',
})

const route = useRoute()
const router = useRouter()
const demoStore = useDemoStore()
const isPreview = computed(() => route.name === 'drawing-preview')

async function syncDrawing() {
  await demoStore.initialize()
  const drawingId = String(route.params.drawingId ?? '')
  if (drawingId) {
    demoStore.openDrawing(drawingId)
  } else {
    demoStore.clearCurrentDrawing()
  }
}

onMounted(() => {
  void syncDrawing()
})
watch(() => route.params.drawingId, () => {
  void syncDrawing()
})
</script>

<template>
  <div class="page detail-page">
    <template v-if="demoStore.currentDrawing">
      <DrawingDetailHeader />
      <DrawingDetailSubnav />
      <div class="tab-body" :class="{ 'no-scroll': isPreview }">
        <div class="tab-anim" :class="{ 'tab-anim--preview': isPreview }">
          <RouterView />
        </div>
      </div>
    </template>
    <div v-else class="card empty detail-empty">
      <DemoIcon name="file" :size="40" />
      <div class="t">未找到图纸</div>
      <button class="btn primary" type="button" @click="router.push({ name: 'drawing-library' })">返回图纸库</button>
    </div>
  </div>
</template>
