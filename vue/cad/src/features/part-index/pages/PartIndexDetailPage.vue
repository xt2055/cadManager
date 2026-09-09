<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { extractAndSaveTitleBlock } from '@/services/drawing-title-block.service'
import {
  PartIndexRequestError,
  partIndexService,
  type PartIndexDetail,
  type PartIndexFields,
  type PartIndexStatus,
} from '@/services/part-index.service'
import { applyTitleCandidate, emptyPartIndexFields, fieldsFromTitleSpace, isValidPartIndexDrawingDate, partIndexStatusLabels } from '../part-index.helpers'

defineOptions({ name: 'PartIndexDetailPage' })

const route = useRoute()
const router = useRouter()
const attachmentId = computed(() => String(route.params.attachmentId ?? ''))
const detail = ref<PartIndexDetail | null>(null)
const form = ref<PartIndexFields>(emptyPartIndexFields())
const selectedSpaceId = ref<string | null>(null)
const baseline = ref('')
const loading = ref(false)
const saving = ref(false)
const extracting = ref(false)
const rebuilding = ref(false)
const error = ref('')
const conflict = ref(false)
let attachmentGeneration = 0
let loadSequence = 0

const fieldDefinitions: Array<{ key: keyof PartIndexFields, label: string }> = [
  { key: 'drawingNo', label: '图号' },
  { key: 'partName', label: '零件名称' },
  { key: 'material', label: '材料' },
  { key: 'designer', label: '设计人' },
  { key: 'checker', label: '校核人' },
  { key: 'approver', label: '审核人' },
  { key: 'drawingDateRaw', label: '图纸日期（YYYY-MM-DD）' },
  { key: 'scale', label: '比例' },
  { key: 'sheetSize', label: '图幅' },
  { key: 'process', label: '工艺' },
  { key: 'standard', label: '标准' },
  { key: 'company', label: '公司' },
]

const sourceSpaces = computed(() => (detail.value?.sourcePayload?.spaces ?? []).map((space) => ({
  ...space,
  fields: Array.isArray(space.fields) ? space.fields.map((field) => ({
    ...field,
    key: typeof field.key === 'string' ? field.key : '',
    value: typeof field.value === 'string' ? field.value : '',
    candidates: Array.isArray(field.candidates) ? field.candidates.filter((candidate): candidate is string => typeof candidate === 'string') : [],
  })) : [],
  warnings: Array.isArray(space.warnings) ? space.warnings.filter((warning): warning is string => typeof warning === 'string') : [],
})))
const selectableSourceSpaces = computed(() => sourceSpaces.value.filter((space) => space.fields.some((field) =>
  field.value.trim() || field.candidates.some((candidate) => candidate.trim()),
)))
const isDirty = computed(() => baseline.value !== JSON.stringify({ fields: form.value, selectedSpaceId: selectedSpaceId.value }))
const hasSnapshotDrift = computed(() => Boolean(detail.value && detail.value.snapshotRevision !== detail.value.sourceSnapshotRevision))
const hasInvalidDrawingDate = computed(() => Boolean(form.value.drawingDateRaw.trim()) && !isValidPartIndexDrawingDate(form.value.drawingDateRaw))
const canConfirm = computed(() => Boolean(form.value.drawingNo.trim() && form.value.partName.trim()))

function setForm(data: PartIndexDetail) {
  form.value = { ...data.fields }
  selectedSpaceId.value = data.selectedSpaceId
  baseline.value = JSON.stringify({ fields: form.value, selectedSpaceId: selectedSpaceId.value })
}

function isCurrentAttachment(targetAttachmentId: string, generation: number) {
  return attachmentId.value === targetAttachmentId && attachmentGeneration === generation
}

function beginAttachmentLoad() {
  attachmentGeneration += 1
  loadSequence += 1
  detail.value = null
  form.value = emptyPartIndexFields()
  selectedSpaceId.value = null
  baseline.value = ''
  loading.value = false
  saving.value = false
  extracting.value = false
  rebuilding.value = false
  error.value = ''
  conflict.value = false
  void load(attachmentGeneration)
}

async function load(generation = attachmentGeneration) {
  const targetAttachmentId = attachmentId.value
  if (!targetAttachmentId) return
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  conflict.value = false
  try {
    const data = await partIndexService.detail(targetAttachmentId)
    if (!isCurrentAttachment(targetAttachmentId, generation) || sequence !== loadSequence) return
    detail.value = data
    setForm(data)
  } catch (cause) {
    if (!isCurrentAttachment(targetAttachmentId, generation) || sequence !== loadSequence) return
    detail.value = null
    error.value = cause instanceof Error ? cause.message : '加载零件索引详情失败'
  } finally {
    if (isCurrentAttachment(targetAttachmentId, generation) && sequence === loadSequence) loading.value = false
  }
}

function reloadWithConfirmation() {
  if (isDirty.value && !window.confirm('重新加载会替换当前未提交的修改，是否继续？')) return
  void load()
}

function handleWriteError(cause: unknown, fallback: string) {
  error.value = cause instanceof Error ? cause.message : fallback
  conflict.value = cause instanceof PartIndexRequestError && cause.status === 409
}

async function save(action: 'save' | 'confirm') {
  if (!detail.value || saving.value || extracting.value || rebuilding.value) return
  const targetAttachmentId = attachmentId.value
  const generation = attachmentGeneration
  const currentDetail = detail.value
  saving.value = true
  error.value = ''
  conflict.value = false
  try {
    const data = await partIndexService.edit(targetAttachmentId, {
      versionId: currentDetail.versionId,
      expectedRevision: currentDetail.revision,
      expectedSnapshotRevision: currentDetail.snapshotRevision,
      action,
      selectedSpaceId: selectedSpaceId.value,
      fields: { ...form.value },
    })
    if (!isCurrentAttachment(targetAttachmentId, generation)) return
    detail.value = data
    setForm(data)
  } catch (cause) {
    if (isCurrentAttachment(targetAttachmentId, generation)) handleWriteError(cause, '保存零件信息失败')
  } finally {
    if (isCurrentAttachment(targetAttachmentId, generation)) saving.value = false
  }
}

async function resetToAutomatic() {
  if (!detail.value || saving.value || extracting.value || rebuilding.value) return
  if (!window.confirm('恢复识别值会清除当前版本全部人工字段和确认状态，是否继续？')) return
  const targetAttachmentId = attachmentId.value
  const generation = attachmentGeneration
  const currentDetail = detail.value
  saving.value = true
  error.value = ''
  conflict.value = false
  try {
    const data = await partIndexService.edit(targetAttachmentId, {
      versionId: currentDetail.versionId,
      expectedRevision: currentDetail.revision,
      expectedSnapshotRevision: currentDetail.snapshotRevision,
      action: 'reset',
    })
    if (!isCurrentAttachment(targetAttachmentId, generation)) return
    detail.value = data
    setForm(data)
  } catch (cause) {
    if (isCurrentAttachment(targetAttachmentId, generation)) handleWriteError(cause, '恢复自动识别结果失败')
  } finally {
    if (isCurrentAttachment(targetAttachmentId, generation)) saving.value = false
  }
}

async function rebuildIndex() {
  if (!detail.value || rebuilding.value) return
  if (isDirty.value && !window.confirm('重建索引会替换当前未保存的修改，是否继续？')) return
  const targetAttachmentId = attachmentId.value
  const generation = attachmentGeneration
  const currentDetail = detail.value
  rebuilding.value = true
  error.value = ''
  conflict.value = false
  try {
    const data = await partIndexService.rebuild(targetAttachmentId, currentDetail.versionId, currentDetail.snapshotRevision)
    if (!isCurrentAttachment(targetAttachmentId, generation)) return
    detail.value = data.detail
    setForm(data.detail)
  } catch (cause) {
    if (isCurrentAttachment(targetAttachmentId, generation)) handleWriteError(cause, '重建零件索引失败')
  } finally {
    if (isCurrentAttachment(targetAttachmentId, generation)) rebuilding.value = false
  }
}

async function extractAgain() {
  if (!detail.value || extracting.value) return
  if (isDirty.value && !window.confirm('重新提取前请先保存或放弃本地修改。继续会替换未提交内容，是否继续？')) return
  const targetAttachmentId = attachmentId.value
  const generation = attachmentGeneration
  extracting.value = true
  error.value = ''
  conflict.value = false
  try {
    await extractAndSaveTitleBlock(targetAttachmentId, true)
    if (!isCurrentAttachment(targetAttachmentId, generation)) return
    await load(generation)
  } catch (cause) {
    if (!isCurrentAttachment(targetAttachmentId, generation)) return
    await load(generation)
    if (isCurrentAttachment(targetAttachmentId, generation)) handleWriteError(cause, '重新提取标题栏失败')
  } finally {
    if (isCurrentAttachment(targetAttachmentId, generation)) extracting.value = false
  }
}

function openProject(drawingNo: string) {
  router.push({ name: RouteName.DrawingPreview, params: { drawingId: drawingNo } })
}

function openViewer() {
  if (!detail.value?.defaultProjectDrawingNo) return
  router.push({
    name: RouteName.DrawingViewer,
    params: { drawingId: detail.value.defaultProjectDrawingNo },
    query: { fileId: detail.value.attachmentId },
  })
}

function chooseSpace(nextSpaceId: string | null) {
  if (nextSpaceId === selectedSpaceId.value) return
  if (saving.value || extracting.value || rebuilding.value) return
  if (isDirty.value && !window.confirm('切换识别布局会放弃当前未保存修改，是否继续？')) return
  selectedSpaceId.value = nextSpaceId
  if (!nextSpaceId) return
  const space = selectableSourceSpaces.value.find((item) => item.id === nextSpaceId)
  if (space) form.value = fieldsFromTitleSpace(space, form.value.sheetSize)
}

function useCandidate(sourceKey: string, candidate: string) {
  if (saving.value || extracting.value || rebuilding.value) return
  form.value = applyTitleCandidate(form.value, sourceKey, candidate)
}

function statusClass(value: PartIndexStatus) {
  return `status-${value}`
}

watch(attachmentId, beginAttachmentLoad)
onMounted(beginAttachmentLoad)
onBeforeUnmount(() => {
  attachmentGeneration += 1
  loadSequence += 1
})
</script>

<template>
  <div class="page part-index-detail">
    <header class="detail-head">
      <div>
        <button class="back-link" type="button" @click="router.push({ name: RouteName.PartIndexLibrary, query: route.query })">
          <DemoIcon name="arrow-left" :size="14" /> 返回零件索引
        </button>
        <div class="eyebrow"><DemoIcon name="file-search" :size="14" /> 零件索引</div>
        <h1>{{ detail?.partName || detail?.fileName || '零件索引详情' }}</h1>
        <p class="subtitle">{{ detail?.drawingNo || detail?.registeredPartNo || detail?.fileName || '—' }}</p>
      </div>
      <div v-if="detail" class="head-actions">
        <span class="status-tag" :class="statusClass(detail.status)">{{ partIndexStatusLabels[detail.status] }}</span>
        <button class="btn" type="button" :disabled="!detail.defaultProjectDrawingNo" @click="openViewer"><DemoIcon name="eye" :size="14" /> 查看图纸</button>
      </div>
    </header>

    <p v-if="error" class="error-message" role="alert">
      {{ error }}
      <button v-if="conflict" class="link" type="button" @click="reloadWithConfirmation">重新加载并核对</button>
      <button v-else class="link" type="button" @click="reloadWithConfirmation">重新加载</button>
    </p>
    <div v-if="loading" class="card state-message">正在加载零件索引信息…</div>

    <template v-else-if="detail">
      <p class="notice">此处修改仅影响零件索引，不修改 CAD 文件或正式零件资料。</p>
      <p v-if="detail.status === 'needs_confirmation'" class="notice">当前信息由系统提取，尚未人工确认。</p>
      <p v-if="hasSnapshotDrift" class="warning-card">标题栏快照与索引修订不一致，请重新提取或重建后核对信息。</p>

      <section class="card overview">
        <div><span>来源文件</span><strong>{{ detail.fileName }}</strong></div>
        <div><span>当前版本</span><strong class="mono">{{ detail.versionId }}</strong></div>
        <div><span>识别状态</span><strong>{{ detail.extractionStatus === 'failed' ? '失败' : detail.extractionStatus === 'pending' ? '未提取' : '已提取' }}</strong></div>
        <div><span>最近确认</span><strong>{{ detail.confirmedAt || '未确认' }}</strong></div>
      </section>

      <section v-if="detail.extractionError" class="card warning-card">
        标题栏提取失败：{{ detail.extractionError }}
      </section>

      <section class="content-grid">
        <section class="card editor-card">
          <div class="card-head">
            <div>
              <h2>零件信息</h2>
              <p>{{ detail.hasManualFields ? '当前显示人工保存的字段；可恢复为自动识别结果。' : '当前显示自动识别结果；确认前请核对。' }}</p>
            </div>
            <div v-if="detail.canWrite" class="card-actions">
              <button class="btn" type="button" :disabled="extracting || saving || rebuilding" @click="extractAgain">{{ extracting ? '提取中…' : '重新提取标题栏' }}</button>
              <button v-if="detail.revision === 0 || hasSnapshotDrift" class="btn" type="button" :disabled="extracting || saving || rebuilding || detail.snapshotRevision === 0" @click="rebuildIndex">{{ rebuilding ? '重建中…' : '重建索引' }}</button>
            </div>
          </div>

          <div v-if="detail.canWrite && selectableSourceSpaces.length" class="space-selector">
            <label>
              标题栏布局
              <select :value="selectedSpaceId ?? ''" class="inp" :disabled="saving || extracting || rebuilding" @change="chooseSpace(($event.target as HTMLSelectElement).value || null)">
                <option value="">人工填写，不绑定布局</option>
                <option v-for="space in selectableSourceSpaces" :key="space.id" :value="space.id">{{ space.name }}</option>
              </select>
            </label>
            <p v-if="selectableSourceSpaces.length > 1 && !selectedSpaceId">检测到多个有效布局，请选择后核对；系统不会默认使用第一个。</p>
            <p v-if="detail.selectedSpaceMissing">此前选择的空间已不存在；请重新选择并保存或确认。</p>
          </div>

          <dl v-if="!detail.canWrite" class="readonly-grid">
            <div v-for="field in fieldDefinitions" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ detail.fields[field.key] || '—' }}</dd></div>
          </dl>
          <form v-else class="field-grid" @submit.prevent="save('save')">
            <label v-for="field in fieldDefinitions" :key="field.key">
              <span>{{ field.label }}</span>
              <input v-model="form[field.key]" class="inp" :disabled="saving || extracting || rebuilding" />
              <small v-if="field.key === 'drawingDateRaw' && hasInvalidDrawingDate" class="field-warning">该日期不会参与日期筛选，建议改为 YYYY-MM-DD。</small>
            </label>
            <div class="edit-actions">
              <button class="btn" type="submit" :disabled="saving || extracting || rebuilding">{{ saving ? '保存中…' : '保存草稿' }}</button>
              <button class="btn primary" type="button" :disabled="saving || extracting || rebuilding || !canConfirm" @click="save('confirm')">确认信息</button>
              <button class="btn" type="button" :disabled="saving || extracting || rebuilding || !detail.hasManualFields" @click="resetToAutomatic">恢复识别值</button>
            </div>
          </form>
        </section>

        <aside class="card source-card">
          <h2>标题栏识别来源</h2>
          <p v-if="!sourceSpaces.length" class="muted">当前版本还没有可用的标题栏快照。</p>
          <template v-for="space in sourceSpaces" :key="space.id">
            <section class="source-space">
              <strong>{{ space.name }}</strong>
              <p v-for="warning in space.warnings" :key="warning" class="warning-copy">{{ warning }}</p>
              <dl>
                <template v-for="field in space.fields" :key="field.key">
                  <div v-if="field.value || field.candidates.length">
                    <dt>{{ field.key }}</dt>
                    <dd>{{ field.value || '未确定' }}</dd>
                    <button v-for="candidate in field.candidates" :key="candidate" class="candidate" type="button" :disabled="!detail.canWrite || saving || extracting || rebuilding" @click="useCandidate(field.key, candidate)">填入：{{ candidate }}</button>
                  </div>
                </template>
              </dl>
            </section>
          </template>
        </aside>
      </section>

      <section class="card project-card">
        <h2>来源项目</h2>
        <p>零件索引只引用原项目文件，不创建 DWG 副本。</p>
        <div v-if="detail.projects.length" class="project-list">
          <button v-for="project in detail.projects" :key="project.drawingId" type="button" class="project-row" @click="openProject(project.drawingNo)">
            <strong>{{ project.projectCode || project.drawingNo }}</strong>
            <span>{{ project.projectName || project.drawingNo }}</span>
            <small>{{ project.relationTypes.join('、') }}</small>
          </button>
        </div>
        <p v-else class="muted">当前附件没有可用的项目关联。</p>
        <p class="audit-copy">最近编辑：{{ detail.editedAt || '无' }}；最近确认：{{ detail.confirmedAt || '无' }}</p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.part-index-detail { display: flex; flex-direction: column; gap: 16px; min-height: 100%; padding: 22px 26px; }
.detail-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 18px; }
.back-link { display: inline-flex; align-items: center; gap: 5px; margin: 0 0 12px; padding: 0; color: var(--text-2); border: 0; background: transparent; cursor: pointer; font: inherit; font-size: 12px; }
.eyebrow { display: flex; align-items: center; gap: 6px; color: var(--text-2); font-size: 12px; }
h1 { margin: 6px 0 3px; font-size: 25px; }
.subtitle { margin: 0; color: var(--text-2); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.head-actions, .card-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.head-actions { padding-top: 8px; }
.status-tag { display: inline-flex; padding: 4px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--text-2); font-size: 12px; white-space: nowrap; }
.status-confirmed { color: var(--success, #248a4d); }
.status-recheck, .status-failed { color: var(--danger, #c43b3b); }
.status-needs_confirmation { color: var(--warning, #9a6500); }
.error-message, .warning-card, .notice { margin: 0; padding: 11px 14px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.error-message { color: var(--danger, #c43b3b); }
.warning-card { color: var(--warning, #9a6500); }
.notice { color: var(--text-2); font-size: 12px; }
.state-message { padding: 48px 20px; color: var(--text-2); text-align: center; }
.overview { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; padding: 16px; }
.overview div { min-width: 0; }
.overview span { display: block; margin-bottom: 5px; color: var(--text-2); font-size: 12px; }
.overview strong { display: block; overflow-wrap: anywhere; font-size: 13px; }
.content-grid { display: grid; grid-template-columns: minmax(0, 2fr) minmax(260px, 1fr); gap: 16px; align-items: start; }
.editor-card, .source-card, .project-card { padding: 18px; }
.card-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; padding-bottom: 14px; border-bottom: 1px solid var(--line); }
h2 { margin: 0; font-size: 16px; }
.card-head p, .project-card > p, .source-card > p { margin: 7px 0 0; color: var(--text-2); font-size: 12px; line-height: 1.6; }
.space-selector { margin: 14px 0; padding: 12px; border: 1px solid var(--line); border-radius: 6px; }
.space-selector label { display: flex; align-items: center; gap: 12px; color: var(--text-2); font-size: 12px; }
.space-selector select { flex: 1; }
.space-selector p { margin: 8px 0 0; color: var(--warning, #9a6500); font-size: 12px; }
.field-grid, .readonly-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; margin-top: 16px; }
.field-grid label { display: flex; flex-direction: column; gap: 6px; color: var(--text-2); font-size: 12px; }
.field-warning { color: var(--warning, #9a6500); line-height: 1.4; }
.readonly-grid dt, .source-space dt { color: var(--text-2); font-size: 12px; }
.readonly-grid dd { margin: 6px 0 0; overflow-wrap: anywhere; font-size: 14px; }
.edit-actions { grid-column: 1 / -1; display: flex; flex-wrap: wrap; gap: 8px; padding-top: 4px; }
.source-space { margin-top: 14px; padding-top: 14px; border-top: 1px solid var(--line); }
.source-space dl { display: grid; gap: 10px; margin: 10px 0 0; }
.source-space dd { margin: 4px 0; overflow-wrap: anywhere; font-size: 12px; }
.candidate { display: block; max-width: 100%; margin: 4px 0 0; padding: 0; overflow-wrap: anywhere; color: var(--text-2); border: 0; background: transparent; cursor: pointer; font: inherit; font-size: 12px; text-align: left; }
.candidate:hover:not(:disabled) { color: var(--text-1); text-decoration: underline; }
.candidate:disabled { cursor: default; }
.warning-copy { margin: 6px 0; color: var(--warning, #9a6500); font-size: 12px; }
.project-list { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
.project-row { display: grid; gap: 3px; min-width: 220px; padding: 10px; text-align: left; color: var(--text-1); border: 1px solid var(--line); border-radius: 6px; background: var(--panel); cursor: pointer; }
.project-row:hover { background: var(--hover); }
.project-row span, .project-row small, .muted, .audit-copy { color: var(--text-2); }
.project-row span { font-size: 12px; }
.project-row small, .audit-copy { font-size: 11px; }
.audit-copy { margin-top: 16px !important; }
@media (max-width: 900px) { .overview { grid-template-columns: repeat(2, minmax(0, 1fr)); } .content-grid { grid-template-columns: 1fr; } }
@media (max-width: 680px) { .part-index-detail { padding: 16px; } .detail-head { display: block; } .head-actions { padding-top: 14px; } .field-grid, .readonly-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
