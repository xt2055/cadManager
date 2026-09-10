<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElOption, ElSelect } from 'element-plus'
import 'element-plus/es/components/select/style/css'
import 'element-plus/es/components/option/style/css'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { partIndexService, type PartIndexFilters, type PartIndexItem, type PartIndexOption, type PartIndexPage } from '@/services/part-index.service'
import { automaticPartIndex } from '@/services/part-index-auto.service'
import { uniquePartIndexProjects, partIndexProjectLabel } from '../part-index.helpers'
import '../part-index.css'

defineOptions({ name: 'PartIndexLibraryPage' })
type OptionKind = 'project' | 'material' | 'designer'
const route = useRoute()
const router = useRouter()
const result = ref<PartIndexPage>({ list: [], total: 0, page: 1, pageSize: 20 })
const loading = ref(false)
const error = ref('')
const keyword = ref('')
const projectId = ref('')
const material = ref('')
const designer = ref('')
const options = ref<Record<OptionKind, PartIndexOption[]>>({ project: [], material: [], designer: [] })
const optionLoading = ref<Record<OptionKind, boolean>>({ project: false, material: false, designer: false })
const optionError = ref<Record<OptionKind, boolean>>({ project: false, material: false, designer: false })
const optionHasMore = ref<Record<OptionKind, boolean>>({ project: false, material: false, designer: false })
const optionSequence = { project: 0, material: 0, designer: 0 }
const optionTimers: Partial<Record<OptionKind, ReturnType<typeof setTimeout>>> = {}
let requestController: AbortController | null = null
let sequence = 0
let keywordTimer: ReturnType<typeof setTimeout> | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let disposed = false
const hasFilters = computed(() => Boolean(keyword.value || projectId.value || material.value || designer.value))
const pageCount = computed(() => Math.max(1, Math.ceil(result.value.total / result.value.pageSize)))
const rows = computed(() => [...new Map(result.value.list.map(item => [item.attachmentId, item])).values()])
const optionDefinitions: Array<{ kind: OptionKind; label: string }> = [
  { kind: 'project', label: '项目' }, { kind: 'material', label: '材料' }, { kind: 'designer', label: '设计人' },
]
function queryValue(value: unknown): string {
  return Array.isArray(value) ? String(value[0] ?? '') : typeof value === 'string' ? value : ''
}
function currentPage() { return Math.max(1, Number.parseInt(queryValue(route.query.page), 10) || 1) }
function filters(page = currentPage()): PartIndexFilters {
  return { page, pageSize: 20, keyword: keyword.value.trim(), projectId: projectId.value, material: material.value, designer: designer.value }
}
async function applyFilters(page = 1) {
  clearTimeout(keywordTimer)
  const query: Record<string, string> = {}
  if (page > 1) query.page = String(page)
  if (keyword.value.trim()) query.keyword = keyword.value.trim()
  if (projectId.value) query.project_id = projectId.value
  if (material.value) query.material = material.value
  if (designer.value) query.designer = designer.value
  await router.replace({ name: RouteName.PartIndexLibrary, query })
}
function searchInput() {
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => void applyFilters(), 300)
}
function clearFilters() {
  keyword.value = ''; projectId.value = ''; material.value = ''; designer.value = ''
  void applyFilters()
}
async function load(quiet = false) {
  if (disposed) return
  requestController?.abort()
  const controller = new AbortController()
  requestController = controller
  const current = ++sequence
  if (!quiet) loading.value = true
  error.value = ''
  try {
    const page = await partIndexService.list(filters(), controller.signal)
    if (disposed || current !== sequence) return
    if (page.page > 1 && !page.list.length) {
      await applyFilters(Math.max(1, Math.ceil(page.total / page.pageSize)))
      return
    }
    result.value = page
  } catch (cause) {
    if (!disposed && !controller.signal.aborted && current === sequence) error.value = cause instanceof Error ? cause.message : '加载失败，请重试'
  } finally {
    if (!disposed && current === sequence) loading.value = false
  }
}
function selectedValue(kind: OptionKind) { return kind === 'project' ? projectId.value : kind === 'material' ? material.value : designer.value }
function selectFilter(kind: OptionKind, value: string) {
  if (kind === 'project') projectId.value = value
  else if (kind === 'material') material.value = value
  else designer.value = value
  void applyFilters()
}
async function loadOptions(kind: OptionKind, search = '') {
  const current = ++optionSequence[kind]
  optionLoading.value[kind] = true
  optionError.value[kind] = false
  try {
    const page = await partIndexService.options(kind, search, 100)
    if (disposed || current !== optionSequence[kind]) return
    const selected = options.value[kind].find(option => option.value === selectedValue(kind))
    options.value[kind] = [...new Map([...page.list, ...(selected ? [selected] : [])].map(option => [option.value, option])).values()]
    optionHasMore.value[kind] = page.hasMore
  } catch {
    if (!disposed && current === optionSequence[kind]) optionError.value[kind] = true
  } finally {
    if (!disposed && current === optionSequence[kind]) optionLoading.value[kind] = false
  }
}
function searchOptions(kind: OptionKind, search: string) {
  clearTimeout(optionTimers[kind])
  optionSequence[kind]++
  optionTimers[kind] = setTimeout(() => void loadOptions(kind, search), 250)
}
function openDetail(item: PartIndexItem) {
  void router.push({ name: RouteName.PartIndexDetail, params: { attachmentId: item.attachmentId }, query: route.query })
}
function openViewer(item: PartIndexItem) {
  const project = uniquePartIndexProjects(item.projects)[0]
  if (project) void router.push({ name: RouteName.DrawingViewer, params: { drawingId: project.drawingNo }, query: { fileId: item.attachmentId } })
}
watch(() => route.query, () => {
  clearTimeout(keywordTimer)
  keyword.value = queryValue(route.query.keyword)
  projectId.value = queryValue(route.query.project_id)
  material.value = queryValue(route.query.material)
  designer.value = queryValue(route.query.designer)
  void load()
}, { deep: true, immediate: true })
for (const { kind } of optionDefinitions) void loadOptions(kind)
watch(() => automaticPartIndex.revision, () => {
  if (refreshTimer) return
  refreshTimer = setTimeout(() => { refreshTimer = undefined; void load(true) }, 800)
})
onBeforeUnmount(() => {
  disposed = true
  requestController?.abort()
  clearTimeout(keywordTimer)
  clearTimeout(refreshTimer)
  Object.values(optionTimers).forEach(timer => clearTimeout(timer))
})
</script>

<template>
  <div class="page part-index-page part-index-library">
    <section class="index-panel search-panel" aria-label="查找零件">
      <form @submit.prevent="applyFilters()">
        <div class="search-line">
          <div class="index-search">
            <DemoIcon name="search" :size="20" />
            <input v-model="keyword" aria-label="搜索零件" placeholder="搜索图号、零件名称、项目、材料或设计人" @input="searchInput" />
            <button v-if="keyword" class="clear-search" type="button" aria-label="清空搜索" @click="keyword = ''; applyFilters()">×</button>
          </div>
          <button class="btn primary" type="submit">搜索</button>
        </div>
        <div class="filter-line">
          <div v-for="filter in optionDefinitions" :key="filter.kind" class="filter-field">
            <label :for="`part-filter-${filter.kind}`">{{ filter.label }}</label>
            <ElSelect :id="`part-filter-${filter.kind}`" :model-value="selectedValue(filter.kind)" :aria-label="filter.label" :placeholder="`全部${filter.label}`" filterable clearable remote :remote-method="value => searchOptions(filter.kind, value)" :loading="optionLoading[filter.kind]" :no-data-text="optionError[filter.kind] ? '加载失败，请重新打开' : '没有匹配选项'" :teleported="false" @update:model-value="value => selectFilter(filter.kind, value ?? '')" @visible-change="visible => visible && loadOptions(filter.kind)">
              <ElOption v-if="selectedValue(filter.kind) && !options[filter.kind].some(option => option.value === selectedValue(filter.kind))" :value="selectedValue(filter.kind)" :label="filter.kind === 'project' ? '已选项目' : selectedValue(filter.kind)" />
              <ElOption v-for="option in options[filter.kind]" :key="option.value" :value="option.value" :label="option.label" />
              <template v-if="optionHasMore[filter.kind]" #footer><small>输入关键词查找更多{{ filter.label }}</small></template>
            </ElSelect>
          </div>
          <button class="reset-filters" type="button" :disabled="!hasFilters" @click="clearFilters">清空筛选</button>
        </div>
      </form>
    </section>
    <section class="index-panel results-panel" aria-label="零件搜索结果" :aria-busy="loading">
      <div class="results-heading">
        <span>共 <strong>{{ result.total }}</strong> 个零件</span>
        <div class="results-tools">
          <span v-if="automaticPartIndex.running" class="auto-progress" role="status"><span class="activity-dot" />正在补充零件信息 {{ automaticPartIndex.completed }}/{{ automaticPartIndex.total }}</span>
          <button class="btn sm" type="button" :disabled="loading" @click="load()">{{ loading ? '刷新中…' : '刷新' }}</button>
        </div>
      </div>
      <p v-if="error" class="index-error" role="alert">{{ error }} <button class="link" type="button" @click="load()">重试</button></p>
      <div v-if="loading && !rows.length" class="index-empty" role="status">正在加载零件…</div>
      <div v-else-if="!rows.length && !error" class="index-empty">
        <DemoIcon name="search" :size="30" />
        <strong>{{ hasFilters ? '没有找到匹配的零件' : '暂无零件图纸' }}</strong>
        <p>{{ hasFilters ? '试试其他关键词，或清空筛选条件。' : '上传零件图纸后，零件信息将自动出现在这里。' }}</p>
        <button v-if="hasFilters" class="btn" type="button" @click="clearFilters">清空筛选</button>
      </div>
      <div v-else class="index-table-scroll">
        <table class="index-table">
          <thead><tr><th>零件名称</th><th>图号</th><th>材料</th><th>设计人</th><th>图纸日期</th><th>文件 / 项目</th><th class="actions-column">操作</th></tr></thead>
          <tbody>
            <tr v-for="item in rows" :key="item.attachmentId">
              <td><button class="part-name" type="button" @click="openDetail(item)">{{ item.partName || '未命名零件' }}</button></td>
              <td class="drawing-number">{{ item.drawingNo || item.registeredPartNo || '—' }}</td>
              <td>{{ item.material || '—' }}</td><td>{{ item.designer || '—' }}</td><td class="drawing-date">{{ item.drawingDate || item.drawingDateRaw || '—' }}</td>
              <td class="file-project-cell">
                <span class="file-name" :title="item.fileName">{{ item.fileName }}</span>
                <div class="project-tags"><button v-for="project in uniquePartIndexProjects(item.projects)" :key="project.drawingId" type="button" class="project-tag" @click="router.push({ name: RouteName.DrawingPreview, params: { drawingId: project.drawingNo } })">{{ partIndexProjectLabel(project) }}</button></div>
              </td>
              <td><div class="row-actions"><button class="btn sm" type="button" :disabled="!item.projects?.length" @click="openViewer(item)">查看图纸</button><button class="link" type="button" @click="openDetail(item)">详情</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
      <footer v-if="result.total > 0" class="index-pager">
        <span>显示 {{ (result.page - 1) * result.pageSize + 1 }}–{{ Math.min(result.page * result.pageSize, result.total) }} / {{ result.total }}</span>
        <div><button class="btn sm" type="button" :disabled="loading || result.page <= 1" @click="applyFilters(result.page - 1)">上一页</button><span>{{ result.page }} / {{ pageCount }}</span><button class="btn sm" type="button" :disabled="loading || result.page >= pageCount" @click="applyFilters(result.page + 1)">下一页</button></div>
      </footer>
    </section>
  </div>
</template>
