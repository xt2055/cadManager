<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingStructureTab' })

const router = useRouter()
const demoStore = useDemoStore()
const uiStore = useUiStore()
const drawing = computed(() => demoStore.currentDrawing)
const selected = computed(() => demoStore.structure[demoStore.selectedStructureIndex])
</script>

<template>
  <div class="struct-grid">
    <div class="card struct-tree-card">
      <div class="card-title"><DemoIcon name="folder-tree" :size="16" />结构树<span class="hint">总图 → 零件图</span></div>
      <div class="tree">
        <div class="tree-node" :class="{ closed: !demoStore.treeOpen }">
          <button class="row root-row" type="button" @click="demoStore.treeOpen = !demoStore.treeOpen">
            <DemoIcon class="caret" name="chevron-down" :size="15" />
            <DemoIcon class="tico" name="box" :size="15" />
            <span class="tname"><b>{{ drawing?.name }}</b></span><span class="tno">{{ drawing?.no }}</span>
          </button>
          <div class="children">
            <button v-for="(part, index) in demoStore.structure" :key="part.no" class="row" :class="{ sel: demoStore.selectedStructureIndex === index }" type="button" @click="demoStore.selectedStructureIndex = index">
              <span class="caret-placeholder"></span><DemoIcon class="tico" name="file" :size="15" /><span class="tname">{{ part.name }}</span><span v-if="part.borrowFrom" class="tag plain tree-borrow">借用</span><span class="tno">{{ part.no }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="sel-panel">
        <template v-if="selected">
          <div class="card-title selected-title"><DemoIcon name="file" :size="16" />{{ selected.name }} <span class="tno mono">{{ selected.no }}</span></div>
          <div class="kv-grid">
            <div class="kv"><div class="k">图号</div><div class="v mono">{{ selected.no }}</div></div>
            <div class="kv"><div class="k">材料</div><div class="v">{{ selected.material }}</div></div>
            <div class="kv"><div class="k">数量 / 台</div><div class="v mono">× {{ selected.qty }}</div></div>
            <div class="kv"><div class="k">当前版本</div><div class="v mono">{{ selected.ver }}</div></div>
            <div class="kv"><div class="k">状态</div><div class="v"><span class="tag" :class="STATUS[selected.status].c">{{ STATUS[selected.status].t }}</span></div></div>
            <div class="kv"><div class="k">借用来源</div><div class="v">{{ selected.borrowFrom ? selected.borrowFrom : '— 本图原创' }}</div></div>
          </div>
          <div v-if="selected.hasFile" class="selection-actions"><button class="btn primary sm" type="button" @click="router.push({ name: 'drawing-preview', params: { drawingId: selected.no } })"><DemoIcon name="eye" :size="14" />查看图纸</button><button class="btn sm" type="button" @click="uiStore.openModal('upload-version', '上传新版本')"><DemoIcon name="upload" :size="14" />上传新版本</button></div>
          <div v-else class="empty compact-empty"><DemoIcon name="file-plus" :size="34" /><div class="t">尚未上传图纸文件</div><div>结构已搭好，可先流转，稍后补传图纸</div><button class="btn sm primary" type="button" @click="uiStore.openModal('upload-version', '上传新版本')"><DemoIcon name="upload" :size="14" />补传图纸文件</button></div>
          <div class="struct-map">结构关系：<b>{{ drawing?.name }}</b>（总图）→ {{ demoStore.structure.length }} 个零件，变更时可选择是否同步原图。</div>
        </template>
        <template v-else>
          <div class="empty"><DemoIcon name="file" :size="34" /><div class="t">该图纸为零件图，没有下级结构</div><button class="btn sm" type="button" @click="router.push({ name: 'drawing-preview', params: { drawingId: drawing?.no } })"><DemoIcon name="corner-up-left" :size="14" />查看所属总图</button></div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.struct-grid { display: grid; grid-template-columns: 340px 1fr; gap: 14px; height: 100%; min-height: 0; }
.struct-grid .card { display: flex; flex-direction: column; min-height: 0; }
.struct-tree-card { overflow: hidden; }
.tree { flex: 1; padding: 10px 12px; overflow-y: auto; }
.tree-node .row { display: flex; align-items: center; gap: 8px; width: 100%; padding: 9px 10px; border-radius: 9px; color: var(--text-1); font-size: 12.5px; text-align: left; transition: all 0.18s; }
.tree-node .row:hover { background: var(--hover); }
.tree-node .row.sel { background: var(--active); }
.tree-node .row.sel .tname { color: var(--accent); font-weight: 700; }
.tree-node .caret { flex: none; color: var(--text-3); transition: transform 0.25s; }
.tree-node.closed .caret { transform: rotate(-90deg); }
.tree-node.closed > .children { display: none; }
.caret-placeholder { width: 15px; height: 15px; flex: none; }
.tname { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tico { flex: none; color: var(--accent); }
.children { margin-left: 19px; padding-left: 4px; border-left: 1px dashed var(--line-strong); }
.tno { color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 10px; white-space: nowrap; }
.tree-borrow { padding: 1px 7px; font-size: 9.5px; }
.sel-panel { padding: 20px; overflow-y: auto; }
.selected-title { padding: 0 0 13px; }
.selected-title .tno { margin-left: 4px; }
.kv-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 13px 22px; }
.kv .k { margin-bottom: 4px; color: var(--text-3); font-size: 11px; }
.kv .v { font-size: 13px; font-weight: 500; }
.selection-actions { display: flex; gap: 9px; margin-top: 18px; }
.compact-empty { padding: 26px 14px; }
.compact-empty > div:not(.t) { color: var(--text-3); font-size: 11px; }
.struct-map { margin-top: 18px; padding: 14px 16px; border: 1px dashed var(--line-strong); border-radius: 12px; color: var(--text-2); font-size: 11.5px; line-height: 1.9; }
.struct-map b { color: var(--accent); }
@media (max-width: 1180px) { .struct-grid { grid-template-columns: 1fr; } }
</style>
