<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import ReviewWorkspacePanel from '@/features/reviews/components/ReviewWorkspacePanel.vue'
import { activeReviewNode, reviewNodeStatusLabel, toWorkspaceNodes } from '@/features/reviews/review-workspace'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useReviewStore } from '@/stores/review.store'

defineOptions({ name: 'DrawingReviewTab' })

const route = useRoute()
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const authStore = useAuthStore()
const showOverview = ref(false)

const drawingNo = computed(() => String(route.params.drawingId ?? '').trim())
const drawing = computed(() => drawingStore.getDrawing(drawingNo.value) ?? drawingStore.getPart(drawingNo.value))
const reviewCase = computed(() => reviewStore.getCase(drawingNo.value))
const reviewing = computed(() => reviewCase.value ? reviewCase.value.status === 'reviewing' : drawing.value?.status === 'reviewing')
const nodes = computed(() => toWorkspaceNodes(reviewCase.value))
const currentNode = computed(() => activeReviewNode(nodes.value, reviewing.value))

onMounted(() => {
  void Promise.all([drawingStore.load(), reviewStore.load()]).catch(() => undefined)
})
</script>

<template>
  <div class="review-tab-view">
    <ReviewWorkspacePanel
      v-if="!showOverview"
      :drawing-no="drawingNo"
      embedded
      :show-overview="showOverview"
      @toggle-overview="showOverview = true"
    />

    <div v-else class="overview">
      <div class="overview-head">
        <h3>流程总览</h3>
        <button class="btn primary" type="button" @click="showOverview = false">返回签署工作台</button>
      </div>
      <ol class="overview-list">
        <li v-for="node in nodes" :key="node.name" :class="[node.status, { current: node.name === currentNode?.name }]">
          <strong>{{ node.name }}</strong>
          <span>{{ node.assignedName || '待定' }} · {{ reviewNodeStatusLabel(node, currentNode?.name ?? null, reviewing, authStore.currentUser?.id, reviewCase?.status === 'rejected') }}</span>
          <p>{{ node.opinion || '暂无签署意见' }}</p>
        </li>
      </ol>
      <div v-if="!nodes.length" class="card empty">
        <DemoIcon name="stamp" :size="34" />
        <div class="t">尚未初始化审核流程</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.review-tab-view { display: flex; flex-direction: column; gap: 16px; }
.overview-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.overview-head h3 { margin: 0; font-size: 16px; }
.overview-list { display: flex; flex-direction: column; gap: 10px; margin: 0; padding: 0; list-style: none; }
.overview-list li { padding: 14px 16px; border-radius: 12px; background: var(--panel); }
.overview-list li.current { box-shadow: inset 0 0 0 1px var(--accent); }
.overview-list strong { display: block; }
.overview-list span, .overview-list p { color: var(--text-3); font-size: 12px; }
.overview-list p { margin: 6px 0 0; color: var(--text-2); }
</style>
