<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { reviewAnnotationService } from '@/services/review-annotation.service'
import {
  historyViewerQuery,
  isCadFile,
  opinionLines,
  previewableFile,
  recordGroups,
  recordNote,
  recordSummary,
  roundContextLabel,
  roundLabel,
  roundStatusLabel,
  type AnnotationHistoryRound,
} from '../annotation-history'

defineOptions({ name: 'ReviewAnnotationHistory' })

const props = defineProps<{
  drawingNo: string
  currentCaseId?: string
  focusCaseId?: string
}>()

const router = useRouter()
const rounds = ref<AnnotationHistoryRound[]>([])
const loading = ref(false)
const error = ref('')
const expanded = ref<string[]>([])
const copyMessage = ref('')
const copyCaseId = ref('')
let sequence = 0

const drawingNo = computed(() => props.drawingNo.trim())

async function load() {
  const target = drawingNo.value
  if (!target) {
    rounds.value = []
    return
  }
  const current = ++sequence
  loading.value = true
  error.value = ''
  try {
    const items = await reviewAnnotationService.history({ drawingNo: target })
    if (current !== sequence) return
    rounds.value = items
    // 从「已办审核」等入口进来时直接展开目标轮次，其余情况默认展开最新一轮。
    const focus = props.focusCaseId
    expanded.value = focus && items.some((item) => item.caseId === focus) ? [focus] : items[0] ? [items[0].caseId] : []
  } catch (loadError: unknown) {
    if (current !== sequence) return
    rounds.value = []
    error.value = loadError instanceof Error ? loadError.message : '读取标注历史失败'
  } finally {
    if (current === sequence) loading.value = false
  }
}

watch(() => [drawingNo.value, props.focusCaseId], () => { void load() }, { immediate: true })

function isExpanded(round: AnnotationHistoryRound) {
  return expanded.value.includes(round.caseId)
}
function toggle(round: AnnotationHistoryRound) {
  expanded.value = isExpanded(round) ? expanded.value.filter((id) => id !== round.caseId) : [...expanded.value, round.caseId]
}
function statusClass(status: string) {
  if (status === 'rejected') return 'danger'
  if (status === 'published') return 'ok'
  return 'plain'
}
function fileOf(round: AnnotationHistoryRound, attachmentId: string) {
  return round.files.find((file) => file.attachmentId === attachmentId)
}
function openRound(round: AnnotationHistoryRound) {
  const file = previewableFile(round)
  if (file) openFile(round, file.attachmentId)
}
function openFile(round: AnnotationHistoryRound, attachmentId: string) {
  const file = fileOf(round, attachmentId)
  // 非 CAD 文件只能看意见，不能在画布里回放。
  if (!file || !isCadFile(file.name)) return
  void router.push({
    name: RouteName.DrawingViewer,
    params: { drawingId: round.drawingNo || drawingNo.value },
    query: historyViewerQuery(round, file),
  })
}
async function copyRound(round: AnnotationHistoryRound) {
  copyCaseId.value = round.caseId
  try {
    await navigator.clipboard.writeText(opinionLines(round).join('\n'))
    copyMessage.value = `${roundLabel(round)}意见已复制，可粘贴到审核意见中`
  } catch {
    copyMessage.value = '无法自动复制，请手动选择下方意见'
  }
}
</script>

<template>
  <section class="annotation-history card" aria-label="标注历史">
    <div class="history-head">
      <strong>标注历史</strong>
      <span class="history-hint">按轮次留档每次审核的批注；新一轮审核不会带着上一轮的批注，需要时从这里回看。</span>
      <button class="text-button" type="button" :disabled="loading" @click="load">刷新</button>
    </div>

    <p v-if="!drawingNo" class="note">未指定图纸，无法读取标注历史。</p>
    <p v-else-if="loading" class="note" role="status">正在读取历史批注…</p>
    <p v-else-if="error" class="error" role="alert">{{ error }} <button class="text-button" type="button" @click="load">重试</button></p>
    <p v-else-if="!rounds.length" class="note">暂无审核记录。图纸发起审核后，这里会按轮次留下每位审核员的批注。</p>

    <template v-else>
      <article v-for="round in rounds" :key="round.caseId" class="history-round" :class="{ focus: round.caseId === focusCaseId }">
        <header class="round-head">
          <button class="round-toggle" type="button" :aria-expanded="isExpanded(round)" @click="toggle(round)">
            <DemoIcon :name="isExpanded(round) ? 'chevron-down' : 'chevron-right'" :size="14" />
            <strong>{{ roundLabel(round) }}</strong>
            <span class="tag" :class="statusClass(round.status)">{{ roundStatusLabel(round.status) }}</span>
            <span v-if="round.caseId === currentCaseId" class="tag info">本轮</span>
          </button>
          <span class="round-meta">{{ roundContextLabel(round) }} · 发起人 {{ round.initiator || '—' }} · {{ round.startedAt }}<template v-if="round.completedAt"> 至 {{ round.completedAt }}</template></span>
          <button class="btn sm" type="button" :disabled="!previewableFile(round)" @click="openRound(round)">图上查看本轮批注</button>
        </header>

        <div v-if="isExpanded(round)" class="round-body">
          <p v-if="!round.files.length" class="note">本轮审核没有可批注的 CAD 文件。</p>
          <div v-for="group in recordGroups(round)" :key="group.attachmentId" class="file-block">
            <div class="file-head">
              <DemoIcon name="file" :size="13" />
              <strong>{{ group.name || '审核文件' }}</strong>
              <span class="file-count">{{ group.records.reduce((total, record) => total + record.markCount, 0) }} 条批注</span>
              <button class="text-button" type="button" :disabled="!isCadFile(group.name || '')" @click="openFile(round, group.attachmentId)">图上查看</button>
            </div>
            <p v-if="!group.records.length" class="note">这份文件本轮暂无批注。</p>
            <div v-for="record in group.records" :key="record.documentId" class="record">
              <div class="record-summary">{{ recordSummary(record) }}</div>
              <p v-if="record.nodeOpinion" class="record-opinion">节点意见：{{ record.nodeOpinion }}</p>
              <ul v-if="record.texts.length" class="record-texts">
                <li v-for="(text, index) in record.texts" :key="`${record.documentId}-${index}`">{{ text.text }}</li>
              </ul>
              <p v-if="recordNote(record)" class="note">{{ recordNote(record) }}</p>
            </div>
          </div>
          <div class="round-actions">
            <button class="text-button" type="button" @click="copyRound(round)"><DemoIcon name="copy" :size="13" />复制本轮意见清单</button>
            <span v-if="copyCaseId === round.caseId && copyMessage" class="note" role="status">{{ copyMessage }}</span>
          </div>
        </div>
      </article>
    </template>
  </section>
</template>

<style scoped>
.annotation-history { padding: 14px 18px; margin: 12px 0; }
.history-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.history-head strong { font-size: 14px; }
.history-hint { flex: 1; min-width: 200px; color: var(--text-3); font-size: 12px; line-height: 1.6; }
.history-round { margin-top: 12px; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
.history-round.focus { border-color: var(--accent); box-shadow: inset 0 0 0 1px var(--accent-soft); }
.round-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 10px 12px; background: var(--panel-2); }
.round-toggle { display: inline-flex; align-items: center; gap: 8px; border: 0; background: transparent; color: var(--text-1); font: inherit; font-size: 13px; cursor: pointer; padding: 2px; }
.round-toggle strong { font-size: 13px; }
.round-meta { flex: 1; min-width: 180px; color: var(--text-3); font-size: 12px; }
.round-body { padding: 4px 12px 12px; }
.file-block { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
.file-head { display: flex; align-items: center; gap: 8px; font-size: 12.5px; }
.file-head strong { flex: 1; overflow-wrap: anywhere; }
.file-count { color: var(--text-3); font-size: 11.5px; }
.record { margin: 8px 0 0; padding: 10px; border-radius: 6px; background: var(--panel-2); }
.record-summary { color: var(--text-2); font-size: 12px; overflow-wrap: anywhere; }
.record-opinion { margin: 6px 0 0; font-size: 12.5px; line-height: 1.65; overflow-wrap: anywhere; }
.record-texts { margin: 6px 0 0; padding-left: 18px; font-size: 12.5px; line-height: 1.7; overflow-wrap: anywhere; }
.record-texts li { margin: 2px 0; }
.round-actions { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-top: 12px; }
.round-actions .text-button { display: inline-flex; align-items: center; gap: 4px; }
.note { color: var(--text-2); font-size: 12px; line-height: 1.65; margin: 8px 0; }
.error { color: var(--danger); font-size: 12px; line-height: 1.6; }
.text-button { background: transparent; border: 0; color: var(--accent); cursor: pointer; font-size: 12px; padding: 4px; }
button:disabled { opacity: .45; cursor: not-allowed; }
</style>
