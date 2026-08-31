<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { CategoryTreeNode, DrawingStatus } from '@/types/domain.types'

defineOptions({
  name: 'DrawingLibraryPage',
})

const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()

const query = ref('')
const status = ref<DrawingStatus | ''>('')
const selectedCategoryId = ref<string>('') // ''=全部, '__none__'=未分类, 其它=具体分类ID
const categoryTreeSearch = ref('')
const isCategoryPanelCollapsed = ref(false)
const expandedCategoryKeys = ref<Set<string>>(new Set())

const menuFor = ref<string | null>(null)
const expandedProjects = ref<Set<string>>(new Set())

// 计算所有符合条件的图纸列表
const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  return domainStore.drawings.filter((drawing) => {
    const parts = partsForDrawing(drawing.no)
    const matchesQuery =
      !q ||
      [
        drawing.no,
        drawing.name,
        drawing.vendor,
        drawing.project,
        domainStore.getCategoryFullPath(drawing.categoryId),
        ...parts.flatMap((part) => [part.no, part.name]),
      ]
        .join(' ')
        .toLowerCase()
        .includes(q)

    const matchesStatus = !status.value || drawing.status === status.value
    const matchesCategory = matchesSelectedCategory(drawing)
    return matchesQuery && matchesStatus && matchesCategory
  })
})

function matchesSelectedCategory(drawing: { categoryId?: string }): boolean {
  if (!selectedCategoryId.value) return true
  if (selectedCategoryId.value === '__none__') return !drawing.categoryId
  if (drawing.categoryId === selectedCategoryId.value) return true
  // 递归穿透：如果选中了父节点，包含其所有子孙节点下的图纸
  const descendants = domainStore.getDescendantCategoryIds(selectedCategoryId.value)
  return Boolean(drawing.categoryId && descendants.includes(drawing.categoryId))
}

const activeCategoryBreadcrumbs = computed(() => {
  if (!selectedCategoryId.value) return []
  if (selectedCategoryId.value === '__none__') {
    return [{ id: '__none__', name: '未分类图纸' }]
  }
  const path = domainStore.getCategoryPath(selectedCategoryId.value)
  let curr = domainStore.categories.find((c) => c.id === selectedCategoryId.value)
  const items: Array<{ id: string; name: string }> = []
  while (curr) {
    items.unshift({ id: curr.id, name: curr.name })
    curr = curr.parentId ? domainStore.categories.find((c) => c.id === curr!.parentId) : undefined
  }
  return items
})

const unclassifiedCount = computed(() => {
  return domainStore.drawings.filter((d) => !d.categoryId).length
})

function isCategoryExpanded(id: string): boolean {
  if (categoryTreeSearch.value.trim()) return true
  return expandedCategoryKeys.value.has(id)
}

function toggleCategoryExpand(id: string, e?: Event) {
  e?.stopPropagation()
  const next = new Set(expandedCategoryKeys.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedCategoryKeys.value = next
}

function selectCategory(id: string) {
  selectedCategoryId.value = id
}

function clearCategoryFilter() {
  selectedCategoryId.value = ''
}

function partsForDrawing(drawingNo: string) {
  return domainStore.structure
    .filter((part) => part.no.startsWith(`${drawingNo}-`) || part.parentNo === drawingNo)
    .sort((left, right) => left.no.localeCompare(right.no, undefined, { numeric: true }))
}

function isExpanded(no: string) {
  return expandedProjects.value.has(no)
}

function toggleExpanded(no: string) {
  const next = new Set(expandedProjects.value)
  if (next.has(no)) next.delete(no)
  else next.add(no)
  expandedProjects.value = next
}

function openDetail(no: string) {
  domainStore.openDrawing(no)
  router.push({ name: 'drawing-preview', params: { drawingId: no } })
}

function createDrawing() {
  router.push({ name: 'drawing-create' })
}

function toggleMenu(no: string) {
  menuFor.value = menuFor.value === no ? null : no
}

function menuAction(action: string, no: string) {
  menuFor.value = null
  if (action === 'detail') openDetail(no)
  if (action === 'hide') uiStore.toast('图纸已隐藏：用户不可见，管理员可随时恢复，历史完整保留', 'warn')
}

function goCategoryAdmin() {
  router.push({ name: 'admin-categories' })
}
</script>

<template>
  <div class="page library-page-pro">
    <!-- 主界面双栏容器 -->
    <div class="lib-layout-container">
      <!-- 左侧：专业分类树导航栏 -->
      <aside class="lib-category-sidebar card" :class="{ collapsed: isCategoryPanelCollapsed }">
        <div class="cat-sidebar-header">
          <div class="cat-sidebar-title" @click="clearCategoryFilter">
            <DemoIcon name="folder-tree" :size="16" class="title-icon" />
            <span v-if="!isCategoryPanelCollapsed" class="title-text">图纸分类架构</span>
          </div>
          <button
            class="icon-btn collapse-toggle-btn"
            type="button"
            :title="isCategoryPanelCollapsed ? '展开分类导航' : '折叠分类导航'"
            @click="isCategoryPanelCollapsed = !isCategoryPanelCollapsed"
          >
            <DemoIcon :name="isCategoryPanelCollapsed ? 'chevron-right' : 'chevron-left'" :size="14" />
          </button>
        </div>

        <template v-if="!isCategoryPanelCollapsed">
          <!-- 分类过滤搜索框 -->
          <div class="cat-tree-search-wrap">
            <DemoIcon name="search" :size="13" class="search-ico" />
            <input
              v-model="categoryTreeSearch"
              class="cat-search-inp"
              placeholder="快速过滤分类..."
            />
            <button
              v-if="categoryTreeSearch"
              class="clear-ico-btn"
              type="button"
              @click="categoryTreeSearch = ''"
            >
              <DemoIcon name="x" :size="12" />
            </button>
          </div>

          <!-- 快速顶层导航：全部 & 未分类 -->
          <div class="cat-quick-nav">
            <div
              class="quick-nav-item"
              :class="{ active: selectedCategoryId === '' }"
              @click="selectCategory('')"
            >
              <DemoIcon name="layers" :size="15" class="nav-ico" />
              <span class="nav-label">全部图纸</span>
              <span class="nav-badge">{{ domainStore.drawings.length }}</span>
            </div>
            <div
              class="quick-nav-item unclassified"
              :class="{ active: selectedCategoryId === '__none__' }"
              @click="selectCategory('__none__')"
            >
              <DemoIcon name="circle-slash" :size="15" class="nav-ico warning-ico" />
              <span class="nav-label">未分类图纸</span>
              <span class="nav-badge" :class="{ highlight: unclassifiedCount > 0 }">{{ unclassifiedCount }}</span>
            </div>
          </div>

          <div class="cat-sidebar-divider"></div>

          <!-- 树形节点列表 -->
          <div class="cat-tree-body">
            <div v-if="!domainStore.categoryTree.length" class="empty-cat-tip">
              <span>暂无分类</span>
              <button class="btn sm" type="button" @click="goCategoryAdmin">去创建</button>
            </div>

            <template v-for="node in domainStore.categoryTree" :key="node.category.id">
              <div class="tree-nav-node">
                <div
                  class="nav-node-row"
                  :class="{ active: selectedCategoryId === node.category.id }"
                  @click="selectCategory(node.category.id)"
                >
                  <button
                    v-if="node.children?.length"
                    class="node-toggle-btn"
                    type="button"
                    @click="toggleCategoryExpand(node.category.id, $event)"
                  >
                    <DemoIcon name="chevron-right" :size="12" :class="{ rotate: isCategoryExpanded(node.category.id) }" />
                  </button>
                  <span v-else class="node-toggle-placeholder"></span>

                  <DemoIcon
                    :name="node.children?.length ? (isCategoryExpanded(node.category.id) ? 'folder-open' : 'folder') : 'folder'"
                    :size="15"
                    class="folder-ico"
                  />
                  <span class="node-text" :title="node.category.name">{{ node.category.name }}</span>
                  <span class="node-num" :title="`直属 ${node.directCount} / 累计 ${node.totalCount}`">
                    {{ node.totalCount }}
                  </span>
                </div>

                <!-- 递归展开子孙 -->
                <div v-if="node.children?.length && isCategoryExpanded(node.category.id)" class="nav-sub-tree">
                  <component
                    :is="'LibCategorySubTree'"
                    :nodes="node.children"
                    :selected-id="selectedCategoryId"
                    :expanded-keys="expandedCategoryKeys"
                    :search-query="categoryTreeSearch"
                    @select="selectCategory"
                    @toggle-expand="toggleCategoryExpand"
                  />
                </div>
              </div>
            </template>
          </div>

          <!-- 底部快捷配置入口 -->
          <div class="cat-sidebar-footer">
            <button class="btn-manage-link" type="button" @click="goCategoryAdmin">
              <DemoIcon name="settings" :size="13" />
              <span>架构管理与移动</span>
            </button>
          </div>
        </template>
      </aside>

      <!-- 右侧：图纸库主工作区 -->
      <main class="lib-main-content">
        <!-- 顶栏操作区 -->
        <div class="lib-head">
          <div class="lib-head-left">
            <h1 class="lib-main-title">图纸资产库</h1>
            <span class="lib-count-tag">
              已显示 <b>{{ rows.length }}</b> 个总图 · 零件 <b>{{ domainStore.drawingStats?.parts ?? 0 }}</b> 项
            </span>
          </div>

          <div class="lib-head-actions">
            <label class="search-box">
              <DemoIcon name="search" :size="15" />
              <input v-model="query" placeholder="搜索图号 / 名称 / 分类路径 / 零件 / 厂商..." />
              <button v-if="query" class="clear-search-x" type="button" @click="query = ''">
                <DemoIcon name="x" :size="12" />
              </button>
            </label>

            <select v-model="status" class="inp filter-select">
              <option value="">全部状态</option>
              <option v-for="(item, key) in STATUS" :key="key" :value="key">{{ item.t }}</option>
            </select>

            <button class="btn primary" type="button" @click="createDrawing">
              <DemoIcon name="plus" :size="14" />创建图纸
            </button>
          </div>
        </div>

        <!-- 交互式分类面包屑条 -->
        <div v-if="selectedCategoryId" class="active-category-banner">
          <div class="banner-left">
            <DemoIcon name="filter" :size="13" class="filter-icon" />
            <span class="filter-label">当前筛选分类:</span>
            <div class="breadcrumb-trail">
              <button class="crumb-btn root" type="button" @click="clearCategoryFilter">全部图纸</button>
              <template v-for="(crumb, idx) in activeCategoryBreadcrumbs" :key="crumb.id">
                <span class="crumb-sep">/</span>
                <button
                  class="crumb-btn"
                  :class="{ current: idx === activeCategoryBreadcrumbs.length - 1 }"
                  type="button"
                  @click="selectCategory(crumb.id)"
                >
                  {{ crumb.name }}
                </button>
              </template>
            </div>
          </div>
          <button class="btn-clear-cat" type="button" @click="clearCategoryFilter">
            <DemoIcon name="x" :size="12" />清除分类过滤
          </button>
        </div>

        <!-- 图纸表格卡片 -->
        <div class="card library-card">
          <table class="tbl pro-table">
            <thead>
              <tr>
                <th class="project-no-header">项目图号</th>
                <th>名称与说明</th>
                <th>分类层级路径</th>
                <th>厂商/项目部</th>
                <th>状态</th>
                <th>版本</th>
                <th>更新时间</th>
                <th class="operation-column">操作</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="drawing in rows" :key="drawing.no">
                <tr class="drawing-row">
                  <td class="num project-no-cell">
                    <span class="expand-slot">
                      <button
                        v-if="partsForDrawing(drawing.no).length"
                        class="expand-button"
                        type="button"
                        :title="isExpanded(drawing.no) ? '收起零件图' : '展开零件图'"
                        @click="toggleExpanded(drawing.no)"
                      >
                        <DemoIcon name="chevron-down" :size="14" :class="{ collapsed: !isExpanded(drawing.no) }" />
                      </button>
                    </span>
                    <button class="link project-no-link" type="button" @click="openDetail(drawing.no)">
                      {{ drawing.no }}
                    </button>
                  </td>

                  <td class="drawing-name-cell">
                    <div class="drawing-main-title">
                      <span class="name-text">{{ drawing.name }}</span>
                      <span v-if="drawing.borrowFrom" class="tag plain borrow-tag">借用·{{ drawing.borrowFrom }}</span>
                    </div>
                    <div v-if="drawing.remark" class="drawing-sub-remark" :title="drawing.remark">
                      {{ drawing.remark }}
                    </div>
                  </td>

                  <!-- 增强的分类路径展示 -->
                  <td class="category-path-cell">
                    <template v-if="drawing.categoryId">
                      <div
                        class="category-breadcrumb-tag"
                        :title="`完整路径: ${domainStore.getCategoryFullPath(drawing.categoryId)} (点击筛选)`"
                        @click="selectCategory(drawing.categoryId)"
                      >
                        <DemoIcon name="folder" :size="12" class="cat-tag-icon" />
                        <span class="cat-tag-text">{{ domainStore.getCategoryFullPath(drawing.categoryId) }}</span>
                      </div>
                    </template>
                    <span v-else class="tag plain unclassified-tag" @click="selectCategory('__none__')">
                      未分类
                    </span>
                  </td>

                  <td class="vendor-cell">{{ drawing.vendor }}</td>

                  <td>
                    <span class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span>
                  </td>

                  <td class="num ver-cell">{{ drawing.ver }}</td>

                  <td class="num updated-cell">{{ drawing.updated }}</td>

                  <td class="row-actions">
                    <button class="btn sm" type="button" @click="openDetail(drawing.no)">
                      <DemoIcon name="eye" :size="14" />详情
                    </button>
                    <div class="row-menu-wrap">
                      <button class="icon-btn row-menu-button" type="button" @click.stop="toggleMenu(drawing.no)">
                        <DemoIcon name="ellipsis" :size="16" />
                      </button>
                      <div v-if="menuFor === drawing.no" class="dropdown row-dropdown">
                        <button class="dd-item" type="button" @click="menuAction('detail', drawing.no)">
                          <DemoIcon name="eye" :size="14" />查看详情与工艺文件
                        </button>
                        <div class="dd-sep"></div>
                        <button class="dd-item" type="button" @click="menuAction('hide', drawing.no)">
                          <DemoIcon name="eye-off" :size="14" />隐藏图纸（管理员）
                        </button>
                      </div>
                    </div>
                  </td>
                </tr>

                <!-- 展开零件图区域 -->
                <tr v-if="isExpanded(drawing.no)" class="parts-row">
                  <td colspan="8">
                    <div class="parts-panel">
                      <div class="parts-panel-head">
                        <DemoIcon name="layers" :size="13" />
                        <span>所属零件与结构明细 (共 {{ partsForDrawing(drawing.no).length }} 项)</span>
                      </div>
                      <div class="parts-list-grid">
                        <button
                          v-for="part in partsForDrawing(drawing.no)"
                          :key="part.no"
                          class="part-link-card"
                          type="button"
                          @click="openDetail(part.no)"
                        >
                          <DemoIcon name="file" :size="14" class="part-ico" />
                          <div class="part-card-body">
                            <span class="part-card-name">{{ part.name }}</span>
                            <span class="part-card-no mono">{{ part.no }}</span>
                          </div>
                          <span v-if="part.borrowFrom" class="tag plain borrow-tag">借用·{{ part.borrowFrom }}</span>
                          <DemoIcon name="arrow-up-right" :size="13" class="arrow-ico" />
                        </button>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>

              <!-- 空状态提示 -->
              <tr v-if="!rows.length">
                <td colspan="8">
                  <div class="empty-state-box">
                    <DemoIcon name="search-x" :size="40" class="empty-ico" />
                    <div class="empty-title">没有找到匹配的图纸</div>
                    <p class="empty-desc">
                      {{ selectedCategoryId ? '当前分类下暂无图纸，可尝试切换分类或清空过滤条件' : '请尝试调整搜索关键字或状态筛选' }}
                    </p>
                    <button v-if="selectedCategoryId" class="btn sm" type="button" @click="clearCategoryFilter">
                      查看全部图纸
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </main>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, type PropType } from 'vue'

const LibCategorySubTree = defineComponent({
  name: 'LibCategorySubTree',
  components: { DemoIcon },
  props: {
    nodes: {
      type: Array as PropType<CategoryTreeNode[]>,
      required: true,
    },
    selectedId: {
      type: String,
      default: '',
    },
    expandedKeys: {
      type: Object as PropType<Set<string>>,
      required: true,
    },
    searchQuery: {
      type: String,
      default: '',
    },
  },
  emits: ['select', 'toggle-expand'],
  template: `
    <div class="sub-nav-nodes">
      <div v-for="node in nodes" :key="node.category.id" class="tree-nav-node">
        <div
          class="nav-node-row"
          :class="{ active: selectedId === node.category.id }"
          :style="{ paddingLeft: (node.level * 14 + 6) + 'px' }"
          @click="$emit('select', node.category.id)"
        >
          <button
            v-if="node.children?.length"
            class="node-toggle-btn"
            type="button"
            @click="$emit('toggle-expand', node.category.id, $event)"
          >
            <DemoIcon name="chevron-right" :size="12" :class="{ rotate: searchQuery.trim() || expandedKeys.has(node.category.id) }" />
          </button>
          <span v-else class="node-toggle-placeholder"></span>

          <DemoIcon
            :name="node.children?.length ? ((searchQuery.trim() || expandedKeys.has(node.category.id)) ? 'folder-open' : 'folder') : 'tag'"
            :size="14"
            class="folder-ico"
          />
          <span class="node-text" :title="node.category.name">{{ node.category.name }}</span>
          <span class="node-num">{{ node.totalCount }}</span>
        </div>

        <div v-if="node.children?.length && (searchQuery.trim() || expandedKeys.has(node.category.id))" class="nav-sub-tree">
          <LibCategorySubTree
            :nodes="node.children"
            :selected-id="selectedId"
            :expanded-keys="expandedKeys"
            :search-query="searchQuery"
            @select="(id) => $emit('select', id)"
            @toggle-expand="(id, e) => $emit('toggle-expand', id, e)"
          />
        </div>
      </div>
    </div>
  `,
})

export default {
  components: {
    LibCategorySubTree,
  },
}
</script>

<style scoped>
.library-page-pro {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
}

.lib-layout-container {
  display: flex;
  gap: 16px;
  align-items: stretch;
  flex: 1;
  min-height: 0;
}

/* 左侧分类导航栏 */
.lib-category-sidebar {
  width: 260px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 14px;
  transition: width 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  overflow: hidden;
  background: var(--bg-card, #1f232b);
  border: 1px solid var(--line, #2d3340);
  border-radius: 12px;
}

.lib-category-sidebar.collapsed {
  width: 52px;
  padding: 14px 8px;
}

.cat-sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.cat-sidebar-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 13.5px;
  color: var(--text-1, #f3f4f6);
  cursor: pointer;
}

.title-icon {
  color: var(--accent, #3b82f6);
}

.collapse-toggle-btn {
  color: var(--text-3, #6b7280);
}

.cat-tree-search-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  background: var(--bg-1, #16181e);
  border: 1px solid var(--line, #2d3340);
  border-radius: 6px;
  margin-bottom: 10px;
}

.cat-search-inp {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  color: var(--text-1, #f3f4f6);
  font-size: 12px;
  outline: none;
}

.clear-ico-btn {
  border: none;
  background: transparent;
  color: var(--text-3, #6b7280);
  cursor: pointer;
  padding: 0;
}

.cat-quick-nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.quick-nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  color: var(--text-2, #9ca3af);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.quick-nav-item:hover {
  background: rgba(255, 255, 255, 0.05);
  color: var(--text-1, #f3f4f6);
}

.quick-nav-item.active {
  background: rgba(59, 130, 246, 0.15);
  color: var(--accent-light, #93c5fd);
  font-weight: 500;
}

.warning-ico {
  color: var(--warn, #f59e0b);
}

.nav-label {
  flex: 1;
}

.nav-badge {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.06);
  color: var(--text-3, #6b7280);
}

.nav-badge.highlight {
  background: rgba(245, 158, 11, 0.2);
  color: #f59e0b;
}

.cat-sidebar-divider {
  height: 1px;
  background: var(--line, #2d3340);
  margin: 10px 0;
}

.cat-tree-body {
  flex: 1;
  overflow-y: auto;
  margin-right: -6px;
  padding-right: 6px;
}

.nav-node-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  border-radius: 6px;
  color: var(--text-2, #9ca3af);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.15s;
}

.nav-node-row:hover {
  background: rgba(255, 255, 255, 0.05);
  color: var(--text-1, #f3f4f6);
}

.nav-node-row.active {
  background: rgba(59, 130, 246, 0.15);
  color: var(--accent-light, #93c5fd);
  font-weight: 500;
}

.node-toggle-btn {
  border: none;
  background: transparent;
  color: var(--text-3, #6b7280);
  cursor: pointer;
  padding: 1px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
}

.node-toggle-btn .rotate {
  transform: rotate(90deg);
}

.node-toggle-placeholder {
  width: 14px;
}

.folder-ico {
  color: var(--accent, #3b82f6);
  flex-shrink: 0;
}

.node-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-num {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-3, #6b7280);
}

.cat-sidebar-footer {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--line, #2d3340);
}

.btn-manage-link {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 100%;
  padding: 6px;
  border: 1px dashed var(--line, #2d3340);
  background: transparent;
  color: var(--text-3, #6b7280);
  border-radius: 6px;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s;
}

.btn-manage-link:hover {
  border-color: var(--accent, #3b82f6);
  color: var(--accent, #3b82f6);
  background: rgba(59, 130, 246, 0.05);
}

/* 右侧主工作区 */
.lib-main-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.lib-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.lib-head-left {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.lib-main-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--text-1, #f3f4f6);
  margin: 0;
}

.lib-count-tag {
  color: var(--text-3, #6b7280);
  font-size: 12px;
}

.lib-head-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: var(--bg-card, #1f232b);
  border: 1px solid var(--line, #2d3340);
  border-radius: 8px;
  min-width: 260px;
}

.search-box input {
  border: none;
  background: transparent;
  outline: none;
  color: var(--text-1, #f3f4f6);
  font-size: 12.5px;
  width: 100%;
}

.clear-search-x {
  border: none;
  background: transparent;
  color: var(--text-3, #6b7280);
  cursor: pointer;
  padding: 2px;
}

/* 分类面包屑条 */
.active-category-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px;
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.2);
  border-radius: 8px;
}

.banner-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  overflow: hidden;
}

.filter-icon {
  color: var(--accent, #3b82f6);
  flex-shrink: 0;
}

.filter-label {
  color: var(--text-3, #6b7280);
  font-size: 12px;
  flex-shrink: 0;
}

.breadcrumb-trail {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12.5px;
  min-width: 0;
  overflow: hidden;
}

.crumb-btn {
  border: none;
  background: transparent;
  color: var(--text-2, #9ca3af);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
  transition: color 0.15s;
}

.crumb-btn:hover {
  color: var(--accent, #3b82f6);
}

.crumb-btn.current {
  color: var(--accent-light, #93c5fd);
  font-weight: 600;
}

.crumb-sep {
  color: var(--text-3, #6b7280);
  font-size: 11px;
}

.btn-clear-cat {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--text-3, #6b7280);
  font-size: 11.5px;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
}

.btn-clear-cat:hover {
  color: var(--text-1, #f3f4f6);
  background: rgba(255, 255, 255, 0.1);
}

/* 表格区域 */
.library-card {
  padding: 0;
  overflow: hidden;
  border-radius: 10px;
}

.pro-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
}

.pro-table th {
  padding: 10px 14px;
  text-align: left;
  color: var(--text-3, #6b7280);
  font-weight: 500;
  font-size: 12px;
  border-bottom: 1px solid var(--line, #2d3340);
  background: rgba(0, 0, 0, 0.15);
}

.pro-table td {
  padding: 10px 14px;
  border-bottom: 1px solid var(--line, #2d3340);
  vertical-align: middle;
}

.drawing-row:hover {
  background: rgba(255, 255, 255, 0.02);
}

.project-no-cell {
  white-space: nowrap;
  display: flex;
  align-items: center;
  gap: 6px;
}

.project-no-link {
  font-family: 'JetBrains Mono', monospace;
  font-weight: 600;
  color: var(--accent-light, #93c5fd);
}

.drawing-main-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 500;
}

.drawing-sub-remark {
  font-size: 11px;
  color: var(--text-3, #6b7280);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 220px;
}

.category-breadcrumb-tag {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  background: rgba(59, 130, 246, 0.12);
  border: 1px solid rgba(59, 130, 246, 0.25);
  border-radius: 6px;
  color: var(--accent-light, #93c5fd);
  font-size: 11.5px;
  cursor: pointer;
  max-width: 240px;
  transition: all 0.15s;
}

.category-breadcrumb-tag:hover {
  background: rgba(59, 130, 246, 0.22);
  border-color: var(--accent, #3b82f6);
}

.cat-tag-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.unclassified-tag {
  cursor: pointer;
}

.unclassified-tag:hover {
  border-color: var(--text-2, #9ca3af);
}

.borrow-tag {
  font-size: 10.5px;
  color: #f59e0b;
  border-color: rgba(245, 158, 11, 0.3);
  background: rgba(245, 158, 11, 0.1);
}

/* 零件抽屉 */
.parts-row {
  background: rgba(0, 0, 0, 0.25);
}

.parts-panel {
  padding: 14px 20px;
}

.parts-panel-head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2, #9ca3af);
  margin-bottom: 10px;
}

.parts-list-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 8px;
}

.part-link-card {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: var(--bg-1, #16181e);
  border: 1px solid var(--line, #2d3340);
  border-radius: 6px;
  color: var(--text-1, #f3f4f6);
  cursor: pointer;
  text-align: left;
  transition: all 0.15s;
}

.part-link-card:hover {
  border-color: var(--accent, #3b82f6);
  background: rgba(59, 130, 246, 0.05);
}

.part-card-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.part-card-name {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.part-card-no {
  font-size: 10.5px;
  color: var(--text-3, #6b7280);
}

.empty-state-box {
  padding: 40px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.empty-ico {
  color: var(--text-3, #6b7280);
}

.empty-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-2, #9ca3af);
}

.empty-desc {
  font-size: 12px;
  color: var(--text-3, #6b7280);
  margin-bottom: 6px;
}
</style>
