<script setup lang="ts">
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'ReviewPendingPage' })

const router = useRouter()
const demoStore = useDemoStore()
const uiStore = useUiStore()

function openDrawing(no: string) {
  demoStore.openDrawing(no)
  router.push({ name: 'drawing-preview', params: { drawingId: no } })
}

function decide(index: number, approved: boolean) {
  demoStore.approveReview(index)
  uiStore.toast(approved ? '审核已通过 · 已通知发起人' : '已驳回 · 发起人将收到通知与驳回意见', approved ? 'ok' : 'warn')
}
</script>

<template>
  <div class="page review-page">
    <div class="section-head"><h3>图纸审核</h3><span class="lib-count">无序审核 · 节点可并行处理</span></div>
    <template v-if="demoStore.myReviews.length">
      <div v-for="(review, index) in demoStore.myReviews" :key="review.no" class="card review-row">
        <div class="feed-ic review-icon"><DemoIcon name="clipboard-check" :size="20" /></div>
        <div class="review-main"><div class="review-title"><b>{{ review.name }}</b><span class="dh-no review-no">{{ review.no }}</span></div><div class="review-meta">待审节点 <b>{{ review.node }}</b> · {{ review.by }} 发起于 {{ review.time }} · 无序流程</div></div>
        <button class="btn sm" type="button" @click="openDrawing(review.no)"><DemoIcon name="eye" :size="14" />查看图纸</button><button class="btn sm primary" type="button" @click="decide(index, true)"><DemoIcon name="check" :size="14" />同意</button><button class="btn sm danger" type="button" @click="decide(index, false)"><DemoIcon name="x" :size="14" />驳回</button>
      </div>
    </template>
    <div v-else class="card"><div class="empty"><DemoIcon name="check-circle-2" :size="34" /><div class="t">太棒了，暂无待办审核</div></div></div>
    <div class="section-head section-head-gap"><h3 class="small-heading">近期已办</h3></div>
    <div class="card"><div class="empty"><DemoIcon name="clipboard-check" :size="34" /><div class="t">暂无已办审核记录</div></div></div>
  </div>
</template>

<style scoped>
.review-page { min-width: 0; }
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 13px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.section-head-gap { margin-top: 22px; }
.small-heading { font-size: 14px !important; }
.review-row { display: flex; align-items: center; gap: 16px; margin-bottom: 12px; padding: 18px 20px; }
.review-icon { width: 42px; height: 42px; border-radius: 12px; }
.review-icon svg { color: var(--warn); }
.review-main { flex: 1; min-width: 0; }
.review-title { display: flex; align-items: center; gap: 9px; }
.review-title b { font-size: 14px; }
.review-no { padding: 3px 8px; font-size: 10.5px; }
.review-meta { margin-top: 5px; color: var(--text-3); font-size: 11.5px; }
.review-meta b { color: var(--warn); }
@media (max-width: 760px) { .review-row { align-items: flex-start; flex-wrap: wrap; } .review-main { min-width: calc(100% - 58px); } }
</style>
