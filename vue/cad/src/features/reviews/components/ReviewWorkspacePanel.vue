<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import ChangeReviewEvidence from './ChangeReviewEvidence.vue'
import { opinionDraftKey, readOpinionDraft, writeOpinionDraft } from '../review-opinion-draft'
import {
  activeReviewNode,
  canSignReviewNode,
  canStartRegularReview,
  reviewNodeStatusLabel,
  toWorkspaceNodes,
} from '@/features/reviews/review-workspace'
import { RouteName } from '@/router/route-names'
import { drawingCommandService } from '@/app/container'
import { changeRequestService, type ChangeRequest } from '@/services/change-request.service'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { isDrawingDecider } from '@/modules/drawing/drawing-authority'
import { useReviewStore } from '@/stores/review.store'
import { useUiStore } from '@/stores/ui.store'
import { reviewAnnotationService, type ReviewAnnotationFile } from '@/services/review-annotation.service'

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
const changeEvidenceReady = ref(false)
const annotationFiles = ref<ReviewAnnotationFile[]>([])
const annotationFilesOpen = ref(false)
const annotationFilesLoading = ref(false)
const annotationFilesError = ref('')

async function openAnnotations() {
  const review = reviewCase.value
  if (!review || annotationFilesLoading.value) return
  annotationFilesOpen.value = true; annotationFilesLoading.value = true; annotationFilesError.value = ''
  try {
    const files = await reviewAnnotationService.files(review.id)
    if (reviewCase.value?.id !== review.id) return
    annotationFiles.value = files
    if (files.length === 1) browseAnnotation(files[0]!)
  } catch (e) { annotationFilesError.value = e instanceof Error ? e.message : '读取审核图纸失败' }
  finally { annotationFilesLoading.value = false }
}
function browseAnnotation(file: ReviewAnnotationFile) {
  void router.push({ name: RouteName.DrawingViewer, params: { drawingId: effectiveNo.value }, query: { fileId: file.attachmentId, versionId: file.versionId, reviewCaseId: reviewCase.value?.id, from: 'review' } })
}

const drawingNo = computed(() => props.drawingNo.trim())
const drawing = computed(() => drawingStore.getDrawing(drawingNo.value) ?? drawingStore.getPart(drawingNo.value))
// 进入工作台的图号可能是资源 ID（UUID）；审核案例始终以可读图号关联，命中后用真实图号查询。
const effectiveNo = computed(() => drawing.value?.no || drawingNo.value)
const reviewCase = computed(() => reviewStore.getCase(effectiveNo.value))
const reviewing = computed(() => reviewCase.value ? reviewCase.value.status === 'reviewing' : drawing.value?.status === 'reviewing')
const nodes = computed(() => toWorkspaceNodes(reviewCase.value))
const currentNode = computed(() => activeReviewNode(nodes.value, reviewing.value))
const canSign = computed(() => (!reviewCase.value?.changeSubmissionId || changeEvidenceReady.value) && canSignReviewNode(currentNode.value, currentNode.value?.name, authStore.currentUser, reviewing.value))
const doneCount = computed(() => nodes.value.filter((node) => node.status === 'pass').length)
const percent = computed(() => (nodes.value.length ? Math.round((doneCount.value / nodes.value.length) * 100) : 0))
const isPart = computed(() => Boolean(drawing.value && 'parentNo' in drawing.value))
// 存档是创建人级决定：有负责人时归负责人，无负责人时回落创建人，管理员始终可以。
const isDecider = computed(() => isDrawingDecider(drawing.value, authStore.currentUser))
const canArchive = computed(() => !isPart.value && drawing.value?.status === 'published' && isDecider.value)
const canStartReview = computed(() => {
  const item = drawing.value
  const current = authStore.currentUser
  return canStartRegularReview(item, current,
    changeRequestLoading.value || changeRequestError.value || Boolean(changeRequest.value))
})
const missingCase = computed(() => reviewing.value && !reviewCase.value)
const isRejected = computed(() => reviewCase.value?.status === 'rejected')
const currentUserId = computed(() => authStore.currentUser?.id)

const changeRequest = ref<ChangeRequest | null>(null)
const changeRequestLoading = ref(false)
const changeRequestError = ref(false)
let changeRequestSequence = 0

async function loadChangeRequest() {
  const sequence = ++changeRequestSequence
  changeRequest.value = null
  changeRequestError.value = false
  const item = drawing.value
  changeRequestLoading.value = Boolean(item)
  if (!item) return
  try {
    const userId = authStore.currentUser?.id
    const list = await changeRequestService.listByDrawing(String(item.id))
    const open = list.filter((request) => ['pending_approval', 'executing', 'pending_verify'].includes(request.status))
    const request = open.find((request) => userId && request.executorId === userId) ?? open[0]
    const detail = request ? await changeRequestService.get(request.id) : null
    if (sequence !== changeRequestSequence) return
    changeRequest.value = detail
  } catch {
    if (sequence === changeRequestSequence) changeRequestError.value = true
  } finally {
    if (sequence === changeRequestSequence) changeRequestLoading.value = false
  }
}

watch(
  [() => drawing.value?.id, () => authStore.currentUser?.id],
  () => { void loadChangeRequest() },
  { immediate: true },
)

// 变更申请通过、设计员改完图纸后，由执行人在审核工作台重新发起完整审核。
const canRestartChangeReview = computed(() => {
  const request = changeRequest.value
  const current = authStore.currentUser
  if (!request || !current || request.status !== 'executing') return false
  return request.executorId === current.id
})

async function handleRestartChangeReview() {
  const request = changeRequest.value
  if (!request || !canRestartChangeReview.value || submitting.value) return
  const last = request.submissions?.[request.submissions.length - 1]
  submitting.value = true
  try {
    await changeRequestService.submit(
      request.id,
      last?.actualChanges?.trim() || '完成指定变更并提交完整审核',
      last?.proposedAttributes ?? {},
    )
    drawingStore.invalidate()
    await Promise.all([reviewStore.load(), drawingStore.load(), loadChangeRequest()])
    uiStore.toast('已重新发起审核，请按节点顺序签署', 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '重新发起审核失败', 'warn')
  } finally {
    submitting.value = false
  }
}

const draftKey = computed(() => {
  const review = reviewCase.value
  const node = currentNode.value
  const user = authStore.currentUser
  if (!review || !node || !user || !canSignReviewNode(node, node.name, user, reviewing.value)) return ''
  return opinionDraftKey({ userId: user.id, caseId: review.id, startedAt: review.startedAt,
    submissionId: review.changeSubmissionId, node: node.name, order: node.order })
})
let activeDraftKey = ''
let restoringOpinion = false
watch(draftKey, key => {
  restoringOpinion = true
  activeDraftKey = key
  opinionText.value = key ? readOpinionDraft(key) : ''
  restoringOpinion = false
}, { immediate: true, flush: 'sync' })
watch(opinionText, text => {
  if (activeDraftKey && !restoringOpinion) writeOpinionDraft(activeDraftKey, text)
}, { flush: 'sync' })

onMounted(() => {
  void Promise.all([drawingStore.load(), reviewStore.load()]).catch(() => undefined)
})

function openDrawingFiles() {
  if (!effectiveNo.value) return
  void router.push({ name: RouteName.DrawingPreview, params: { drawingId: effectiveNo.value }, query: { from: 'review', reviewNo: effectiveNo.value } })
}

function nodeStatus(node: (typeof nodes.value)[number]) {
  return reviewNodeStatusLabel(node, currentNode.value?.name ?? null, reviewing.value, currentUserId.value, isRejected.value)
}

async function handleStartReview() {
  if (!effectiveNo.value || !canStartReview.value) return
  try {
    await reviewStore.startCase(effectiveNo.value)
    uiStore.toast(`图纸「${effectiveNo.value}」已发起审核，请按顺序完成各节点签署`, 'ok')
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
  if (submitting.value || !canSign.value) return
  const node = currentNode.value
  const review = reviewCase.value
  if (!node || !review) return
  if (!opinionText.value.trim() && action === 'rejected') {
    uiStore.toast('驳回审核必须填写审核意见与整改要求', 'warn')
    return
  }
  submitting.value = true
  const submittedDraftKey = activeDraftKey
  try {
    const updatedCase = await reviewStore.submitNode(review.id, node.name, action, opinionText.value.trim())
    if (submittedDraftKey) writeOpinionDraft(submittedDraftKey, '')
    if (updatedCase.status !== 'reviewing') {
      drawingStore.invalidate()
      await drawingStore.load()
    }
    uiStore.toast(
      action === 'pass' ? `节点「${node.name}」已审核通过` : `节点「${node.name}」已驳回，发起人将收到整改通知`,
      action === 'pass' ? 'ok' : 'warn',
    )
    const next = reviewStore.getCase(effectiveNo.value)
    const nextMine = next && next.status === 'reviewing'
      ? activeReviewNode(toWorkspaceNodes(next), true)
      : null
    if (nextMine && canSignReviewNode(nextMine, nextMine.name, authStore.currentUser, true)) {
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
      <ChangeReviewEvidence v-if="reviewCase?.changeSubmissionId" :key="reviewCase.changeSubmissionId" :drawing-no="effectiveNo" :submission-id="reviewCase.changeSubmissionId" @ready="changeEvidenceReady = $event" />
      <div class="workspace-hero card">
        <div class="hero-main">
          <div class="hero-kicker">当前审核对象</div>
          <h2>{{ drawing?.name || reviewCase?.drawingName || effectiveNo }}</h2>
          <p>
            图号 {{ effectiveNo }}
            <template v-if="reviewCase"> · 发起人 {{ reviewCase.initiator }} · {{ reviewCase.startedAt }}</template>
          </p>
        </div>
        <div class="hero-actions">
          <button v-if="reviewCase" class="btn primary" type="button" :disabled="annotationFilesLoading" @click="openAnnotations"><DemoIcon name="pencil" :size="14" />{{ annotationFilesLoading ? '正在读取…' : canSign ? '图纸批注' : '查看图纸批注' }}</button>
          <button class="btn" type="button" @click="openDrawingFiles"><DemoIcon name="eye" :size="14" />查阅图纸</button>
          <button v-if="embedded" class="btn" type="button" @click="emit('toggle-overview')">
            <DemoIcon name="workflow" :size="14" />{{ showOverview ? '返回签署工作台' : '查看流程总览' }}
          </button>
        </div>
      </div>

      <section v-if="annotationFilesOpen" class="annotation-files card" aria-label="选择审核图纸">
        <div class="annotation-files-head"><strong>选择要批注的图纸</strong><button class="btn sm" @click="annotationFilesOpen = false">收起</button></div>
        <p v-if="annotationFilesLoading" role="status">正在读取本轮审核的固定版本…</p>
        <p v-else-if="annotationFilesError" role="alert">{{ annotationFilesError }} <button class="btn sm" @click="openAnnotations">重试</button></p>
        <p v-else-if="!annotationFiles.length">本轮审核暂无可批注的 CAD 文件。请先检查图纸文件是否已完成转换。</p>
        <button v-for="file in annotationFiles" v-else :key="file.attachmentId" class="annotation-file" @click="browseAnnotation(file)"><span>{{ file.name }}</span><small>{{ file.version }} · {{ file.markCount ? `${file.markCount} 条批注` : '暂无批注' }}</small><DemoIcon name="arrow-right" :size="14" /></button>
      </section>

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
            <p class="current-meta">意见已在本次会话中保留，查图返回后可继续填写；提交成功后清除。</p>
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

          <template v-else-if="changeRequestLoading || changeRequestError">
            <div class="current-badge mute">{{ changeRequestLoading ? '正在加载' : '加载失败' }}</div>
            <p class="current-meta">{{ changeRequestLoading ? '正在确认变更工单与送审权限。' : '变更工单加载失败，请重试后发起审核。' }}</p>
            <button v-if="changeRequestError" class="btn" type="button" @click="loadChangeRequest">重试</button>
          </template>

          <template v-else-if="changeRequest?.status === 'executing'">
            <div class="current-badge mute">变更待送审</div>
            <h3>变更成果待发起完整审核</h3>
            <p class="current-meta">{{ canRestartChangeReview ? '完成修改并结束编辑后，可直接发起完整审核；本工单上一轮已通过的节点会保留。' : `等待指定修改人「${changeRequest.executorName || '待定'}」完成修改并发起审核。` }}</p>
            <button v-if="canRestartChangeReview" class="btn primary" type="button" :disabled="submitting" @click="handleRestartChangeReview">
              <DemoIcon name="rotate-cw" :size="14" />{{ submitting ? '正在提交…' : '发起完整审核' }}
            </button>
          </template>

          <template v-else-if="isRejected || missingCase">
            <div class="current-badge danger">{{ missingCase ? '流程未初始化' : '流程已驳回' }}</div>
            <h3>{{ missingCase ? '需要重新初始化审核' : '等待发起人重新发起' }}</h3>
            <p class="current-meta">{{ missingCase ? '图纸处于审核中，但没有可签署的案例。' : '审核员不能重新发起。已通过节点会保留，发起后从驳回节点继续。' }}</p>
            <button v-if="canRestartChangeReview" class="btn primary" type="button" :disabled="submitting" @click="handleRestartChangeReview">
              <DemoIcon name="rotate-cw" :size="14" />重新发起审核
            </button>
            <button v-else-if="canStartReview" class="btn primary" type="button" @click="handleStartReview">
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
            <div class="current-badge mute">{{ canRestartChangeReview ? '变更待送审' : '尚未进入审核' }}</div>
            <h3>{{ canRestartChangeReview ? '变更成果待发起完整审核' : '当前没有可签署节点' }}</h3>
            <p class="current-meta">{{ canRestartChangeReview ? '变更申请已通过、图纸修改已完成。在此重新发起完整审核，已通过节点会保留。' : '图纸仍是草稿或尚未发起审核。请由图纸创建者发起流程后再回来处理。' }}</p>
            <button v-if="canRestartChangeReview" class="btn primary" type="button" :disabled="submitting" @click="handleRestartChangeReview">
              <DemoIcon name="rotate-cw" :size="14" />发起完整审核
            </button>
            <button v-else-if="canStartReview" class="btn primary" type="button" @click="handleStartReview">开始发起审核流程</button>
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
.annotation-files { padding: 14px 18px; margin: 12px 0; }
.annotation-files-head { display: flex; justify-content: space-between; align-items: center; font-size: 13px; }
.annotation-files p { color: var(--text-2); font-size: 12px; line-height: 1.6; }
.annotation-file { display: flex; align-items: center; gap: 12px; width: 100%; padding: 12px 0; border: 0; border-bottom: 1px solid var(--line); color: var(--text-1); background: transparent; text-align: left; cursor: pointer; }
.annotation-file:hover { color: var(--accent); }
.annotation-file span { flex: 1; overflow-wrap: anywhere; }
.annotation-file small { color: var(--text-2); }
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
