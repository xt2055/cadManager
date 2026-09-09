<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { useAuthStore } from '@/stores/auth.store'
import { extractAndSaveTitleBlock } from '@/services/drawing-title-block.service'
import {
  partIndexService,
  type PartIndexFilters,
  type PartIndexBackfillError,
  type PartIndexItem,
  type PartIndexOption,
  type PartIndexPage,
  type PartIndexStatus,
} from '@/services/part-index.service'
import {
  appendPartIndexBackfillErrors,
  canBatchExtract,
  extractAndRebuildPartIndex,
  partIndexStatusLabels,
  runSerialPartIndexBatch,
  type BatchRunResult,
} from '../part-index.helpers'

defineOptions({ name: 'PartIndexLibraryPage' })
type OptionKind = 'project' | 'material' | 'designer'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const result = ref<PartIndexPage>({ list: [], total: 0, page: 1, pageSize: 20 })
const loading = ref(false)
const error = ref('')
const keyword = ref('')
const projectId = ref('')
const material = ref('')
const designer = ref('')
const dateFrom = ref('')
const dateTo = ref('')
const status = ref<PartIndexStatus | ''>('')
const projectOptions = ref<PartIndexOption[]>([])
const materialOptions = ref<PartIndexOption[]>([])
const designerOptions = ref<PartIndexOption[]>([])
const projectOptionKeyword = ref('')
const materialOptionKeyword = ref('')
const designerOptionKeyword = ref('')
const optionHasMore = ref<Record<OptionKind, boolean>>({ project: false, material: false, designer: false })
const selectedAttachmentIds = ref<string[]>([])
const batchRunning = ref(false)
const stopBatchRequested = ref(false)
const batchResult = ref<BatchRunResult | null>(null)
const backfilling = ref(false)
const backfillSummary = ref('')
const stopBackfillRequested = ref(false)
const backfillErrors = ref<PartIndexBackfillError[]>([])
const omittedBackfillErrors = ref(0)
const maximumBackfillErrors = 100

let requestController: AbortController | null = null
let requestSequence = 0
let keywordTimer: ReturnType<typeof setTimeout> | null = null
let applyingRoute = false
let skipNextKeywordRouteUpdate = false
let disposed = false
const optionRequestSequence: Record<OptionKind, number> = { project: 0, material: 0, designer: 0 }
const optionTimers: Partial<Record<OptionKind, ReturnType<typeof setTimeout>>> = {}

function queryValue(value: unknown): string {
  return Array.isArray(value) ? String(value[0] ?? '') : typeof value === 'string' ? value : ''
}

function queryPage(value: unknown): number {
  const parsed = Number.parseInt(queryValue(value), 10)
  return Number.isInteger(parsed) && parsed >= 1 ? parsed : 1
}

function restoreFilters() {
  if (keywordTimer) {
    clearTimeout(keywordTimer)
    keywordTimer = null
  }
  applyingRoute = true
  const nextKeyword = queryValue(route.query.keyword)
  if (keyword.value !== nextKeyword) skipNextKeywordRouteUpdate = true
  keyword.value = nextKeyword
  projectId.value = queryValue(route.query.project_id)
  material.value = queryValue(route.query.material)
  designer.value = queryValue(route.query.designer)
  dateFrom.value = queryValue(route.query.date_from)
  dateTo.value = queryValue(route.query.date_to)
  const requestedStatus = queryValue(route.query.status)
  status.value = ['pending', 'failed', 'needs_confirmation', 'confirmed', 'recheck'].includes(requestedStatus)
    ? requestedStatus as PartIndexStatus
    : ''
  applyingRoute = false
}

function filters(page = queryPage(route.query.page)): PartIndexFilters {
  return {
    page,
    pageSize: 20,
    keyword: keyword.value.trim(),
    projectId: projectId.value,
    material: material.value,
    designer: designer.value,
    dateFrom: dateFrom.value,
    dateTo: dateTo.value,
    status: status.value,
  }
}

function queryFor(filtersValue: PartIndexFilters): Record<string, string> {
  const query: Record<string, string> = {}
  if (filtersValue.page > 1) query.page = String(filtersValue.page)
  if (filtersValue.keyword) query.keyword = filtersValue.keyword
  if (filtersValue.projectId) query.project_id = filtersValue.projectId
  if (filtersValue.material) query.material = filtersValue.material
  if (filtersValue.designer) query.designer = filtersValue.designer
  if (filtersValue.dateFrom) query.date_from = filtersValue.dateFrom
  if (filtersValue.dateTo) query.date_to = filtersValue.dateTo
  if (filtersValue.status) query.status = filtersValue.status
  return query
}

async function replaceRoute(page = 1) {
  await router.replace({ name: RouteName.PartIndexLibrary, query: queryFor(filters(page)) })
}

async function load() {
  if (disposed) return
  requestController?.abort()
  const controller = new AbortController()
  requestController = controller
  const sequence = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const page = await partIndexService.list(filters(), controller.signal)
    if (disposed || sequence !== requestSequence) return
    result.value = page
    const visible = new Set(page.list.filter(canBatchExtract).map((item) => item.attachmentId))
    selectedAttachmentIds.value = selectedAttachmentIds.value.filter((id) => visible.has(id))
  } catch (cause) {
    if (disposed || controller.signal.aborted || sequence !== requestSequence) return
    error.value = cause instanceof Error ? cause.message : '加载零件索引失败'
  } finally {
    if (!disposed && sequence === requestSequence) loading.value = false
  }
}

function optionKeyword(kind: OptionKind): string {
  if (kind === 'project') return projectOptionKeyword.value
  if (kind === 'material') return materialOptionKeyword.value
  return designerOptionKeyword.value
}

function setOptions(kind: OptionKind, options: PartIndexOption[]) {
  if (kind === 'project') projectOptions.value = options
  if (kind === 'material') materialOptions.value = options
  if (kind === 'designer') designerOptions.value = options
}

async function loadOptions(kind: OptionKind, keywordValue = optionKeyword(kind)) {
  if (disposed) return
  const sequence = ++optionRequestSequence[kind]
  try {
    const page = await partIndexService.options(kind, keywordValue, 50)
    if (disposed || sequence !== optionRequestSequence[kind]) return
    setOptions(kind, page.list)
    optionHasMore.value = { ...optionHasMore.value, [kind]: page.hasMore }
  } catch {
    // 筛选选项不可用时，关键字和其他条件仍可以查询。
  }
}

function scheduleOptionLoad(kind: OptionKind) {
  const timer = optionTimers[kind]
  if (timer) clearTimeout(timer)
  optionTimers[kind] = setTimeout(() => void loadOptions(kind), 300)
}

function applyFilters() {
  void replaceRoute(1)
}

function clearFilters() {
  keyword.value = ''
  projectId.value = ''
  material.value = ''
  designer.value = ''
  dateFrom.value = ''
  dateTo.value = ''
  status.value = ''
  void replaceRoute(1)
}

function openDetail(item: PartIndexItem) {
  router.push({
    name: RouteName.PartIndexDetail,
    params: { attachmentId: item.attachmentId },
    query: route.query,
  })
}

function openProject(item: PartIndexItem) {
  const project = item.projects[0]
  if (project) router.push({ name: RouteName.DrawingPreview, params: { drawingId: project.drawingNo } })
}

function statusClass(value: PartIndexStatus) {
  return `status-${value}`
}

function isSelected(attachmentId: string) {
  return selectedAttachmentIds.value.includes(attachmentId)
}

function toggleSelected(attachmentId: string, checked: boolean) {
  selectedAttachmentIds.value = checked
    ? [...new Set([...selectedAttachmentIds.value, attachmentId])]
    : selectedAttachmentIds.value.filter((id) => id !== attachmentId)
}

const selectedItems = computed(() => result.value.list.filter((item) => selectedAttachmentIds.value.includes(item.attachmentId) && canBatchExtract(item)))
const hasFilters = computed(() => Boolean(keyword.value || projectId.value || material.value || designer.value || dateFrom.value || dateTo.value || status.value))

async function extractSelected() {
  const items = selectedItems.value
  if (disposed || !items.length || batchRunning.value) return
  batchRunning.value = true
  stopBatchRequested.value = false
  batchResult.value = { completed: 0, succeeded: 0, failures: [], stopped: false }
  try {
    batchResult.value = await runSerialPartIndexBatch(
      items,
      () => stopBatchRequested.value,
      (item) => extractAndRebuildPartIndex(
        () => extractAndSaveTitleBlock(item.attachmentId, item.extractionStatus === 'failed'),
        (versionId, snapshotRevision) => partIndexService.rebuild(item.attachmentId, versionId, snapshotRevision),
      ),
      (progress) => {
        if (!disposed) batchResult.value = progress
      },
    )
  } finally {
    if (!disposed) {
      batchRunning.value = false
      await load()
    }
  }
}

async function backfill() {
  if (disposed || !authStore.hasRole('admin') || backfilling.value) return
  if (!window.confirm('将仅根据已有标题栏快照补建索引，不会读取 CAD 文件。是否继续？')) return
  backfilling.value = true
  stopBackfillRequested.value = false
  backfillSummary.value = ''
  backfillErrors.value = []
  omittedBackfillErrors.value = 0
  let cursor: string | null = null
  const totals = { scanned: 0, created: 0, updated: 0, unchanged: 0, skipped: 0, failed: 0 }
  try {
    do {
      const page = await partIndexService.backfill({ afterAttachmentId: cursor, limit: 50 })
      if (disposed) return
      totals.scanned += page.scanned
      totals.created += page.created
      totals.updated += page.updated
      totals.unchanged += page.unchanged
      totals.skipped += page.skipped
      totals.failed += page.failed
      const collected = appendPartIndexBackfillErrors(backfillErrors.value, page.errors, maximumBackfillErrors)
      backfillErrors.value = collected.items
      omittedBackfillErrors.value += collected.omitted
      cursor = page.hasMore ? page.nextCursor : null
    } while (cursor && !stopBackfillRequested.value)
    if (!disposed) {
      backfillSummary.value = `${stopBackfillRequested.value ? '补建已停止' : '补建完成'}：扫描 ${totals.scanned}，新建 ${totals.created}，更新 ${totals.updated}，失败 ${totals.failed}。`
      await load()
    }
  } catch (cause) {
    if (!disposed) backfillSummary.value = cause instanceof Error ? cause.message : '补建索引失败'
  } finally {
    if (!disposed) backfilling.value = false
  }
}

watch(keyword, () => {
  if (applyingRoute || skipNextKeywordRouteUpdate) {
    skipNextKeywordRouteUpdate = false
    return
  }
  if (keywordTimer) clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => void replaceRoute(1), 300)
})

watch(() => route.query, () => {
  restoreFilters()
  void loadOptions('project')
  void loadOptions('material')
  void loadOptions('designer')
  void load()
}, { deep: true, immediate: true })

watch(projectOptionKeyword, () => scheduleOptionLoad('project'))
watch(materialOptionKeyword, () => scheduleOptionLoad('material'))
watch(designerOptionKeyword, () => scheduleOptionLoad('designer'))

onBeforeUnmount(() => {
  disposed = true
  requestSequence += 1
  optionRequestSequence.project += 1
  optionRequestSequence.material += 1
  optionRequestSequence.designer += 1
  stopBatchRequested.value = true
  stopBackfillRequested.value = true
  requestController?.abort()
  if (keywordTimer) clearTimeout(keywordTimer)
  Object.values(optionTimers).forEach((timer) => {
    if (timer) clearTimeout(timer)
  })
})
</script>

<template>
  <div class="page part-index-library">
    <header class="page-head">
      <div>
        <div class="eyebrow"><DemoIcon name="boxes" :size="14" /> CAD 数据索引</div>
        <h1>零件索引</h1>
        <p>按零件、图号、项目或标题栏字段反查历史图纸；原始 DWG 始终保留在来源项目中。</p>
      </div>
      <span class="total-count">共 {{ result.total }} 条</span>
    </header>

    <section class="card filters-card">
      <form class="filter-grid" @submit.prevent="applyFilters">
        <label class="keyword-field">
          <span>搜索</span>
          <div class="search-box">
            <DemoIcon name="search" :size="15" />
            <input v-model="keyword" placeholder="图号、零件名称、项目编号、材料、设计人" />
          </div>
        </label>
        <label>
          <span>项目</span>
          <input v-model="projectOptionKeyword" class="inp option-search" placeholder="输入项目继续筛选" @focus="loadOptions('project')" />
          <select v-model="projectId" class="inp" @focus="loadOptions('project')" @change="applyFilters">
            <option value="">全部项目</option>
            <option v-if="projectId && !projectOptions.some((option) => option.value === projectId)" :value="projectId">{{ projectId }}</option>
            <option v-for="option in projectOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
          <small v-if="optionHasMore.project">结果较多，请继续输入项目关键词。</small>
        </label>
        <label>
          <span>材料</span>
          <input v-model="materialOptionKeyword" class="inp option-search" placeholder="输入材料继续筛选" @focus="loadOptions('material')" />
          <select v-model="material" class="inp" @focus="loadOptions('material')" @change="applyFilters">
            <option value="">全部材料</option>
            <option v-if="material && !materialOptions.some((option) => option.value === material)" :value="material">{{ material }}</option>
            <option v-for="option in materialOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
          <small v-if="optionHasMore.material">结果较多，请继续输入材料关键词。</small>
        </label>
        <label>
          <span>设计人</span>
          <input v-model="designerOptionKeyword" class="inp option-search" placeholder="输入设计人继续筛选" @focus="loadOptions('designer')" />
          <select v-model="designer" class="inp" @focus="loadOptions('designer')" @change="applyFilters">
            <option value="">全部设计人</option>
            <option v-if="designer && !designerOptions.some((option) => option.value === designer)" :value="designer">{{ designer }}</option>
            <option v-for="option in designerOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
          <small v-if="optionHasMore.designer">结果较多，请继续输入设计人关键词。</small>
        </label>
        <label>
          <span>开始日期</span>
          <input v-model="dateFrom" class="inp" type="date" @change="applyFilters" />
        </label>
        <label>
          <span>结束日期</span>
          <input v-model="dateTo" class="inp" type="date" @change="applyFilters" />
        </label>
        <label>
          <span>状态</span>
          <select v-model="status" class="inp" @change="applyFilters">
            <option value="">全部状态</option>
            <option v-for="(label, value) in partIndexStatusLabels" :key="value" :value="value">{{ label }}</option>
          </select>
        </label>
        <div class="filter-actions">
          <button class="btn primary" type="submit" :disabled="loading">查询</button>
          <button class="btn" type="button" :disabled="loading" @click="clearFilters">重置</button>
        </div>
      </form>
    </section>

    <section class="toolbar">
      <div>
        <button class="btn" type="button" :disabled="batchRunning || !selectedItems.length" @click="extractSelected">
          {{ batchRunning ? '提取中…' : `提取所选${selectedItems.length ? `（${selectedItems.length}）` : ''}` }}
        </button>
        <button v-if="batchRunning" class="btn" type="button" @click="stopBatchRequested = true">停止（当前文件完成后）</button>
      </div>
      <div v-if="authStore.hasRole('admin')" class="toolbar-admin-actions">
        <button v-if="backfilling" class="btn" type="button" @click="stopBackfillRequested = true">停止补建（当前批完成后）</button>
        <button v-else class="btn" type="button" :disabled="batchRunning" @click="backfill">补建已有快照索引</button>
      </div>
    </section>
    <p v-if="batchResult" class="batch-message">
      提取进度：{{ batchResult.completed }} / {{ selectedItems.length || batchResult.completed }}，成功 {{ batchResult.succeeded }}，失败 {{ batchResult.failures.length }}{{ batchResult.stopped ? '（已停止）' : '' }}
      <span v-if="batchResult.failures.length">。{{ batchResult.failures.join('；') }}</span>
    </p>
    <p v-if="backfillSummary" class="batch-message">{{ backfillSummary }}</p>
    <details v-if="backfillErrors.length" class="backfill-errors">
      <summary>
        补建失败详情（显示 {{ backfillErrors.length }} 项<span v-if="omittedBackfillErrors">，另有 {{ omittedBackfillErrors }} 项未展开</span>）
      </summary>
      <ul>
        <li v-for="item in backfillErrors" :key="`${item.attachmentId}:${item.message}`">
          <code>{{ item.attachmentId }}</code>：{{ item.message }}
        </li>
      </ul>
    </details>
    <p v-if="error" class="error-message" role="alert">{{ error }} <button class="link" type="button" @click="load">重试</button></p>

    <section class="card table-card">
      <div v-if="loading" class="state-message">正在加载零件索引…</div>
      <div v-else class="table-scroll">
        <table class="tbl">
          <thead>
            <tr>
              <th>零件名称</th>
              <th>标题栏图号</th>
              <th>文件名</th>
              <th>关联项目</th>
              <th>材料</th>
              <th>设计人</th>
              <th>图纸日期</th>
              <th>处理状态</th>
              <th class="actions-column">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in result.list" :key="item.attachmentId">
              <td>
                <label v-if="canBatchExtract(item)" class="batch-check">
                  <input :checked="isSelected(item.attachmentId)" type="checkbox" @change="toggleSelected(item.attachmentId, ($event.target as HTMLInputElement).checked)" />
                  <strong>{{ item.partName || '未识别' }}</strong>
                </label>
                <strong v-else>{{ item.partName || '未识别' }}</strong>
              </td>
              <td class="mono">{{ item.drawingNo || item.registeredPartNo || '—' }}</td>
              <td class="mono">{{ item.fileName }}</td>
              <td>
                <button v-if="item.projects.length" class="link project-link" type="button" @click="openProject(item)">
                  {{ item.projects[0]?.projectCode || item.projects[0]?.drawingNo }}<span v-if="item.projectCount > 1">，另 {{ item.projectCount - 1 }} 个</span>
                </button>
                <span v-else>—</span>
                <details v-if="item.projectCount > 1">
                  <summary>查看全部关联项目</summary>
                  <button v-for="project in item.projects" :key="project.drawingId" class="link project-line" type="button" @click="router.push({ name: RouteName.DrawingPreview, params: { drawingId: project.drawingNo } })">
                    {{ project.projectCode || project.drawingNo }} / {{ project.projectName || project.drawingNo }}
                  </button>
                </details>
              </td>
              <td>{{ item.material || '—' }}</td>
              <td>{{ item.designer || '—' }}</td>
              <td class="mono">{{ item.drawingDate || item.drawingDateRaw || '—' }}</td>
              <td><span class="status-tag" :class="statusClass(item.status)">{{ partIndexStatusLabels[item.status] }}</span></td>
              <td class="row-actions"><button class="btn sm" type="button" @click="openDetail(item)">查看</button></td>
            </tr>
            <tr v-if="!result.list.length">
              <td colspan="9">
                <div class="state-message">{{ hasFilters ? '没有找到匹配的零件。' : '暂无零件 CAD 附件。' }}</div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <footer v-if="result.total > result.pageSize" class="pager">
        <span>第 {{ result.page }} / {{ Math.max(1, Math.ceil(result.total / result.pageSize)) }} 页</span>
        <div>
          <button class="btn sm" type="button" :disabled="loading || result.page <= 1" @click="replaceRoute(result.page - 1)">上一页</button>
          <button class="btn sm" type="button" :disabled="loading || result.page * result.pageSize >= result.total" @click="replaceRoute(result.page + 1)">下一页</button>
        </div>
      </footer>
    </section>
  </div>
</template>

<style scoped>
.part-index-library { display: flex; flex-direction: column; gap: 16px; min-height: 100%; padding: 22px 26px; }
.page-head, .toolbar { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
.toolbar { align-items: center; }
.toolbar > div, .filter-actions, .toolbar-admin-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.eyebrow { display: flex; align-items: center; gap: 6px; color: var(--text-2); font-size: 12px; }
h1 { margin: 6px 0 8px; font-size: 25px; }
.page-head p { margin: 0; color: var(--text-2); font-size: 13px; }
.total-count { margin-top: 8px; padding: 6px 10px; border: 1px solid var(--line); border-radius: 999px; color: var(--text-2); font-size: 12px; white-space: nowrap; }
.filters-card { padding: 16px; }
.filter-grid { display: grid; grid-template-columns: minmax(280px, 2fr) repeat(3, minmax(120px, 1fr)); gap: 12px; align-items: end; }
.filter-grid label { display: flex; flex-direction: column; gap: 6px; min-width: 0; color: var(--text-2); font-size: 12px; }
.filter-grid small { color: var(--text-2); font-size: 11px; line-height: 1.35; }
.option-search { min-height: 30px; font-size: 12px; }
.keyword-field { grid-column: span 2; }
.search-box { display: flex; align-items: center; gap: 8px; padding: 0 10px; height: 36px; border: 1px solid var(--line); border-radius: 6px; color: var(--text-2); background: var(--panel); }
.search-box input { width: 100%; border: 0; outline: 0; color: var(--text-1); background: transparent; font: inherit; }
.inp { width: 100%; min-height: 36px; }
.error-message, .batch-message { margin: 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); }
.error-message { color: var(--danger, #c43b3b); }
.batch-message { color: var(--text-2); font-size: 12px; line-height: 1.7; }
.backfill-errors { margin: 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 6px; color: var(--danger, #c43b3b); background: var(--panel); font-size: 12px; line-height: 1.6; }
.backfill-errors ul { max-height: 220px; margin: 8px 0 0; padding-left: 20px; overflow: auto; }
.backfill-errors code { color: var(--text-2); }
.table-card { overflow: hidden; }
.table-scroll { overflow-x: auto; }
table { width: 100%; min-width: 1120px; }
th { white-space: nowrap; }
.actions-column, .row-actions { text-align: center; }
.project-link, .project-line { text-align: left; }
.project-line { display: block; margin-top: 5px; }
details { margin-top: 5px; color: var(--text-2); font-size: 11px; }
summary { cursor: pointer; }
.batch-check { display: inline-flex; align-items: center; gap: 7px; cursor: pointer; }
.status-tag { display: inline-flex; padding: 3px 7px; border: 1px solid var(--line); border-radius: 999px; color: var(--text-2); font-size: 12px; white-space: nowrap; }
.status-confirmed { color: var(--success, #248a4d); }
.status-recheck, .status-failed { color: var(--danger, #c43b3b); }
.status-needs_confirmation { color: var(--warning, #9a6500); }
.state-message { padding: 42px 20px; color: var(--text-2); text-align: center; }
.pager { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 12px 16px; border-top: 1px solid var(--line); color: var(--text-2); font-size: 12px; }
.pager div { display: flex; gap: 8px; }
@media (max-width: 1100px) { .filter-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } .keyword-field { grid-column: span 3; } }
@media (max-width: 720px) { .part-index-library { padding: 16px; } .page-head { display: block; } .total-count { display: inline-flex; margin-top: 12px; } .filter-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .keyword-field { grid-column: span 2; } }
</style>
