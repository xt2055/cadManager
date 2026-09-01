<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingStatus } from '@/types/domain.types'

defineOptions({ name: 'DrawingLibraryPage' })

const domainStore = useDomainStore()
const router = useRouter()
const uiStore = useUiStore()

const query = ref('')
const status = ref<DrawingStatus | ''>('')
const attributeFilters = ref<Record<string, string>>({})
const menuFor = ref<string | null>(null)
const expandedProjects = ref<Set<string>>(new Set())

const activeFilterCount = computed(() => Object.values(attributeFilters.value).filter(Boolean).length)

const activeFiltersList = computed(() => {
  return domainStore.sortedAttributes
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

const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  const st = status.value
  return domainStore.drawings.filter((drawing) => {
    if (domainStore.hiddenList.some((item) => item.no === drawing.no)) return false
    if (st && drawing.status !== st) return false

    const attributeText = domainStore.sortedAttributes.flatMap((attribute) => [
      attribute.name,
      domainStore.attributeFieldName(attribute.id, drawing.attributeValues?.[attribute.id]),
    ]).join(' ').toLowerCase()

    const matchesQuery = !q || [
      drawing.no,
      drawing.name,
      drawing.project,
      drawing.vendor,
      drawing.remark ?? '',
      attributeText,
    ].some((val) => val.toLowerCase().includes(q))
    if (!matchesQuery) return false

    const matchesAttributes = domainStore.sortedAttributes.every((attribute) => {
      const selected = attributeFilters.value[attribute.id]
      return !selected || drawing.attributeValues?.[attribute.id] === selected
    })
    return matchesAttributes
  })
})

function partsForDrawing(drawingNo: string) {
  return domainStore.structure.filter((part) => part.parentNo === drawingNo)
}

function toggleExpanded(drawingNo: string) {
  if (expandedProjects.value.has(drawingNo)) {
    expandedProjects.value.delete(drawingNo)
  } else {
    expandedProjects.value.add(drawingNo)
  }
}

function clearFilters() {
  attributeFilters.value = {}
}

function removeFilter(attributeId: string) {
  delete attributeFilters.value[attributeId]
}

function openDetail(drawingNo: string) {
  menuFor.value = null
  domainStore.openDrawing(drawingNo)
  router.push({ name: 'drawing-preview', params: { drawingId: drawingNo } })
}

function toggleMenu(drawingNo: string) {
  menuFor.value = menuFor.value === drawingNo ? null : drawingNo
}

function menuAction(action: 'detail' | 'hide', drawingNo: string) {
  menuFor.value = null
  if (action === 'detail') openDetail(drawingNo)
  if (action === 'hide') {
    domainStore.hideDrawing(drawingNo)
    uiStore.toast(`图纸「${drawingNo}」已隐藏`, 'ok')
  }
}

onMounted(() => {
  domainStore.initialize().catch(() => undefined)
})
</script>

<template>
  <div class="page drawing-library-page">
    <header class="library-head">
      <div class="head-left">
        <div class="eyebrow"><DemoIcon name="layers" :size="14" /> 企业图纸资产中心</div>
        <h1>工程图纸库</h1>
        <p>支持按企业标准化业务属性组合筛选，已收录 {{ rows.length }} 份项目总图</p>
      </div>
      <div class="library-actions">
        <button class="btn" type="button" @click="router.push({ name: 'admin-attributes' })">
          <DemoIcon name="sliders-horizontal" :size="14" />属性管理
        </button>
        <button class="btn primary" type="button" @click="router.push({ name: 'drawing-create' })">
          <DemoIcon name="plus" :size="14" />创建图纸
        </button>
      </div>
    </header>

    <!-- 属性筛选面板 -->
    <section v-if="domainStore.sortedAttributes.length" class="attribute-filter-panel card">
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
        <div v-for="attribute in domainStore.sortedAttributes" :key="attribute.id" class="filter-field">
          <div class="filter-label">
            <span>{{ attribute.name }}</span>
            <em v-if="attribute.required">必填项</em>
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
        <label class="search-box">
          <DemoIcon name="search" :size="15" />
          <input v-model="query" placeholder="搜索图号、名称、项目号、责任单位、属性字段…" />
          <button v-if="query" class="clear-search" type="button" @click="query = ''">
            <DemoIcon name="x" :size="12" />
          </button>
        </label>
        <div class="toolbar-right">
          <select v-model="status" class="inp status-select">
            <option value="">全部状态</option>
            <option v-for="(item, key) in STATUS" :key="key" :value="key">{{ item.t }}</option>
          </select>
        </div>
      </div>

      <div class="table-scroll">
        <table class="tbl library-table">
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
                    :title="expandedProjects.has(drawing.no) ? '收起零件' : '展开零件清单'"
                    @click="toggleExpanded(drawing.no)"
                  >
                    <DemoIcon name="chevron-down" :size="13" :class="{ collapsed: !expandedProjects.has(drawing.no) }" />
                  </button>
                  <button class="link drawing-no-link" type="button" @click="openDetail(drawing.no)">
                    {{ drawing.no }}
                  </button>
                </td>

                <td>
                  <strong>{{ drawing.name }}</strong>
                  <small v-if="drawing.remark">{{ drawing.remark }}</small>
                </td>

                <!-- 属性规格标签化展示 -->
                <td class="attribute-summary-cell">
                  <div v-if="domainStore.sortedAttributes.some((attr) => drawing.attributeValues?.[attr.id])" class="attr-pill-group">
                    <template v-for="attr in domainStore.sortedAttributes" :key="attr.id">
                      <span v-if="drawing.attributeValues?.[attr.id]" class="attr-tag">
                        <span class="attr-k">{{ attr.name }}:</span>
                        <span class="attr-v">{{ domainStore.attributeFieldName(attr.id, drawing.attributeValues?.[attr.id]) }}</span>
                      </span>
                    </template>
                  </div>
                  <span v-else class="tag plain empty-attr-tag">未指定属性</span>
                </td>

                <td>{{ drawing.vendor || '—' }}</td>
                <td><span class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span></td>
                <td class="mono">{{ drawing.ver }}</td>
                <td class="mono text-time">{{ drawing.updated }}</td>
                <td class="row-actions">
                  <button class="btn sm" type="button" @click="openDetail(drawing.no)">
                    <DemoIcon name="eye" :size="13" />详情
                  </button>
                  <button class="icon-btn" type="button" @click="toggleMenu(drawing.no)">
                    <DemoIcon name="ellipsis" :size="15" />
                  </button>
                  <div v-if="menuFor === drawing.no" class="dropdown row-dropdown">
                    <button class="dd-item" type="button" @click="menuAction('detail', drawing.no)">查看详情</button>
                    <button class="dd-item danger" type="button" @click="menuAction('hide', drawing.no)">隐藏图纸</button>
                  </div>
                </td>
              </tr>

              <!-- 展开的零件清单折叠面板 -->
              <tr v-if="expandedProjects.has(drawing.no)" class="parts-row">
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
                  <strong>没有找到符合条件的图纸</strong>
                  <span>尝试调整搜索关键词或重置业务属性筛选</span>
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

.dropdown {
  position: absolute;
  right: 0;
  top: calc(100% + 4px);
  z-index: 20;
  min-width: 120px;
  padding: 5px;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
}

.dd-item {
  display: block;
  width: 100%;
  padding: 7px 9px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--text-2);
  text-align: left;
  cursor: pointer;
  font-size: 11.5px;
}

.dd-item:hover {
  background: var(--hover);
  color: var(--text-1);
}

.dd-item.danger {
  color: var(--danger);
}

.dd-item.danger:hover {
  background: color-mix(in srgb, var(--danger) 12%, transparent);
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
