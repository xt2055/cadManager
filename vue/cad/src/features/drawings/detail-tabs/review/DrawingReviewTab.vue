<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { ReviewNode } from '@/types/domain.types'

defineOptions({ name: 'DrawingReviewTab' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

const currentItem = computed(() => domainStore.currentDrawing)

const done = computed(() => domainStore.currentReviewNodes.filter((node) => node.status === 'pass').length)
const total = computed(() => domainStore.currentReviewNodes.length)
const percent = computed(() => (total.value ? Math.round((done.value / total.value) * 100) : 0))
const isReviewing = computed(() => currentItem.value?.status === 'reviewing')
const isPublished = computed(() => currentItem.value?.status === 'published')

// 顺序流转：节点按 order 排序，第一个待处理节点为当前活动节点，仅它可签署。
const sortedNodes = computed(() => domainStore.currentReviewNodes
  .slice()
  .sort((a, b) => (a.order ?? 0) - (b.order ?? 0)))
const activeNodeName = computed(() => (isReviewing.value
  ? sortedNodes.value.find((node) => node.status === 'pending')?.name ?? null
  : null))

// 意见表单当前展开的节点
const editingNodeName = ref<string | null>(null)
const opinionText = ref('')

function openOpinionForm(node: ReviewNode) {
  editingNodeName.value = node.name
  opinionText.value = node.opinion || ''
}

function cancelOpinion() {
  editingNodeName.value = null
  opinionText.value = ''
}

async function handleStartReview() {
  if (!currentItem.value) return
  try {
    await domainStore.startReview(currentItem.value.no)
    uiStore.toast(`图纸「${currentItem.value.no}」已发起审核，请按顺序完成各节点签署`, 'ok')
  } catch (error) {
    console.error('发起审核失败', error)
    uiStore.toast(error instanceof Error ? error.message : '发起审核失败', 'warn')
  }
}

async function handleDecision(nodeName: string, action: 'pass' | 'rejected') {
  if (!currentItem.value) return
  if (!opinionText.value.trim() && action === 'rejected') {
    uiStore.toast('驳回审核必须填写审核意见与整改要求', 'warn')
    return
  }

  try {
    await domainStore.submitNodeReview(
      currentItem.value.no,
      nodeName,
      action,
      opinionText.value.trim(),
      '当前审核人',
      domainStore.currentReviewCase?.id,
    )
    uiStore.toast(
      action === 'pass'
        ? `节点「${nodeName}」已审核通过`
        : `节点「${nodeName}」已驳回，发起人将收到整改通知`,
      action === 'pass' ? 'ok' : 'warn',
    )
    editingNodeName.value = null
    opinionText.value = ''
  } catch (error) {
    console.error('提交审核意见失败', error)
    uiStore.toast(error instanceof Error ? error.message : '提交审核意见失败', 'warn')
  }
}
</script>

<template>
  <div class="review-tab-view">
    <!-- 顶部审核总体状态卡片 -->
    <div class="card card-pad review-summary-card">
      <div class="summary-left">
        <div class="summary-badge-wrap">
          <DemoIcon name="stamp" :size="24" />
        </div>
        <div class="summary-texts">
          <div class="summary-title-row">
            <h3>图纸工程审核流转中心</h3>
            <span v-if="isReviewing" class="tag info">审核中 · 顺序流转</span>
            <span v-else-if="isPublished" class="tag ok">已全部通过 · 已发布</span>
            <span v-else class="tag mute">草稿状态 · 待发起审核</span>
          </div>
          <p>节点按设计 → 校对 → 审核 → 工艺 → 批准顺序签署；前序节点通过后，下一节点才会开放审核。</p>
        </div>
      </div>

      <div class="summary-actions">
        <button
          v-if="!isReviewing && !isPublished"
          class="btn primary lg"
          type="button"
          @click="handleStartReview"
        >
          <DemoIcon name="play-circle" :size="16" />开始发起审核流程
        </button>
        <button
          v-else-if="isReviewing"
          class="btn lg"
          type="button"
          @click="uiStore.toast('审核正在流转中，请等待各专业节点签署完成', 'info')"
        >
          <DemoIcon name="clock" :size="16" />审核流转中 ({{ done }}/{{ total }})
        </button>
        <button
          v-else
          class="btn lg"
          type="button"
          @click="handleStartReview"
        >
          <DemoIcon name="refresh-cw" :size="16" />重新发起新版审核
        </button>
      </div>
    </div>

    <!-- 进度条 -->
    <div v-if="domainStore.currentReviewNodes.length" class="review-progress card">
      <DemoIcon name="workflow" :size="17" />
      <b>流转进度看板</b>
      <div class="rp-track">
        <div class="rp-fill" :style="{ width: `${percent}%` }"></div>
      </div>
      <span class="rp-txt">{{ done }} / {{ domainStore.currentReviewNodes.length }} 已完成 · {{ percent }}%</span>
    </div>

    <!-- 可视化流程拓扑图与节点列表 -->
    <div v-if="domainStore.currentReviewNodes.length" class="review-diagram-area">
      <div class="section-subhead">
        <DemoIcon name="git-commit" :size="15" />
        <h4>审核节点流程（按顺序签署）</h4>
      </div>

      <div class="review-nodes-grid">
        <div
          v-for="node in sortedNodes"
          :key="node.name"
          class="card rn-card"
          :class="[node.status, { 'is-active': node.name === activeNodeName }]"
        >
          <div class="rn-top">
            <div class="rn-ring">
              <DemoIcon :name="node.status === 'pass' ? 'check' : node.status === 'rejected' ? 'x' : node.name === activeNodeName ? 'pencil' : 'clock'" :size="16" />
            </div>
            <div class="rn-info">
              <div class="rn-name">{{ node.name }}</div>
              <div class="rn-user">责任人 · {{ node.user }}</div>
            </div>
            <span v-if="node.name === activeNodeName" class="tag warn">当前节点</span>
            <span v-else class="tag" :class="node.status === 'pass' ? 'ok' : node.status === 'rejected' ? 'danger' : 'mute'">
              {{ node.status === 'pass' ? '已同意' : node.status === 'rejected' ? '已驳回' : '等待前置节点' }}
            </span>
          </div>

          <!-- 历史或已签署意见 -->
          <div class="rn-opinion-box">
            <span class="op-lbl">审核意见：</span>
            <div class="op-txt">{{ node.opinion || '暂无签署意见…' }}</div>
          </div>

          <div class="rn-time">{{ node.time }}</div>

          <!-- 审核意见填写抽屉/展开框（仅当前活动节点可展开） -->
          <div v-if="editingNodeName === node.name" class="opinion-editor-box">
            <label>请填写针对本图纸的评审意见：</label>
            <textarea
              v-model="opinionText"
              class="inp opinion-input"
              placeholder="如：尺寸公差标注完整，加工工艺合理，同意投产。"
              rows="3"
            ></textarea>
            <div class="editor-actions">
              <button class="btn sm" type="button" @click="cancelOpinion">取消</button>
              <button class="btn sm danger" type="button" @click="handleDecision(node.name, 'rejected')">
                <DemoIcon name="x" :size="13" />驳回
              </button>
              <button class="btn sm primary" type="button" @click="handleDecision(node.name, 'pass')">
                <DemoIcon name="check" :size="13" />同意通过
              </button>
            </div>
          </div>

          <!-- 操作按钮栏：仅当前活动节点可审核 -->
          <div v-else-if="node.name === activeNodeName" class="rn-acts">
            <button class="btn sm primary" type="button" @click="openOpinionForm(node)">
              <DemoIcon name="pencil" :size="13" />填写意见并审核
            </button>
          </div>

          <!-- 非当前节点：等待提示 -->
          <div v-else-if="node.status === 'pending' && isReviewing" class="rn-waiting">
            <DemoIcon name="clock" :size="12" />
            <span>等待前置节点通过后开放审核</span>
          </div>
        </div>
      </div>
    </div>

    <div v-else class="card empty">
      <DemoIcon name="stamp" :size="38" />
      <div class="t">尚未初始化审核流程</div>
      <p>点击上方「开始发起审核流程」装配企业标准化顺序审核节点</p>
    </div>
  </div>
</template>

<style scoped>
.review-tab-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.review-summary-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.summary-left {
  display: flex;
  align-items: center;
  gap: 14px;
}
.summary-badge-wrap {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  flex: none;
  border-radius: 12px;
  background: var(--accent-soft);
  color: var(--accent);
}
.summary-texts {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.summary-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.summary-title-row h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}
.summary-texts p {
  margin: 0;
  color: var(--text-3);
  font-size: 12px;
}
.summary-actions {
  display: flex;
  gap: 10px;
}
.btn.lg {
  padding: 9px 18px;
  font-size: 13px;
}
.review-progress {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 20px;
}
.review-progress > svg {
  color: var(--accent);
}
.rp-track {
  flex: 1;
  height: 8px;
  overflow: hidden;
  border-radius: 99px;
  background: var(--panel-2);
}
.rp-fill {
  height: 100%;
  border-radius: 99px;
  background: linear-gradient(90deg, var(--accent), var(--accent-2));
  transition: width 0.6s cubic-bezier(0.2, 0.8, 0.3, 1);
}
.rp-txt {
  color: var(--text-2);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  white-space: nowrap;
}
.section-subhead {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  color: var(--text-2);
}
.section-subhead h4 {
  margin: 0;
  font-size: 13.5px;
  font-weight: 600;
}
.review-nodes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}
.rn-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px 18px;
  border-radius: var(--radius);
}
.rn-top {
  display: flex;
  align-items: center;
  gap: 10px;
}
.rn-ring {
  display: grid;
  width: 36px;
  height: 36px;
  flex: none;
  place-items: center;
  border: 2px solid var(--line-strong);
  border-radius: 50%;
  color: var(--text-3);
}
.rn-card.pass .rn-ring {
  border-color: var(--ok);
  background: rgb(52 211 153 / 12%);
  color: var(--ok);
}
.rn-card.rejected .rn-ring {
  border-color: var(--danger);
  background: rgb(248 113 113 / 12%);
  color: var(--danger);
}
.rn-card.pending .rn-ring {
  border-color: var(--warn);
  color: var(--warn);
}
.rn-card.is-active {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}
.rn-card.is-active .rn-ring {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}
.rn-waiting {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  margin-top: auto;
  padding: 7px 10px;
  border: 1px dashed var(--line);
  border-radius: 7px;
  color: var(--text-3);
  font-size: 11px;
}
.rn-info {
  flex: 1;
  min-width: 0;
}
.rn-name {
  font-size: 13.5px;
  font-weight: 700;
}
.rn-user {
  color: var(--text-3);
  font-size: 11px;
}
.rn-opinion-box {
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--panel-2);
  font-size: 11.5px;
}
.op-lbl {
  color: var(--text-3);
}
.op-txt {
  margin-top: 2px;
  color: var(--text-1);
  line-height: 1.5;
}
.rn-time {
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
}
.opinion-editor-box {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px;
  border-radius: 8px;
  border: 1px solid var(--accent);
  background: var(--panel);
}
.opinion-editor-box label {
  font-size: 11px;
  color: var(--text-2);
}
.opinion-input {
  font-size: 12px;
}
.editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  margin-top: 4px;
}
.rn-acts {
  display: flex;
  margin-top: auto;
}
.rn-acts button {
  width: 100%;
  justify-content: center;
}
@media (max-width: 760px) {
  .review-summary-card {
    flex-direction: column;
    align-items: flex-start;
  }
  .review-nodes-grid {
    grid-template-columns: 1fr;
  }
}
</style>
