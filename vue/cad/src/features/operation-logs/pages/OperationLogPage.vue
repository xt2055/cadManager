<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import CandidateInput from '@/features/operation-logs/components/CandidateInput.vue'
import { mergeSelectOptions, paginationPages, paginationRange, type SelectOption } from '@/features/operation-logs/operation-log.helpers'
import { listOperationLogOptions, type OperationLogPage } from '@/services/drawing-operation-log.service'
import { useAuditStore } from '@/stores/audit.store'
import { ACTIVITY_LABELS, type ActivityType } from '@/types/domain.types'

defineOptions({ name: 'OperationLogPage' })

type CandidateKind = 'drawing' | 'actor'

const auditStore = useAuditStore()
const page = ref(1)
const pageSize = ref(20)
const filterAct = ref<ActivityType | ''>('')
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

const actionOptions = Object.entries(ACTIVITY_LABELS) as Array<[ActivityType, string]>
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

const colors: Record<string, string> = { 查看图纸: 'info', 新建图纸: 'ok', 修改图纸: 'warn', 创建分支: 'info', 上传文件: 'warn', 下载文件: 'plain', 删除文件: 'danger', 审核操作: 'info', '解析 EXB': 'plain' }

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
            <th style="width: 170px;">时间</th>
            <th style="width: 140px;">操作人</th>
            <th style="width: 120px;">操作</th>
            <th style="width: 200px;">图纸</th>
            <th>结果</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in rows" :key="item.id">
            <td class="num updated">{{ item.time }}</td>
            <td class="operator">{{ item.user }}</td>
            <td>
              <span class="tag" :class="colors[ACTIVITY_LABELS[item.act]] ?? 'mute'">
                {{ ACTIVITY_LABELS[item.act] }}
              </span>
            </td>
            <td class="num link">{{ item.drawingNo || '—' }}</td>
            <td class="source">
              <span class="tag" :class="item.result === 'success' ? 'ok' : 'danger'">
                {{ item.result === 'success' ? '成功' : '失败' }}
              </span>
            </td>
          </tr>
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
