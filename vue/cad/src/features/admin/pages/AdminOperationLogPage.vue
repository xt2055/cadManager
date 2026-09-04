<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import type { OperationLogPage } from '@/services/drawing-operation-log.service'
import { useAdminStore } from '@/stores/admin.store'
import { useAuditStore } from '@/stores/audit.store'
import { ACTIVITY_LABELS, type ActivityLog, type ActivityResult, type ActivityTargetType, type ActivityType } from '@/types/domain.types'

defineOptions({ name: 'AdminOperationLogPage' })

const adminStore = useAdminStore()
const auditStore = useAuditStore()
const page = ref(1)
const pageSize = ref(20)
const query = ref('')
const drawingNo = ref('')
const action = ref<ActivityType | ''>('')
const targetType = ref<ActivityTargetType | ''>('')
const actorId = ref('')
const result = ref<ActivityResult | ''>('')
const from = ref('')
const to = ref('')
const loading = ref(false)
const errorMessage = ref('')
const resultPage = ref<OperationLogPage>({ list: [], total: 0, page: 1, pageSize: 20 })
const selectedLog = ref<ActivityLog | null>(null)

const actionOptions = Object.entries(ACTIVITY_LABELS) as Array<[ActivityType, string]>
const targetOptions: Array<[ActivityTargetType, string]> = [
  ['drawing', '图纸'],
  ['part', '零件'],
  ['file', '文件'],
  ['review', '审核'],
  ['branch', '分支'],
]
const adminUsers = computed(() => adminStore.users.filter((user) => user.roles.includes('admin')))
const totalPages = computed(() => Math.max(1, Math.ceil(resultPage.value.total / pageSize.value)))
const rows = computed(() => resultPage.value.list)

const feedIcons: Record<ActivityType, string> = {
  view: 'eye', create: 'plus', edit: 'pencil', branch: 'git-branch', upload: 'upload',
  download: 'download', delete: 'trash-2', check: 'check-circle-2', parse: 'file-search',
}
const colors: Record<ActivityType, string> = {
  view: 'info', create: 'ok', edit: 'warn', branch: 'info', upload: 'warn',
  download: 'plain', delete: 'danger', check: 'info', parse: 'plain',
}

function targetLabel(value: string): string {
  return targetOptions.find(([code]) => code === value)?.[1] || value || '未知对象'
}

function formatDetail(detail: Record<string, unknown> | undefined): string {
  return detail && Object.keys(detail).length ? JSON.stringify(detail, null, 2) : '暂无附加详情'
}

async function loadLogs() {
  loading.value = true
  errorMessage.value = ''
  try {
    resultPage.value = await auditStore.loadAdmin({
      page: page.value,
      pageSize: pageSize.value,
      keyword: query.value.trim() || undefined,
      drawingNo: drawingNo.value.trim() || undefined,
      action: action.value || undefined,
      targetType: targetType.value || undefined,
      actorId: actorId.value || undefined,
      result: result.value || undefined,
      from: from.value || undefined,
      to: to.value || undefined,
    })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '管理员操作日志读取失败'
    resultPage.value = { list: [], total: 0, page: page.value, pageSize: pageSize.value }
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  void loadLogs()
}

function resetFilters() {
  query.value = ''
  drawingNo.value = ''
  action.value = ''
  targetType.value = ''
  actorId.value = ''
  result.value = ''
  from.value = ''
  to.value = ''
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

function openDetail(log: ActivityLog) {
  selectedLog.value = log
}

onMounted(() => {
  void Promise.all([loadLogs(), adminStore.loadUsers()])
})
</script>

<template>
  <div class="page admin-page log-admin-page">
    <div class="section-head">
      <div>
        <h3>操作日志</h3>
        <span class="lib-count">仅显示管理员执行的操作 · 共 {{ resultPage.total }} 条</span>
      </div>
      <button class="btn" type="button" :disabled="loading" @click="loadLogs">
        <DemoIcon name="refresh-cw" :size="14" />刷新
      </button>
    </div>

    <section class="card filter-card">
      <div class="filter-grid">
        <label class="filter-item keyword-item"><span>关键字</span><input v-model="query" class="inp" placeholder="搜索操作描述、图号、名称" @keyup.enter="search" /></label>
        <label class="filter-item"><span>图号</span><input v-model="drawingNo" class="inp" placeholder="输入图号" @keyup.enter="search" /></label>
        <label class="filter-item"><span>操作人</span><select v-model="actorId" class="inp"><option value="">全部管理员</option><option v-for="user in adminUsers" :key="user.id" :value="user.id">{{ user.displayName }}（{{ user.account }}）</option></select></label>
        <label class="filter-item"><span>操作类型</span><select v-model="action" class="inp"><option value="">全部操作</option><option v-for="[code, label] in actionOptions" :key="code" :value="code">{{ label }}</option></select></label>
        <label class="filter-item"><span>对象类型</span><select v-model="targetType" class="inp"><option value="">全部对象</option><option v-for="[code, label] in targetOptions" :key="code" :value="code">{{ label }}</option></select></label>
        <label class="filter-item"><span>结果</span><select v-model="result" class="inp"><option value="">全部结果</option><option value="success">成功</option><option value="failed">失败</option></select></label>
        <label class="filter-item"><span>开始日期</span><input v-model="from" class="inp" type="date" /></label>
        <label class="filter-item"><span>结束日期</span><input v-model="to" class="inp" type="date" /></label>
      </div>
      <div class="filter-actions"><button class="btn primary" type="button" @click="search"><DemoIcon name="search" :size="14" />查询</button><button class="btn" type="button" @click="resetFilters"><DemoIcon name="rotate-ccw" :size="14" />重置</button></div>
    </section>

    <div v-if="errorMessage" class="note error-note"><DemoIcon name="alert-triangle" :size="15" /><span>{{ errorMessage }}</span><button class="btn sm" type="button" @click="loadLogs">重试</button></div>

    <section class="card log-table-card">
      <div v-if="loading" class="loading-state"><DemoIcon name="loader-circle" :size="20" />正在读取管理员操作日志...</div>
      <div v-else-if="!rows.length" class="empty"><DemoIcon name="scroll-text" :size="34" /><div class="t">暂无符合条件的管理员操作</div></div>
      <div v-else class="table-scroll">
        <table class="tbl log-table">
          <thead><tr><th>时间</th><th>操作人</th><th>操作</th><th>对象</th><th>图号</th><th>操作描述</th><th>结果</th><th>详情</th></tr></thead>
          <tbody>
            <tr v-for="item in rows" :key="item.id">
              <td class="mono time-cell">{{ item.time }}</td>
              <td><strong>{{ item.user }}</strong><small v-if="item.userAccount" class="sub-text">{{ item.userAccount }}</small></td>
              <td><span class="activity-tag" :class="colors[item.act]"><DemoIcon :name="feedIcons[item.act]" :size="13" />{{ ACTIVITY_LABELS[item.act] }}</span></td>
              <td><span class="tag plain">{{ targetLabel(item.targetType) }}</span></td>
              <td class="mono link">{{ item.drawingNo || '—' }}</td>
              <td class="summary-cell">{{ item.txt || '—' }}</td>
              <td><span class="tag" :class="item.result === 'success' ? 'ok' : 'danger'">{{ item.result === 'success' ? '成功' : '失败' }}</span></td>
              <td><button class="icon-btn" type="button" title="查看详情" @click="openDetail(item)"><DemoIcon name="list" :size="15" /></button></td>
            </tr>
          </tbody>
        </table>
      </div>
      <footer v-if="resultPage.total" class="pagination">
        <span>第 {{ page }} / {{ totalPages }} 页，共 {{ resultPage.total }} 条</span>
        <label>每页<select v-model.number="pageSize" class="inp page-size" @change="changePageSize"><option :value="20">20</option><option :value="50">50</option><option :value="100">100</option></select>条</label>
        <button class="btn sm" type="button" :disabled="page <= 1 || loading" @click="changePage(page - 1)"><DemoIcon name="chevron-left" :size="14" />上一页</button>
        <button class="btn sm" type="button" :disabled="page >= totalPages || loading" @click="changePage(page + 1)">下一页<DemoIcon name="chevron-right" :size="14" /></button>
      </footer>
    </section>

    <div v-if="selectedLog" class="detail-overlay" role="presentation" @click.self="selectedLog = null">
      <aside class="detail-drawer" role="dialog" aria-modal="true" aria-label="操作日志详情">
        <header class="drawer-head"><div><span class="eyebrow">管理员审计详情</span><h2>{{ selectedLog.txt }}</h2></div><button class="icon-btn" type="button" aria-label="关闭详情" @click="selectedLog = null"><DemoIcon name="x" :size="17" /></button></header>
        <div class="drawer-body">
          <div class="detail-grid"><div><span>操作人</span><strong>{{ selectedLog.user }}</strong></div><div><span>时间</span><strong class="mono">{{ selectedLog.time }}</strong></div><div><span>操作类型</span><strong>{{ ACTIVITY_LABELS[selectedLog.act] }}</strong></div><div><span>对象类型</span><strong>{{ targetLabel(selectedLog.targetType) }}</strong></div><div><span>图号</span><strong class="mono">{{ selectedLog.drawingNo || '—' }}</strong></div><div><span>结果</span><strong :class="selectedLog.result === 'success' ? 'success-text' : 'danger-text'">{{ selectedLog.result === 'success' ? '成功' : '失败' }}</strong></div></div>
          <div class="detail-section"><span class="detail-label">操作描述</span><p>{{ selectedLog.txt }}</p></div>
          <div class="detail-section"><span class="detail-label">附加详情</span><pre>{{ formatDetail(selectedLog.detail) }}</pre></div>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.log-admin-page { position: relative; }
.section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 4px 0 6px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.lib-count { display: block; margin-top: 4px; color: var(--text-3); font-size: 11px; }
.filter-card { padding: 14px; margin-bottom: 14px; }
.filter-grid { display: grid; grid-template-columns: repeat(4, minmax(130px, 1fr)); gap: 10px; }
.filter-item { display: flex; flex-direction: column; gap: 5px; min-width: 0; }
.filter-item span { color: var(--text-3); font-size: 11px; }
.filter-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
.error-note { display: flex; align-items: center; gap: 8px; margin-bottom: 14px; color: var(--danger); }
.error-note span { flex: 1; }
.log-table-card { overflow: hidden; }
.table-scroll { overflow-x: auto; }
.log-table { min-width: 980px; }
.time-cell { white-space: nowrap; color: var(--text-3); font-size: 11px; }
.sub-text { display: block; margin-top: 3px; color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 10px; }
.activity-tag { display: inline-flex; align-items: center; gap: 5px; padding: 4px 7px; border-radius: 6px; font-size: 11px; }
.activity-tag.info { color: var(--accent); background: var(--accent-soft); }.activity-tag.ok { color: var(--success, #35b87f); background: color-mix(in srgb, var(--success, #35b87f) 12%, transparent); }.activity-tag.warn { color: var(--warning, #e9a33a); background: color-mix(in srgb, var(--warning, #e9a33a) 12%, transparent); }.activity-tag.plain { color: var(--text-2); background: var(--hover); }.activity-tag.danger { color: var(--danger); background: color-mix(in srgb, var(--danger) 12%, transparent); }
.summary-cell { max-width: 260px; color: var(--text-2); }
.loading-state { display: flex; align-items: center; justify-content: center; gap: 8px; min-height: 180px; color: var(--text-3); font-size: 12px; }
.pagination { display: flex; align-items: center; justify-content: flex-end; gap: 10px; padding: 12px 14px; border-top: 1px solid var(--line); color: var(--text-3); font-size: 11px; }
.pagination label { display: inline-flex; align-items: center; gap: 5px; }.page-size { width: 58px; padding: 5px 7px; }
.detail-overlay { position: fixed; z-index: 40; inset: 0; display: flex; justify-content: flex-end; background: rgb(0 0 0 / 42%); }
.detail-drawer { width: min(520px, 92vw); height: 100%; overflow: auto; background: var(--panel-top); box-shadow: -10px 0 30px rgb(0 0 0 / 20%); }
.drawer-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; padding: 22px; border-bottom: 1px solid var(--line); }.drawer-head h2 { margin-top: 7px; font-size: 17px; line-height: 1.45; }.eyebrow { color: var(--accent); font-size: 10px; letter-spacing: .08em; }.drawer-body { padding: 20px 22px; }.detail-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 14px; }.detail-grid div { display: flex; flex-direction: column; gap: 5px; }.detail-grid span, .detail-label { color: var(--text-3); font-size: 11px; }.detail-grid strong { color: var(--text-1); font-size: 12px; }.detail-section { margin-top: 22px; }.detail-section p { margin-top: 8px; color: var(--text-2); line-height: 1.7; }.detail-section pre { max-height: 300px; overflow: auto; margin-top: 8px; padding: 12px; border: 1px solid var(--line); border-radius: 8px; color: var(--text-2); background: var(--panel); font: 11px/1.6 'JetBrains Mono', monospace; white-space: pre-wrap; word-break: break-word; }.success-text { color: var(--success, #35b87f) !important; }.danger-text { color: var(--danger) !important; }
@media (max-width: 900px) { .filter-grid { grid-template-columns: repeat(2, minmax(130px, 1fr)); } }
@media (max-width: 600px) { .filter-grid { grid-template-columns: 1fr; }.pagination { flex-wrap: wrap; justify-content: center; }.detail-grid { grid-template-columns: 1fr; } }
</style>
