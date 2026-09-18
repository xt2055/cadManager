<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRefs, watch } from 'vue'
import { onBeforeRouteLeave, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS } from '@/constants/drawing-status'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useAttributeStore } from '@/stores/attribute.store'
import { useDrawingLibraryUiStore } from '@/stores/drawing-library-ui.store'
import { useUiStore } from '@/stores/ui.store'
import { formatReadableDateTime } from '@/utils/date-time'
import { drawingMediaLabel } from '@/utils/model-formats'
import type { DrawingCreateMode } from '@/features/drawings/create/drawing-create-modes'
import type { DrawingSummaryView } from '@/modules/drawing'
import CreateDrawingDialog from '@/features/drawings/components/CreateDrawingDialog.vue'

defineOptions({ name: 'DrawingLibraryPage' })

const drawingStore = useDrawingStore()
const attributeStore = useAttributeStore()
const authStore = useAuthStore()
const router = useRouter()

const viewState = useDrawingLibraryUiStore().forUser(authStore.currentUser?.id || '')
const { query, status, media, mode, attributeFilters, expandedProjects } = toRefs(viewState)
const pageElement = ref<HTMLElement | null>(null)
const tableElement = ref<HTMLElement | null>(null)
const loading = ref(true)
const loadError = ref('')
let disposed = false
const hasFilters = computed(() => Boolean(query.value.trim() || status.value || media.value || (mode.value === 'drawing' && activeFilterCount.value)))
const availableStatuses = Object.entries(STATUS).filter(([key]) => key !== 'disabled')
const isAdmin = computed(() => authStore.hasRole('admin'))
const uiStore = useUiStore()

// 建档权只对计划员与管理员开放；其他账号看到的是「原因 + 解决入口」，而不是一个点了没反应的按钮。
const canCreateDrawing = computed(() => authStore.canCreateDrawing)
const canAssignTasks = computed(() => authStore.canAssignTasks)
const createDeniedHint = computed(() => '创建图纸需要计划员或管理员权限；设计人员请等待计划员在任务管理台指派图纸，指派后会出现在工作台的「我的任务」中。')

const activeFilterCount = computed(() => Object.values(attributeFilters.value).filter(Boolean).length)

const activeFiltersList = computed(() => {
  return attributeStore.sortedAttributes
    .map((attr) => {
      const fieldId = attributeFilters.value[attr.id]
      if (!fieldId) return null
      const field = attr.fields.find((f) => f.id === fieldId)
      return {
        attributeId: attr.id,
        attributeName: attr.name,
        fieldName: field?.name || '未知选项',
      }
    })
    .filter(Boolean) as Array<{ attributeId: string; attributeName: string; fieldName: string }>
})

function partsForDrawing(drawingNo: string) {
  return drawingStore.parts.filter((part) => part.parentNo === drawingNo)
}

function partMatchesQuery(part: ReturnType<typeof partsForDrawing>[number], q: string): boolean {
  return [part.no, ...part.fileNames].some((value) => value.toLowerCase().includes(q))
}

function projectNoForPart(part: ReturnType<typeof partsForDrawing>[number]): string {
  return part.project || drawingStore.getDrawing(part.parentNo)?.project || ''
}

function partFileNames(part: ReturnType<typeof partsForDrawing>[number]): string[] {
  return part.fileNames
}

function matchesPartSearch(part: ReturnType<typeof partsForDrawing>[number], q: string): boolean {
  if (!q) return true
  return [part.no, part.parentNo, projectNoForPart(part), ...partFileNames(part)]
    .some((value) => value.toLowerCase().includes(q))
}

/* 检索命中零件时只统计、不展开：命中数用来告诉用户「这张卡为什么被搜出来」。
   早先这里直接把命中的父图放进展开集合，结果一搜图号前缀或项目号，整屏卡片的零件清单一齐炸开。 */
const matchedPartCounts = computed(() => {
  const q = query.value.trim().toLowerCase()
  const counts = new Map<string, number>()
  if (!q) return counts
  for (const part of drawingStore.parts) {
    if (!partMatchesQuery(part, q)) continue
    counts.set(part.parentNo, (counts.get(part.parentNo) ?? 0) + 1)
  }
  return counts
})
function matchedPartCount(drawingNo: string): number {
  return matchedPartCounts.value.get(drawingNo) ?? 0
}
function isPartMatched(part: ReturnType<typeof partsForDrawing>[number]): boolean {
  const q = query.value.trim().toLowerCase()
  return Boolean(q) && partMatchesQuery(part, q)
}

const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  const st = status.value
  return drawingStore.drawings.filter((drawing) => {
    if (drawing.status === 'disabled') return false
    if (media.value && drawingMediaLabel(drawing) !== media.value) return false
    if (st && drawing.status !== st) return false

    const attributeText = attributeStore.sortedAttributes.flatMap((attribute) => [
      attribute.name,
      attributeStore.fieldName(attribute.id, drawing.attributeValues?.[attribute.id]),
    ]).join(' ').toLowerCase()

    const matchesQuery = !q || [
      drawing.no,
      drawing.name,
      drawing.project,
      drawing.vendor,
      drawing.remark ?? '',
      attributeText,
    ].some((val) => val.toLowerCase().includes(q))
    const matchesPart = Boolean(q) && partsForDrawing(drawing.no).some((part) => partMatchesQuery(part, q))
    if (!matchesQuery && !matchesPart) return false

    const matchesAttributes = attributeStore.sortedAttributes.every((attribute) => {
      const selected = attributeFilters.value[attribute.id]
      return !selected || drawing.attributeValues?.[attribute.id] === selected
    })
    return matchesAttributes
  })
})

const partRows = computed(() => {
  const q = query.value.trim().toLowerCase()
  const st = status.value
  return drawingStore.parts.filter((part) => {
    if (part.status === 'disabled') return false
    if (media.value && drawingMediaLabel(part) !== media.value) return false
    if (st && part.status !== st) return false
    return matchesPartSearch(part, q)
  })
})

// 展开与否只看用户点过谁：命中零件不再自动展开（详见 matchedPartCounts 的注释）。
function isProjectExpanded(drawingNo: string): boolean {
  return expandedProjects.value.has(drawingNo)
}

function toggleExpanded(drawingNo: string) {
  if (expandedProjects.value.has(drawingNo)) {
    expandedProjects.value.delete(drawingNo)
  } else {
    expandedProjects.value.add(drawingNo)
  }
}

function clearFilters() {
  query.value = ''
  status.value = ''
  media.value = ''
  attributeFilters.value = {}
}

function removeFilter(attributeId: string) {
  delete attributeFilters.value[attributeId]
}

function openDetail(drawingNo: string) {
  const owner = drawingStore.getDrawing(drawingNo) ?? drawingStore.getPart(drawingNo)
  router.push({ name: owner && drawingMediaLabel(owner) === '3D' ? 'drawing-models' : 'drawing-preview', params: { drawingId: drawingNo } })
}

function assigneeName(drawing: DrawingSummaryView): string {
  return drawing.assignees?.[0]?.name?.trim() || ''
}

function creatorName(drawing: DrawingSummaryView): string {
  return drawing.createdBy?.trim() || '—'
}

function assigneeInitial(drawing: DrawingSummaryView): string {
  return assigneeName(drawing).slice(0, 1) || '?'
}

async function loadLibrary(force = false) {
  loading.value = true
  loadError.value = ''
  const results = await Promise.allSettled([force ? drawingStore.refresh() : drawingStore.load(), attributeStore.load()])
  if (disposed) return
  if (results.some(result => result.status === 'rejected')) loadError.value = '图纸或筛选条件加载失败，请检查连接后重试。'
  loading.value = false
  await nextTick()
  if (pageElement.value) pageElement.value.scrollTop = viewState.scrollTop
  if (tableElement.value) tableElement.value.scrollLeft = viewState.tableScrollLeft
}

// 创建图纸入口：三种方式共用同一页面，用 query.mode 区分。
const createMenuOpen = ref(false)

function openCreate(createMode: DrawingCreateMode) {
  createMenuOpen.value = false
  void router.push({ name: 'drawing-create', query: { mode: createMode } })
}

onMounted(() => {
  void loadLibrary()
})
onBeforeUnmount(() => {
  disposed = true
})
onBeforeRouteLeave(() => {
  viewState.scrollTop = pageElement.value?.scrollTop ?? 0
  viewState.tableScrollLeft = tableElement.value?.scrollLeft ?? 0
})
watch([query, status, media, mode, attributeFilters], () => {
  viewState.scrollTop = 0
  if (pageElement.value) pageElement.value.scrollTop = 0
}, { deep: true })
</script>

<template>
  <div ref="pageElement" class="page drawing-library-page" :aria-busy="loading">
    <header class="library-head">
      <div class="head-left">
        <div class="eyebrow"><DemoIcon name="layers" :size="14" /> 企业图纸资产中心</div>
        <h1>工程图纸库</h1>
        <p v-if="loading">正在加载图纸库…</p>
        <p v-else-if="loadError">图纸库暂时不可用</p>
        <p v-else-if="mode === 'drawing'">{{ hasFilters ? '找到' : '已收录' }} {{ rows.length }} 份项目总图</p>
        <p v-else>{{ hasFilters ? '找到' : '已收录' }} {{ partRows.length }} 个零件</p>
      </div>
      <div class="library-actions">
        <button v-if="isAdmin" class="btn" type="button" @click="router.push({ name: 'admin-attributes' })">
          <DemoIcon name="sliders-horizontal" :size="14" />属性管理
        </button>
        <button
          v-if="canAssignTasks"
          class="btn"
          type="button"
          @click="router.push({ name: 'task-board' })"
        >
          <DemoIcon name="clipboard-list" :size="14" />任务管理台
        </button>
        <button
          v-else
          class="btn"
          type="button"
          :title="createDeniedHint"
          @click="uiStore.toast(createDeniedHint, 'warn')"
        >
          <DemoIcon name="info" :size="14" />创建图纸需计划员权限
        </button>
        <button
          v-if="canCreateDrawing"
          class="btn primary"
          type="button"
          aria-haspopup="dialog"
          :aria-expanded="createMenuOpen"
          @click="createMenuOpen = true"
        >
          <DemoIcon name="plus" :size="14" />创建图纸
        </button>
      </div>
    </header>

    <CreateDrawingDialog v-model:open="createMenuOpen" />

    <!-- 属性筛选面板 -->
    <section v-if="mode === 'drawing' && attributeStore.sortedAttributes.length" class="attribute-filter-panel card">
      <div class="filter-panel-head">
        <div class="filter-head-title">
          <DemoIcon name="filter" :size="14" />
          <strong>业务属性组合筛选</strong>
          <span v-if="activeFilterCount" class="filter-count-badge">已启用 {{ activeFilterCount }} 项筛选</span>
        </div>
        <button v-if="activeFilterCount" class="btn-clear" type="button" @click="clearFilters">
          <DemoIcon name="rotate-ccw" :size="12" />重置所有筛选
        </button>
      </div>

      <div class="filter-grid">
        <div v-for="attribute in attributeStore.sortedAttributes" :key="attribute.id" class="filter-field">
          <div class="filter-label">
            <span>{{ attribute.name }}</span>
          </div>
          <div class="filter-select-wrap">
            <select v-model="attributeFilters[attribute.id]" class="inp filter-select" :class="{ 'is-selected': Boolean(attributeFilters[attribute.id]) }">
              <option value="">全部{{ attribute.name }}</option>
              <option v-for="field in attribute.fields.filter((item) => item.enabled)" :key="field.id" :value="field.id">
                {{ field.name }}
              </option>
            </select>
            <span class="filter-caret"><DemoIcon name="chevron-down" :size="12" /></span>
          </div>
        </div>
      </div>

      <!-- 已选筛选胶囊条 -->
      <div v-if="activeFiltersList.length" class="active-chips-bar">
        <span class="chips-label">过滤条件:</span>
        <div class="chips-wrap">
          <span v-for="chip in activeFiltersList" :key="chip.attributeId" class="filter-chip">
            <span class="chip-k">{{ chip.attributeName }}:</span>
            <span class="chip-v">{{ chip.fieldName }}</span>
            <button class="chip-remove" type="button" title="移除此条件" @click="removeFilter(chip.attributeId)">
              <DemoIcon name="x" :size="11" />
            </button>
          </span>
        </div>
      </div>
    </section>

    <!-- 图纸列表表格卡片 -->
    <section class="card library-table-card">
      <div class="table-toolbar">
        <div class="view-mode-switch" role="tablist" aria-label="图纸库模式">
          <button class="mode-btn" :class="{ active: mode === 'drawing' }" type="button" @click="mode = 'drawing'">
            <DemoIcon name="layers" :size="13" />总图模式
          </button>
          <button class="mode-btn" :class="{ active: mode === 'part' }" type="button" @click="mode = 'part'">
            <DemoIcon name="file" :size="13" />零件模式
          </button>
        </div>
        <label class="search-box">
          <DemoIcon name="search" :size="15" />
          <input v-model="query" :placeholder="mode === 'part' ? '搜索零件文件名、所属图号、项目号…' : '搜索图号、名称、项目号、责任单位、属性字段…'" />
          <button v-if="query" class="clear-search" type="button" aria-label="清空搜索" @click="query = ''">
            <DemoIcon name="x" :size="12" />
          </button>
        </label>
        <div class="toolbar-right">
          <select v-model="media" class="inp status-select" aria-label="图纸维度">
            <option value="">全部图纸类型</option>
            <option value="2D">仅 2D</option><option value="3D">仅 3D</option><option value="2D + 3D">2D + 3D</option>
            <option value="未上传图纸">未上传图纸</option>
          </select>
           <select v-model="status" class="inp status-select" aria-label="图纸状态">
             <option value="">全部状态</option>
             <option v-for="[key, item] in availableStatuses" :key="key" :value="key">{{ item.t }}</option>
           </select>
           <button v-if="hasFilters" class="btn" type="button" @click="clearFilters">清空全部筛选</button>
        </div>
      </div>

      <div v-if="loading" class="empty-state-view" role="status"><strong>正在加载图纸…</strong></div>
      <div v-else-if="loadError" class="empty-state-view" role="alert">
        <strong>{{ loadError }}</strong><button class="btn primary" type="button" @click="loadLibrary(true)">重新加载</button>
      </div>
      <div v-else-if="mode === 'drawing'" class="drawing-card-grid">
        <article
          v-for="drawing in rows"
          :key="drawing.no"
          class="drawing-card"
          role="button"
          tabindex="0"
          @click="openDetail(drawing.no)"
          @keydown.enter.self="openDetail(drawing.no)"
        >
          <!-- hover 时右上角滑出的打开箭头 -->
          <span class="dc-open-hint" aria-hidden="true">
            <DemoIcon name="arrow-up-right" :size="13" />
          </span>

          <!-- 顶部：状态 + 介质 -->
          <header class="dc-top">
            <span class="tag dc-status" :class="STATUS[drawing.status].c">
              <i class="dc-status-dot"></i>{{ STATUS[drawing.status].t }}
            </span>
            <span class="dc-media mono">{{ drawingMediaLabel(drawing) }}</span>
          </header>

          <!-- 主体：图号 / 名称 / 备注 -->
          <div class="dc-body">
            <div class="dc-no-row">
              <span class="dc-no mono">{{ drawing.no }}</span>
              <button
                v-if="partsForDrawing(drawing.no).length"
                class="dc-expand"
                type="button"
                :title="isProjectExpanded(drawing.no) ? '收起零件' : '展开零件清单'"
                :aria-expanded="isProjectExpanded(drawing.no)"
                @click.stop="toggleExpanded(drawing.no)"
              >
                <DemoIcon name="chevron-down" :size="12" :class="{ collapsed: !isProjectExpanded(drawing.no) }" />
                {{ partsForDrawing(drawing.no).length }} 个零件
                <span v-if="matchedPartCount(drawing.no)" class="dc-expand-hit">命中 {{ matchedPartCount(drawing.no) }}</span>
              </button>
            </div>
            <h3 class="dc-name" :title="drawing.name">{{ drawing.name }}</h3>
            <p v-if="drawing.remark" class="dc-remark" :title="drawing.remark">{{ drawing.remark }}</p>
          </div>

          <!-- 业务属性 -->
          <div class="dc-attrs">
            <template v-for="attr in attributeStore.sortedAttributes" :key="attr.id">
              <span v-if="drawing.attributeValues?.[attr.id]" class="dc-attr">
                <span class="dc-attr-k">{{ attr.name }}</span>
                <span class="dc-attr-v">{{ attributeStore.fieldName(attr.id, drawing.attributeValues?.[attr.id]) }}</span>
              </span>
            </template>
            <span
              v-if="!attributeStore.sortedAttributes.some((attr) => drawing.attributeValues?.[attr.id])"
              class="dc-attr dc-attr-empty"
            >未指定属性</span>
          </div>

          <!-- 底部：负责人 + 版本信息 -->
          <footer class="dc-foot">
            <div class="dc-owner" :class="{ 'is-empty': !assigneeName(drawing) }">
              <span class="dc-avatar">{{ assigneeInitial(drawing) }}</span>
              <span class="dc-owner-meta">
                <b>{{ assigneeName(drawing) || '待指派' }}</b>
                <i>负责人</i>
              </span>
            </div>
            <div class="dc-foot-right">
              <span class="dc-updated mono">{{ drawing.version }} · {{ formatReadableDateTime(drawing.updatedAt, '—') }}</span>
              <span v-if="creatorName(drawing) !== '—'" class="dc-creator">由 {{ creatorName(drawing) }} 创建</span>
            </div>
          </footer>

          <!-- 展开的零件清单 -->
          <div v-if="isProjectExpanded(drawing.no)" class="dc-parts" @click.stop>
            <span class="dc-parts-title">
              <DemoIcon name="folder-tree" :size="13" />
              项目零件结构清单（共 {{ partsForDrawing(drawing.no).length }} 个<template v-if="matchedPartCount(drawing.no)">，命中 {{ matchedPartCount(drawing.no) }} 个</template>）
            </span>
            <div class="dc-parts-chips">
              <button
                v-for="part in partsForDrawing(drawing.no)"
                :key="part.no"
                class="part-link-chip"
                :class="{ 'is-hit': isPartMatched(part) }"
                type="button"
                @click.stop="openDetail(part.no)"
              >
                <DemoIcon name="file" :size="12" />
                <span class="part-name-txt">{{ part.name }}</span>
                <span class="mono part-no-txt">{{ part.no }}</span>
              </button>
            </div>
          </div>
        </article>

        <div v-if="!rows.length" class="empty-state-view">
          <DemoIcon name="search-x" :size="36" />
          <strong>{{ hasFilters ? '没有找到符合条件的图纸' : '图纸库还没有图纸' }}</strong>
          <span>{{ hasFilters ? '调整关键词，或清空筛选后重新查找。' : '创建第一份图纸，开始建立项目档案。' }}</span>
          <button v-if="hasFilters" class="btn" type="button" @click="clearFilters">清空全部筛选</button>
          <template v-else-if="canCreateDrawing">
            <button class="btn primary" type="button" @click="openCreate('legacy')">上传老图纸</button>
            <button class="btn" type="button" @click="openCreate('new')">创建新图纸</button>
          </template>
          <template v-else>
            <span class="empty-state-hint">{{ createDeniedHint }}</span>
            <button class="btn" type="button" @click="router.push({ name: 'dashboard' })">回到工作台查看我的任务</button>
          </template>
        </div>
      </div>
      <div v-else ref="tableElement" class="table-scroll">
        <table class="tbl library-table part-library-table">
          <thead>
            <tr>
              <th style="width: 150px;">零件图号</th>
              <th style="width: 240px;">文件名</th>
              <th style="width: 150px;">所属图号</th>
              <th style="width: 140px;">项目号</th>
              <th>零件名称</th>
              <th style="width: 90px;">状态</th>
              <th style="width: 70px;">版本</th>
              <th style="width: 90px; text-align: center;">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="part in partRows" :key="part.no">
              <td class="mono no-cell">
                <button class="link drawing-no-link" type="button" @click="openDetail(part.no)">
                  {{ part.no }}
                </button>
              </td>
              <td>
                <span v-if="partFileNames(part).length" class="part-file-list">
                  {{ partFileNames(part).join('、') }}
                </span>
                <span v-else class="muted-text">未上传零件图纸</span>
              </td>
              <td class="mono">{{ part.parentNo || '—' }}</td>
              <td class="mono">{{ projectNoForPart(part) || '—' }}</td>
              <td>
                <strong>{{ part.name }}</strong>
                <span class="tag plain">{{ drawingMediaLabel(part) }}</span>
                <small v-if="part.remark">{{ part.remark }}</small>
              </td>
              <td><span class="tag" :class="STATUS[part.status].c">{{ STATUS[part.status].t }}</span></td>
              <td class="mono">{{ part.version }}</td>
              <td class="row-actions">
                <button class="btn sm" type="button" @click="openDetail(part.no)">
                  <DemoIcon name="eye" :size="13" />详情
                </button>
              </td>
            </tr>
            <tr v-if="!partRows.length">
              <td colspan="8">
                <div class="empty-state-view">
                  <DemoIcon name="search-x" :size="36" />
                  <strong>{{ hasFilters ? '没有找到符合条件的零件' : '还没有零件图纸' }}</strong>
                  <span>{{ hasFilters ? '调整关键词，或清空筛选后重新查找。' : '创建项目并导入零件图后，会显示在这里。' }}</span>
                  <button v-if="hasFilters" class="btn" type="button" @click="clearFilters">清空全部筛选</button>
                  <template v-else-if="canCreateDrawing">
                    <button class="btn primary" type="button" @click="openCreate('legacy')">上传老图纸</button>
                    <button class="btn" type="button" @click="openCreate('new')">创建新图纸</button>
                  </template>
                  <template v-else>
                    <span class="empty-state-hint">{{ createDeniedHint }}</span>
                    <button class="btn" type="button" @click="router.push({ name: 'dashboard' })">回到工作台查看我的任务</button>
                  </template>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<style scoped>
.drawing-library-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 100%;
  padding: 22px 26px;
}

.library-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
}

.head-left h1 {
  margin: 5px 0 4px;
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 900;
}

.head-left p {
  margin: 0;
  color: var(--text-3);
  font-size: 12px;
}

.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--accent);
  font: 11px 'JetBrains Mono', monospace;
  letter-spacing: 0.5px;
}

.library-actions {
  display: flex;
  gap: 8px;
}

/* ================= 筛选面板 ================= */
.attribute-filter-panel {
  padding: 14px 18px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: var(--panel);
}

.filter-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.filter-head-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--text-1);
}

.filter-head-title strong {
  font-size: 13px;
  font-weight: 700;
}

.filter-count-badge {
  padding: 2px 7px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 11px;
  font-weight: 600;
}

.btn-clear {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  color: var(--accent);
  cursor: pointer;
  font-size: 11.5px;
  padding: 4px 6px;
  border-radius: 4px;
  transition: background 0.15s;
}

.btn-clear:hover {
  background: var(--hover);
}

.filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px 14px;
}

.filter-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.filter-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-2);
}

.filter-label em {
  font-size: 10px;
  font-style: normal;
  color: var(--warn);
}

.filter-select-wrap {
  position: relative;
  display: flex;
  align-items: center;
}

.filter-select {
  width: 100%;
  height: 32px;
  padding: 0 24px 0 8px;
  font-size: 12px;
  background: var(--panel-2);
  border-color: var(--line);
}

.filter-select.is-selected {
  border-color: var(--accent);
  background: var(--active);
  font-weight: 600;
  color: var(--accent);
}

.filter-caret {
  position: absolute;
  right: 8px;
  color: var(--text-3);
  pointer-events: none;
  display: flex;
}

/* ================= 筛选胶囊条 ================= */
.active-chips-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--line);
}

.chips-label {
  font-size: 11px;
  color: var(--text-3);
  white-space: nowrap;
}

.chips-wrap {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.filter-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--accent-soft);
  border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent);
  color: var(--accent);
  font-size: 11px;
}

.chip-k {
  opacity: 0.8;
}

.chip-v {
  font-weight: 600;
}

.chip-remove {
  display: grid;
  place-items: center;
  width: 14px;
  height: 14px;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--accent);
  cursor: pointer;
}

.chip-remove:hover {
  background: var(--accent);
  color: var(--panel);
}

/* ================= 表格与工具栏 ================= */
.library-table-card {
  padding: 0;
  overflow: hidden;
}

.table-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--line);
}

.view-mode-switch {
  display: inline-flex;
  flex-shrink: 0;
  padding: 3px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-2);
}

.mode-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 26px;
  padding: 0 9px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  font-size: 11px;
  font-weight: 600;
}

.mode-btn.active {
  background: var(--accent-soft);
  color: var(--accent);
}

.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  max-width: 460px;
  padding: 7px 10px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-2);
  color: var(--text-3);
}

.search-box input {
  width: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-1);
  font-size: 12px;
}

.clear-search {
  border: 0;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  display: grid;
  place-items: center;
}

.toolbar-right {
  display: flex;
  gap: 8px;
}

.status-select {
  width: 120px;
  height: 32px;
  font-size: 12px;
}

.part-file-list {
  display: block;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-1);
}

.muted-text {
  color: var(--text-3);
}

.table-scroll {
  overflow-x: auto;
}

.library-table {
  min-width: 960px;
  width: 100%;
  border-collapse: collapse;
}

.library-table th {
  padding: 10px 14px;
  background: var(--panel-2);
  color: var(--text-3);
  font-size: 11px;
  font-weight: 600;
  text-align: left;
  border-bottom: 1px solid var(--line);
}

.library-table td {
  padding: 10px 14px;
  color: var(--text-2);
  border-bottom: 1px solid var(--line);
  vertical-align: middle;
}

.library-table td strong {
  display: block;
  color: var(--text-1);
  font-size: 12.5px;
}

.library-table td small {
  display: block;
  max-width: 220px;
  margin-top: 2px;
  overflow: hidden;
  color: var(--text-3);
  font-size: 10.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.no-cell {
  white-space: nowrap;
  font-weight: 600;
}

.drawing-no-link {
  color: var(--accent) !important;
  font-size: 12.5px;
}

.row-actions {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  white-space: nowrap;
}

/* ================= 总图卡片视图 ================= */
.drawing-card-grid {
  display: grid;
  /* 展开零件会让卡片变高；默认的 stretch 会把同一行的其它卡一起撑高，看起来像「全都展开了」。 */
  align-items: start;
  grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
  gap: 14px;
  padding: 16px;
}

.drawing-card-grid > .empty-state-view {
  grid-column: 1 / -1;
}

.drawing-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 15px 16px 14px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--panel-2);
  cursor: pointer;
  transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
  animation: dc-in 0.25s ease;
}

@keyframes dc-in {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
}

/* 顶部高光线：hover 时显现，给出微妙的“被激活”感 */
.drawing-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 14px;
  right: 14px;
  height: 2px;
  border-radius: 2px;
  background: linear-gradient(90deg, var(--accent), transparent 75%);
  opacity: 0;
  transition: opacity 0.18s ease;
}

.drawing-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--accent) 45%, var(--line));
  box-shadow: var(--shadow);
}

.drawing-card:hover::before {
  opacity: 1;
}

.drawing-card:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

/* hover 时右上角滑出的打开箭头 */
.dc-open-hint {
  position: absolute;
  top: 12px;
  right: 12px;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 7px;
  background: var(--accent-soft);
  color: var(--accent);
  opacity: 0;
  transform: translate(-3px, 3px);
  transition: opacity 0.18s ease, transform 0.18s ease;
  pointer-events: none;
}

.drawing-card:hover .dc-open-hint {
  opacity: 1;
  transform: translate(0, 0);
}

/* ---------- 顶部行：状态 + 介质 ---------- */
.dc-top {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-right: 26px; /* 给 hover 箭头留位 */
}

.dc-status {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

/* 状态标签自带发光圆点，隐藏全局 .tag 的默认圆点避免重复 */
.dc-status::before {
  display: none;
}

.dc-status-dot {
  display: inline-block;
  flex-shrink: 0;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentcolor;
  box-shadow: 0 0 6px currentcolor;
}

.dc-media {
  padding: 2px 7px;
  border: 1px solid var(--line);
  border-radius: 5px;
  background: var(--panel);
  color: var(--text-3);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.5px;
  white-space: nowrap;
}

/* ---------- 主体：图号 / 名称 / 备注 ---------- */
.dc-body {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.dc-no-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-height: 20px;
}

.dc-no {
  overflow: hidden;
  color: var(--text-3);
  font-size: 11px;
  letter-spacing: 0.3px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dc-expand {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  height: 20px;
  padding: 0 8px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  font-size: 10.5px;
  transition: all 0.15s ease;
}

.dc-expand:hover {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}

.dc-expand svg {
  transition: transform 0.18s ease;
}

.dc-expand .collapsed {
  transform: rotate(-90deg);
}

/* 检索命中零件：只在卡片上标出「命中 N」，不代替用户展开清单，避免一搜关键词整屏卡牌同时炸开。 */
.dc-expand-hit {
  padding: 0 5px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
}

.dc-name {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--text-1);
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 800;
  line-height: 1.4;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  transition: color 0.15s ease;
}

.drawing-card:hover .dc-name {
  color: var(--accent);
}

.dc-remark {
  margin: 0;
  overflow: hidden;
  color: var(--text-3);
  font-size: 11.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ---------- 属性区 ---------- */
.dc-attrs {
  display: flex;
  flex-wrap: wrap;
  align-content: flex-start;
  gap: 6px;
  min-height: 24px;
}

.dc-attr {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 100%;
  padding: 3px 8px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--panel);
  font-size: 11px;
  line-height: 1.5;
}

.dc-attr-k {
  flex-shrink: 0;
  color: var(--text-3);
}

.dc-attr-v {
  overflow: hidden;
  color: var(--text-1);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dc-attr-empty {
  border-style: dashed;
  background: transparent;
  color: var(--text-3);
}

/* ---------- 底部：负责人 + 版本信息 ---------- */
.dc-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid var(--line);
}

.dc-owner {
  display: flex;
  align-items: center;
  gap: 9px;
  min-width: 0;
}

.dc-avatar {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 30px;
  height: 30px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--accent-soft), color-mix(in srgb, var(--accent) 24%, transparent));
  color: var(--accent);
  font-size: 13px;
  font-weight: 800;
}

.dc-owner-meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 1.3;
}

.dc-owner-meta b {
  overflow: hidden;
  color: var(--text-1);
  font-size: 12.5px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dc-owner-meta i {
  color: var(--text-3);
  font-size: 10px;
  font-style: normal;
  letter-spacing: 0.4px;
}

.dc-owner.is-empty .dc-avatar {
  border: 1px dashed var(--line-strong);
  background: transparent;
  color: var(--text-3);
}

.dc-owner.is-empty .dc-owner-meta b {
  color: var(--text-3);
  font-weight: 600;
}

.dc-foot-right {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 2px;
  flex-shrink: 0;
  text-align: right;
}

.dc-updated,
.dc-creator {
  font-size: 10.5px;
  white-space: nowrap;
}

.dc-updated {
  color: var(--text-2);
}

.dc-creator {
  color: var(--text-3);
}

/* ---------- 展开的零件清单 ---------- */
.dc-parts {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel);
  cursor: default;
}

.dc-parts-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text-3);
  font-size: 11px;
}

.dc-parts-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.part-link-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 4px 10px;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--panel-2);
  color: var(--text-2);
  cursor: pointer;
  font-size: 11px;
  transition: all 0.15s ease;
}

.part-link-chip:hover {
  transform: translateY(-1px);
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}

.part-name-txt {
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.part-no-txt {
  color: var(--text-3);
  font-size: 10px;
}

.part-link-chip:hover .part-no-txt {
  color: inherit;
  opacity: 0.7;
}

/* 展开后命中的零件本身再强调一次，用户能直接看到这张卡为什么被搜出来。 */
.part-link-chip.is-hit {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}

.part-link-chip.is-hit .part-no-txt {
  color: inherit;
  opacity: 0.7;
}

.empty-state-view {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 40px 10px;
  color: var(--text-3);
}

.empty-state-view strong {
  font-size: 13px;
  color: var(--text-2);
}

.empty-state-view span {
  font-size: 11.5px;
}

/* 无建档权时的说明文字：读起来是解释而不是报错，但必须比普通提示更容易被看到。 */
.empty-state-hint {
  max-width: 460px;
  line-height: 1.6;
  color: var(--text-2);
}

@media (max-width: 768px) {
  .drawing-library-page {
    padding: 14px;
  }
  .library-head {
    flex-direction: column;
  }
  .library-actions {
    width: 100%;
  }
  .library-actions .btn {
    flex: 1;
    justify-content: center;
  }
  .table-toolbar {
    flex-wrap: wrap;
  }
  .search-box {
    max-width: none;
    flex-basis: 100%;
  }
}
</style>
