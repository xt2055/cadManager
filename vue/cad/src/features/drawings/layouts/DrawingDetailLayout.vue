<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import DrawingDetailHeader from '../components/detail/DrawingDetailHeader.vue'
import DrawingDetailSubnav from '../components/detail/DrawingDetailSubnav.vue'
import { useDrawingStore } from '@/stores/drawing.store'
import { useWorkspaceStore } from '@/stores/workspace.store'

defineOptions({
  name: 'DrawingDetailLayout',
})

const route = useRoute()
const router = useRouter()
const drawingStore = useDrawingStore()
const workspaceStore = useWorkspaceStore()
const isPreview = computed(() => route.name === 'drawing-preview')
const navCollapsed = ref(false)
const loading = ref(true)
const loadError = ref('')
let loadSequence = 0
const drawing = computed(() => {
  const id = String(route.params.drawingId ?? '')
  return drawingStore.getDrawing(id) ?? drawingStore.getPart(id)
})

async function syncDrawing(force = false) {
  const sequence = ++loadSequence
  const drawingId = String(route.params.drawingId ?? '')
  loading.value = true
  loadError.value = ''
  try {
    await (force ? drawingStore.refresh() : drawingStore.load())
    if (sequence !== loadSequence) return
    if (drawingId) {
      let item = drawingStore.getDrawing(drawingId) ?? drawingStore.getPart(drawingId)
      if (!item && !force) {
        await drawingStore.refresh()
        if (sequence !== loadSequence) return
        item = drawingStore.getDrawing(drawingId) ?? drawingStore.getPart(drawingId)
      }
      if (item) {
        if ('parentNo' in item) workspaceStore.selectPart(item.id)
        else workspaceStore.selectDrawing(item.id)
        void drawingStore.refreshDesigner(drawingId).catch(error => console.warn('读取设计人失败', error))
      } else workspaceStore.clearSelection()
    } else workspaceStore.clearSelection()
  } catch (error) {
    if (sequence === loadSequence) {
      loadError.value = error instanceof Error ? error.message : '请检查连接后重试'
    }
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

watch(() => route.params.drawingId, () => {
  void syncDrawing()
}, { immediate: true })
onBeforeUnmount(() => { loadSequence++ })
</script>

<template>
  <div class="page detail-page">
    <div v-if="loading" class="card empty detail-empty" role="status">
      <div class="t">正在加载图纸…</div>
    </div>
    <div v-else-if="loadError" class="card empty detail-empty" role="alert">
      <div class="t">图纸加载失败</div><p>{{ loadError }}</p>
      <button class="btn primary" type="button" @click="syncDrawing(true)">重新加载</button>
      <button class="btn" type="button" @click="router.push({ name: 'drawing-library' })">返回图纸库</button>
    </div>
    <template v-else-if="drawing">
      <DrawingDetailHeader />
      <div class="detail-split" :class="{ 'nav-collapsed': navCollapsed }">
        <DrawingDetailSubnav />
        <button
          class="detail-nav-toggle"
          type="button"
          :aria-label="navCollapsed ? '展开导航' : '折叠导航'"
          :aria-expanded="!navCollapsed"
          @click="navCollapsed = !navCollapsed"
        >
          <DemoIcon :name="navCollapsed ? 'chevron-right' : 'chevron-left'" :size="14" />
        </button>
        <div class="tab-body" :class="{ 'no-scroll': isPreview }">
          <div class="tab-anim" :class="{ 'tab-anim--preview': isPreview }">
            <RouterView />
          </div>
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
