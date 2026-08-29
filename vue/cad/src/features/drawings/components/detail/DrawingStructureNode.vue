<script setup lang="ts">
import { ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import type { StructureTreeNode } from '@/types/structure.types'

defineOptions({ name: 'DrawingStructureNode' })

const props = defineProps<{
  node: StructureTreeNode
  selectedNo: string
  depth?: number
}>()

const emit = defineEmits<{
  select: [partNo: string]
}>()

const expanded = ref(true)
const depth = props.depth ?? 0

function selectNode() {
  emit('select', props.node.part.no)
}
</script>

<template>
  <div class="structure-node">
    <div class="structure-node__row" :class="{ selected: selectedNo === node.part.no }" :style="{ paddingLeft: `${10 + depth * 18}px` }">
      <button v-if="node.children.length" class="structure-node__toggle" type="button" :aria-label="expanded ? '折叠子节点' : '展开子节点'" @click.stop="expanded = !expanded">
        <DemoIcon name="chevron-down" :size="13" :class="{ collapsed: !expanded }" />
      </button>
      <span v-else class="structure-node__toggle-placeholder"></span>
      <button class="structure-node__select" type="button" @click="selectNode">
        <DemoIcon class="structure-node__icon" :name="node.children.length ? 'folder-tree' : 'file'" :size="15" />
        <span class="structure-node__main">
          <span class="structure-node__name" :title="node.part.name">{{ node.part.name }}</span>
          <span class="structure-node__badges">
            <span v-if="node.part.otherFiles?.length" class="tag mute structure-node__other-count">其他 {{ node.part.otherFiles.length }}</span>
            <span v-if="node.part.borrowFrom" class="tag plain structure-node__borrow">借用</span>
          </span>
        </span>
        <span class="structure-node__meta">
          <span class="structure-node__no" :title="node.part.no">{{ node.part.no }}</span>
          <span v-if="node.part.borrowFrom" class="structure-node__source" :title="`来源：${node.part.borrowFrom}`">来源 {{ node.part.borrowFrom }}</span>
        </span>
      </button>
    </div>
    <div v-if="expanded && node.children.length" class="structure-node__children">
      <DrawingStructureNode
        v-for="child in node.children"
        :key="child.part.no"
        :node="child"
        :selected-no="selectedNo"
        :depth="depth + 1"
        @select="emit('select', $event)"
      />
    </div>
  </div>
</template>

<style scoped>
.structure-node__row {
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 48px;
  border-radius: 9px;
  transition: background 0.18s;
}

.structure-node__row:hover,
.structure-node__row.selected {
  background: var(--active);
}

.structure-node__toggle,
.structure-node__toggle-placeholder {
  display: grid;
  width: 22px;
  height: 28px;
  flex: none;
  place-items: center;
  color: var(--text-3);
}

.structure-node__toggle svg {
  transition: transform 0.18s;
}

.structure-node__toggle svg.collapsed {
  transform: rotate(-90deg);
}

.structure-node__select {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  grid-template-rows: auto auto;
  column-gap: 8px;
  row-gap: 2px;
  min-width: 0;
  flex: 1;
  padding: 6px 8px 6px 0;
  color: var(--text-1);
  text-align: left;
}

.structure-node__icon {
  grid-row: 1 / span 2;
  flex: none;
  color: var(--accent);
}

.structure-node__main,
.structure-node__meta {
  display: flex;
  align-items: center;
  min-width: 0;
}

.structure-node__main {
  gap: 6px;
}

.structure-node__badges {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: none;
}

.structure-node__name {
  flex: 1 1 5em;
  min-width: 0;
  overflow-wrap: anywhere;
  line-height: 1.35;
  white-space: normal;
}

.structure-node__row.selected .structure-node__name {
  color: var(--accent);
  font-weight: 700;
}

.structure-node__no {
  min-width: 0;
  overflow: hidden;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.structure-node__meta {
  gap: 8px;
  color: var(--text-3);
}

.structure-node__source {
  min-width: 0;
  overflow: hidden;
  color: var(--text-3);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.structure-node__other-count,
.structure-node__borrow {
  flex: none;
  padding: 1px 6px;
  font-size: 9px;
}

@media (max-width: 520px) {
  .structure-node__row {
    min-height: 54px;
  }

  .structure-node__select {
    padding-top: 7px;
    padding-bottom: 7px;
  }

  .structure-node__main {
    align-items: flex-start;
    flex-direction: column;
    gap: 3px;
  }

  .structure-node__badges {
    margin-bottom: 1px;
  }

  .structure-node__meta {
    gap: 6px;
  }

  .structure-node__no {
    max-width: 100%;
  }

  .structure-node__source {
    max-width: 42%;
  }
}

.structure-node__children {
  margin-left: 20px;
  border-left: 1px dashed var(--line-strong);
}
</style>
