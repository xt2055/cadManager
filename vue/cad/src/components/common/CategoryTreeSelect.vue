<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { CategoryTreeNode } from '@/types/domain.types'

defineOptions({ name: 'CategoryTreeSelect' })

const props = withDefaults(
  defineProps<{
    modelValue?: string
    placeholder?: string
    allowClear?: boolean
    allowQuickCreate?: boolean
    disabled?: boolean
  }>(),
  {
    modelValue: '',
    placeholder: '请选择图纸分类',
    allowClear: true,
    allowQuickCreate: true,
    disabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', value: string, node?: CategoryTreeNode): void
}>()

const domainStore = useDomainStore()
const uiStore = useUiStore()

const isOpen = ref(false)
const searchQuery = ref('')
const containerRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)
const expandedKeys = ref<Set<string>>(new Set())

// 快速新建子分类
const quickCreateMode = ref(false)
const quickCreateParentId = ref<string | undefined>(undefined)
const quickCreateName = ref('')
const isCreating = ref(false)

const tree = computed(() => domainStore.categoryTree)

const selectedPath = computed(() => {
  if (!props.modelValue) return []
  return domainStore.getCategoryPath(props.modelValue)
})

const selectedFullPath = computed(() => {
  if (!props.modelValue) return ''
  return selectedPath.value.join(' / ')
})

const selectedCategoryName = computed(() => {
  if (!props.modelValue) return ''
  return domainStore.categoryName(props.modelValue)
})

// 展平用于搜索的列表
interface FlatCategoryItem {
  id: string
  name: string
  parentId?: string
  fullPath: string
  path: string[]
  level: number
}

const flatCategories = computed<FlatCategoryItem[]>(() => {
  const result: FlatCategoryItem[] = []
  function traverse(nodes: CategoryTreeNode[]) {
    for (const node of nodes) {
      result.push({
        id: node.category.id,
        name: node.category.name,
        parentId: node.category.parentId,
        fullPath: node.fullPath,
        path: node.path,
        level: node.level,
      })
      if (node.children?.length) {
        traverse(node.children)
      }
    }
  }
  traverse(tree.value)
  return result
})

const searchResults = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return []
  return flatCategories.value.filter((item) => {
    return item.name.toLowerCase().includes(q) || item.fullPath.toLowerCase().includes(q)
  })
})

function toggleDropdown() {
  if (props.disabled) return
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    searchQuery.value = ''
    quickCreateMode.value = false
    // 默认展开所有祖先
    if (props.modelValue) {
      expandToNode(props.modelValue)
    }
    nextTick(() => {
      searchInputRef.value?.focus()
    })
  }
}

function expandToNode(targetId: string) {
  let curr = domainStore.categories.find((c) => c.id === targetId)
  while (curr?.parentId) {
    expandedKeys.value.add(curr.parentId)
    curr = domainStore.categories.find((c) => c.id === curr!.parentId)
  }
}

function isExpanded(id: string): boolean {
  // 搜索时全展开
  if (searchQuery.value.trim()) return true
  return expandedKeys.value.has(id)
}

function toggleExpand(id: string, e?: Event) {
  e?.stopPropagation()
  const next = new Set(expandedKeys.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedKeys.value = next
}

function selectNode(id: string) {
  emit('update:modelValue', id)
  emit('change', id)
  isOpen.value = false
}

function clearSelection(e: Event) {
  e.stopPropagation()
  emit('update:modelValue', '')
  emit('change', '')
}

function startQuickCreate(parentId?: string, e?: Event) {
  e?.stopPropagation()
  quickCreateParentId.value = parentId
  quickCreateName.value = ''
  quickCreateMode.value = true
}

async function submitQuickCreate() {
  const name = quickCreateName.value.trim()
  if (!name) {
    uiStore.toast('请输入分类名称', 'warn')
    return
  }
  if (isCreating.value) return
  isCreating.value = true
  try {
    const created = await domainStore.addCategory(name, quickCreateParentId.value)
    uiStore.toast(`分类「${created.name}」已创建`, 'ok')
    if (quickCreateParentId.value) {
      expandedKeys.value.add(quickCreateParentId.value)
    }
    selectNode(created.id)
    quickCreateMode.value = false
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '创建分类失败', 'warn')
  } finally {
    isCreating.value = false
  }
}

function cancelQuickCreate() {
  quickCreateMode.value = false
}

function handleClickOutside(e: MouseEvent) {
  if (containerRef.value && !containerRef.value.contains(e.target as Node)) {
    isOpen.value = false
    quickCreateMode.value = false
  }
}

watch(
  () => props.modelValue,
  (val) => {
    if (val) expandToNode(val)
  },
  { immediate: true },
)

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<template>
  <div ref="containerRef" class="tree-select" :class="{ open: isOpen, disabled }">
    <!-- 触发器框 -->
    <div class="tree-select-trigger" @click="toggleDropdown">
      <div v-if="modelValue" class="selected-view">
        <span class="category-badge">
          <DemoIcon name="folder" :size="13" class="badge-icon" />
          <span class="badge-name">{{ selectedCategoryName }}</span>
        </span>
        <span v-if="selectedPath.length > 1" class="path-hint" :title="selectedFullPath">
          {{ selectedPath.slice(0, -1).join(' / ') }} /
        </span>
      </div>
      <span v-else class="placeholder">{{ placeholder }}</span>

      <div class="trigger-actions">
        <button
          v-if="allowClear && modelValue && !disabled"
          class="clear-btn"
          type="button"
          title="清空分类"
          @click="clearSelection"
        >
          <DemoIcon name="x" :size="13" />
        </button>
        <DemoIcon name="chevron-down" :size="14" class="arrow-icon" :class="{ rotate: isOpen }" />
      </div>
    </div>

    <!-- 下拉弹层 -->
    <transition name="dropdown-fade">
      <div v-if="isOpen" class="tree-select-dropdown">
        <!-- 搜索输入框 -->
        <div class="dropdown-search">
          <DemoIcon name="search" :size="14" class="search-icon" />
          <input
            ref="searchInputRef"
            v-model="searchQuery"
            class="search-input"
            placeholder="输入名称或完整路径检索..."
            @click.stop
          />
          <button v-if="searchQuery" class="clear-search-btn" type="button" @click="searchQuery = ''">
            <DemoIcon name="x" :size="12" />
          </button>
        </div>

        <!-- 快捷新建区域 -->
        <div v-if="allowQuickCreate" class="quick-create-bar" @click.stop>
          <template v-if="!quickCreateMode">
            <button class="btn-text-action" type="button" @click="startQuickCreate(undefined)">
              <DemoIcon name="plus" :size="13" />新建根分类
            </button>
          </template>
          <div v-else class="quick-create-form">
            <div class="create-parent-tip">
              {{ quickCreateParentId ? `新建「${domainStore.categoryName(quickCreateParentId)}」的子分类:` : '新建根分类:' }}
            </div>
            <div class="create-input-row">
              <input
                v-model="quickCreateName"
                class="inp sm create-input"
                placeholder="分类名称"
                @keyup.enter="submitQuickCreate"
              />
              <button class="btn sm primary" type="button" :disabled="isCreating" @click="submitQuickCreate">确定</button>
              <button class="btn sm" type="button" @click="cancelQuickCreate">取消</button>
            </div>
          </div>
        </div>

        <!-- 列表内容区 -->
        <div class="tree-list-wrap" @click.stop>
          <!-- 搜索结果模式 -->
          <div v-if="searchQuery.trim()" class="search-result-list">
            <div v-if="!searchResults.length" class="empty-tip">未找到匹配分类</div>
            <div
              v-for="item in searchResults"
              :key="item.id"
              class="search-result-item"
              :class="{ active: item.id === modelValue }"
              @click="selectNode(item.id)"
            >
              <DemoIcon name="folder" :size="14" class="item-icon" />
              <div class="result-texts">
                <span class="result-name">{{ item.name }}</span>
                <span class="result-path">{{ item.fullPath }}</span>
              </div>
              <DemoIcon v-if="item.id === modelValue" name="check" :size="14" class="check-icon" />
            </div>
          </div>

          <!-- 树形层级模式 -->
          <div v-else class="tree-nodes">
            <!-- 根级别未分类选项 -->
            <div
              class="tree-node-row special-none"
              :class="{ active: !modelValue }"
              @click="clearSelection($event); isOpen = false"
            >
              <span class="node-indent" style="width: 8px;"></span>
              <DemoIcon name="circle-slash" :size="14" class="node-icon muted" />
              <span class="node-label muted">未分类</span>
              <DemoIcon v-if="!modelValue" name="check" :size="14" class="check-icon" />
            </div>

            <div v-if="!tree.length" class="empty-tip">暂无分类，请在上方新建</div>

            <!-- 递归渲染树节点 -->
            <template v-for="node in tree" :key="node.category.id">
              <div
                class="tree-node-item"
                :style="{ '--indent-level': node.level }"
              >
                <div
                  class="tree-node-row"
                  :class="{ active: node.category.id === modelValue }"
                  @click="selectNode(node.category.id)"
                >
                  <!-- 缩进垫片 -->
                  <span class="node-indent" :style="{ width: `${node.level * 18 + 8}px` }"></span>

                  <!-- 折叠箭头 -->
                  <button
                    v-if="node.children?.length"
                    class="node-expand-btn"
                    type="button"
                    @click="toggleExpand(node.category.id, $event)"
                  >
                    <DemoIcon name="chevron-right" :size="12" :class="{ rotate: isExpanded(node.category.id) }" />
                  </button>
                  <span v-else class="node-expand-placeholder"></span>

                  <!-- 图标与名称 -->
                  <DemoIcon
                    :name="node.children?.length ? (isExpanded(node.category.id) ? 'folder-open' : 'folder') : 'tag'"
                    :size="14"
                    class="node-icon"
                  />
                  <span class="node-label">{{ node.category.name }}</span>

                  <span class="node-count" :title="`直属 ${node.directCount} / 累计 ${node.totalCount}`">
                    {{ node.totalCount }}
                  </span>

                  <!-- 行内快速加子分类 -->
                  <button
                    v-if="allowQuickCreate"
                    class="inline-add-btn"
                    type="button"
                    title="在此分类下新建子分类"
                    @click="startQuickCreate(node.category.id, $event)"
                  >
                    <DemoIcon name="plus" :size="12" />
                  </button>

                  <DemoIcon v-if="node.category.id === modelValue" name="check" :size="14" class="check-icon" />
                </div>

                <!-- 子节点递归展开 -->
                <div v-if="node.children?.length && isExpanded(node.category.id)" class="tree-node-children">
                  <component
                    :is="'CategoryTreeSelectSubNodes'"
                    :nodes="node.children"
                    :model-value="modelValue"
                    :expanded-keys="expandedKeys"
                    :allow-quick-create="allowQuickCreate"
                    @select="selectNode"
                    @toggle-expand="toggleExpand"
                    @quick-create="startQuickCreate"
                  />
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<script lang="ts">
// 递归子组件定义
import { defineComponent, type PropType } from 'vue'

const CategoryTreeSelectSubNodes = defineComponent({
  name: 'CategoryTreeSelectSubNodes',
  components: { DemoIcon },
  props: {
    nodes: {
      type: Array as PropType<CategoryTreeNode[]>,
      required: true,
    },
    modelValue: {
      type: String,
      default: '',
    },
    expandedKeys: {
      type: Object as PropType<Set<string>>,
      required: true,
    },
    allowQuickCreate: {
      type: Boolean,
      default: true,
    },
  },
  emits: ['select', 'toggle-expand', 'quick-create'],
  template: `
    <div class="sub-nodes">
      <div v-for="node in nodes" :key="node.category.id" class="tree-node-item">
        <div
          class="tree-node-row"
          :class="{ active: node.category.id === modelValue }"
          @click="$emit('select', node.category.id)"
        >
          <span class="node-indent" :style="{ width: (node.level * 18 + 8) + 'px' }"></span>

          <button
            v-if="node.children?.length"
            class="node-expand-btn"
            type="button"
            @click="$emit('toggle-expand', node.category.id, $event)"
          >
            <DemoIcon name="chevron-right" :size="12" :class="{ rotate: expandedKeys.has(node.category.id) }" />
          </button>
          <span v-else class="node-expand-placeholder"></span>

          <DemoIcon
            :name="node.children?.length ? (expandedKeys.has(node.category.id) ? 'folder-open' : 'folder') : 'tag'"
            :size="14"
            class="node-icon"
          />
          <span class="node-label">{{ node.category.name }}</span>

          <span class="node-count" :title="'直属 ' + node.directCount + ' / 累计 ' + node.totalCount">
            {{ node.totalCount }}
          </span>

          <button
            v-if="allowQuickCreate"
            class="inline-add-btn"
            type="button"
            title="在此分类下新建子分类"
            @click="$emit('quick-create', node.category.id, $event)"
          >
            <DemoIcon name="plus" :size="12" />
          </button>

          <DemoIcon v-if="node.category.id === modelValue" name="check" :size="14" class="check-icon" />
        </div>

        <div v-if="node.children?.length && expandedKeys.has(node.category.id)" class="tree-node-children">
          <CategoryTreeSelectSubNodes
            :nodes="node.children"
            :model-value="modelValue"
            :expanded-keys="expandedKeys"
            :allow-quick-create="allowQuickCreate"
            @select="(id) => $emit('select', id)"
            @toggle-expand="(id, e) => $emit('toggle-expand', id, e)"
            @quick-create="(id, e) => $emit('quick-create', id, e)"
          />
        </div>
      </div>
    </div>
  `,
})

export default {
  components: {
    CategoryTreeSelectSubNodes,
  },
}
</script>

<style scoped>
.tree-select {
  position: relative;
  width: 100%;
  font-size: 13px;
  user-select: none;
}

.tree-select.disabled {
  opacity: 0.6;
  cursor: not-allowed;
  pointer-events: none;
}

.tree-select-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 36px;
  padding: 4px 10px;
  background: var(--bg-1, #1a1d24);
  border: 1px solid var(--line, #2d3340);
  border-radius: 8px;
  cursor: pointer;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.tree-select-trigger:hover {
  border-color: var(--accent, #3b82f6);
}

.tree-select.open .tree-select-trigger {
  border-color: var(--accent, #3b82f6);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.2);
}

.selected-view {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
  overflow: hidden;
}

.category-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 8px;
  background: rgba(59, 130, 246, 0.15);
  border: 1px solid rgba(59, 130, 246, 0.3);
  border-radius: 6px;
  color: var(--accent-light, #93c5fd);
  font-weight: 500;
  font-size: 12.5px;
  max-width: 100%;
}

.badge-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.path-hint {
  color: var(--text-3, #6b7280);
  font-size: 11.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.placeholder {
  color: var(--text-3, #6b7280);
}

.trigger-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: 8px;
  color: var(--text-3, #6b7280);
}

.clear-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border-radius: 4px;
  background: transparent;
  border: none;
  color: var(--text-3, #6b7280);
  cursor: pointer;
}

.clear-btn:hover {
  color: var(--text-1, #f3f4f6);
  background: rgba(255, 255, 255, 0.1);
}

.arrow-icon {
  transition: transform 0.2s ease;
}

.arrow-icon.rotate {
  transform: rotate(180deg);
}

/* 下拉弹层 */
.tree-select-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  z-index: 1050;
  background: var(--bg-card, #1f232b);
  border: 1px solid var(--line, #2d3340);
  border-radius: 10px;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.5), 0 8px 10px -6px rgba(0, 0, 0, 0.5);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  max-height: 360px;
}

.dropdown-search {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--line, #2d3340);
  background: var(--bg-2, #16181e);
}

.search-icon {
  color: var(--text-3, #6b7280);
  flex-shrink: 0;
}

.search-input {
  flex: 1;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text-1, #f3f4f6);
  font-size: 12.5px;
}

.clear-search-btn {
  border: none;
  background: transparent;
  color: var(--text-3, #6b7280);
  cursor: pointer;
  padding: 2px;
}

.quick-create-bar {
  padding: 6px 12px;
  background: rgba(59, 130, 246, 0.08);
  border-bottom: 1px solid var(--line, #2d3340);
}

.btn-text-action {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: transparent;
  border: none;
  color: var(--accent, #3b82f6);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
}

.btn-text-action:hover {
  background: rgba(59, 130, 246, 0.15);
}

.quick-create-form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.create-parent-tip {
  font-size: 11px;
  color: var(--text-2, #9ca3af);
}

.create-input-row {
  display: flex;
  gap: 6px;
}

.create-input {
  flex: 1;
  min-width: 0;
}

.tree-list-wrap {
  flex: 1;
  overflow-y: auto;
  padding: 6px 0;
}

.tree-node-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px 6px 4px;
  cursor: pointer;
  color: var(--text-2, #9ca3af);
  transition: background 0.15s, color 0.15s;
  position: relative;
}

.tree-node-row:hover {
  background: rgba(255, 255, 255, 0.05);
  color: var(--text-1, #f3f4f6);
}

.tree-node-row.active {
  background: rgba(59, 130, 246, 0.15);
  color: var(--accent-light, #93c5fd);
  font-weight: 500;
}

.node-indent {
  flex-shrink: 0;
}

.node-expand-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  background: transparent;
  color: var(--text-3, #6b7280);
  cursor: pointer;
  border-radius: 3px;
  flex-shrink: 0;
  transition: color 0.15s;
}

.node-expand-btn:hover {
  color: var(--text-1, #f3f4f6);
  background: rgba(255, 255, 255, 0.1);
}

.node-expand-btn .rotate {
  transform: rotate(90deg);
}

.node-expand-placeholder {
  width: 18px;
  flex-shrink: 0;
}

.node-icon {
  flex-shrink: 0;
  color: var(--accent, #3b82f6);
}

.node-icon.muted {
  color: var(--text-3, #6b7280);
}

.node-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-label.muted {
  color: var(--text-3, #6b7280);
}

.node-count {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-3, #6b7280);
  background: rgba(255, 255, 255, 0.05);
  padding: 1px 6px;
  border-radius: 10px;
}

.inline-add-btn {
  display: none;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  color: var(--text-3, #6b7280);
  cursor: pointer;
  border-radius: 4px;
  margin-left: 4px;
}

.tree-node-row:hover .inline-add-btn {
  display: inline-flex;
}

.inline-add-btn:hover {
  color: var(--accent, #3b82f6);
  background: rgba(59, 130, 246, 0.2);
}

.check-icon {
  color: var(--accent, #3b82f6);
  margin-left: 4px;
  flex-shrink: 0;
}

/* 搜索结果列表 */
.search-result-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  cursor: pointer;
  transition: background 0.15s;
}

.search-result-item:hover {
  background: rgba(255, 255, 255, 0.05);
}

.search-result-item.active {
  background: rgba(59, 130, 246, 0.15);
  color: var(--accent-light, #93c5fd);
}

.result-texts {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.result-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-1, #f3f4f6);
}

.result-path {
  font-size: 11px;
  color: var(--text-3, #6b7280);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-tip {
  padding: 24px;
  text-align: center;
  color: var(--text-3, #6b7280);
  font-size: 12.5px;
}

.dropdown-fade-enter-active,
.dropdown-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-fade-enter-from,
.dropdown-fade-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>
