<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { composePartNo, defaultProjectNo, isBorrowedNumber, validateManualPartNo } from '@/utils/drawing-number-completion'

defineOptions({ name: 'PartNumberInputModal' })

/**
 * 手工补录图号弹窗（兜底路径）。
 *
 * 图号主来源是图幅：DWG/DXF 读本地文件，EXB 先转 DWG 再读。只有两边都读不到图号时（图幅里确实没写、
 * 或解析不出）才用这个弹窗。用户只填后几位，项目号自动带出来；项目号可改 —— 借用件本来就跨项目号。
 */
export interface PartNumberInputRow {
  /** 文件 id，回传给调用方定位是哪个文件。 */
  id: string
  name: string
  /** 图幅里读到的候选图号（可能为空）；有多个候选时供用户点选。 */
  candidates: string[]
  /** 为什么需要手工补录，直接展示给用户。 */
  reason: string
}

const props = defineProps<{
  rows: PartNumberInputRow[]
  /** 当前项目图号，用于推导默认项目号。 */
  rootDrawingNo: string
  /** 正在提交：关闭与确认同时禁用。 */
  busy: boolean
}>()

const emit = defineEmits<{
  close: []
  /** partNo 为空串表示用户跳过了该文件（调用方按「其他文件」处理）。 */
  confirm: [entries: Array<{ id: string; partNo: string }>]
}>()

interface Draft {
  projectNo: string
  suffix: string
  skipped: boolean
}

const drafts = ref<Record<string, Draft>>({})

function blankDraft(): Draft {
  return { projectNo: defaultProjectNo(props.rootDrawingNo), suffix: '', skipped: false }
}

// 每次打开都按当前文件重建草稿；沿用上一批输入会把 A 文件的图号带到 B 文件上。
watch(
  () => props.rows,
  (rows) => {
    drafts.value = Object.fromEntries(rows.map((row) => [row.id, blankDraft()]))
  },
  { immediate: true, deep: false },
)

function draftOf(id: string): Draft {
  return drafts.value[id] ?? blankDraft()
}

function partNoOf(id: string): string {
  const draft = draftOf(id)
  return composePartNo(draft.projectNo, draft.suffix)
}

function errorOf(id: string): string {
  const draft = draftOf(id)
  if (draft.skipped) return ''
  const partNo = partNoOf(id)
  if (!draft.suffix.trim()) return '请输入图号后几位'
  return validateManualPartNo(partNo)
}

function borrowedOf(id: string): boolean {
  return isBorrowedNumber(partNoOf(id), props.rootDrawingNo)
}

/** 候选图号直接可用时，点一下就把「项目号 + 后几位」一起填好。 */
function applyCandidate(id: string, candidate: string) {
  const draft = draftOf(id)
  const projectNo = defaultProjectNo(props.rootDrawingNo)
  if (candidate.toLowerCase().startsWith(projectNo.toLowerCase()) && projectNo) {
    draft.projectNo = projectNo
    draft.suffix = candidate.slice(projectNo.length).replace(/^[-\s]+/, '')
    return
  }
  draft.projectNo = ''
  draft.suffix = candidate
}

const includesRow = computed(() => props.rows.filter((row) => !draftOf(row.id).skipped))
const filledCount = computed(() => includesRow.value.filter((row) => !errorOf(row.id)).length)
const hasError = computed(() => props.rows.some((row) => Boolean(errorOf(row.id))))
const canConfirm = computed(() => !props.busy && includesRow.value.length > 0 && !hasError.value)

function confirm() {
  if (!canConfirm.value) return
  emit('confirm', props.rows.map((row) => ({
    id: row.id,
    partNo: draftOf(row.id).skipped ? '' : partNoOf(row.id),
  })))
}
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal card part-number-modal">
      <div class="modal-head">
        <div class="modal-title">
          <DemoIcon name="pencil" :size="18" />
          <span>补录零件图号</span>
        </div>
        <button class="btn sm close-btn" type="button" :disabled="busy" @click="emit('close')">✕</button>
      </div>

      <div class="modal-body part-number-body">
        <div class="part-number-hint">
          <DemoIcon name="info" :size="14" />
          <span>这些文件的图幅里没有读到图号（EXB 已尝试转换后读取）。请只填图号后面几位，项目号会自动补上；借用件请改项目号。</span>
        </div>

        <ul class="part-number-rows">
          <li v-for="row in rows" :key="row.id" class="part-number-row" :class="{ 'is-skipped': draftOf(row.id).skipped }">
            <div class="row-head">
              <label class="file-name-cell">
                <DemoIcon name="file" :size="14" />
                <b :title="row.name">{{ row.name }}</b>
              </label>
              <label class="skip-box">
                <input
                  type="checkbox"
                  :checked="draftOf(row.id).skipped"
                  :disabled="busy"
                  @change="draftOf(row.id).skipped = ($event.target as HTMLInputElement).checked"
                />
                <span>跳过（归入其他文件）</span>
              </label>
            </div>

            <p class="row-reason">{{ row.reason }}</p>

            <template v-if="!draftOf(row.id).skipped">
              <div v-if="row.candidates.length" class="row-candidates">
                <span class="cand-label">图幅候选：</span>
                <button
                  v-for="candidate in row.candidates"
                  :key="candidate"
                  class="cand-chip mono"
                  type="button"
                  :disabled="busy"
                  @click="applyCandidate(row.id, candidate)"
                >{{ candidate }}</button>
              </div>

              <div class="row-fields">
                <label class="field">
                  <span>项目号</span>
                  <input
                    v-model="draftOf(row.id).projectNo"
                    class="inp mono"
                    type="text"
                    :disabled="busy"
                    aria-label="项目号"
                  />
                </label>
                <label class="field">
                  <span>图号后几位</span>
                  <input
                    v-model="draftOf(row.id).suffix"
                    class="inp mono"
                    type="text"
                    placeholder="例如 01-01c"
                    :disabled="busy"
                    aria-label="图号后几位"
                  />
                </label>
              </div>

              <div class="row-preview">
                <span class="preview-label">完整图号</span>
                <span class="preview-value mono">{{ partNoOf(row.id) || '—' }}</span>
                <span v-if="borrowedOf(row.id)" class="borrow-tag">
                  <DemoIcon name="git-branch" :size="12" />借用件
                </span>
              </div>
              <p v-if="errorOf(row.id)" class="row-error" role="alert">{{ errorOf(row.id) }}</p>
            </template>
          </li>
        </ul>
      </div>

      <div class="modal-foot">
        <span v-if="filledCount || includesRow.length < rows.length" class="foot-summary">
          已填 {{ filledCount }} / {{ includesRow.length }} 项{{ includesRow.length < rows.length ? `，跳过 ${rows.length - includesRow.length} 项` : '' }}
        </span>
        <button class="btn" type="button" :disabled="busy" @click="emit('close')">取消</button>
        <button class="btn primary" type="button" :disabled="!canConfirm" @click="confirm">
          <DemoIcon name="check" :size="14" />
          {{ busy ? '提交中...' : '确认图号' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 只放本弹窗专属样式；弹窗外壳来自上面的共享 modal-chrome。 */
.part-number-modal {
  width: 620px;
  max-width: 92vw;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.45);
  display: flex;
  flex-direction: column;
  max-height: 85vh;
}

.part-number-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  min-height: 0;
}

.part-number-hint {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12.5px;
  color: var(--text-2);
  line-height: 1.5;
  background: var(--panel-2);
  padding: 8px 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.part-number-rows {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.part-number-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--panel-2);
}

/* 跳过的行整块压暗：一眼能看出这批里哪些不建零件。 */
.part-number-row.is-skipped {
  opacity: 0.6;
}

.row-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.file-name-cell :deep(svg) {
  flex-shrink: 0;
  color: var(--accent);
}

.file-name-cell b {
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
}

.skip-box {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.row-reason {
  margin: 0;
  color: var(--text-3);
  font-size: 11.5px;
  line-height: 1.5;
}

.row-candidates {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.cand-label {
  color: var(--text-3);
  font-size: 11.5px;
}

.cand-chip {
  padding: 3px 8px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--panel);
  color: var(--text-2);
  font-size: 11px;
}

.cand-chip:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}

.row-fields {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 10px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.field > span {
  color: var(--text-3);
  font-size: 11px;
}

.row-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.preview-label {
  flex-shrink: 0;
  color: var(--text-3);
  font-size: 11px;
}

.preview-value {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--accent);
  font-size: 12.5px;
  font-weight: 600;
}

.borrow-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  padding: 2px 7px;
  border: 1px solid var(--line);
  border-radius: 999px;
  color: var(--text-2);
  font-size: 10.5px;
}

.row-error {
  margin: 0;
  color: var(--danger);
  font-size: 11.5px;
}


/* .modal-foot 的 display/对齐由共享 modal-chrome 提供；这里只把汇总文字推到最左。 */
.foot-summary {
  margin-right: auto;
  color: var(--text-3);
  font-size: 11.5px;
}
</style>
