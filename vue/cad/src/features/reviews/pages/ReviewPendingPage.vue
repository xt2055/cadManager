<script setup lang="ts">
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'ReviewPendingPage' })

const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()

function startReviewingFlow(no: string) {
  domainStore.openDrawing(no)
  uiStore.toast(`进入图纸「${no}」详情：请先查阅图纸文件，再前往审核流程签署意见`, 'info')
  router.push({ name: 'drawing-preview', params: { drawingId: no } })
}
</script>

<template>
  <div class="page review-page">
    <div class="section-head">
      <h3>待我审核任务</h3>
      <span class="lib-count">点击进入图纸详情 · 审阅图纸后完成签署与意见录入</span>
    </div>

    <template v-if="domainStore.myPendingReviews.length">
      <div v-for="review in domainStore.myPendingReviews" :key="`${review.reviewCaseId}-${review.node}`" class="card review-row">
        <div class="feed-ic review-icon">
          <DemoIcon name="clipboard-check" :size="20" />
        </div>

        <div class="review-main">
          <div class="review-title">
            <b>{{ review.name }}</b>
            <span class="dh-no review-no">{{ review.no }}</span>
          </div>
          <div class="review-meta">
            待审节点 <b>{{ review.node }}</b> · 责任人 {{ review.by }} · 发起于 {{ review.time }} · 顺序流转
          </div>
        </div>

        <div class="review-row-actions">
          <button class="btn primary" type="button" @click="startReviewingFlow(review.no)">
            <DemoIcon name="eye" :size="14" />查看并开始审核
          </button>
        </div>
      </div>
    </template>

    <div v-else class="card">
      <div class="empty">
        <DemoIcon name="check-circle-2" :size="34" />
        <div class="t">太棒了，暂无待办审核任务</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.review-page {
  min-width: 0;
}
.section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 4px 0 16px;
}
.section-head h3 {
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 800;
}
.lib-count {
  color: var(--text-3);
  font-size: 11.5px;
}
.review-row {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding: 16px 20px;
  border-radius: var(--radius);
}
.review-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
}
.review-icon svg {
  color: var(--warn);
}
.review-main {
  flex: 1;
  min-width: 0;
}
.review-title {
  display: flex;
  align-items: center;
  gap: 10px;
}
.review-title b {
  font-size: 14px;
}
.review-no {
  padding: 2px 8px;
  font-size: 11px;
}
.review-meta {
  margin-top: 6px;
  color: var(--text-3);
  font-size: 12px;
}
.review-meta b {
  color: var(--warn);
}
.review-row-actions {
  display: flex;
  gap: 8px;
}
@media (max-width: 760px) {
  .review-row {
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .review-main {
    min-width: calc(100% - 60px);
  }
  .review-row-actions {
    width: 100%;
    margin-left: 60px;
  }
}
</style>
