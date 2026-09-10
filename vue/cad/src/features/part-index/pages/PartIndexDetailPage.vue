<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { useUiStore } from '@/stores/ui.store'
import { PartIndexRequestError, partIndexService, type PartIndexDetail, type PartIndexFields } from '@/services/part-index.service'
import { automaticPartIndex } from '@/services/part-index-auto.service'
import { emptyPartIndexFields, fieldsFromTitleSpace, isValidPartIndexDrawingDate, uniquePartIndexProjects, partIndexProjectLabel } from '../part-index.helpers'
import '../part-index.css'

defineOptions({ name: 'PartIndexDetailPage' })
const route = useRoute()
const router = useRouter()
const uiStore = useUiStore()
const attachmentId = computed(() => String(route.params.attachmentId ?? ''))
const detail = ref<PartIndexDetail | null>(null)
const form = ref<PartIndexFields>(emptyPartIndexFields())
const selectedSpaceId = ref<string | null>(null)
const baseline = ref('')
const loading = ref(false)
const saving = ref(false)
const editing = ref(false)
const saved = ref(false)
const error = ref('')
const conflict = ref(false)
const newerInformation = ref(false)
let generation = 0
let loadSequence = 0
const groups: Array<{ label: string; fields: Array<{ key: keyof PartIndexFields; label: string }> }> = [
  { label: '基本信息', fields: [{ key: 'drawingNo', label: '图号' }, { key: 'partName', label: '零件名称' }, { key: 'material', label: '材料' }] },
  { label: '设计与校审', fields: [{ key: 'designer', label: '设计人' }, { key: 'checker', label: '校核人' }, { key: 'approver', label: '审核人' }, { key: 'process', label: '工艺' }, { key: 'standard', label: '标准' }, { key: 'company', label: '公司' }] },
  { label: '图纸规格', fields: [{ key: 'drawingDateRaw', label: '图纸日期' }, { key: 'scale', label: '比例' }, { key: 'sheetSize', label: '图幅' }] },
]
const projects = computed(() => uniquePartIndexProjects(detail.value?.projects))
const selectableSpaces = computed(() => (detail.value?.sourcePayload?.spaces ?? []).filter(space =>
  Array.isArray(space.fields) && space.fields.some(field => typeof field.value === 'string' && field.value.trim()),
))
const isDirty = computed(() => baseline.value !== JSON.stringify({ fields: form.value, selectedSpaceId: selectedSpaceId.value }))
const invalidDate = computed(() => Boolean(form.value.drawingDateRaw.trim()) && !isValidPartIndexDrawingDate(form.value.drawingDateRaw))
function setForm(data: PartIndexDetail) {
  form.value = { ...data.fields }
  selectedSpaceId.value = data.selectedSpaceId
  baseline.value = JSON.stringify({ fields: form.value, selectedSpaceId: selectedSpaceId.value })
}
function current(id: string, requestGeneration: number) { return attachmentId.value === id && generation === requestGeneration }
async function load() {
  const id = attachmentId.value
  const requestGeneration = generation
  const sequence = ++loadSequence
  if (!id) return
  loading.value = true
  error.value = ''
  conflict.value = false
  try {
    const data = await partIndexService.detail(id)
    if (!current(id, requestGeneration) || sequence !== loadSequence) return
    detail.value = data
    setForm(data)
    newerInformation.value = false
  } catch (cause) {
    if (current(id, requestGeneration) && sequence === loadSequence) error.value = cause instanceof Error ? cause.message : '加载零件信息失败'
  } finally {
    if (current(id, requestGeneration) && sequence === loadSequence) loading.value = false
  }
}
function confirmDiscard(action: () => void) {
  if (!isDirty.value) { action(); return }
  const id = attachmentId.value
  const requestGeneration = generation
  uiStore.confirm('放弃未保存的修改？', '本次编辑尚未保存。', {
    confirmText: '放弃修改',
    onConfirm: () => { if (current(id, requestGeneration) && !saving.value) action() },
  })
}
function cancelEdit() {
  confirmDiscard(() => {
    if (detail.value) setForm(detail.value)
    editing.value = false
    if (newerInformation.value) void load()
  })
}
function reload() { confirmDiscard(() => { editing.value = false; void load() }) }
async function save() {
  if (!detail.value?.canWrite || saving.value || loading.value || !isDirty.value) return
  const id = attachmentId.value
  const requestGeneration = generation
  const previous = detail.value
  saving.value = true
  error.value = ''
  conflict.value = false
  try {
    const data = await partIndexService.edit(id, {
      versionId: previous.versionId, expectedRevision: previous.revision,
      expectedSnapshotRevision: previous.snapshotRevision, action: 'save',
      selectedSpaceId: selectedSpaceId.value, fields: { ...form.value },
    })
    if (!current(id, requestGeneration)) return
    detail.value = data
    setForm(data)
    editing.value = false
    saved.value = true
    newerInformation.value = false
  } catch (cause) {
    if (!current(id, requestGeneration)) return
    error.value = cause instanceof Error ? cause.message : '保存零件信息失败'
    conflict.value = cause instanceof PartIndexRequestError && cause.status === 409
  } finally {
    if (current(id, requestGeneration)) saving.value = false
  }
}
function chooseSpace(value: string) {
  if (saving.value) return
  confirmDiscard(() => {
    selectedSpaceId.value = value || null
    const space = selectableSpaces.value.find(item => item.id === value)
    if (space) form.value = fieldsFromTitleSpace(space, form.value.sheetSize)
  })
}
function openViewer() {
  if (!detail.value?.defaultProjectDrawingNo) return
  const drawingId = detail.value.defaultProjectDrawingNo
  confirmDiscard(() => { void router.push({ name: RouteName.DrawingViewer, params: { drawingId }, query: { fileId: attachmentId.value } }) })
}
function openProject(drawingNo: string) {
  confirmDiscard(() => { void router.push({ name: RouteName.DrawingPreview, params: { drawingId: drawingNo } }) })
}
function back() {
  confirmDiscard(() => { void router.push({ name: RouteName.PartIndexLibrary, query: route.query }) })
}
watch(attachmentId, () => {
  generation++; loadSequence++
  detail.value = null
  editing.value = false; saving.value = false; saved.value = false; newerInformation.value = false
  form.value = emptyPartIndexFields(); selectedSpaceId.value = null
  baseline.value = JSON.stringify({ fields: form.value, selectedSpaceId: null })
  void load()
}, { immediate: true })
watch(() => automaticPartIndex.revision, () => {
  if (automaticPartIndex.attachmentId !== attachmentId.value) return
  if (editing.value || saving.value) newerInformation.value = true
  else void load()
})
onBeforeUnmount(() => { generation++; loadSequence++ })
</script>

<template>
  <div class="page part-index-page part-index-detail">
    <header class="detail-navigation">
      <button class="back-link" type="button" :disabled="saving" @click="back"><DemoIcon name="arrow-left" :size="16" />返回列表</button>
      <button v-if="detail" class="btn primary" type="button" :disabled="saving || !detail.defaultProjectDrawingNo" @click="openViewer"><DemoIcon name="eye" :size="16" />查看图纸</button>
    </header>
    <p v-if="error" class="index-panel index-error" role="alert">{{ error }} <button class="link" type="button" :disabled="saving" @click="reload">{{ conflict ? '重新加载并核对' : '重试' }}</button></p>
    <div v-if="loading && !detail" class="index-panel index-empty" role="status">正在加载零件信息…</div>
    <section v-else-if="detail" class="index-panel detail-panel" :aria-busy="loading || saving">
      <div class="detail-title">
        <h1>零件信息</h1>
        <span v-if="saved && !editing" class="save-success" role="status">修改已保存</span>
        <button v-if="detail.canWrite && !editing" class="btn" type="button" :disabled="loading" @click="editing = true; saved = false">编辑信息</button>
      </div>
      <p v-if="newerInformation" class="detail-notice" role="status">图纸信息已更新。<button class="link" type="button" :disabled="saving" @click="reload">重新加载</button></p>
      <p v-else-if="detail.extractionStatus === 'pending'" class="detail-notice" role="status">零件信息正在自动补充。</p>
      <p v-else-if="detail.extractionStatus === 'failed'" class="detail-notice" role="status">部分信息暂未读取成功，系统会自动重试。</p>
      <form @submit.prevent="save">
        <div v-if="editing && (selectableSpaces.length > 1 || detail.selectedSpaceMissing)" class="layout-choice">
          <label for="part-layout">选择图纸布局</label>
          <select id="part-layout" :value="selectedSpaceId ?? ''" class="inp" :disabled="saving" @change="chooseSpace(($event.target as HTMLSelectElement).value)">
            <option value="">手动填写</option><option v-for="space in selectableSpaces" :key="space.id" :value="space.id">{{ space.name }}</option>
          </select>
        </div>
        <section v-for="group in groups" :key="group.label" class="detail-group">
          <h2>{{ group.label }}</h2>
          <div class="detail-fields">
            <div v-for="field in group.fields" :key="field.key" class="detail-field">
              <label :for="editing ? `part-${field.key}` : undefined">{{ field.label }}</label>
              <template v-if="editing">
                <input :id="`part-${field.key}`" v-model="form[field.key]" class="inp" :disabled="saving || loading" :placeholder="field.key === 'drawingDateRaw' ? 'YYYY-MM-DD' : `填写${field.label}`" />
                <small v-if="field.key === 'drawingDateRaw' && invalidDate" class="field-warning">建议使用 YYYY-MM-DD 格式。</small>
              </template>
              <p v-else :class="{ 'number-value': field.key === 'drawingNo', 'empty-value': !detail.fields[field.key] }">{{ detail.fields[field.key] || '—' }}</p>
            </div>
          </div>
        </section>
        <div class="detail-file">
          <DemoIcon name="file" :size="18" />
          <div><span class="detail-file-name">{{ detail.fileName }}</span><div class="project-tags"><button v-for="project in projects" :key="project.drawingId" type="button" class="project-tag" :disabled="saving" @click="openProject(project.drawingNo)">{{ partIndexProjectLabel(project) }}</button></div></div>
        </div>
        <footer v-if="editing" class="detail-edit-actions">
          <span>{{ isDirty ? '有未保存的修改' : '修改后可保存' }}</span>
          <button class="btn" type="button" :disabled="saving" @click="cancelEdit">取消</button>
          <button class="btn primary" type="submit" :disabled="saving || loading || !isDirty">{{ saving ? '保存中…' : '保存修改' }}</button>
        </footer>
      </form>
    </section>
  </div>
</template>
