<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import {
  activeReviewNode,
  canSignReviewNode,
  reviewNodeStatusLabel,
  toWorkspaceNodes,
} from '@/features/reviews/review-workspace'
import { RouteName } from '@/router/route-names'
import { drawingCommandService } from '@/app/container'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useReviewStore } from '@/stores/review.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'ReviewWorkspacePanel' })

const props = withDefaults(defineProps<{
  drawingNo: string
  embedded?: boolean
  showOverview?: boolean
}>(), {
  embedded: false,
  showOverview: false,
})

const emit = defineEmits<{
  (e: 'toggle-overview'): void
}>()

const router = useRouter()
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const authStore = useAuthStore()
const uiStore = useUiStore()

const opinionText = ref('')
const submitting = ref(false)

const drawingNo = computed(() => props.drawingNo.trim())
const drawing = computed(() => drawingStore.getDrawing(drawingNo.value) ?? drawingStore.getPart(drawingNo.value))
const reviewCase = computed(() => reviewStore.getCase(drawingNo.value))
const reviewing = computed(() => reviewCase.value ? reviewCase.value.status === 'reviewing' : drawing.value?.status === 'reviewing')
const nodes = computed(() => toWorkspaceNodes(reviewCase.value))
const currentNode = computed(() => activeReviewNode(nodes.value, reviewing.value))
const canSign = computed(() => canSignReviewNode(currentNode.value, currentNode.value?.name, authStore.currentUser, reviewing.value))
const doneCount = computed(() => nodes.value.filter((node) => node.status === 'pass').length)
const percent = computed(() => (nodes.value.length ? Math.round((doneCount.value / nodes.value.length) * 100) : 0))
const isPart = computed(() => Boolean(drawing.value && 'parentNo' in drawing.value))
const isAdmin = computed(() => authStore.currentUser?.roles?.includes('admin') ?? false)
const isCreator = computed(() => {
  const item = drawing.value
  const current = authStore.currentUser
  if (!item || !current) return false
  return ('createdBy' in item && item.createdBy) === current.displayName
})
const canArchive = computed(() => !isPart.value && drawing.value?.status === 'published' && (isCreator.value || isAdmin.value))
const canStartReview = computed(() => {
  const item = drawing.value
  const current = authStore.currentUser
  if (!item || !current) return false
  if (current.roles?.includes('admin')) return true
  return ('createdBy' in item && item.createdBy) === current.displayName
})
const missingCase = computed(() => reviewing.value && !reviewCase.value)
const isRejected = computed(() => reviewCase.value?.status === 'rejected')
const currentUserId = computed(() => authStore.currentUser?.id)

watch(currentNode, (node) => {
  opinionText.value = node?.status === 'pending' ? '' : (node?.opinion || '')
}, { immediate: true })

onMounted(() => {
  void Promise.all([drawingStore.load(), reviewStore.load()]).catch(() => undefined)
})

function openDrawingFiles() {
  if (!drawingNo.value) return
  void router.push({ name: RouteName.DrawingPreview, params: { drawingId: drawingNo.value } })
}

function nodeStatus(node: (typeof nodes.value)[number]) {
  return reviewNodeStatusLabel(node, currentNode.value?.name ?? null, reviewing.value, currentUserId.value, isRejected.value)
}

async function handleStartReview() {
  if (!drawingNo.value) return
  try {
    await reviewStore.startCase(drawingNo.value)
    uiStore.toast(`图纸「${drawingNo.value}」已发起审核，请按顺序完成各节点签署`, 'ok')
  } catch (error) {
    console.error('发起审核失败', error)
    uiStore.toast(error instanceof Error ? error.message : '发起审核失败', 'warn')
  }
}

function archiveDrawing() {
  const item = drawing.value
  if (!item || isPart.value) return
  uiStore.confirm('存档图纸', `确定将「${item.no}」存档吗？\n存档后图纸进入只读保护，如需修改请发起变更工单并经管理员审批。`, {
    confirmText: '存档',
    onConfirm: async () => {
      try {
        await drawingCommandService.archive(item.no)
        drawingStore.invalidate()
        await drawingStore.load()
        uiStore.toast('图纸已存档', 'ok')
      } catch (error) {
        uiStore.toast(error instanceof Error ? error.message : '图纸状态更新失败', 'warn')
      }
    },
  })
}

async function handleDecision(action: 'pass' | 'rejected') {
  const node = currentNode.value
  const review = reviewCase.value
  if (!node || !review) return
  if (!opinionText.value.trim() && action === 'rejected') {
    uiStore.toast('驳回审核必须填写审核意见与整改要求', 'warn')
    return
  }
  submitting.value = true
  try {
    const updatedCase = await reviewStore.submitNode(review.id, node.name, action, opinionText.value.trim())
    if (updatedCase.status !== 'reviewing') {
      drawingStore.invalidate()
      await drawingStore.load()
    }
    uiStore.toast(
      action === 'pass' ? `节点「${node.name}」已审核通过` : `节点「${node.name}」已驳回，发起人将收到整改通知`,
      action === 'pass' ? 'ok' : 'warn',
    )
    const next = reviewStore.getCase(drawingNo.value)
    const nextMine = next && next.status === 'reviewing'
      ? activeReviewNode(toWorkspaceNodes(next), true)
      : null
    if (nextMine && canSignReviewNode(nextMine, nextMine.name, authStore.currentUser, true)) {
      opinionText.value = ''
      return
    }
    if (!props.embedded) void router.push({ name: RouteName.ReviewPending })
  } catch (error) {
    console.error('提交审核意见失败', error)
    uiStore.toast(error instanceof Error ? error.message : '提交审核意见失败', 'warn')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="review-workspace" :class="{ embedded }">
    <div v-if="!embedded" class="section-head">
      <button class="btn sm" type="button" @click="router.push({ name: RouteName.ReviewPending })">
        <DemoIcon name="arrow-left" :size="14" />返回待办
      </button>
      <h3>审核工作台</h3>
      <span class="lib-count">只处理当前轮到你的节点，意见与结论在此提交</span>
    </div>

    <div v-if="!drawingNo" class="card empty">
      <DemoIcon name="clipboard-check" :size="36" />
      <div class="t">未指定图纸</div>
    </div>

    <template v-else>
      <div class="workspace-hero card">
        <div class="hero-main">
          <div class="hero-kicker">当前审核对象</div>
          <h2>{{ drawing?.name || reviewCase?.drawingName || drawingNo }}</h2>
          <p>
            图号 {{ drawingNo }}
            <template v-if="reviewCase"> · 发起人 {{ reviewCase.initiator }} · {{ reviewCase.startedAt }}</template>
          </p>
        </div>
        <div class="hero-actions">
          <button class="btn" type="button" @click="openDrawingFiles"><DemoIcon name="eye" :size="14" />查阅图纸</button>
          <button v-if="embedded" class="btn" type="button" @click="emit('toggle-overview')">
            <DemoIcon name="workflow" :size="14" />{{ showOverview ? '返回签署工作台' : '查看流程总览' }}
          </button>
        </div>
      </div>

      <div class="workspace-layout">
        <section class="card current-panel">
          <template v-if="canSign && currentNode">
            <div class="current-badge">当前节点 · 待我签署</div>
            <h3>{{ currentNode.name }}</h3>
            <p class="current-meta">责任人 {{ currentNode.assignedName || authStore.currentUser?.displayName }} · 第 {{ currentNode.order }} 步</p>
            <label class="opinion-label" for="review-opinion">本节点审核意见</label>
            <textarea
              id="review-opinion"
              v-model="opinionText"
              class="inp opinion-input"
              rows="6"
              placeholder="请填写本节点意见。通过可写简要结论；驳回必须写明问题和整改要求。"
            ></textarea>
            <div class="current-actions">
              <button class="btn danger" type="button" :disabled="submitting" @click="handleDecision('rejected')">
                <DemoIcon name="x" :size="14" />驳回
              </button>
              <button class="btn primary" type="button" :disabled="submitting" @click="handleDecision('pass')">
                <DemoIcon name="check" :size="14" />同意通过
              </button>
            </div>
          </template>

          <template v-else-if="reviewing && currentNode">
            <div class="current-badge wait">还未到你</div>
            <h3>{{ currentNode.name }}</h3>
            <p class="current-meta">当前轮到「{{ currentNode.assignedName || '待定' }}」签署，你还不能填写意见。</p>
            <button class="btn" type="button" @click="openDrawingFiles">先查阅图纸文件</button>
          </template>

          <template v-else-if="isRejected || missingCase">
            <div class="current-badge danger">{{ missingCase ? '流程未初始化' : '流程已驳回' }}</div>
            <h3>{{ missingCase ? '需要重新初始化审核' : '等待发起人重新发起' }}</h3>
            <p class="current-meta">{{ missingCase ? '图纸处于审核中，但没有可签署的案例。' : '审核员不能重新发起。已通过节点会保留，发起后从驳回节点继续。' }}</p>
            <button v-if="canStartReview" class="btn primary" type="button" @click="handleStartReview">
              {{ missingCase ? '重新初始化审核流程' : '重新发起审核' }}
            </button>
          </template>

          <template v-else-if="reviewCase?.status === 'published' || drawing?.status === 'published'">
            <div class="current-badge ok">审核已完成</div>
            <h3>全部节点已通过</h3>
            <p class="current-meta">可在此直接存档，或到已办审核中查看签署记录。</p>
            <button v-if="canArchive" class="btn primary" type="button" @click="archiveDrawing">
              <DemoIcon name="shield-check" :size="14" />存档图纸
            </button>
          </template>

          <template v-else>
            <div class="current-badge mute">尚未进入审核</div>
            <h3>当前没有可签署节点</h3>
            <p class="current-meta">图纸仍是草稿或尚未发起审核。请由图纸创建者发起流程后再回来处理。</p>
            <button v-if="canStartReview" class="btn primary" type="button" @click="handleStartReview">开始发起审核流程</button>
          </template>
        </section>

        <aside class="card flow-panel">
          <div class="flow-head">
            <b>节点顺序</b>
            <span>{{ doneCount }} / {{ nodes.length || 0 }} · {{ percent }}%</span>
          </div>
          <ol v-if="nodes.length" class="flow-list">
            <li
              v-for="node in nodes"
              :key="node.name"
              :class="[node.status, { current: node.name === currentNode?.name, waiting: node.status === 'pending' && node.name !== currentNode?.name }]"
            >
              <span class="dot">
                <DemoIcon :name="node.status === 'pass' ? 'check' : node.status === 'rejected' ? 'x' : node.name === currentNode?.name ? 'pencil' : 'clock'" :size="13" />
              </span>
              <div>
                <strong>{{ node.name }}</strong>
                <small>{{ node.assignedName || '待定' }} · {{ nodeStatus(node) }}</small>
                <p v-if="node.opinion">{{ node.opinion }}</p>
              </div>
            </li>
          </ol>
          <div v-else class="empty-flow">暂无审核节点。发起审核后将按流程顺序显示。</div>
        </aside>
      </div>
    </template>
  </div>
</template>

<style scoped>
.review-workspace { min-width: 0; }
.review-workspace.embedded { padding-top: 0; }
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 16px; }
.section-head h3 { font-family: var(--font-display); font-size: 16px; font-weight: 800; }
.lib-count { color: var(--text-3); font-size: 11.5px; }
.workspace-hero { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 22px; }
.hero-kicker { color: var(--accent); font-size: 11px; font-weight: 700; letter-spacing: 0.04em; }
.hero-main h2 { margin: 4px 0 6px; font-size: 18px; }
.hero-main p { margin: 0; color: var(--text-3); font-size: 12px; }
.hero-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.workspace-layout { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(280px, 0.8fr); gap: 16px; align-items: start; }
.current-panel { padding: 22px; }
.current-badge { display: inline-flex; margin-bottom: 10px; padding: 4px 10px; border-radius: 999px; background: var(--accent-soft); color: var(--accent); font-size: 12px; font-weight: 700; }
.current-badge.wait { background: rgb(251 191 36 / 14%); color: var(--warn); }
.current-badge.danger { background: rgb(248 113 113 / 14%); color: var(--danger); }
.current-badge.ok { background: rgb(52 211 153 / 14%); color: var(--ok); }
.current-badge.mute { background: var(--panel-2); color: var(--text-3); }
.current-panel h3 { margin: 0 0 6px; font-size: 22px; }
.current-meta { margin: 0 0 16px; color: var(--text-3); font-size: 13px; }
.opinion-label { display: block; margin-bottom: 8px; font-size: 13px; font-weight: 600; }
.opinion-input { width: 100%; min-height: 140px; resize: vertical; }
.current-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 14px; }
.flow-panel { padding: 16px 18px; }
.flow-head { display: flex; justify-content: space-between; margin-bottom: 12px; font-size: 12px; color: var(--text-3); }
.flow-list { display: flex; flex-direction: column; gap: 10px; margin: 0; padding: 0; list-style: none; }
.flow-list li { display: flex; gap: 10px; padding: 10px; border-radius: 10px; background: var(--panel-2); }
.flow-list li.current { background: var(--accent-soft); box-shadow: inset 0 0 0 1px var(--accent); }
.flow-list li.waiting { opacity: 0.72; }
.dot { display: grid; width: 26px; height: 26px; flex: none; place-items: center; border-radius: 50%; background: var(--panel); color: var(--text-3); }
.flow-list li.pass .dot { color: var(--ok); }
.flow-list li.rejected .dot { color: var(--danger); }
.flow-list li.current .dot { color: var(--accent); }
.flow-list strong { display: block; font-size: 13px; }
.flow-list small { color: var(--text-3); font-size: 11px; }
.flow-list p { margin: 4px 0 0; color: var(--text-2); font-size: 12px; }
.empty-flow { color: var(--text-3); font-size: 12px; }
@media (max-width: 900px) {
  .workspace-hero, .workspace-layout { grid-template-columns: 1fr; }
  .workspace-hero { flex-direction: column; align-items: flex-start; }
}
</style>
