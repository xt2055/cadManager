<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import DrawingStructureNode from '@/features/drawings/components/detail/DrawingStructureNode.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'
import { isEquivalentAssemblyNo, isSameDrawingFamily, parseStandaloneDrawingFileName } from '@/utils/drawing-number-parser'
import { formatReadableDateTime } from '@/utils/date-time'
import type { StructurePart } from '@/types/domain.types'
import type { StructureTreeNode } from '@/types/structure.types'

defineOptions({ name: 'DrawingStructureTab' })

const router = useRouter()
const domainStore = useDomainStore()
const drawing = computed(() => domainStore.currentDrawing)
const allParts = computed(() => domainStore.structure)
const treeNodes = computed<StructureTreeNode[]>(() => {
  if (!drawing.value) return []
  const childrenByParent = new Map<string, StructurePart[]>()
  const partByNo = new Map(allParts.value.map((part) => [part.no, part]))
  const knownAssemblyNos = [
    drawing.value.no,
    ...('files' in drawing.value ? (drawing.value.files ?? []).map((file) => parseStandaloneDrawingFileName(file.name).no) : []),
  ].filter(Boolean)
  const isUnderCurrentDrawing = (part: StructurePart): boolean => {
    if (!drawing.value) return false
    if (isEquivalentAssemblyNo(part.no, drawing.value.no, knownAssemblyNos)) return false
    if (isEquivalentAssemblyNo(part.parentNo, drawing.value.no, knownAssemblyNos) || isSameDrawingFamily(part.no, drawing.value.no)) return true
    const visited = new Set<string>()
    let parentNo = part.parentNo
    while (parentNo && !visited.has(parentNo)) {
      if (isEquivalentAssemblyNo(parentNo, drawing.value.no, knownAssemblyNos)) return true
      visited.add(parentNo)
      parentNo = partByNo.get(parentNo)?.parentNo || ''
    }
    // 只有存在明确的当前项目编号时才使用项目字段兜底，避免把其他项目同名零件混入树。
    const currentProject = 'project' in drawing.value ? drawing.value.project : ''
    return Boolean(currentProject && part.project === currentProject && part.parentNo !== drawing.value.no)
  }

  const relevantParts = allParts.value.filter(isUnderCurrentDrawing)
  const relevantPartNos = new Set(relevantParts.map((part) => part.no))
  for (const part of relevantParts) {
    // 父级记录缺失或不在当前相关集合中时挂回当前总图，保证历史数据与借用件始终在树中可见。
    const parentNo = part.parentNo && isEquivalentAssemblyNo(part.parentNo, drawing.value.no, knownAssemblyNos)
      ? drawing.value.no
      : part.parentNo && relevantPartNos.has(part.parentNo)
        ? part.parentNo
      : drawing.value.no
    const children = childrenByParent.get(parentNo) ?? []
    children.push(part)
    childrenByParent.set(parentNo, children)
  }

  const build = (parentNo: string, visited: Set<string>): StructureTreeNode[] => {
    return (childrenByParent.get(parentNo) ?? [])
      .slice()
      .sort((left, right) => left.no.localeCompare(right.no, undefined, { numeric: true }))
      .filter((part) => !visited.has(part.no))
      .map((part) => {
        const nextVisited = new Set(visited)
        nextVisited.add(part.no)
        return { part, children: build(part.no, nextVisited) }
      })
  }

  return build(drawing.value.no, new Set([drawing.value.no]))
})
const parts = computed(() => {
  const flatten = (nodes: StructureTreeNode[]): StructurePart[] => nodes.flatMap((node) => [node.part, ...flatten(node.children)])
  return flatten(treeNodes.value)
})
const selected = computed(() => parts.value[domainStore.selectedStructureIndex])
const otherFiles = computed(() => {
  return drawing.value?.otherFiles ?? []
})

function openPartDetail(partNo: string) {
  domainStore.openDrawing(partNo)
  router.push({ name: 'drawing-properties', params: { drawingId: partNo } })
}

function selectPart(partNo: string) {
  const index = parts.value.findIndex((part) => part.no === partNo)
  if (index >= 0) domainStore.selectedStructureIndex = index
}

</script>

<template>
  <div class="struct-grid">
    <div class="card struct-tree-card">
      <div class="card-title">
        <DemoIcon name="folder-tree" :size="16" />
        工程装配结构树
        <span class="hint">总图 → 零件图</span>
      </div>
       <div class="tree">
         <div class="tree-node" :class="{ closed: !domainStore.treeOpen }">
           <button class="row root-row" type="button" @click="domainStore.treeOpen = !domainStore.treeOpen">
             <DemoIcon class="caret" name="chevron-down" :size="15" />
             <DemoIcon class="tico" name="box" :size="15" />
             <span class="root-copy">
               <span class="tname"><b>{{ drawing?.name }}</b></span>
               <span class="root-meta"><span class="tno">{{ drawing?.no }}</span><span class="root-kind">总图</span></span>
             </span>
           </button>
           <div class="children">
             <DrawingStructureNode
               v-for="node in treeNodes"
               :key="node.part.no"
               :node="node"
               :selected-no="selected?.no ?? ''"
               @select="selectPart"
             />
             <div v-if="!treeNodes.length" class="tree-empty">暂无按图号识别的下级零件</div>
           </div>
           <div v-if="otherFiles.length" class="other-files-group">
             <div class="other-files-title"><DemoIcon name="file-text" :size="14" />其他文件 <span>{{ otherFiles.length }}</span></div>
             <div v-for="file in otherFiles" :key="file.id" class="other-file-row">
               <DemoIcon name="file" :size="13" />
               <span>{{ file.name }}</span>
             </div>
           </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="sel-panel">
        <template v-if="selected">
          <div class="card-title selected-title">
            <DemoIcon name="file" :size="16" />
            {{ selected.name }}
            <span class="tno mono">{{ selected.no }}</span>
          </div>

             <div class="kv-grid">
               <div class="kv"><div class="k">零件图号</div><div class="v mono">{{ selected.no }}</div></div>
             <div class="kv"><div class="k">零件材料</div><div class="v">{{ selected.material || '—' }}</div></div>
              <div class="kv"><div class="k">制造类别</div><div class="v"><span class="tag info">{{ selected.partType }}</span></div></div>
             <div class="kv"><div class="k">单机装配数量</div><div class="v mono">× {{ selected.qty }}</div></div>
            <div class="kv"><div class="k">当前发布版本</div><div class="v mono">{{ selected.ver }}</div></div>
            <div class="kv"><div class="k">生命周期状态</div><div class="v"><span class="tag" :class="STATUS[selected.status].c">{{ STATUS[selected.status].t }}</span></div></div>
             <div class="kv"><div class="k">借用来源</div><div class="v">{{ selected.borrowFrom ? selected.borrowFrom : '— 本项目原创' }}</div></div>
           </div>

           <div class="detail-files-section">
             <div class="detail-files-title"><DemoIcon name="file-text" :size="15" />关联文件</div>
             <div v-if="selected.files?.length || selected.otherFiles?.length" class="detail-files-list">
               <div v-for="file in [...(selected.files ?? []), ...(selected.otherFiles ?? [])]" :key="file.id" class="detail-file-row">
                 <div class="detail-file-name"><DemoIcon name="file" :size="14" /><span :title="file.name">{{ file.name }}</span></div>
                 <span class="detail-file-meta">大小 {{ file.size }}</span>
                 <span class="detail-file-meta">上传 {{ formatReadableDateTime(file.uploadedAt, '历史记录') }}</span>
               </div>
             </div>
             <div v-else class="detail-file-empty">暂无关联图纸文件</div>
           </div>

           <div class="selection-actions">
             <button class="btn sm" type="button" @click="openPartDetail(selected.no)">
               <DemoIcon name="pencil" :size="14" />编辑零件属性
             </button>
           </div>

              <div class="struct-map">
                结构关系提示：<b>{{ drawing?.name }}</b>（总图装配）→ 共关联 {{ parts.length }} 个零件图；系统按照“去掉最后一级图号后缀”的规则生成多级父子关系，不符合命名规则的文件归入“其他文件”。
          </div>
        </template>

        <template v-else>
          <div class="empty">
            <DemoIcon name="file" :size="34" />
            <div class="t">当前对象为独立零件图，没有下级装配结构</div>
            <button class="btn sm" type="button" @click="router.push({ name: 'drawing-library' })">
              <DemoIcon name="corner-up-left" :size="14" />返回图纸库
            </button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.struct-grid {
  display: grid;
  grid-template-columns: 340px 1fr;
  gap: 14px;
  height: 100%;
  min-height: 0;
}
.struct-grid .card {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.struct-tree-card {
  overflow: hidden;
}
.tree {
  flex: 1;
  min-width: 0;
  padding: 10px 12px 14px;
  overflow-y: auto;
  overflow-x: hidden;
}
.tree-empty {
  padding: 16px 10px;
  color: var(--text-3);
  font-size: 11.5px;
}
.other-files-group {
  margin: 12px 10px 4px;
  padding-top: 10px;
  border-top: 1px dashed var(--line-strong);
}
.other-files-title {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-2);
  font-size: 11px;
}
.other-files-title svg { color: var(--text-3); }
.other-files-title span { color: var(--text-3); font-family: 'JetBrains Mono', monospace; }
.other-file-row {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 7px 3px;
  color: var(--text-3);
  font-size: 10.5px;
}
.other-file-row span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tree-node .row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 9px 10px;
  border-radius: 9px;
  color: var(--text-1);
  font-size: 12.5px;
  text-align: left;
  transition: all 0.18s;
}

.root-copy,
.root-meta {
  min-width: 0;
}

.root-copy {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 3px;
}
.tree-node .row:hover {
  background: var(--hover);
}
.tree-node .row.sel {
  background: var(--active);
}
.tree-node .row.sel .tname {
  color: var(--accent);
  font-weight: 700;
}
.tree-node .caret {
  flex: none;
  color: var(--text-3);
  transition: transform 0.25s;
}
.tree-node.closed .caret {
  transform: rotate(-90deg);
}
.tree-node.closed > .children {
  display: none;
}
.caret-placeholder {
  width: 15px;
  height: 15px;
  flex: none;
}
.tname {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tico {
  flex: none;
  color: var(--accent);
}
.children {
  margin-left: 19px;
  padding-left: 4px;
  border-left: 1px dashed var(--line-strong);
}
.tno {
  min-width: 0;
  overflow: hidden;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.root-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.root-kind {
  color: var(--text-3);
  font-size: 10px;
}
.tree-borrow {
  padding: 1px 7px;
  font-size: 9.5px;
}
.sel-panel {
  padding: 20px;
  overflow-y: auto;
}
.selected-title {
  padding: 0 0 13px;
}
.selected-title .tno {
  margin-left: 4px;
}
.kv-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 13px 22px;
}
.kv .k {
  margin-bottom: 4px;
  color: var(--text-3);
  font-size: 11px;
}
.kv .v {
  font-size: 13px;
  font-weight: 500;
}
.selection-actions {
  display: flex;
  gap: 9px;
  margin-top: 18px;
}
.detail-files-section {
  margin-top: 18px;
  padding-top: 15px;
  border-top: 1px solid var(--line);
}
.detail-files-title {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 9px;
  color: var(--text-2);
  font-size: 12px;
  font-weight: 700;
}
.detail-files-title svg { color: var(--accent); }
.detail-files-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.detail-file-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 12px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--panel-2);
}
.detail-file-name {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 7px;
  font-size: 12px;
}
.detail-file-name svg { flex: none; color: var(--accent); }
.detail-file-name span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.detail-file-meta {
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  white-space: nowrap;
}
.detail-file-empty {
  color: var(--text-3);
  font-size: 11px;
}

@media (max-width: 760px) {
  .struct-grid {
    display: flex;
    flex-direction: column;
    gap: 10px;
    height: auto;
  }

  .struct-tree-card {
    max-height: min(62vh, 560px);
  }

  .struct-grid > .card:last-child {
    min-height: 320px;
  }

  .tree {
    padding-right: 8px;
    padding-left: 8px;
  }

  .tree-node .row {
    padding: 8px;
  }

  .children {
    margin-left: 14px;
    padding-left: 2px;
  }

  .sel-panel {
    padding: 16px;
  }
}

@media (max-width: 420px) {
  .struct-tree-card {
    max-height: 64vh;
  }

  .tree-node .row {
    gap: 6px;
  }

  .root-meta {
    gap: 6px;
  }
}
.struct-map {
  margin-top: 18px;
  padding: 14px 16px;
  border: 1px dashed var(--line-strong);
  border-radius: 12px;
  color: var(--text-2);
  font-size: 11.5px;
  line-height: 1.9;
}
.struct-map b {
  color: var(--accent);
}
@media (max-width: 1180px) {
  .struct-grid {
    grid-template-columns: 1fr;
  }
}
@media (max-width: 760px) {
  .detail-file-row { grid-template-columns: 1fr; gap: 3px; }
}
</style>
