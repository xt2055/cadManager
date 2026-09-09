<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { extractAndSaveTitleBlock, loadTitleBlock, savedTitleBlocks } from '@/services/drawing-title-block.service'
import { RouteName } from '@/router/route-names'

const props = defineProps<{
  files: Array<{ id: string; name: string; version?: string; role?: 'assembly' | 'part' | 'other' }>
  embedded?: boolean
  systemNo?: string
  material?: string
  partIndexEnabled?: boolean
}>()
const router = useRouter()
const files = computed(() => props.files.filter(file => /\.(exb|dwg|dxf)$/i.test(file.name)))
const fileId = ref('')
const spaceId = ref('')
const error = ref('')
const busy = ref(false)
const loading = ref(false)
let generation = 0
const record = computed(() => savedTitleBlocks[fileId.value])
const selectedFile = computed(() => files.value.find(file => file.id === fileId.value))
const canOpenPartIndex = computed(() => props.partIndexEnabled === true && selectedFile.value?.role === 'part')
const spaces = computed(() => record.value?.payload?.spaces ?? [])
const space = computed(() => spaces.value.find(item => item.id === spaceId.value))
const populatedSpaces = computed(() => spaces.value.filter(item => item.fields.some(field => field.value || field.candidates.length)))
watch(spaces, value => {
  spaceId.value = value.find(item => item.fields.some(field => field.value || field.candidates.length))?.id ?? value[0]?.id ?? ''
}, { immediate: true })
async function load() {
  const current = ++generation
  const id = fileId.value
  error.value = ''
  if (!id) { loading.value = false; return }
  loading.value = true
  try { await loadTitleBlock(id) }
  catch (cause) { if (current === generation) error.value = cause instanceof Error ? cause.message : '读取已保存信息失败' }
  finally { if (current === generation) loading.value = false }
}
watch(() => files.value.map(file => `${file.id}:${file.version ?? ''}`).join('|'), () => {
  if (!files.value.some(file => file.id === fileId.value)) fileId.value = files.value[0]?.id ?? ''
  else void load()
}, { immediate: true })
watch(fileId, () => void load(), { immediate: true })
async function extract() {
  const id = fileId.value
  busy.value = true
  error.value = ''
  try { await extractAndSaveTitleBlock(id, true) }
  catch (cause) { if (fileId.value === id) error.value = cause instanceof Error ? cause.message : '提取保存失败' }
  finally { busy.value = false }
}
function openPartIndex() {
  if (fileId.value) router.push({ name: RouteName.PartIndexDetail, params: { attachmentId: fileId.value } })
}
const currentFields = computed(() => loading.value || error.value || record.value?.payload?.error ? [] : space.value?.fields ?? [])
const titleName = computed(() => currentFields.value.find(field => field.key === 'name')?.value || '')
const titleNumber = computed(() => currentFields.value.find(field => field.key === 'number')?.value || props.systemNo || '')
const visibleFields = computed(() => currentFields.value.filter(field => {
  if (field.key === 'date' && !field.value && !field.candidates.length) return false
  if (!props.embedded) return true
  if (field.key === 'name' && field.value) return false
  if (field.key === 'material') return false
  if (field.key === 'number' && !field.candidates.length && !field.value) return false
  if (field.key === 'number' && field.value) return false
  return true
}))
const materialField = computed(() => currentFields.value.find(field => field.key === 'material'))
</script>

<template>
  <section class="saved-title" :class="{ embedded }">
    <div class="saved-title-head">
      <slot name="heading" :title-name="titleName" :title-number="titleNumber"><h3>图纸信息</h3></slot>
      <div class="saved-title-actions">
        <button v-if="canOpenPartIndex && fileId" class="extract-action" type="button" :disabled="busy || loading" @click="openPartIndex">零件索引</button>
        <button v-if="record?.canWrite" class="extract-action" type="button" :disabled="busy || loading" @click="extract">{{ busy ? '提取中…' : record.payload ? '重新提取' : '提取信息' }}</button>
      </div>
    </div>
    <p v-if="!files.length">暂无 CAD 图纸文件。</p>
    <template v-else>
      <label v-if="files.length > 1" class="saved-title-selector">
        <select v-model="fileId" aria-label="切换图纸文件" :disabled="busy"><option v-for="file in files" :key="file.id" :value="file.id">{{ file.name }}</option></select>
      </label>
      <p v-if="loading" role="status">正在加载…</p>
      <p v-if="error" role="alert">{{ error }} <button class="btn" type="button" :disabled="busy || loading" @click="load">重新读取</button></p>
      <template v-if="record && !loading && !error">
        <p v-if="!record.payload">{{ record.hasPrevious ? '图纸已更新，请重新提取。' : '暂无图纸信息，可点击“提取信息”。' }}</p>
        <p v-else-if="record.payload.error" role="alert">上次提取失败：{{ record.payload.error }}</p>
        <template v-else>
          <label v-if="populatedSpaces.length > 1" class="saved-title-selector">
            <select v-model="spaceId" aria-label="切换图纸布局"><option v-for="item in populatedSpaces" :key="item.id" :value="item.id">{{ item.name }}</option></select>
          </label>
          <p v-if="!space?.fields.some(field => field.value || field.candidates.length)">该空间未识别到标题栏信息，文字可能已转成线条或图片，或标题栏格式暂不支持。</p>
          <p v-for="warning in space?.warnings" :key="warning">{{ warning }}</p>
        </template>
      </template>
    </template>
    <dl v-if="embedded || visibleFields.length" class="saved-title-grid">
      <div v-if="embedded">
        <dt>材料</dt><dd>{{ materialField?.value || material || '—' }}</dd>
        <p v-if="!material && !materialField?.value && materialField?.candidates.length" class="saved-title-note">待核对候选：{{ materialField.candidates.join('、') }}</p>
      </div>
      <slot name="fields" />
      <div v-for="field in visibleFields" :key="field.key">
        <dt>{{ field.label }}</dt>
        <dd>{{ field.value || '—' }}<small v-if="field.value && field.source.includes('待核对')" class="review-hint" :title="field.source" tabindex="0" :aria-label="field.source">ⓘ</small></dd>
        <p v-if="!field.value && field.candidates.length" class="saved-title-note">待核对候选：{{ field.candidates.join('、') }}</p>
      </div>
    </dl>
  </section>
</template>

<style scoped>
.saved-title { margin: 20px 0; padding: 20px 0 0; border-top: 1px solid var(--line); }
.extract-action { border: 0; background: transparent; color: var(--text-2); font: inherit; font-size: 12px; padding: 5px 8px; border-radius: 4px; cursor: pointer; }
.extract-action:hover { color: var(--text-1); background: var(--panel); }
.extract-action:focus-visible { outline: 2px solid var(--text-2); outline-offset: 2px; }
.extract-action:disabled { opacity: 0.5; cursor: default; }
.review-hint { margin-left: 8px; white-space: nowrap; }
.saved-title-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.saved-title-actions { display: inline-flex; align-items: center; gap: 4px; }
.saved-title-head h3 { margin: 0; font-size: 15px; }
.saved-title.embedded { margin: 0; padding: 0; border: 0; }
.saved-title-head .extract-action { flex-shrink: 0; }
.saved-title-grid :deep(dt) { color: var(--text-2); font-size: 12px; }
.saved-title-grid :deep(dd) { margin: 6px 0; font-size: 14px; overflow-wrap: anywhere; }
.saved-title-note, .saved-title-grid small { color: var(--text-2); font-size: 12px; line-height: 1.7; overflow-wrap: anywhere; }
.saved-title-selector { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; font-size: 13px; margin: 12px 0; }
.saved-title-selector select { max-width: 100%; padding: 8px 12px; border: 1px solid var(--line); border-radius: 6px; color: var(--text-1); background: var(--panel); }
.saved-title-grid { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 18px; }
.saved-title-grid dt { color: var(--text-2); font-size: 12px; }
.saved-title-grid dd { margin: 6px 0; font-size: 14px; overflow-wrap: anywhere; }
@media(max-width: 850px) { .saved-title-grid { grid-template-columns: repeat(2,minmax(0,1fr)); } }
</style>
