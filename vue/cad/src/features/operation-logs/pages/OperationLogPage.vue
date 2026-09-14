<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import CandidateInput from '@/features/operation-logs/components/CandidateInput.vue'
import { mergeSelectOptions, paginationPages, paginationRange, type SelectOption } from '@/features/operation-logs/operation-log.helpers'
import { listOperationLogOptions, type OperationLogPage } from '@/services/drawing-operation-log.service'
import { useAuditStore } from '@/stores/audit.store'
import { ACTIVITY_LABELS, type ActivityLog } from '@/types/domain.types'

defineOptions({ name: 'OperationLogPage' })

type CandidateKind = 'drawing' | 'actor'

const auditStore = useAuditStore()
const page = ref(1)
const pageSize = ref(20)
const filterAct = ref<string>('')
const filterDrawing = ref('')
const filterUser = ref('')
const keyword = ref('')
const loading = ref(false)
const errorMessage = ref('')
const resultPage = ref<OperationLogPage>({ list: [], total: 0, page: 1, pageSize: 20 })
const options = ref<Record<CandidateKind, SelectOption[]>>({ drawing: [], actor: [] })
const optionLoading = ref<Record<CandidateKind, boolean>>({ drawing: false, actor: false })
const optionSequence = { drawing: 0, actor: 0 }
const optionTimers: Partial<Record<CandidateKind, ReturnType<typeof setTimeout>>> = {}
let requestGeneration = 0
let disposed = false

// 变更/审核类动作不在通用 ActivityType 枚举内，单独补充中文标签与配色。
const EXTRA_ACTION_LABELS: Record<string, string> = {
  change_request_create: '发起变更申请',
  change_request_approve: '审批通过变更',
  change_request_reject: '驳回变更申请',
  change_request_return: '退回修改',
  change_request_cancel: '终止变更工单',
  change_request_submit: '提交变更成果',
  change_request_verify: '完整审核通过',
  change_request_waive_verify: '免验收放行',
}

const ACTION_COLORS: Record<string, string> = {
  view: 'info', create: 'ok', edit: 'warn', branch: 'info', upload: 'warn',
  download: 'plain', delete: 'danger', check: 'info', parse: 'plain',
  change_request_create: 'info', change_request_approve: 'ok', change_request_reject: 'danger',
  change_request_return: 'warn', change_request_cancel: 'danger', change_request_submit: 'warn',
  change_request_verify: 'ok', change_request_waive_verify: 'mute',
}

const DETAIL_LABELS: Record<string, string> = {
  fileName: '文件', fileId: '文件ID', version: '版本', role: '角色', author: '作者',
  changedFields: '修改字段', oldPartNo: '原零件号', newPartNo: '新零件号',
  partCount: '零件数', fileCount: '文件数', sourceDrawingNo: '来源图号',
  newDrawingNo: '新图号', sourcePartNo: '来源零件号', count: '数量',
  importedCount: '导入条目', parentNo: '父件号', reason: '原因', operation: '操作',
  uploadSessionId: '上传会话', targetType: '目标类型',
}

function actionLabel(code: string): string {
  return (ACTIVITY_LABELS as Record<string, string>)[code] || EXTRA_ACTION_LABELS[code] || code
}

function actionTone(code: string): string {
  return ACTION_COLORS[code] || 'mute'
}

function formatDetailValue(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (Array.isArray(value)) return value.map(formatDetailValue).filter(Boolean).join('、')
  if (typeof value === 'object') {
    return Object.entries(value as Record<string, unknown>)
      .map(([key, val]) => `${key}: ${formatDetailValue(val)}`)
      .join('；')
  }
  return String(value)
}

function detailEntries(detail?: Record<string, unknown>): Array<{ label: string; value: string }> {
  if (!detail) return []
  return Object.entries(detail)
    .filter(([, value]) => value !== null && value !== undefined && value !== '')
    .map(([key, value]) => ({ label: DETAIL_LABELS[key] || key, value: formatDetailValue(value) }))
}

function canExpand(item: ActivityLog): boolean {
  return detailEntries(item.detail).length > 0
}

const expandedId = ref('')

function toggleDetail(id: string) {
  expandedId.value = expandedId.value === id ? '' : id
}

const actionOptions: Array<[string, string]> = [
  ...Object.entries(ACTIVITY_LABELS),
  ...Object.entries(EXTRA_ACTION_LABELS),
]
const rows = computed(() => resultPage.value.list)
const totalPages = computed(() => Math.max(1, Math.ceil(resultPage.value.total / pageSize.value)))
const pageNumbers = computed(() => paginationPages(page.value, totalPages.value))
const visibleRange = computed(() => paginationRange(page.value, pageSize.value, resultPage.value.total))

function selectedValue(kind: CandidateKind) {
  return kind === 'drawing' ? filterDrawing.value : filterUser.value
}

async function loadOptions(kind: CandidateKind, search = '') {
  const current = ++optionSequence[kind]
  optionLoading.value[kind] = true
  try {
    const result = await listOperationLogOptions(false, kind, search)
    if (disposed || current !== optionSequence[kind]) return
    options.value[kind] = mergeSelectOptions(result.list, selectedValue(kind))
  } finally {
    if (!disposed && current === optionSequence[kind]) optionLoading.value[kind] = false
  }
}

function searchOptions(kind: CandidateKind, search: string) {
  clearTimeout(optionTimers[kind])
  optionSequence[kind]++
  optionTimers[kind] = setTimeout(() => void loadOptions(kind, search), 250)
}

async function loadLogs() {
  const generation = ++requestGeneration
  loading.value = true
  errorMessage.value = ''
  try {
    const next = await auditStore.loadDrawing({
      page: page.value,
      pageSize: pageSize.value,
      action: filterAct.value || undefined,
      drawingNo: filterDrawing.value.trim() || undefined,
      actor: filterUser.value.trim() || undefined,
      keyword: keyword.value.trim() || undefined,
    })
    if (generation !== requestGeneration) return
    resultPage.value = next
  } catch (error) {
    if (generation !== requestGeneration) return
    errorMessage.value = error instanceof Error ? error.message : '操作记录读取失败'
    resultPage.value = { list: [], total: 0, page: page.value, pageSize: pageSize.value }
  } finally {
    if (generation === requestGeneration) loading.value = false
  }
}

function search() {
  page.value = 1
  void loadLogs()
}

function resetFilters() {
  filterAct.value = ''
  filterDrawing.value = ''
  filterUser.value = ''
  keyword.value = ''
  search()
}

function changePage(nextPage: number) {
  if (nextPage < 1 || nextPage > totalPages.value || nextPage === page.value) return
  page.value = nextPage
  void loadLogs()
}

function changePageSize() {
  page.value = 1
  void loadLogs()
}

onMounted(() => {
  void loadLogs()
})

onBeforeUnmount(() => {
  disposed = true
  for (const timer of Object.values(optionTimers)) clearTimeout(timer)
})
</script>

<template>
  <div class="page log-page">
    <div class="section-head">
      <div>
        <h3>操作记录</h3>
        <span class="lib-count">只读 · 追加式图纸操作记录 · 显示 {{ visibleRange }} / {{ resultPage.total }} 条</span>
      </div>
      <button class="btn" type="button" :disabled="loading" @click="loadLogs">
        <DemoIcon name="refresh-cw" :size="14" />刷新
      </button>
    </div>

    <!-- 筛选面板：对齐统一的设计语言 -->
    <div class="card log-filter-card">
      <div class="filter-grid">
        <div class="filter-field">
          <label>图号</label>
          <CandidateInput
            v-model="filterDrawing"
            placeholder="搜索或输入图号"
            :options="options.drawing"
            :loading="optionLoading.drawing"
            @search="kw => searchOptions('drawing', kw)"
            @select="() => search()"
            @enter="search"
          />
        </div>

        <div class="filter-field">
          <label>操作人</label>
          <CandidateInput
            v-model="filterUser"
            placeholder="搜索或输入姓名、账号"
            :options="options.actor"
            :loading="optionLoading.actor"
            @search="kw => searchOptions('actor', kw)"
            @select="() => search()"
            @enter="search"
          />
        </div>

        <div class="filter-field">
          <label>操作类型</label>
          <select v-model="filterAct" class="inp" @change="search">
            <option value="">全部操作</option>
            <option v-for="[code, label] in actionOptions" :key="code" :value="code">{{ label }}</option>
          </select>
        </div>

        <div class="filter-field keyword-field">
          <label>关键字</label>
          <input
            v-model="keyword"
            class="inp"
            type="text"
            placeholder="搜索操作内容、图名等"
            @keyup.enter="search"
          />
        </div>
      </div>

      <div class="filter-actions">
        <button class="btn primary sm" type="button" :disabled="loading" @click="search">
          <DemoIcon name="search" :size="13" />查询
        </button>
        <button class="btn sm" type="button" :disabled="loading" @click="resetFilters">
          <DemoIcon name="rotate-ccw" :size="13" />重置
        </button>
      </div>
    </div>

    <div v-if="errorMessage" class="note error-note">
      <DemoIcon name="alert-triangle" :size="15" />
      <span>{{ errorMessage }}</span>
      <button class="btn sm" type="button" @click="loadLogs">重试</button>
    </div>

    <!-- 表格展示卡片 -->
    <div class="card log-table-card">
      <div v-if="loading" class="loading-state">
        <DemoIcon name="loader-circle" :size="20" />
        正在读取操作记录...
      </div>
      <table v-else-if="rows.length" class="tbl">
        <thead>
          <tr>
            <th style="width: 160px;">时间</th>
            <th style="width: 120px;">操作人</th>
            <th style="width: 130px;">操作</th>
            <th>操作内容</th>
            <th style="width: 150px;">图纸</th>
            <th style="width: 80px;">结果</th>
            <th style="width: 70px;"></th>
          </tr>
        </thead>
        <tbody>
          <template v-for="item in rows" :key="item.id">
            <tr :class="{ 'row-expanded': expandedId === item.id }">
              <td class="num updated">{{ item.time }}</td>
              <td class="operator">{{ item.user }}</td>
              <td>
                <span class="tag" :class="actionTone(item.act)">
                  {{ actionLabel(item.act) }}
                </span>
              </td>
              <td class="log-summary">{{ item.txt || '—' }}</td>
              <td class="num link">{{ item.drawingNo || '—' }}</td>
              <td class="source">
                <span class="tag" :class="item.result === 'success' ? 'ok' : 'danger'">
                  {{ item.result === 'success' ? '成功' : '失败' }}
                </span>
              </td>
              <td class="detail-cell">
                <button v-if="canExpand(item)" class="btn sm" type="button" @click="toggleDetail(item.id)">
                  {{ expandedId === item.id ? '收起' : '详情' }}
                </button>
                <span v-else class="empty-muted">—</span>
              </td>
            </tr>
            <tr v-if="expandedId === item.id" class="detail-row">
              <td colspan="7">
                <dl class="log-detail">
                  <div v-for="entry in detailEntries(item.detail)" :key="entry.label" class="detail-pair">
                    <dt>{{ entry.label }}</dt>
                    <dd>{{ entry.value }}</dd>
                  </div>
                </dl>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
      <div v-else class="empty">
        <DemoIcon name="scroll-text" :size="34" />
        <div class="t">暂无符合条件的操作记录</div>
      </div>

      <footer v-if="resultPage.total" class="pagination">
        <span>第 {{ page }} / {{ totalPages }} 页，共 {{ resultPage.total }} 条</span>
        <label>每页
          <select v-model.number="pageSize" class="inp page-size" :disabled="loading" @change="changePageSize">
            <option :value="20">20</option>
            <option :value="50">50</option>
            <option :value="100">100</option>
          </select>条
        </label>
        <button class="btn sm" type="button" :disabled="page <= 1 || loading" @click="changePage(page - 1)">
          <DemoIcon name="chevron-left" :size="14" />上一页
        </button>
        <button
          v-for="item in pageNumbers"
          :key="item"
          class="btn sm page-number"
          :class="{ active: item === page }"
          type="button"
          :disabled="loading || item === page"
          @click="changePage(item)"
        >
          {{ item }}
        </button>
        <button class="btn sm" type="button" :disabled="page >= totalPages || loading" @click="changePage(page + 1)">
          下一页<DemoIcon name="chevron-right" :size="14" />
        </button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 4px 0 14px;
}
.section-head h3 {
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 800;
  color: var(--text-1);
}
.lib-count {
  display: block;
  margin-top: 4px;
  color: var(--text-3);
  font-size: 11.5px;
}

/* 筛选面板布局：网格自适应 */
.log-filter-card {
  padding: 16px;
  margin-bottom: 14px;
}
.filter-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  align-items: flex-end;
}
.filter-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}
.filter-field label {
  color: var(--text-2);
  font-size: 11.5px;
  font-weight: 500;
}
.filter-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px dashed var(--line);
}

.error-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  padding: 10px 14px;
  background: color-mix(in srgb, var(--danger) 10%, transparent);
  border: 1px solid var(--danger);
  border-radius: 8px;
  color: var(--danger);
  font-size: 12px;
}
.error-note span {
  flex: 1;
}

/* 结果列表与分页 */
.log-table-card {
  overflow: visible;
}
.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 180px;
  color: var(--text-3);
  font-size: 12.5px;
}
.operator {
  font-weight: 500;
  color: var(--text-1);
}
.source {
  color: var(--text-3);
}
.log-summary {
  color: var(--text-2);
  font-size: 12px;
  line-height: 1.5;
}
.detail-cell {
  text-align: center;
}
.row-expanded {
  background: var(--panel-2);
}
.detail-row > td {
  padding: 0;
  background: var(--panel-2);
}
.log-detail {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 24px;
  margin: 0;
  padding: 12px 16px;
  border-top: 1px dashed var(--line);
}
.detail-pair {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 140px;
}
.detail-pair dt {
  color: var(--text-3);
  font-size: 11px;
}
.detail-pair dd {
  margin: 0;
  color: var(--text-1);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.pagination {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
  padding: 12px 16px;
  border-top: 1px solid var(--line);
  color: var(--text-3);
  font-size: 11.5px;
}
.pagination label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.page-size {
  width: 60px;
  padding: 4px 6px;
  height: 30px;
}
.page-number {
  min-width: 32px;
}
.page-number.active {
  color: var(--accent-ink);
  border-color: var(--accent);
  background: var(--accent);
}

@media (max-width: 1024px) {
  .filter-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
@media (max-width: 600px) {
  .section-head {
    flex-direction: column;
    align-items: flex-start;
  }
  .filter-grid {
    grid-template-columns: 1fr;
  }
  .pagination {
    justify-content: center;
  }
}
</style>
