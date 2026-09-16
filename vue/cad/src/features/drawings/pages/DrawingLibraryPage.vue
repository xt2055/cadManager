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
import { CREATE_MODE_OPTIONS, type DrawingCreateMode } from '@/features/drawings/create/drawing-create-modes'

defineOptions({ name: 'DrawingLibraryPage' })

const drawingStore = useDrawingStore()
const attributeStore = useAttributeStore()
const authStore = useAuthStore()
const router = useRouter()

const viewState = useDrawingLibraryUiStore().forUser(authStore.currentUser?.id || '')
const { query, status, media, mode, attributeFilters, expandedProjects, collapsedProjects } = toRefs(viewState)
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

const matchingPartProjects = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return new Set<string>()
  return new Set(
    drawingStore.parts
      .filter((part) => partMatchesQuery(part, q))
      .map((part) => part.parentNo),
  )
})

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

function isProjectExpanded(drawingNo: string): boolean {
  return !collapsedProjects.value.has(drawingNo) && (expandedProjects.value.has(drawingNo) || matchingPartProjects.value.has(drawingNo))
}

function toggleExpanded(drawingNo: string) {
  if (isProjectExpanded(drawingNo)) {
    expandedProjects.value.delete(drawingNo)
    collapsedProjects.value.add(drawingNo)
  } else {
    collapsedProjects.value.delete(drawingNo)
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
const createMenuElement = ref<HTMLElement | null>(null)
const createModeEntries = CREATE_MODE_OPTIONS

function openCreate(createMode: DrawingCreateMode) {
  createMenuOpen.value = false
  void router.push({ name: 'drawing-create', query: { mode: createMode } })
}

function closeCreateMenuOnOutsideClick(event: MouseEvent) {
  if (!createMenuOpen.value) return
  if (createMenuElement.value?.contains(event.target as Node)) return
  createMenuOpen.value = false
}

function closeCreateMenuOnEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') createMenuOpen.value = false
}
onMounted(() => {
  void loadLibrary()
  document.addEventListener('click', closeCreateMenuOnOutsideClick)
  document.addEventListener('keydown', closeCreateMenuOnEscape)
})
onBeforeUnmount(() => {
  disposed = true
  document.removeEventListener('click', closeCreateMenuOnOutsideClick)
  document.removeEventListener('keydown', closeCreateMenuOnEscape)
})
onBeforeRouteLeave(() => {
  viewState.scrollTop = pageElement.value?.scrollTop ?? 0
  viewState.tableScrollLeft = tableElement.value?.scrollLeft ?? 0
})
watch(query, () => { collapsedProjects.value.clear() })
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
        <div v-if="canCreateDrawing" ref="createMenuElement" class="create-menu">
          <button
            class="btn primary"
            type="button"
            aria-haspopup="menu"
            :aria-expanded="createMenuOpen"
            @click="createMenuOpen = !createMenuOpen"
          >
            <DemoIcon name="plus" :size="14" />创建图纸<DemoIcon name="chevron-down" :size="12" />
          </button>
          <div v-if="createMenuOpen" class="dropdown create-menu-panel" role="menu" aria-label="创建图纸方式">
            <button
              v-for="option in createModeEntries"
              :key="option.value"
              class="dd-item create-menu-item"
              type="button"
              role="menuitem"
              @click="openCreate(option.value)"
            >
              <DemoIcon :name="option.icon" :size="15" />
              <span class="create-menu-text">
                <b>{{ option.title }}</b>
                <small>{{ option.sub }}</small>
              </span>
            </button>
          </div>
        </div>
      </div>
    </header>

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
      <div v-else ref="tableElement" class="table-scroll">
        <table v-if="mode === 'drawing'" class="tbl library-table">
          <thead>
            <tr>
              <th style="width: 140px;">总图图号</th>
              <th style="width: 180px;">图纸名称</th>
              <th>业务属性规格</th>
              <th style="width: 120px;">责任单位</th>
              <th style="width: 90px;">状态</th>
              <th style="width: 70px;">版本</th>
              <th style="width: 110px;">更新时间</th>
              <th style="width: 100px; text-align: center;">操作</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="drawing in rows" :key="drawing.no">
              <tr>
                <td class="mono no-cell">
                  <button
                    v-if="partsForDrawing(drawing.no).length"
                    class="expand-btn"
                    type="button"
                    :title="isProjectExpanded(drawing.no) ? '收起零件' : '展开零件清单'"
                    :aria-expanded="isProjectExpanded(drawing.no)"
                    @click="toggleExpanded(drawing.no)"
                  >
                    <DemoIcon name="chevron-down" :size="13" :class="{ collapsed: !isProjectExpanded(drawing.no) }" />
                  </button>
                  <button class="link drawing-no-link" type="button" @click="openDetail(drawing.no)">
                    {{ drawing.no }}
                  </button>
                </td>

                <td>
                  <strong>{{ drawing.name }}</strong>
                  <span class="tag plain">{{ drawingMediaLabel(drawing) }}</span>
                  <small v-if="drawing.remark">{{ drawing.remark }}</small>
                </td>

                <!-- 属性规格标签化展示 -->
                <td class="attribute-summary-cell">
                  <div v-if="attributeStore.sortedAttributes.some((attr) => drawing.attributeValues?.[attr.id])" class="attr-pill-group">
                    <template v-for="attr in attributeStore.sortedAttributes" :key="attr.id">
                      <span v-if="drawing.attributeValues?.[attr.id]" class="attr-tag">
                        <span class="attr-k">{{ attr.name }}:</span>
                        <span class="attr-v">{{ attributeStore.fieldName(attr.id, drawing.attributeValues?.[attr.id]) }}</span>
                      </span>
                    </template>
                  </div>
                  <span v-else class="tag plain empty-attr-tag">未指定属性</span>
                </td>

                <td>{{ drawing.vendor || '—' }}</td>
                <td><span class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span></td>
                <td class="mono">{{ drawing.version }}</td>
                <td class="mono text-time">{{ formatReadableDateTime(drawing.updatedAt, '—') }}</td>
                <td class="row-actions">
                  <button class="btn sm" type="button" @click="openDetail(drawing.no)">
                    <DemoIcon name="eye" :size="13" />详情
                  </button>
                </td>
              </tr>

              <!-- 展开的零件清单折叠面板 -->
              <tr v-if="isProjectExpanded(drawing.no)" class="parts-row">
                <td colspan="8">
                  <div class="parts-panel">
                    <span class="parts-title">
                      <DemoIcon name="folder-tree" :size="13" />
                      项目零件结构清单（共 {{ partsForDrawing(drawing.no).length }} 个零件）
                    </span>
                    <div class="parts-chips">
                      <button
                        v-for="part in partsForDrawing(drawing.no)"
                        :key="part.no"
                        class="part-link-chip"
                        type="button"
                        @click="openDetail(part.no)"
                      >
                        <DemoIcon name="file" :size="12" />
                        <span class="part-name-txt">{{ part.name }}</span>
                        <span class="mono part-no-txt">{{ part.no }}</span>
                      </button>
                    </div>
                  </div>
                </td>
              </tr>
            </template>

            <tr v-if="!rows.length">
              <td colspan="8">
                <div class="empty-state-view">
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
              </td>
            </tr>
          </tbody>
        </table>
        <table v-else class="tbl library-table part-library-table">
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

/* 创建图纸下拉：三种创建方式（创建新图纸 / 上传老图纸 / 从老图纸分叉） */
.create-menu {
  position: relative;
}

.create-menu-panel {
  top: calc(100% + 6px);
  right: 0;
  min-width: 250px;
}

.create-menu-item {
  align-items: flex-start;
}

.create-menu-text {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.create-menu-text b {
  font-size: 12.5px;
  font-weight: 600;
}

.create-menu-text small {
  color: var(--text-3);
  font-size: 10.5px;
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

.expand-btn {
  margin-right: 6px;
  padding: 2px;
  border: 0;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  vertical-align: middle;
}

.expand-btn .collapsed {
  transform: rotate(-90deg);
}

/* ================= 属性规格标签化 ================= */
.attribute-summary-cell {
  max-width: 320px;
}

.attr-pill-group {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 6px;
}

.attr-tag {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  font-size: 11px;
  white-space: nowrap;
}

.attr-k {
  color: var(--text-3);
}

.attr-v {
  color: var(--text-1);
  font-weight: 600;
}

.empty-attr-tag {
  font-size: 10.5px;
  opacity: 0.7;
}

.text-time {
  font-size: 11.5px;
  color: var(--text-3);
}

.row-actions {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  white-space: nowrap;
}

/* ================= 展开零件卡片 ================= */
.parts-row {
  background: var(--panel-2);
}

.parts-panel {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 18px;
}

.parts-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--text-3);
  font-size: 11px;
}

.parts-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.part-link-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 9px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--panel);
  color: var(--text-2);
  cursor: pointer;
  font-size: 11.5px;
  transition: all 0.15s;
}

.part-link-chip:hover {
  border-color: var(--accent);
  color: var(--accent);
}

.part-name-txt {
  font-weight: 500;
}

.part-no-txt {
  color: var(--text-3);
  font-size: 10.5px;
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
