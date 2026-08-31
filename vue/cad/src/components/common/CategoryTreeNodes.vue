<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import type { CategoryTreeNode } from '@/types/domain.types'

defineOptions({
  name: 'CategoryTreeNodes',
})

withDefaults(
  defineProps<{
    nodes: CategoryTreeNode[]
    selectedId?: string
    expandedKeys?: Set<string>
    searchQuery?: string
    variant?: 'select' | 'nav' | 'admin'
    allowQuickCreate?: boolean
  }>(),
  {
    selectedId: '',
    expandedKeys: () => new Set<string>(),
    searchQuery: '',
    variant: 'nav',
    allowQuickCreate: false,
  },
)

const emit = defineEmits<{
  (e: 'select', id: string): void
  (e: 'toggle-expand', id: string, event?: Event): void
  (e: 'quick-create', parentId: string, event?: Event): void
  (e: 'create-child', parentId: string): void
  (e: 'edit', id: string, name: string, parentId?: string): void
  (e: 'remove', id: string, name: string): void
}>()

function isExpanded(id: string, expandedKeys: Set<string>, searchQuery: string): boolean {
  if (searchQuery.trim()) return true
  return expandedKeys.has(id)
}
</script>

<template>
  <div class="ctn-list">
    <div v-for="node in nodes" :key="node.category.id" class="ctn-block">
      <div
        class="ctn-row"
        :class="[`ctn-row--${variant}`, { active: selectedId === node.category.id }]"
        :style="{ paddingLeft: `${node.level * 16 + 6}px` }"
        @click="emit('select', node.category.id)"
      >
        <!-- 折叠按钮 -->
        <button
          v-if="node.children?.length"
          class="ctn-expand"
          type="button"
          tabindex="-1"
          @click.stop="emit('toggle-expand', node.category.id, $event)"
        >
          <DemoIcon name="chevron-right" :size="12" :class="{ rotate: isExpanded(node.category.id, expandedKeys, searchQuery) }" />
        </button>
        <span v-else class="ctn-expand-placeholder"></span>

        <!-- 图标 -->
        <DemoIcon
          :name="node.children?.length && isExpanded(node.category.id, expandedKeys, searchQuery) ? 'folder-open' : 'folder'"
          :size="14"
          class="ctn-folder-ico"
        />

        <!-- 名称 -->
        <span class="ctn-name" :title="node.category.name">{{ node.category.name }}</span>

        <!-- 数量徽标 -->
        <span class="ctn-count" :title="`直属 ${node.directCount} / 累计 ${node.totalCount}`">
          {{ node.totalCount }}
        </span>

        <!-- select 变体：行内快捷新建 + 选中勾 -->
        <template v-if="variant === 'select'">
          <button
            v-if="allowQuickCreate"
            class="ctn-inline-add"
            type="button"
            title="在此分类下新建子分类"
            @click.stop="emit('quick-create', node.category.id, $event)"
          >
            <DemoIcon name="plus" :size="12" />
          </button>
          <DemoIcon v-if="selectedId === node.category.id" name="check" :size="14" class="ctn-check" />
        </template>

        <!-- admin 变体：悬浮操作组 -->
        <div v-if="variant === 'admin'" class="ctn-hover-actions" @click.stop>
          <button class="ctn-act" type="button" title="在此分类下新建子分类" @click="emit('create-child', node.category.id)">
            <DemoIcon name="plus" :size="12" />
          </button>
          <button
            class="ctn-act"
            type="button"
            title="重命名分类"
            @click="emit('edit', node.category.id, node.category.name, node.category.parentId)"
          >
            <DemoIcon name="pencil" :size="12" />
          </button>
          <button
            class="ctn-act danger"
            type="button"
            title="删除分类"
            @click="emit('remove', node.category.id, node.category.name)"
          >
            <DemoIcon name="trash-2" :size="12" />
          </button>
        </div>
      </div>

      <!-- 递归子级 -->
      <div
        v-if="node.children?.length && isExpanded(node.category.id, expandedKeys, searchQuery)"
        class="ctn-children"
      >
        <CategoryTreeNodes
          :nodes="node.children"
          :selected-id="selectedId"
          :expanded-keys="expandedKeys"
          :search-query="searchQuery"
          :variant="variant"
          :allow-quick-create="allowQuickCreate"
          @select="(id) => emit('select', id)"
          @toggle-expand="(id, e) => emit('toggle-expand', id, e)"
          @quick-create="(id, e) => emit('quick-create', id, e)"
          @create-child="(id) => emit('create-child', id)"
          @edit="(id, name, pid) => emit('edit', id, name, pid)"
          @remove="(id, name) => emit('remove', id, name)"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.ctn-list {
  display: flex;
  flex-direction: column;
}

.ctn-block {
  display: flex;
  flex-direction: column;
}

.ctn-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  border-radius: 6px;
  color: var(--text-2);
  font-size: 12.5px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
  min-width: 0;
}

.ctn-row:hover {
  background: var(--hover);
  color: var(--text-1);
}

.ctn-row.active {
  background: var(--active);
  color: var(--accent);
  font-weight: 500;
}

.ctn-expand {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  border-radius: 3px;
  flex-shrink: 0;
}

.ctn-expand:hover {
  color: var(--text-1);
  background: var(--hover);
}

.ctn-expand .rotate {
  transform: rotate(90deg);
}

.ctn-expand-placeholder {
  width: 14px;
  flex-shrink: 0;
}

.ctn-folder-ico {
  color: var(--accent);
  flex-shrink: 0;
}

.ctn-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.ctn-count {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-3);
  flex-shrink: 0;
}

.ctn-row--select .ctn-count {
  padding: 1px 6px;
  border-radius: 10px;
  background: var(--panel-2);
}

/* select 变体：行内快捷新建 */
.ctn-inline-add {
  display: none;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  border-radius: 4px;
  flex-shrink: 0;
}

.ctn-row--select:hover .ctn-inline-add {
  display: inline-flex;
}

.ctn-inline-add:hover {
  color: var(--accent);
  background: var(--active);
}

.ctn-check {
  color: var(--accent);
  margin-left: 4px;
  flex-shrink: 0;
}

/* admin 变体：悬浮操作组 */
.ctn-hover-actions {
  display: none;
  align-items: center;
  gap: 2px;
  margin-left: 4px;
  flex-shrink: 0;
}

.ctn-row--admin:hover .ctn-hover-actions {
  display: flex;
}

.ctn-act {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 3px;
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  border-radius: 4px;
}

.ctn-act:hover {
  color: var(--text-1);
  background: var(--hover);
}

.ctn-act.danger:hover {
  color: var(--danger);
  background: var(--hover);
}
</style>
