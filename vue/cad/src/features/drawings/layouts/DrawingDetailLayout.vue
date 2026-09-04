<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import DrawingDetailHeader from '../components/detail/DrawingDetailHeader.vue'
import DrawingDetailSubnav from '../components/detail/DrawingDetailSubnav.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useWorkspaceStore } from '@/stores/workspace.store'

defineOptions({
  name: 'DrawingDetailLayout',
})

const route = useRoute()
const router = useRouter()
const domainStore = useDomainStore()
const drawingStore = useDrawingStore()
const workspaceStore = useWorkspaceStore()
const isPreview = computed(() => route.name === 'drawing-preview')
const drawing = computed(() => {
  const id = String(route.params.drawingId ?? '')
  return drawingStore.getDrawing(id) ?? drawingStore.getPart(id)
})

async function syncDrawing() {
  const drawingId = String(route.params.drawingId ?? '')
  await Promise.all([drawingStore.load(), domainStore.initialize()])
  if (drawingId) {
    const item = drawingStore.getDrawing(drawingId) ?? drawingStore.getPart(drawingId)
    if (item) {
      if ('parentNo' in item) workspaceStore.selectPart(item.id)
      else workspaceStore.selectDrawing(item.id)
    }
    // 详情 Tab 尚在迁移期，保留旧 Store 的当前身份桥接。
    domainStore.openDrawing(drawingId)
    await domainStore.refreshDrawingDesigner(drawingId).catch((error) => {
      console.warn('读取图纸标题栏设计人失败', error)
    })
  } else {
    workspaceStore.clearSelection()
    domainStore.clearCurrentDrawing()
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
    <template v-if="drawing">
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
