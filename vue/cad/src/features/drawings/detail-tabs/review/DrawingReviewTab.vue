<script setup lang="ts">
import { computed } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingReviewTab' })

const demoStore = useDemoStore()
const uiStore = useUiStore()
const done = computed(() => demoStore.reviewNodes.filter((node) => node.status === 'pass').length)
const percent = computed(() => demoStore.reviewNodes.length ? Math.round((done.value / demoStore.reviewNodes.length) * 100) : 0)

async function passNode(name: string) {
  try {
    await demoStore.setReviewNodeStatus(name, 'pass', '同意。')
    uiStore.toast(done.value === demoStore.reviewNodes.length ? '全部节点通过 · 版本已自动转为「已发布」' : `「${name}」已同意`)
  } catch (error) {
    console.error('保存审核节点失败', error)
    uiStore.toast('审核节点保存失败，请稍后重试', 'warn')
  }
}

async function rejectNode(name: string) {
  try {
    await demoStore.setReviewNodeStatus(name, 'pending', '请补充技术要求后重新提交。')
    uiStore.toast(`「${name}」已驳回 · 原版本与审核记录全部保留`, 'warn')
  } catch (error) {
    console.error('保存审核节点失败', error)
    uiStore.toast('审核节点保存失败，请稍后重试', 'warn')
  }
}
</script>

<template>
  <div v-if="demoStore.reviewNodes.length" class="review-progress card">
    <DemoIcon name="stamp" :size="17" /><b>审核流程</b><div class="rp-track"><div class="rp-fill" :style="{ width: `${percent}%` }"></div></div><span class="rp-txt">{{ done }} / {{ demoStore.reviewNodes.length }} · {{ percent }}%</span><span class="tag info">无序 · 可并行</span>
  </div>
  <div v-if="demoStore.reviewNodes.length" class="review-nodes">
    <div v-for="node in demoStore.reviewNodes" :key="node.name" class="card rn-card" :class="node.status === 'pass' ? 'pass' : 'pending'">
      <div class="rn-top"><div class="rn-ring"><DemoIcon :name="node.status === 'pass' ? 'check' : 'clock'" :size="15" /></div><div><div class="rn-name">{{ node.name }}</div><div class="rn-user">审核人 · {{ node.user }}</div></div><span class="tag" :class="node.status === 'pass' ? 'ok' : 'warn'">{{ node.status === 'pass' ? '已同意' : '待审核' }}</span></div>
      <div class="rn-opinion">{{ node.opinion || '等待审核意见…' }}</div><div class="rn-time">{{ node.time }}</div>
      <div v-if="node.status !== 'pass'" class="rn-acts"><button class="btn sm primary" type="button" @click="passNode(node.name)"><DemoIcon name="check" :size="14" />同意</button><button class="btn sm danger" type="button" @click="rejectNode(node.name)"><DemoIcon name="x" :size="14" />驳回</button></div>
    </div>
  </div>
  <div v-else class="card empty">
    <DemoIcon name="stamp" :size="34" />
    <div class="t">暂无审核流程数据</div>
  </div>
  <div class="note"><DemoIcon name="info" :size="14" /><div><b>无序审核：</b>各节点可同时进行、不分先后；全部必需节点通过后版本自动转为「已发布」。驳回不删除任何记录——原版本与审核意见全部保留，修改生成新版本后可重新发起审核。</div></div>
</template>

<style scoped>
.review-progress { display: flex; align-items: center; gap: 14px; margin-bottom: 16px; padding: 15px 20px; }
.review-progress > svg { color: var(--accent); }
.rp-track { flex: 1; height: 8px; overflow: hidden; border-radius: 99px; background: var(--panel-2); }
.rp-fill { height: 100%; border-radius: 99px; background: linear-gradient(90deg, var(--accent), var(--accent-2)); transition: width 0.6s cubic-bezier(0.2, 0.8, 0.3, 1); }
html[data-skin='tech'] .rp-fill { box-shadow: 0 0 12px var(--glow); }
.rp-txt { color: var(--text-2); font-family: 'JetBrains Mono', monospace; font-size: 12px; white-space: nowrap; }
.review-nodes { display: grid; grid-template-columns: repeat(4, 1fr); gap: 13px; margin-bottom: 16px; }
.rn-card { display: flex; flex-direction: column; gap: 9px; padding: 16px; }
.rn-top { display: flex; align-items: center; gap: 10px; }
.rn-ring { display: grid; width: 34px; height: 34px; flex: none; place-items: center; border: 2px solid var(--line-strong); border-radius: 50%; color: var(--text-3); }
.rn-card.pass .rn-ring { border-color: var(--ok); background: rgb(52 211 153 / 10%); color: var(--ok); }
.rn-card.pending .rn-ring { border-color: var(--warn); color: var(--warn); }
html[data-skin='tech'] .rn-card.pending .rn-ring { animation: ring-pulse 1.6s infinite; }
@keyframes ring-pulse { 0%, 100% { box-shadow: 0 0 0 0 rgb(251 191 36 / 40%); } 50% { box-shadow: 0 0 0 6px transparent; } }
.rn-name { font-size: 13.5px; font-weight: 700; }
.rn-user { color: var(--text-3); font-size: 11px; }
.rn-top > .tag { margin-left: auto; }
.rn-opinion { min-height: 38px; padding: 8px 10px; border-radius: 8px; background: var(--panel-2); color: var(--text-2); font-size: 11.5px; line-height: 1.6; }
.rn-time { color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 10.5px; }
.rn-acts { display: flex; gap: 7px; margin-top: auto; }
@media (max-width: 1180px) { .review-nodes { grid-template-columns: repeat(2, 1fr); } }
@media (max-width: 760px) { .review-progress { align-items: flex-start; flex-wrap: wrap; } .rp-track { width: 100%; flex-basis: 100%; } .review-nodes { grid-template-columns: 1fr; } }
</style>
