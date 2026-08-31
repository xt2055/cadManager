<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import CategoryTreeSelect from '@/components/common/CategoryTreeSelect.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { CategoryTreeNode, Drawing } from '@/types/domain.types'

defineOptions({ name: 'CategoryManagementPage' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

// 当前选中的节点进行查看和维护
const selectedNodeId = ref<string>('') // ''=全部, '__none__'=未分类, 或具体分类ID
const treeSearch = ref('')
const expandedTreeKeys = ref<Set<string>>(new Set())

// 新建/编辑分类表单弹层或行内操作
const createModalOpen = ref(false)
const modalParentId = ref<string | undefined>(undefined)
const modalCategoryName = ref('')
const modalIsEdit = ref(false)
const modalEditId = ref('')
const isSubmitting = ref(false)

// 右侧图纸管理与批量移动
const batchTargetCategoryId = ref<string>('')
const selectedDrawingNos = ref<Set<string>>(new Set())
const drawingFilterQuery = ref('')

const tree = computed(() => domainStore.categoryTree)

const unclassifiedDrawings = computed(() => {
  return domainStore.drawings.filter((d) => !d.categoryId)
})

// 根据左侧选中的分类筛选右侧图纸
const currentCategoryDrawings = computed(() => {
  const q = drawingFilterQuery.value.trim().toLowerCase()
  let list: Drawing[] = []
  if (selectedNodeId.value === '__none__') {
    list = unclassifiedDrawings.value
  } else if (!selectedNodeId.value) {
    list = domainStore.drawings
  } else {
    // 穿透包含子孙
    const descendantIds = domainStore.getDescendantCategoryIds(selectedNodeId.value)
    list = domainStore.drawings.filter((d) => d.categoryId && descendantIds.includes(d.categoryId))
  }

  if (!q) return list
  return list.filter((d) => {
    return (
      d.no.toLowerCase().includes(q) ||
      d.name.toLowerCase().includes(q) ||
      (d.project || '').toLowerCase().includes(q) ||
      domainStore.getCategoryFullPath(d.categoryId).toLowerCase().includes(q)
    )
  })
})

const selectedNode = computed<CategoryTreeNode | null>(() => {
  if (!selectedNodeId.value || selectedNodeId.value === '__none__') return null
  function find(nodes: CategoryTreeNode[]): CategoryTreeNode | null {
    for (const n of nodes) {
      if (n.category.id === selectedNodeId.value) return n
      if (n.children?.length) {
        const found = find(n.children)
        if (found) return found
      }
    }
    return null
  }
  return find(tree.value)
})

const isAllSelected = computed(() => {
  if (!currentCategoryDrawings.value.length) return false
  return currentCategoryDrawings.value.every((d) => selectedDrawingNos.value.has(d.no))
})

function toggleSelectAll() {
  if (isAllSelected.value) {
    selectedDrawingNos.value = new Set()
  } else {
    const next = new Set<string>()
    for (const d of currentCategoryDrawings.value) {
      next.add(d.no)
    }
    selectedDrawingNos.value = next
  }
}

function toggleSelectDrawing(no: string) {
  const next = new Set(selectedDrawingNos.value)
  if (next.has(no)) next.delete(no)
  else next.add(no)
  selectedDrawingNos.value = next
}

function isTreeExpanded(id: string): boolean {
  if (treeSearch.value.trim()) return true
  return expandedTreeKeys.value.has(id)
}

function toggleTreeExpand(id: string, e?: Event) {
  e?.stopPropagation()
  const next = new Set(expandedTreeKeys.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedTreeKeys.value = next
}

function selectNode(id: string) {
  selectedNodeId.value = id
  selectedDrawingNos.value = new Set()
}

// 模态弹窗新建/编辑
function openCreateModal(parentId?: string) {
  modalIsEdit.value = false
  modalEditId.value = ''
  modalParentId.value = parentId
  modalCategoryName.value = ''
  createModalOpen.value = true
}

function openEditModal(id: string, currentName: string, parentId?: string) {
  modalIsEdit.value = true
  modalEditId.value = id
  modalParentId.value = parentId
  modalCategoryName.value = currentName
  createModalOpen.value = true
}

async function handleModalSubmit() {
  const name = modalCategoryName.value.trim()
  if (!name) {
    uiStore.toast('请输入分类名称', 'warn')
    return
  }
  if (isSubmitting.value) return
  isSubmitting.value = true
  try {
    if (modalIsEdit.value) {
      await domainStore.renameCategory(modalEditId.value, name)
      uiStore.toast('分类已重命名', 'ok')
    } else {
      const created = await domainStore.addCategory(name, modalParentId.value)
      if (modalParentId.value) {
        expandedTreeKeys.value.add(modalParentId.value)
      }
      selectNode(created.id)
      uiStore.toast(`分类「${created.name}」已创建`, 'ok')
    }
    createModalOpen.value = false
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '操作失败', 'warn')
  } finally {
    isSubmitting.value = false
  }
}

// 安全删除
function handleDeleteCategory(id: string, name: string) {
  const directCount = domainStore.countDrawingsInCategory(id, false)
  const totalCount = domainStore.countDrawingsInCategory(id, true)
  const category = domainStore.categories.find((c) => c.id === id)
  const hasChildren = domainStore.categories.some((c) => c.parentId === id)

  if (hasChildren) {
    uiStore.toast(`分类「${name}」下包含子分类，请先移除子分类后再删除`, 'warn')
    return
  }

  if (directCount > 0) {
    uiStore.toast(`分类「${name}」下还有 ${directCount} 张图纸，请先移动这些图纸后再删除`, 'warn')
    return
  }

  uiStore.confirm(
    `确认删除分类「${name}」？`,
    '删除后该分类将从架构中移除。由于该分类下没有图纸与子分类，操作是安全的。',
    {
      danger: true,
      confirmText: '确认删除',
      onConfirm: async () => {
        try {
          await domainStore.deleteCategory(id)
          if (selectedNodeId.value === id) {
            selectedNodeId.value = ''
          }
          uiStore.toast(`分类「${name}」已删除`, 'ok')
        } catch (error) {
          uiStore.toast(error instanceof Error ? error.message : '删除失败', 'warn')
        }
      },
    },
  )
}

// 批量移动图纸
async function handleBatchMove() {
  if (!selectedDrawingNos.value.size) {
    uiStore.toast('请先勾选需要移动的图纸', 'warn')
    return
  }
  const targetId = batchTargetCategoryId.value
  const targetLabel = targetId ? domainStore.getCategoryFullPath(targetId) : '未分类'

  try {
    await domainStore.batchSetDrawingCategory(Array.from(selectedDrawingNos.value), targetId)
    uiStore.toast(`已将 ${selectedDrawingNos.value.size} 张图纸移动到「${targetLabel}」`, 'ok')
    selectedDrawingNos.value = new Set()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '批量移动失败', 'warn')
  }
}

// 单张图纸快速调分类
async function handleSingleMove(drawingNo: string, targetId: string) {
  try {
    await domainStore.setDrawingCategory(drawingNo, targetId)
    uiStore.toast(`图纸 ${drawingNo} 分类已更新`, 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '移动失败', 'warn')
  }
}

onMounted(() => {
  domainStore.initialize().catch(() => undefined)
})
</script>

<template>
  <div class="category-admin-workbench">
    <!-- 顶部标题与说明 -->
    <header class="admin-top-bar card">
      <div class="top-left">
        <div class="header-icon-wrap">
          <DemoIcon name="folder-tree" :size="20" class="main-icon" />
        </div>
        <div class="header-texts">
          <h1>图纸分类架构工作台</h1>
          <p>
            支持无限层级多叉树（例如：油缸 → 安装方法 → 耳环安装 → 焊接工艺）。可在左侧维护结构树，在右侧对图纸进行批量归类与调配。
          </p>
        </div>
      </div>
      <div class="top-stats">
        <div class="stat-pill">
          <span class="stat-num">{{ domainStore.categories.length }}</span>
          <span class="stat-label">分类节点</span>
        </div>
        <div class="stat-pill">
          <span class="stat-num">{{ domainStore.drawings.length }}</span>
          <span class="stat-label">总图纸数</span>
        </div>
        <div class="stat-pill warning" :class="{ highlight: unclassifiedDrawings.length > 0 }">
          <span class="stat-num">{{ unclassifiedDrawings.length }}</span>
          <span class="stat-label">待归类图纸</span>
        </div>
      </div>
    </header>

    <!-- 工作台双栏主体 -->
    <div class="workbench-body">
      <!-- 左栏：分类树编辑器 -->
      <section class="tree-panel card">
        <div class="panel-head">
          <div class="head-title">
            <DemoIcon name="layers" :size="15" />
            <span>分类树目录</span>
          </div>
          <button class="btn sm primary" type="button" @click="openCreateModal(undefined)">
            <DemoIcon name="plus" :size="13" />新建根分类
          </button>
        </div>

        <div class="tree-filter-box">
          <DemoIcon name="search" :size="13" class="search-ico" />
          <input v-model="treeSearch" class="tree-search-inp" placeholder="搜索分类名称..." />
        </div>

        <!-- 顶层快速项：全部 / 未分类 -->
        <div class="tree-special-nodes">
          <div
            class="special-row"
            :class="{ active: selectedNodeId === '' }"
            @click="selectNode('')"
          >
            <DemoIcon name="layers" :size="15" class="spec-ico" />
            <span class="spec-label">全部图纸</span>
            <span class="spec-badge">{{ domainStore.drawings.length }}</span>
          </div>
          <div
            class="special-row unclassified"
            :class="{ active: selectedNodeId === '__none__' }"
            @click="selectNode('__none__')"
          >
            <DemoIcon name="circle-slash" :size="15" class="spec-ico warn" />
            <span class="spec-label">未分类图纸池</span>
            <span class="spec-badge" :class="{ warn: unclassifiedDrawings.length > 0 }">
              {{ unclassifiedDrawings.length }}
            </span>
          </div>
        </div>

        <div class="tree-divider"></div>

        <!-- 递归树容器 -->
        <div class="tree-scroll-container">
          <div v-if="!tree.length" class="empty-tree-tip">
            <DemoIcon name="folder-plus" :size="32" class="empty-ico" />
            <p>尚未建立分类架构</p>
            <button class="btn sm primary" type="button" @click="openCreateModal(undefined)">
              立即创建第一个分类
            </button>
          </div>

          <template v-for="node in tree" :key="node.category.id">
            <div class="tree-item-block">
              <div
                class="tree-node-row"
                :class="{ active: selectedNodeId === node.category.id }"
                @click="selectNode(node.category.id)"
              >
                <button
                  v-if="node.children?.length"
                  class="expand-toggle-btn"
                  type="button"
                  @click="toggleTreeExpand(node.category.id, $event)"
                >
                  <DemoIcon name="chevron-right" :size="12" :class="{ rotate: isTreeExpanded(node.category.id) }" />
                </button>
                <span v-else class="expand-placeholder"></span>

                <DemoIcon
                  :name="node.children?.length ? (isTreeExpanded(node.category.id) ? 'folder-open' : 'folder') : 'folder'"
                  :size="15"
                  class="node-folder-ico"
                />

                <span class="node-title-text" :title="node.category.name">{{ node.category.name }}</span>

                <span class="node-count-badge" :title="`直属 ${node.directCount} / 包含子级共 ${node.totalCount}`">
                  {{ node.totalCount }}
                </span>

                <!-- 悬浮动作按钮组 -->
                <div class="node-hover-actions" @click.stop>
                  <button
                    class="node-act-btn"
                    type="button"
                    title="在此分类下增加子分类"
                    @click="openCreateModal(node.category.id)"
                  >
                    <DemoIcon name="plus" :size="12" />
                  </button>
                  <button
                    class="node-act-btn"
                    type="button"
                    title="重命名分类"
                    @click="openEditModal(node.category.id, node.category.name, node.category.parentId)"
                  >
                    <DemoIcon name="pencil" :size="12" />
                  </button>
                  <button
                    class="node-act-btn danger"
                    type="button"
                    title="删除分类"
                    @click="handleDeleteCategory(node.category.id, node.category.name)"
                  >
                    <DemoIcon name="trash-2" :size="12" />
                  </button>
                </div>
              </div>

              <!-- 递归渲染子分类 -->
              <div v-if="node.children?.length && isTreeExpanded(node.category.id)" class="tree-sub-branch">
                <component
                  :is="'AdminCategorySubTree'"
                  :nodes="node.children"
                  :selected-id="selectedNodeId"
                  :expanded-keys="expandedTreeKeys"
                  :search-query="treeSearch"
                  @select="selectNode"
                  @toggle-expand="toggleTreeExpand"
                  @create-child="openCreateModal"
                  @edit="openEditModal"
                  @delete="handleDeleteCategory"
                />
              </div>
            </div>
          </template>
        </div>
      </section>

      <!-- 右栏：图纸归配工作区 -->
      <section class="drawings-panel card">
        <div class="drawings-panel-head">
          <div class="panel-current-scope">
            <DemoIcon name="folder" :size="16" class="scope-ico" />
            <div class="scope-texts">
              <div class="scope-name">
                {{
                  selectedNodeId === '__none__'
                    ? '未分类图纸池'
                    : selectedNodeId === ''
                    ? '全部图纸'
                    : domainStore.getCategoryFullPath(selectedNodeId)
                }}
              </div>
              <div class="scope-desc">
                共 {{ currentCategoryDrawings.length }} 张图纸
                <span v-if="selectedDrawingNos.size" class="selected-badge">
                  (已勾选 {{ selectedDrawingNos.size }} 张)
                </span>
              </div>
            </div>
          </div>

          <!-- 批量操作条 -->
          <div class="batch-action-bar">
            <div class="batch-selector-wrap">
              <CategoryTreeSelect
                v-model="batchTargetCategoryId"
                placeholder="选择移动目标分类..."
                :allow-quick-create="false"
              />
            </div>
            <button
              class="btn primary sm"
              type="button"
              :disabled="!selectedDrawingNos.size || batchTargetCategoryId === selectedNodeId"
              @click="handleBatchMove"
            >
              <DemoIcon name="arrow-right-left" :size="13" />
              批量移动
            </button>
          </div>
        </div>

        <!-- 筛选与搜索工具条 -->
        <div class="drawings-tool-bar">
          <div class="table-search-box">
            <DemoIcon name="search" :size="14" />
            <input v-model="drawingFilterQuery" placeholder="在当前视图过滤图号、名称、项目..." />
          </div>

          <button
            v-if="currentCategoryDrawings.length"
            class="btn sm btn-select-all"
            type="button"
            @click="toggleSelectAll"
          >
            <DemoIcon :name="isAllSelected ? 'check-square' : 'square'" :size="13" />
            {{ isAllSelected ? '取消全选' : '全选当前' }}
          </button>
        </div>

        <!-- 图纸明细表格 -->
        <div class="drawings-table-container">
          <table class="tbl workbench-tbl">
            <thead>
              <tr>
                <th style="width: 40px;">
                  <input
                    type="checkbox"
                    :checked="isAllSelected"
                    :disabled="!currentCategoryDrawings.length"
                    @change="toggleSelectAll"
                  />
                </th>
                <th>项目图号</th>
                <th>图纸名称</th>
                <th>当前完整分类路径</th>
                <th style="width: 220px;">快速调配分类</th>
              </tr>
            </thead>
            <tbody>
              <template v-for="drawing in currentCategoryDrawings" :key="drawing.no">
                <tr :class="{ 'row-checked': selectedDrawingNos.has(drawing.no) }">
                  <td>
                    <input
                      type="checkbox"
                      :checked="selectedDrawingNos.has(drawing.no)"
                      @change="toggleSelectDrawing(drawing.no)"
                    />
                  </td>
                  <td class="mono font-semibold">{{ drawing.no }}</td>
                  <td>{{ drawing.name }}</td>
                  <td>
                    <span v-if="drawing.categoryId" class="tag-path">
                      {{ domainStore.getCategoryFullPath(drawing.categoryId) }}
                    </span>
                    <span v-else class="tag plain warn-tag">未分类</span>
                  </td>
                  <td>
                    <div class="inline-move-wrap">
                      <CategoryTreeSelect
                        :model-value="drawing.categoryId || ''"
                        placeholder="更改分类"
                        :allow-quick-create="false"
                        @update:model-value="(val) => handleSingleMove(drawing.no, val)"
                      />
                    </div>
                  </td>
                </tr>
              </template>

              <tr v-if="!currentCategoryDrawings.length">
                <td colspan="5">
                  <div class="empty-drawings-tip">
                    <DemoIcon name="check-circle" :size="32" class="empty-ico" />
                    <p>当前分类下暂无图纸</p>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <!-- 创建/编辑分类弹窗 -->
    <div v-if="createModalOpen" class="modal-overlay" @click.self="createModalOpen = false">
      <div class="modal-card">
        <div class="modal-head">
          <h3>{{ modalIsEdit ? '重命名分类' : '新建图纸分类' }}</h3>
          <button class="icon-btn" type="button" @click="createModalOpen = false">
            <DemoIcon name="x" :size="15" />
          </button>
        </div>

        <div class="modal-body">
          <div v-if="!modalIsEdit" class="form-group">
            <label>所属父级分类</label>
            <div class="parent-display">
              <DemoIcon name="folder" :size="14" class="parent-ico" />
              <span>{{ modalParentId ? domainStore.getCategoryFullPath(modalParentId) : '（顶级根分类）' }}</span>
            </div>
          </div>

          <div class="form-group">
            <label>分类名称 <span class="required">*</span></label>
            <input
              v-model="modalCategoryName"
              class="inp modal-inp"
              placeholder="如：油缸 / 耳环安装 / 焊接工艺"
              autofocus
              @keyup.enter="handleModalSubmit"
            />
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" @click="createModalOpen = false">取消</button>
          <button class="btn primary" type="button" :disabled="isSubmitting" @click="handleModalSubmit">
            {{ isSubmitting ? '保存中...' : '确认保存' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { defineComponent, type PropType } from 'vue'

const AdminCategorySubTree = defineComponent({
  name: 'AdminCategorySubTree',
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
  emits: ['select', 'toggle-expand', 'create-child', 'edit', 'delete'],
  template: `
    <div class="tree-sub-nodes">
      <div v-for="node in nodes" :key="node.category.id" class="tree-item-block">
        <div
          class="tree-node-row"
          :class="{ active: selectedId === node.category.id }"
          :style="{ paddingLeft: (node.level * 16 + 6) + 'px' }"
          @click="$emit('select', node.category.id)"
        >
          <button
            v-if="node.children?.length"
            class="expand-toggle-btn"
            type="button"
            @click="$emit('toggle-expand', node.category.id, $event)"
          >
            <DemoIcon name="chevron-right" :size="12" :class="{ rotate: searchQuery.trim() || expandedKeys.has(node.category.id) }" />
          </button>
          <span v-else class="expand-placeholder"></span>

          <DemoIcon
            :name="node.children?.length ? ((searchQuery.trim() || expandedKeys.has(node.category.id)) ? 'folder-open' : 'folder') : 'folder'"
            :size="15"
            class="node-folder-ico"
          />

          <span class="node-title-text" :title="node.category.name">{{ node.category.name }}</span>

          <span class="node-count-badge">{{ node.totalCount }}</span>

          <div class="node-hover-actions" @click.stop>
            <button
              class="node-act-btn"
              type="button"
              title="在此分类下增加子分类"
              @click="$emit('create-child', node.category.id)"
            >
              <DemoIcon name="plus" :size="12" />
            </button>
            <button
              class="node-act-btn"
              type="button"
              title="重命名分类"
              @click="$emit('edit', node.category.id, node.category.name, node.category.parentId)"
            >
              <DemoIcon name="pencil" :size="12" />
            </button>
            <button
              class="node-act-btn danger"
              type="button"
              title="删除分类"
              @click="$emit('delete', node.category.id, node.category.name)"
            >
              <DemoIcon name="trash-2" :size="12" />
            </button>
          </div>
        </div>

        <div v-if="node.children?.length && (searchQuery.trim() || expandedKeys.has(node.category.id))" class="tree-sub-branch">
          <AdminCategorySubTree
            :nodes="node.children"
            :selected-id="selectedId"
            :expanded-keys="expandedKeys"
            :search-query="searchQuery"
            @select="(id) => $emit('select', id)"
            @toggle-expand="(id, e) => $emit('toggle-expand', id, e)"
            @create-child="(pid) => $emit('create-child', pid)"
            @edit="(id, n, pid) => $emit('edit', id, n, pid)"
            @delete="(id, n) => $emit('delete', id, n)"
          />
        </div>
      </div>
    </div>
  `,
})

export default {
  components: {
    AdminCategorySubTree,
  },
}
</script>

<style scoped>
.category-admin-workbench {
  display: flex;
  flex-direction: column;
  gap: 14px;
  height: 100%;
  padding: 16px 20px;
  min-width: 0;
}

/* 顶部信息条 */
.admin-top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--radius, 10px);
}

.top-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.header-icon-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
}

.header-texts h1 {
  font-size: 16px;
  font-weight: 600;
  margin: 0;
  color: var(--text-1);
}

.header-texts p {
  font-size: 12px;
  color: var(--text-3);
  margin: 2px 0 0;
}

.top-stats {
  display: flex;
  gap: 12px;
}

.stat-pill {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 6px 14px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 8px;
}

.stat-num {
  font-family: 'JetBrains Mono', monospace;
  font-size: 16px;
  font-weight: 700;
  color: var(--text-1);
}

.stat-label {
  font-size: 11px;
  color: var(--text-3);
}

.stat-pill.warning.highlight {
  border-color: var(--warn);
  background: rgb(251 191 36 / 9%);
}

.stat-pill.warning.highlight .stat-num {
  color: var(--warn);
}

/* 工作台双栏布局 */
.workbench-body {
  display: flex;
  gap: 14px;
  flex: 1;
  min-height: 0;
}

/* 左栏：树编辑器 */
.tree-panel {
  width: 320px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 14px;
  border-radius: var(--radius, 10px);
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.head-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  font-size: 13.5px;
  color: var(--text-1);
}

.tree-filter-box {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  margin-bottom: 10px;
}

.tree-search-inp {
  flex: 1;
  border: none;
  background: transparent;
  outline: none;
  font-size: 12px;
  color: var(--text-1);
}

.tree-special-nodes {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.special-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-radius: 6px;
  color: var(--text-2);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.15s;
}

.special-row:hover {
  background: var(--hover);
}

.special-row.active {
  background: var(--active);
  color: var(--accent);
  font-weight: 500;
}

.spec-ico.warn {
  color: var(--warn);
}

.spec-label {
  flex: 1;
}

.spec-badge {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 10px;
  background: var(--panel-2);
  color: var(--text-3);
}

.spec-badge.warn {
  background: var(--active);
  color: var(--warn);
}

.tree-divider {
  height: 1px;
  background: var(--line);
  margin: 10px 0;
}

.tree-scroll-container {
  flex: 1;
  overflow-y: auto;
  margin-right: -6px;
  padding-right: 6px;
}

.tree-node-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-radius: 6px;
  color: var(--text-2);
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.15s;
  position: relative;
}

.tree-node-row:hover {
  background: var(--hover);
  color: var(--text-1);
}

.tree-node-row.active {
  background: var(--active);
  color: var(--accent);
  font-weight: 500;
}

.expand-toggle-btn {
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  padding: 1px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
}

.expand-toggle-btn .rotate {
  transform: rotate(90deg);
}

.expand-placeholder {
  width: 14px;
}

.node-folder-ico {
  color: var(--accent);
  flex-shrink: 0;
}

.node-title-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-count-badge {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-3);
}

.node-hover-actions {
  display: none;
  align-items: center;
  gap: 2px;
  margin-left: 4px;
}

.tree-node-row:hover .node-hover-actions {
  display: flex;
}

.node-act-btn {
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  padding: 3px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.node-act-btn:hover {
  color: var(--text-1);
  background: var(--hover);
}

.node-act-btn.danger:hover {
  color: var(--danger);
  background: var(--hover);
}

/* 右栏：图纸工作区 */
.drawings-panel {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: 16px;
  border-radius: var(--radius, 10px);
}

.drawings-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
  gap: 16px;
}

.panel-current-scope {
  display: flex;
  align-items: center;
  gap: 10px;
}

.scope-ico {
  color: var(--accent);
}

.scope-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-1);
}

.scope-desc {
  font-size: 12px;
  color: var(--text-3);
}

.selected-badge {
  color: var(--accent);
  font-weight: 500;
}

.batch-action-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.batch-selector-wrap {
  width: 240px;
}

.drawings-tool-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.table-search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  width: 280px;
}

.table-search-box input {
  border: none;
  background: transparent;
  outline: none;
  font-size: 12px;
  color: var(--text-1);
  width: 100%;
}

.btn-select-all {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.drawings-table-container {
  flex: 1;
  overflow-y: auto;
  border: 1px solid var(--line);
  border-radius: var(--radius, 8px);
}

.workbench-tbl {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
}

.workbench-tbl th {
  padding: 8px 12px;
  text-align: left;
  background: var(--panel-2);
  color: var(--text-3);
  font-weight: 500;
  border-bottom: 1px solid var(--line);
}

.workbench-tbl td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--line);
  vertical-align: middle;
}

.workbench-tbl tr.row-checked {
  background: var(--active);
}

.tag-path {
  display: inline-block;
  padding: 2px 8px;
  background: var(--accent-soft);
  border: 1px solid var(--accent);
  border-radius: 4px;
  color: var(--accent);
  font-size: 11.5px;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.warn-tag {
  color: var(--warn);
  border-color: var(--warn);
}

.inline-move-wrap {
  width: 180px;
}

.empty-drawings-tip,
.empty-tree-tip {
  padding: 40px;
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--text-3);
}

/* 模态弹窗 */
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
}

.modal-card {
  width: 440px;
  background: var(--panel);
  border: 1px solid var(--line-strong, var(--line));
  border-radius: var(--radius, 12px);
  box-shadow: var(--shadow);
  overflow: hidden;
}

.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--line);
}

.modal-head h3 {
  font-size: 15px;
  margin: 0;
  color: var(--text-1);
}

.modal-body {
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-group label {
  font-size: 12px;
  color: var(--text-2);
}

.parent-display {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  color: var(--text-1);
  font-size: 12.5px;
}

.parent-ico {
  color: var(--accent);
}

.modal-inp {
  width: 100%;
}

.modal-foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 18px;
  background: var(--panel-2);
  border-top: 1px solid var(--line);
}

.required {
  color: var(--danger);
}
</style>
