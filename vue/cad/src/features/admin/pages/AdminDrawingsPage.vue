<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import AdminTabs from '../components/AdminTabs.vue'
import {
  deleteAdminDrawing,
  deleteAdminAttachment,
  closeAdminEditSession,
  downloadAdminVersion,
  restoreAdminVersion,
  getAdminDrawing,
  listAdminDrawings,
  setAdminDrawingStatus,
  setAdminPartStatus,
  type AdminAttachment,
  type AdminDrawingSummary,
  type AdminDrawingDetail,
  type AdminDrawingPage,
  type AdminPartSummary,
} from '@/services/admin-drawing.service'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AdminDrawingsPage' })

const uiStore = useUiStore()
const mode = ref<'drawing' | 'part'>('drawing')
const keyword = ref('')
const status = ref('')
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const errorMessage = ref('')
const result = ref<AdminDrawingPage>({ list: [], total: 0, page: 1, pageSize: 20 })
const detail = ref<AdminDrawingDetail | null>(null)
const detailLoading = ref(false)
const busyId = ref('')

const rows = computed(() => result.value.list)
const totalPages = computed(() => Math.max(1, Math.ceil(result.value.total / pageSize.value)))
const drawingRows = computed<AdminDrawingSummary[]>(() => result.value.list as AdminDrawingSummary[])
const partPageRows = computed<AdminPartSummary[]>(() => result.value.list as AdminPartSummary[])
const detailParts = computed<AdminPartSummary[]>(() => detail.value?.parts || [])
const detailAttachments = computed<AdminAttachment[]>(() => detail.value?.attachments || [])

const statusLabels: Record<string, string> = {
  draft: '草稿', reviewing: '审核中', published: '生产中', disabled: '已禁用', archived: '已存档',
}
const statusClasses: Record<string, string> = {
  draft: 'plain', reviewing: 'warn', published: 'ok', disabled: 'danger', archived: 'mute',
}

function statusLabel(value: string) { return statusLabels[value] || value || '未知' }
function statusClass(value: string) { return statusClasses[value] || 'plain' }
function formatTime(value: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
function formatSize(size: number) {
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  if (size >= 1024) return `${(size / 1024).toFixed(0)} KB`
  return `${size} B`
}
function roleLabel(value: string) { return value === 'assembly' ? '总图' : value === 'part' ? '零件图' : value }
function attachmentName(item: AdminAttachment) { return item.currentName || item.originalName }

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    result.value = await listAdminDrawings({ page: page.value, pageSize: pageSize.value, keyword: keyword.value.trim() || undefined, status: status.value || undefined, kind: mode.value })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '后台图纸列表读取失败'
    result.value = { list: [], total: 0, page: page.value, pageSize: pageSize.value }
  } finally {
    loading.value = false
  }
}

function search() { page.value = 1; void load() }
function changePage(next: number) { if (next < 1 || next > totalPages.value || next === page.value) return; page.value = next; void load() }
function changePageSize() { page.value = 1; void load() }
function switchMode(next: 'drawing' | 'part') { mode.value = next; page.value = 1; void load() }

async function openDetail(item: { id: string }) {
  detailLoading.value = true
  try { detail.value = await getAdminDrawing(item.id) } catch (error) { uiStore.toast(error instanceof Error ? error.message : '图纸详情读取失败', 'warn') } finally { detailLoading.value = false }
}

function closeDetail() { detail.value = null }

async function toggleDrawing(item: { id: string; no: string; status: string }) {
  if (busyId.value) return
  const nextStatus = item.status === 'disabled' ? 'draft' : 'disabled'
  const action = nextStatus === 'disabled' ? '禁用' : '启用'
  const confirmed = window.confirm(`确定要${action}图纸「${item.no}」吗？${nextStatus === 'disabled' ? '\n禁用后普通图纸库将不再显示。' : ''}`)
  if (!confirmed) return
  busyId.value = item.id
  try { await setAdminDrawingStatus(item.id, nextStatus); uiStore.toast(`图纸「${item.no}」已${action}`, 'ok'); await load(); if (detail.value?.drawing.id === item.id) detail.value = await getAdminDrawing(item.id) } catch (error) { uiStore.toast(error instanceof Error ? error.message : `图纸${action}失败`, 'warn') } finally { busyId.value = '' }
}

async function togglePart(item: AdminPartSummary) {
  if (busyId.value) return
  const nextStatus = item.status === 'disabled' ? 'draft' : 'disabled'
  const action = nextStatus === 'disabled' ? '禁用' : '启用'
  const confirmed = window.confirm(`确定要${action}零件「${item.no}」吗？`)
  if (!confirmed) return
  busyId.value = item.id
  try { await setAdminPartStatus(item.id, nextStatus); uiStore.toast(`零件「${item.no}」已${action}`, 'ok'); await load() } catch (error) { uiStore.toast(error instanceof Error ? error.message : `零件${action}失败`, 'warn') } finally { busyId.value = '' }
}

async function hardDelete(item: { id: string; no: string }) {
  if (busyId.value) return
  const confirmed = window.confirm(`确定永久删除图纸「${item.no}」吗？\n\n这将同时删除其零件、附件、CAD 版本和物理文件，无法恢复。`)
  if (!confirmed) return
  busyId.value = item.id
  try { await deleteAdminDrawing(item.id); closeDetail(); uiStore.toast(`图纸「${item.no}」已永久删除`, 'ok'); await load() } catch (error) { uiStore.toast(error instanceof Error ? error.message : '图纸永久删除失败', 'warn') } finally { busyId.value = '' }
}

async function removeAttachment(item: AdminAttachment) {
  if (busyId.value) return
  if (!window.confirm(`确定永久删除附件「${attachmentName(item)}」吗？\n\n对应的所有文件版本和物理文件都会被删除，无法恢复。`)) return
  busyId.value = item.id
  try { await deleteAdminAttachment(item.id); uiStore.toast(`附件「${attachmentName(item)}」已删除`, 'ok'); if (detail.value) detail.value = await getAdminDrawing(detail.value.drawing.id) } catch (error) { uiStore.toast(error instanceof Error ? error.message : '附件删除失败', 'warn') } finally { busyId.value = '' }
}

async function downloadVersion(item: AdminDrawingDetail['versions'][number]) {
  try { const blob = await downloadAdminVersion(item.id); const url = URL.createObjectURL(blob); const anchor = document.createElement('a'); anchor.href = url; anchor.download = item.storageKey.split('/').pop() || `${item.version}.dwg`; anchor.click(); URL.revokeObjectURL(url) } catch (error) { uiStore.toast(error instanceof Error ? error.message : '版本下载失败', 'warn') }
}

async function restoreVersion(item: AdminDrawingDetail['versions'][number]) {
  if (!detail.value || busyId.value) return
  if (!window.confirm(`确定以版本 ${item.version} 生成新的当前版本吗？`)) return
  busyId.value = item.id
  try { await restoreAdminVersion(item.id); uiStore.toast(`已从 ${item.version} 生成新版本`, 'ok'); detail.value = await getAdminDrawing(detail.value.drawing.id) } catch (error) { uiStore.toast(error instanceof Error ? error.message : '版本回退失败', 'warn') } finally { busyId.value = '' }
}

async function closeSession(item: Record<string, unknown>) {
  const id = String(item.id || '')
  if (!id || busyId.value) return
  if (!window.confirm(`确定强制结束「${String(item.fileName || '未命名文件')}」的编辑会话吗？`)) return
  busyId.value = id
  try { await closeAdminEditSession(id); uiStore.toast('编辑会话已结束', 'ok'); if (detail.value) detail.value = await getAdminDrawing(detail.value.drawing.id) } catch (error) { uiStore.toast(error instanceof Error ? error.message : '结束编辑会话失败', 'warn') } finally { busyId.value = '' }
}

onMounted(() => { void load() })
</script>

<template>
  <div class="page admin-page admin-drawings-page">
    <div class="section-head">
      <div><h3>后台管理</h3><span class="lib-count">总图、零件图、附件、版本与编辑会话统一管理</span></div>
      <button class="btn" type="button" :disabled="loading" @click="load"><DemoIcon name="refresh-cw" :size="14" />刷新</button>
    </div>
    <AdminTabs active="mgmt" />

    <section class="card filter-card">
      <div class="mode-switch"><button class="mode-btn" :class="{ active: mode === 'drawing' }" type="button" @click="switchMode('drawing')"><DemoIcon name="layers" :size="14" />总图管理</button><button class="mode-btn" :class="{ active: mode === 'part' }" type="button" @click="switchMode('part')"><DemoIcon name="file" :size="14" />零件图管理</button></div>
      <label class="search-box"><DemoIcon name="search" :size="15" /><input v-model="keyword" :placeholder="mode === 'drawing' ? '搜索图号、名称、项目号' : '搜索零件图号、名称、所属总图'" @keyup.enter="search" /></label>
      <select v-model="status" class="inp status-select" @change="search"><option value="">全部状态</option><option value="draft">草稿</option><option value="reviewing">审核中</option><option value="published">生产中</option><option value="disabled">已禁用</option><option value="archived">已存档</option></select>
      <button class="btn primary" type="button" @click="search"><DemoIcon name="search" :size="14" />查询</button>
    </section>

    <div v-if="errorMessage" class="note error-note"><DemoIcon name="alert-triangle" :size="15" /><span>{{ errorMessage }}</span><button class="btn sm" type="button" @click="load">重试</button></div>

    <section class="card table-card">
      <div v-if="loading" class="loading-state"><DemoIcon name="loader-circle" :size="20" />正在读取图纸数据...</div>
      <div v-else-if="!rows.length" class="empty"><DemoIcon name="layers" :size="36" /><div class="t">暂无符合条件的{{ mode === 'drawing' ? '总图' : '零件图' }}</div></div>
      <div v-else class="table-scroll">
        <table class="tbl admin-drawing-table">
          <thead v-if="mode === 'drawing'"><tr><th>图号</th><th>名称</th><th>项目</th><th>状态</th><th>版本</th><th>零件</th><th>附件</th><th>编辑</th><th>更新时间</th><th>操作</th></tr></thead>
          <thead v-else><tr><th>零件图号</th><th>名称</th><th>所属总图</th><th>项目</th><th>状态</th><th>版本</th><th>附件</th><th>编辑</th><th>更新时间</th><th>操作</th></tr></thead>
          <tbody v-if="mode === 'drawing'"><tr v-for="item in drawingRows" :key="item.id"><td class="mono link">{{ item.no }}</td><td><strong>{{ item.name }}</strong><small class="sub-text">{{ item.vendor || '未指定责任单位' }}</small></td><td>{{ item.project }}</td><td><span class="tag" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span></td><td class="mono">{{ item.version }}</td><td class="num">{{ item.partCount }}</td><td class="num">{{ item.attachmentCount }}</td><td class="num" :class="{ 'danger-text': item.sessionCount }">{{ item.sessionCount }}</td><td class="mono time-cell">{{ formatTime(item.updatedAt) }}</td><td class="row-actions"><button class="btn sm" type="button" @click="openDetail(item)"><DemoIcon name="eye" :size="13" />详情</button><button class="btn sm" type="button" :disabled="busyId === item.id" @click="toggleDrawing(item)">{{ item.status === 'disabled' ? '启用' : '禁用' }}</button><button class="icon-btn danger-icon" type="button" title="永久删除" :disabled="busyId === item.id" @click="hardDelete(item)"><DemoIcon name="trash-2" :size="14" /></button></td></tr></tbody>
          <tbody v-else><tr v-for="item in partPageRows" :key="item.id"><td class="mono link">{{ item.no }}</td><td><strong>{{ item.name }}</strong><small class="sub-text">{{ item.material || '—' }}</small></td><td class="mono">{{ item.drawingNo || '—' }}</td><td>{{ item.project }}</td><td><span class="tag" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span></td><td class="mono">{{ item.version }}</td><td class="num">{{ item.attachmentCount }}</td><td class="num" :class="{ 'danger-text': item.sessionCount }">{{ item.sessionCount }}</td><td class="mono time-cell">{{ formatTime(item.updatedAt) }}</td><td class="row-actions"><button class="btn sm" type="button" @click="openDetail({ id: item.drawingId })"><DemoIcon name="eye" :size="13" />所属图纸</button><button class="btn sm" type="button" :disabled="busyId === item.id" @click="togglePart(item)">{{ item.status === 'disabled' ? '启用' : '禁用' }}</button></td></tr></tbody>
        </table>
      </div>
      <footer v-if="result.total" class="pagination"><span>第 {{ page }} / {{ totalPages }} 页，共 {{ result.total }} 条</span><label>每页<select v-model.number="pageSize" class="inp page-size" @change="changePageSize"><option :value="20">20</option><option :value="50">50</option><option :value="100">100</option></select>条</label><button class="btn sm" type="button" :disabled="page <= 1 || loading" @click="changePage(page - 1)">上一页</button><button class="btn sm" type="button" :disabled="page >= totalPages || loading" @click="changePage(page + 1)">下一页</button></footer>
    </section>

    <div v-if="detail || detailLoading" class="detail-overlay" @click.self="closeDetail">
      <aside class="detail-drawer" role="dialog" aria-modal="true" aria-label="图纸管理详情">
        <div v-if="detailLoading" class="loading-state"><DemoIcon name="loader-circle" :size="20" />正在读取详情...</div>
        <template v-else-if="detail">
          <header class="drawer-head"><div><span class="eyebrow">图纸资产详情</span><h2>{{ detail.drawing.name }}</h2><div class="drawer-sub mono">{{ detail.drawing.no }} · {{ detail.drawing.project }}</div></div><button class="icon-btn" type="button" aria-label="关闭详情" @click="closeDetail"><DemoIcon name="x" :size="17" /></button></header>
          <div class="drawer-body">
            <section class="summary-box"><div><span>状态</span><strong><span class="tag" :class="statusClass(detail.drawing.status)">{{ statusLabel(detail.drawing.status) }}</span></strong></div><div><span>当前版本</span><strong class="mono">{{ detail.drawing.version }}</strong></div><div><span>零件</span><strong>{{ detail.drawing.partCount }}</strong></div><div><span>附件</span><strong>{{ detail.drawing.attachmentCount }}</strong></div></section>
            <div class="drawer-actions"><button class="btn" type="button" :disabled="busyId === detail.drawing.id" @click="toggleDrawing(detail.drawing)">{{ detail.drawing.status === 'disabled' ? '启用图纸' : '禁用图纸' }}</button><button class="btn danger-button" type="button" :disabled="busyId === detail.drawing.id" @click="hardDelete(detail.drawing)"><DemoIcon name="trash-2" :size="14" />永久删除</button></div>
            <section class="detail-section"><h3><DemoIcon name="folder-tree" :size="15" />结构零件（{{ detailParts.length }}）</h3><div v-if="!detailParts.length" class="section-empty">暂无零件</div><div v-for="item in detailParts" :key="item.id" class="asset-row"><div><strong>{{ item.no }}</strong><span>{{ item.name }}<template v-if="item.parentNo"> · 父级 {{ item.parentNo }}</template></span></div><div class="asset-row-actions"><span class="tag" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span><button class="btn sm" type="button" @click="togglePart(item)">{{ item.status === 'disabled' ? '启用' : '禁用' }}</button></div></div></section>
            <section class="detail-section"><h3><DemoIcon name="paperclip" :size="15" />附件（{{ detailAttachments.length }}）</h3><div v-if="!detailAttachments.length" class="section-empty">暂无附件</div><div v-for="item in detailAttachments" :key="item.id" class="asset-row"><div><strong>{{ attachmentName(item) }}</strong><span>{{ roleLabel(item.role) }} · {{ formatSize(item.size) }} · {{ item.version }}</span></div><div class="asset-row-actions"><span class="mono asset-meta">{{ item.uploadedBy || '未知' }}</span><button class="icon-btn danger-icon" type="button" title="永久删除附件" :disabled="busyId === item.id" @click="removeAttachment(item)"><DemoIcon name="trash-2" :size="14" /></button></div></div></section>
            <section class="detail-section"><h3><DemoIcon name="history" :size="15" />CAD 版本（{{ detail.versions.length }}）</h3><div v-if="!detail.versions.length" class="section-empty">暂无版本记录</div><div v-for="item in detail.versions" :key="item.id" class="asset-row"><div><strong class="mono">{{ item.version }}</strong><span>{{ item.versionKind }} · {{ formatSize(item.size) }} · {{ item.createdBy || '未知' }}</span></div><div class="asset-row-actions"><span class="mono asset-meta">{{ formatTime(item.createdAt) }}</span><button class="btn sm" type="button" :disabled="busyId === item.id" @click="downloadVersion(item)">下载</button><button v-if="item.versionKind !== 'release'" class="btn sm" type="button" :disabled="busyId === item.id" @click="restoreVersion(item)">回退</button></div></div></section>
            <section class="detail-section"><h3><DemoIcon name="edit-3" :size="15" />活动编辑会话（{{ detail.sessions.length }}）</h3><div v-if="!detail.sessions.length" class="section-empty">暂无活动编辑</div><div v-for="item in detail.sessions" :key="String(item.id)" class="asset-row"><div><strong>{{ String(item.fileName || '未命名文件') }}</strong><span>{{ String(item.user || '未知用户') }} · {{ String(item.status || '') }}</span></div><div class="asset-row-actions"><span class="tag warn">编辑中</span><button class="btn sm" type="button" :disabled="busyId === String(item.id)" @click="closeSession(item)">强制结束</button></div></div></section>
          </div>
        </template>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.admin-drawings-page { position: relative; }.section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 4px 0 6px; }.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }.lib-count { display: block; margin-top: 4px; color: var(--text-3); font-size: 11px; }
.filter-card { display: flex; align-items: center; gap: 10px; padding: 12px 14px; margin-bottom: 14px; }.mode-switch { display: flex; gap: 4px; }.mode-btn { display: inline-flex; align-items: center; gap: 6px; padding: 7px 10px; border: 1px solid var(--line); border-radius: 7px; color: var(--text-3); background: transparent; font-size: 12px; cursor: pointer; }.mode-btn.active, .mode-btn:hover { border-color: var(--accent); color: var(--accent); background: var(--active); }.search-box { display: flex; align-items: center; gap: 7px; min-width: 220px; flex: 1; padding: 0 10px; border: 1px solid var(--line); border-radius: 8px; color: var(--text-3); }.search-box input { width: 100%; padding: 8px 0; border: 0; outline: 0; color: var(--text-1); background: transparent; font-size: 12px; }.status-select { width: 120px; }.error-note { display: flex; align-items: center; gap: 8px; margin-bottom: 14px; color: var(--danger); }.error-note span { flex: 1; }.table-card { overflow: hidden; }.table-scroll { overflow-x: auto; }.admin-drawing-table { min-width: 1100px; }.admin-drawing-table th, .admin-drawing-table td { white-space: nowrap; }.sub-text { display: block; margin-top: 3px; color: var(--text-3); font-size: 10px; }.time-cell { color: var(--text-3); font-size: 11px; }.row-actions { position: relative; white-space: nowrap; }.danger-icon { color: var(--danger); }.danger-text { color: var(--danger) !important; }.loading-state { display: flex; align-items: center; justify-content: center; gap: 8px; min-height: 180px; color: var(--text-3); font-size: 12px; }.pagination { display: flex; align-items: center; justify-content: flex-end; gap: 10px; padding: 12px 14px; border-top: 1px solid var(--line); color: var(--text-3); font-size: 11px; }.pagination label { display: inline-flex; align-items: center; gap: 5px; }.page-size { width: 58px; padding: 5px 7px; }
.detail-overlay { position: fixed; z-index: 40; inset: 0; display: flex; justify-content: flex-end; background: rgb(0 0 0 / 42%); }.detail-drawer { width: min(620px, 94vw); height: 100%; overflow: auto; background: var(--panel-top); box-shadow: -10px 0 30px rgb(0 0 0 / 20%); }.drawer-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 14px; padding: 22px; border-bottom: 1px solid var(--line); }.drawer-head h2 { margin-top: 7px; font-size: 18px; line-height: 1.4; }.eyebrow { color: var(--accent); font-size: 10px; letter-spacing: .08em; }.drawer-sub { margin-top: 7px; color: var(--text-3); font-size: 11px; }.drawer-body { padding: 18px 22px 30px; }.summary-box { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); }.summary-box div { display: flex; flex-direction: column; gap: 6px; }.summary-box span { color: var(--text-3); font-size: 10px; }.summary-box strong { color: var(--text-1); font-size: 13px; }.drawer-actions { display: flex; gap: 8px; margin: 14px 0 20px; }.danger-button { color: var(--danger); }.detail-section { margin-top: 22px; }.detail-section h3 { display: flex; align-items: center; gap: 7px; padding-bottom: 9px; border-bottom: 1px solid var(--line); font-size: 13px; }.section-empty { padding: 15px 0; color: var(--text-3); font-size: 11px; }.asset-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; border-bottom: 1px dashed var(--line); }.asset-row > div:first-child { min-width: 0; }.asset-row strong, .asset-row span { display: block; overflow: hidden; text-overflow: ellipsis; }.asset-row strong { color: var(--text-1); font-size: 12px; }.asset-row div span { margin-top: 4px; color: var(--text-3); font-size: 11px; }.asset-row-actions { display: flex; align-items: center; gap: 7px; flex: none; }.asset-row-actions span { display: inline-block !important; }.asset-meta { flex: none; color: var(--text-3); font-size: 10px; }.asset-row:last-child { border-bottom: 0; }
@media (max-width: 800px) { .filter-card { align-items: stretch; flex-wrap: wrap; }.search-box { order: 3; flex-basis: 100%; }.status-select { flex: 1; }.summary-box { grid-template-columns: repeat(2, 1fr); } }.@media (max-width: 520px) { .pagination { flex-wrap: wrap; justify-content: center; }.asset-row { align-items: flex-start; flex-direction: column; }.asset-row-actions { width: 100%; justify-content: space-between; } }
</style>
