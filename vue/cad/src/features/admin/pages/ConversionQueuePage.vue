<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import {
  fetchConversionJobs,
  fetchConversionLogs,
  retryAllFailedConversions,
  retryConversionJob,
  type ConversionJobItem,
  type ConversionStats,
} from '@/services/admin-conversion.service'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'ConversionQueuePage' })

const uiStore = useUiStore()

const loading = ref(false)
const retrying = ref(false)
const autoRefresh = ref(true)
let timer: ReturnType<typeof setInterval> | null = null

const page = ref(1)
const pageSize = ref(20)
const statusFilter = ref('all')
const keyword = ref('')

const items = ref<ConversionJobItem[]>([])
const total = ref(0)
const stats = ref<ConversionStats>({
  total: 0,
  pending: 0,
  processing: 0,
  retry: 0,
  backoff: 0,
  failed: 0,
})
const caxaStatus = ref('idle')

// 日志相关
const logDrawerOpen = ref(false)
const logLines = ref<string[]>([])
const logLoading = ref(false)
const logPath = ref('')
const logModTime = ref<string | null>(null)
let logTimer: ReturnType<typeof setInterval> | null = null

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

const statusLabels: Record<string, { label: string; tagClass: string }> = {
  processing: { label: '转换中', tagClass: 'tag--processing' },
  pending: { label: '排队中', tagClass: 'tag--pending' },
  retry: { label: '待重试', tagClass: 'tag--retry' },
  backoff: { label: '退避等待', tagClass: 'tag--backoff' },
  failed: { label: '失败', tagClass: 'tag--failed' },
  cancelled: { label: '已取消', tagClass: 'tag--cancelled' },
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

function formatDate(dateStr: string): string {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return dateStr
  return d.toLocaleString('zh-CN', {
    hour12: false,
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

async function loadData(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await fetchConversionJobs({
      page: page.value,
      pageSize: pageSize.value,
      status: statusFilter.value,
      keyword: keyword.value,
    })
    items.value = res.items
    total.value = res.total
    stats.value = res.stats
    caxaStatus.value = res.caxaStatus
  } catch (error) {
    if (!silent) {
      uiStore.toast(error instanceof Error ? error.message : '获取转换队列失败', 'warn')
    }
  } finally {
    if (!silent) loading.value = false
  }
}

async function handleRetry(item: ConversionJobItem) {
  try {
    const res = await retryConversionJob(item.id)
    uiStore.toast(res.message || '已加入转换队列', 'ok')
    await loadData(true)
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '重试任务失败', 'warn')
  }
}

async function handleRetryAllFailed() {
  if (stats.value.failed + stats.value.retry + stats.value.backoff === 0) {
    uiStore.toast('当前没有失败或待重试任务', 'info')
    return
  }
  retrying.value = true
  try {
    const res = await retryAllFailedConversions()
    uiStore.toast(res.message, 'ok')
    await loadData(true)
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '批量重试失败', 'warn')
  } finally {
    retrying.value = false
  }
}

async function loadLogs(silent = false) {
  if (!silent) logLoading.value = true
  try {
    const res = await fetchConversionLogs(300)
    logLines.value = res.lines
    logPath.value = res.path
    logModTime.value = res.modTime || null
  } catch (error) {
    if (!silent) {
      uiStore.toast(error instanceof Error ? error.message : '读取 CAXA 日志失败', 'warn')
    }
  } finally {
    if (!silent) logLoading.value = false
  }
}

function openLogs() {
  logDrawerOpen.value = true
  void loadLogs()
  if (logTimer) clearInterval(logTimer)
  logTimer = setInterval(() => {
    if (logDrawerOpen.value) void loadLogs(true)
  }, 2000)
}

function closeLogs() {
  logDrawerOpen.value = false
  if (logTimer) {
    clearInterval(logTimer)
    logTimer = null
  }
}

function toggleAutoRefresh() {
  if (timer) clearInterval(timer)
  timer = null
  if (autoRefresh.value) {
    timer = setInterval(() => void loadData(true), 3000)
  }
}

function handleSearch() {
  page.value = 1
  void loadData()
}

function changeStatus(status: string) {
  statusFilter.value = status
  page.value = 1
  void loadData()
}

onMounted(() => {
  void loadData()
  toggleAutoRefresh()
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
  if (logTimer) clearInterval(logTimer)
})
</script>

<template>
  <div class="conversion-queue-page">
    <header class="page-head">
      <div>
        <h1>CAD 转换队列监控</h1>
        <p>监控 CAXA 进程转换队列调度、执行状态、重试机制以及实时插件日志。</p>
      </div>
      <div class="page-head__actions">
        <label class="switch">
          <input v-model="autoRefresh" type="checkbox" @change="toggleAutoRefresh" />
          <span>3 秒自动刷新</span>
        </label>
        <button class="action-btn" type="button" @click="openLogs">
          <DemoIcon name="file-text" :size="14" />
          <span>CAXA 插件日志</span>
        </button>
        <button
          class="action-btn action-btn--primary"
          :disabled="retrying || (stats.failed + stats.retry + stats.backoff === 0)"
          type="button"
          @click="handleRetryAllFailed"
        >
          <DemoIcon name="refresh-cw" :size="14" />
          <span>重试全部失败任务 ({{ stats.failed + stats.retry + stats.backoff }})</span>
        </button>
        <button class="ghost-btn" type="button" @click="() => loadData()">
          <DemoIcon name="rotate-cw" :size="14" />
          <span>刷新</span>
        </button>
      </div>
    </header>

    <!-- 统计指标 -->
    <section class="metrics-grid">
      <div class="metric-card" :class="{ 'metric-card--active': stats.processing > 0 }">
        <div class="metric-card__header">
          <span>正在转换</span>
          <DemoIcon name="loader" :size="16" class="spin-icon" />
        </div>
        <div class="metric-card__val">{{ stats.processing }}</div>
        <div class="metric-card__desc">CAXA 状态: {{ caxaStatus === 'busy' ? '处理中' : '就绪待命' }}</div>
      </div>

      <div class="metric-card">
        <div class="metric-card__header">
          <span>等待排队</span>
          <DemoIcon name="clock" :size="16" />
        </div>
        <div class="metric-card__val">{{ stats.pending }}</div>
        <div class="metric-card__desc">等待插件拉取消费</div>
      </div>

      <div class="metric-card" :class="{ 'metric-card--warn': stats.retry + stats.backoff > 0 }">
        <div class="metric-card__header">
          <span>退避重试</span>
          <DemoIcon name="alert-triangle" :size="16" />
        </div>
        <div class="metric-card__val">{{ stats.retry + stats.backoff }}</div>
        <div class="metric-card__desc">快速重试 / 慢速重试中</div>
      </div>

      <div class="metric-card" :class="{ 'metric-card--danger': stats.failed > 0 }">
        <div class="metric-card__header">
          <span>转换失败</span>
          <DemoIcon name="x-circle" :size="16" />
        </div>
        <div class="metric-card__val">{{ stats.failed }}</div>
        <div class="metric-card__desc">达最大重试上限</div>
      </div>
    </section>

    <!-- 筛选栏 -->
    <section class="card filter-bar">
      <div class="status-tabs">
        <button
          v-for="s in [
            { id: 'all', label: '全部' },
            { id: 'processing', label: '进行中' },
            { id: 'pending', label: '排队中' },
            { id: 'retry', label: '待重试' },
            { id: 'failed', label: '已失败' },
          ]"
          :key="s.id"
          class="status-tab"
          :class="{ active: statusFilter === s.id }"
          type="button"
          @click="changeStatus(s.id)"
        >
          {{ s.label }}
        </button>
      </div>

      <div class="search-box">
        <DemoIcon name="search" :size="14" />
        <input
          v-model="keyword"
          placeholder="搜索图号、图纸名或源文件名..."
          @keyup.enter="handleSearch"
        />
      </div>

      <span class="count-tag">共 {{ total }} 项</span>
    </section>

    <!-- 列表数据 -->
    <section class="card table-card">
      <div v-if="loading && !items.length" class="empty-state">加载中...</div>
      <div v-else-if="!items.length" class="empty-state">
        <DemoIcon name="inbox" :size="32" />
        <p>暂无符合条件的转换任务</p>
      </div>
      <div v-else class="table-wrap">
        <table class="queue-table">
          <thead>
            <tr>
              <th style="width: 24%">源文件 / 关联图纸</th>
              <th style="width: 10%">文件大小</th>
              <th style="width: 12%">状态</th>
              <th style="width: 8%">尝试次数</th>
              <th style="width: 16%">下次执行 / 更新时间</th>
              <th style="width: 20%">错误详情</th>
              <th style="width: 10%; text-align: right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in items" :key="row.id">
              <td>
                <div class="source-info">
                  <span class="source-name" :title="row.sourceName">{{ row.sourceName }}</span>
                  <div class="drawing-tag" v-if="row.drawingNo">
                    <span>{{ row.drawingNo }}</span>
                    <span v-if="row.drawingName"> - {{ row.drawingName }}</span>
                  </div>
                </div>
              </td>
              <td>
                <span class="size-text">{{ formatBytes(row.sourceSize) }}</span>
              </td>
              <td>
                <span class="status-pill" :class="statusLabels[row.status]?.tagClass || 'tag--other'">
                  {{ statusLabels[row.status]?.label || row.status }}
                </span>
              </td>
              <td>
                <span class="attempts-text">{{ row.attempts }} 次</span>
              </td>
              <td>
                <div class="time-col">
                  <span>更新: {{ formatDate(row.updatedAt) }}</span>
                  <small v-if="row.status === 'retry' || row.status === 'backoff'">
                    下次: {{ formatDate(row.nextAttemptAt) }}
                  </small>
                </div>
              </td>
              <td>
                <div v-if="row.lastError" class="error-cell" :title="row.lastError">
                  {{ row.lastError }}
                </div>
                <span v-else class="text-muted">-</span>
              </td>
              <td style="text-align: right">
                <button
                  v-if="row.status === 'failed' || row.status === 'retry' || row.status === 'backoff'"
                  class="mini-btn mini-btn--primary"
                  type="button"
                  @click="handleRetry(row)"
                >
                  <DemoIcon name="rotate-cw" :size="12" />
                  <span>重试</span>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 分页 -->
      <footer v-if="totalPages > 1" class="pagination-bar">
        <span class="page-info">第 {{ page }} / {{ totalPages }} 页</span>
        <div class="page-actions">
          <button
            class="ghost-btn mini"
            :disabled="page <= 1"
            type="button"
            @click="() => { page--; void loadData() }"
          >
            上一页
          </button>
          <button
            class="ghost-btn mini"
            :disabled="page >= totalPages"
            type="button"
            @click="() => { page++; void loadData() }"
          >
            下一页
          </button>
        </div>
      </footer>
    </section>

    <!-- CAXA 插件执行日志抽屉 -->
    <div v-if="logDrawerOpen" class="log-drawer-backdrop" @click="closeLogs">
      <div class="log-drawer" @click.stop>
        <header class="log-drawer__header">
          <div class="log-drawer__title">
            <DemoIcon name="terminal" :size="18" />
            <h3>CAXA 插件运行日志 (最近 300 行)</h3>
          </div>
          <div class="log-drawer__actions">
            <span v-if="logModTime" class="mod-time">日志修改时间: {{ formatDate(logModTime) }}</span>
            <button class="ghost-btn mini" type="button" @click="() => loadLogs()">
              <DemoIcon name="refresh-cw" :size="13" />
              <span>刷新日志</span>
            </button>
            <button class="ghost-btn mini icon-only" type="button" @click="closeLogs">
              <DemoIcon name="x" :size="16" />
            </button>
          </div>
        </header>

        <div class="log-drawer__body">
          <div v-if="logLoading && !logLines.length" class="log-empty">读取日志中...</div>
          <div v-else-if="!logLines.length" class="log-empty">暂无 CAXA 日志</div>
          <pre v-else class="log-terminal"><code><template v-for="(line, idx) in logLines" :key="idx"><span :class="{
            'log-line--accent': line.includes('Attempting') || line.includes('saveAs'),
            'log-line--warn': line.includes('AutoDismiss') || line.includes('IDCANCEL') || line.includes('retrying'),
            'log-line--danger': line.includes('failed') || line.includes('Error') || line.includes('timed out'),
          }">{{ line }}</span>
</template></code></pre>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.conversion-queue-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
}

.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
}
.page-head h1 {
  font-size: 20px;
  font-weight: 600;
  margin-bottom: 4px;
}
.page-head p {
  color: var(--text-3);
  font-size: 13px;
}
.page-head__actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-shrink: 0;
}

.switch {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-2);
  cursor: pointer;
}

.action-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 12px;
  border: 1px solid var(--line);
  background: var(--panel);
  color: var(--text-1);
  cursor: pointer;
  transition: all 0.2s;
}
.action-btn:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}
.action-btn--primary {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--accent);
}
.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 指标网格 */
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
}
.metric-card {
  padding: 14px 16px;
  border-radius: 10px;
  border: 1px solid var(--line);
  background: var(--panel);
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.metric-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: var(--text-3);
  font-size: 12px;
}
.metric-card__val {
  font-size: 24px;
  font-weight: 700;
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-1);
}
.metric-card__desc {
  font-size: 11px;
  color: var(--text-3);
}

.metric-card--active {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.metric-card--active .metric-card__val {
  color: var(--accent);
}

.metric-card--warn {
  border-color: var(--warn);
}
.metric-card--warn .metric-card__val {
  color: var(--warn);
}

.metric-card--danger {
  border-color: var(--danger);
}
.metric-card--danger .metric-card__val {
  color: var(--danger);
}

/* 筛选栏 */
.filter-bar {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 10px 14px;
}
.status-tabs {
  display: flex;
  gap: 4px;
  background: var(--hover);
  padding: 3px;
  border-radius: 7px;
}
.status-tab {
  padding: 4px 12px;
  border-radius: 5px;
  border: none;
  font-size: 12px;
  background: transparent;
  color: var(--text-2);
  cursor: pointer;
  transition: all 0.2s;
}
.status-tab.active {
  background: var(--panel);
  color: var(--text-1);
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}
.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  padding: 4px 10px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--panel);
}
.search-box input {
  border: none;
  background: transparent;
  outline: none;
  width: 100%;
  font-size: 12px;
  color: var(--text-1);
}
.count-tag {
  font-size: 11.5px;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
}

/* 表格样式 */
.table-card {
  padding: 0;
  overflow: hidden;
}
.table-wrap {
  width: 100%;
  overflow-x: auto;
}
.queue-table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
  font-size: 12px;
}
.queue-table th {
  padding: 10px 14px;
  background: var(--panel-top);
  border-bottom: 1px solid var(--line);
  color: var(--text-3);
  font-weight: 500;
}
.queue-table td {
  padding: 12px 14px;
  border-bottom: 1px solid var(--line);
  color: var(--text-2);
  vertical-align: middle;
}
.queue-table tr:last-child td {
  border-bottom: none;
}
.queue-table tr:hover td {
  background: var(--hover);
}

.source-info {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.source-name {
  font-weight: 600;
  color: var(--text-1);
}
.drawing-tag {
  font-size: 11px;
  color: var(--text-3);
}

.status-pill {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 11px;
  font-weight: 500;
}
.tag--processing { background: rgba(59, 130, 246, 0.15); color: #3b82f6; }
.tag--pending { background: rgba(107, 114, 128, 0.15); color: #9ca3af; }
.tag--retry { background: rgba(245, 158, 11, 0.15); color: #f59e0b; }
.tag--backoff { background: rgba(139, 92, 246, 0.15); color: #8b5cf6; }
.tag--failed { background: rgba(239, 68, 68, 0.15); color: #ef4444; }
.tag--cancelled { background: rgba(156, 163, 175, 0.15); color: #6b7280; }

.error-cell {
  color: var(--danger);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.time-col {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 11px;
}
.mini-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 4px;
  border: 1px solid var(--line);
  background: var(--panel);
  color: var(--text-1);
  font-size: 11px;
  cursor: pointer;
}
.mini-btn--primary {
  border-color: var(--accent);
  color: var(--accent);
  background: var(--accent-soft);
}
.mini-btn:hover {
  filter: brightness(1.1);
}

.pagination-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  border-top: 1px solid var(--line);
  font-size: 12px;
  color: var(--text-3);
}

.spin-icon {
  animation: spin 1.5s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 48px;
  color: var(--text-3);
}

/* 日志抽屉 */
.log-drawer-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  justify-content: flex-end;
  z-index: 1000;
}
.log-drawer {
  width: 650px;
  max-width: 90vw;
  height: 100%;
  background: #181a1f;
  color: #abb2bf;
  display: flex;
  flex-direction: column;
  box-shadow: -4px 0 16px rgba(0, 0, 0, 0.3);
}
.log-drawer__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid #282c34;
  background: #21252b;
}
.log-drawer__title {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #fff;
  font-size: 13px;
}
.log-drawer__actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.mod-time {
  font-size: 10.5px;
  color: #5c6370;
}
.log-drawer__body {
  flex: 1;
  overflow: auto;
  padding: 12px;
}
.log-terminal {
  margin: 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
}
.log-line--accent { color: #61afef; }
.log-line--warn { color: #e5c07b; }
.log-line--danger { color: #e06c75; font-weight: bold; }
</style>
