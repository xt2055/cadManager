<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingStatus } from '@/types/domain.types'

defineOptions({
  name: 'DrawingLibraryPage',
})

const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()
const query = ref('')
const status = ref<DrawingStatus | ''>('')
const menuFor = ref<string | null>(null)
const expandedProjects = ref<Set<string>>(new Set())

const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  return domainStore.drawings.filter((drawing) => {
    const parts = partsForDrawing(drawing.no)
    const matchesQuery = !q || [drawing.no, drawing.name, drawing.vendor, drawing.project, ...parts.flatMap((part) => [part.no, part.name])].join(' ').toLowerCase().includes(q)
    const matchesStatus = !status.value || drawing.status === status.value
    return matchesQuery && matchesStatus
  })
})

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
  if (action === 'borrow') uiStore.openModal('borrow-drawing', '借用图纸')
  if (action === 'hide') uiStore.toast('图纸已隐藏：用户不可见，管理员可随时恢复，历史完整保留', 'warn')
}
</script>

<template>
  <div class="page library-page">
    <div class="lib-head">
      <span class="lib-title">图纸库</span>
      <span class="lib-count">已列出 {{ rows.length }} 个总图 · 零件 {{ domainStore.drawingStats?.parts ?? 0 }} 项 · 总计 {{ domainStore.drawingStats?.total ?? 0 }}</span>

       <label class="search-box">
         <DemoIcon name="search" :size="15" />
         <input v-model="query" placeholder="搜索项目 / 图号 / 零件名称 / 厂商…" />
       </label>

      <select v-model="status" class="inp filter-select">
        <option value="">全部状态</option>
        <option v-for="(item, key) in STATUS" :key="key" :value="key">{{ item.t }}</option>
      </select>

      <button class="btn primary" type="button" @click="createDrawing">
        <DemoIcon name="plus" :size="14" />创建图纸
      </button>
    </div>

    <div class="card library-card">
      <table class="tbl">
        <thead>
          <tr>
             <th>项目图号</th>
             <th>名称</th>
             <th>厂商</th>
            <th>状态</th>
            <th>版本</th>
            <th>更新</th>
            <th class="operation-column">操作</th>
          </tr>
        </thead>
        <tbody>
           <template v-for="drawing in rows" :key="drawing.no">
           <tr>
             <td class="num project-no-cell">
               <button v-if="partsForDrawing(drawing.no).length" class="expand-button" type="button" :title="isExpanded(drawing.no) ? '收起零件图' : '展开零件图'" @click="toggleExpanded(drawing.no)">
                 <DemoIcon name="chevron-down" :size="14" :class="{ collapsed: !isExpanded(drawing.no) }" />
               </button>
               <button class="link project-no-link" type="button" @click="openDetail(drawing.no)">{{ drawing.no }}</button>
             </td>
             <td class="drawing-name">
               {{ drawing.name }}
               <span v-if="drawing.borrowFrom" class="tag plain borrow-tag">借用·{{ drawing.borrowFrom }}</span>
             </td>
             <td>{{ drawing.vendor }}</td>
            <td><span class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span></td>
            <td class="num">{{ drawing.ver }}</td>
            <td class="num updated">{{ drawing.updated }}</td>
            <td class="row-actions">
              <button class="btn sm" type="button" @click="openDetail(drawing.no)"><DemoIcon name="eye" :size="14" />详情</button>
              <div class="row-menu-wrap">
                <button class="icon-btn row-menu-button" type="button" @click.stop="toggleMenu(drawing.no)"><DemoIcon name="ellipsis" :size="16" /></button>
                <div v-if="menuFor === drawing.no" class="dropdown row-dropdown">
                  <button class="dd-item" type="button" @click="menuAction('detail', drawing.no)"><DemoIcon name="eye" :size="14" />查看详情与文件</button>
                  <button class="dd-item" type="button" @click="menuAction('borrow', drawing.no)"><DemoIcon name="share-2" :size="14" />借用此图</button>
                  <div class="dd-sep"></div>
                  <button class="dd-item" type="button" @click="menuAction('hide', drawing.no)"><DemoIcon name="eye-off" :size="14" />隐藏图纸（管理员）</button>
                </div>
              </div>
             </td>
           </tr>
           <tr v-if="isExpanded(drawing.no)" class="parts-row">
             <td colspan="7">
               <div class="parts-panel">
                 <button v-for="part in partsForDrawing(drawing.no)" :key="part.no" class="part-link" type="button" @click="openDetail(part.no)">
                   <DemoIcon name="file" :size="13" />
                   <span class="part-link__name">{{ part.name }}</span>
                   <span class="part-link__no mono">{{ part.no }}</span>
                   <span v-if="part.borrowFrom" class="tag plain borrow-tag">借用·{{ part.borrowFrom }}</span>
                   <DemoIcon name="arrow-up-right" :size="13" />
                 </button>
               </div>
             </td>
           </tr>
           </template>
           <tr v-if="!rows.length">
             <td colspan="7">
              <div class="empty"><DemoIcon name="search-x" :size="34" /><div class="t">{{ domainStore.drawings.length ? '没有匹配的图纸，试试更换关键词' : '暂无图纸，请先创建或导入图纸' }}</div></div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.library-page {
  min-width: 0;
}

.lib-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.lib-title {
  margin-right: 4px;
  font-family: var(--font-display);
  font-size: 19px;
  font-weight: 900;
}

.lib-count {
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
}

.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 320px;
  height: 36px;
  margin-left: auto;
  padding: 0 13px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel);
  color: var(--text-3);
  transition: all 0.25s;
}

.search-box:focus-within {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.search-box input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: none;
  font-size: 12.5px;
}

.filter-select {
  width: 110px;
  height: 36px;
}

.library-card {
  overflow: visible;
}

.drawing-name {
  font-weight: 500;
  white-space: nowrap;
}

.project-no-cell {
  white-space: nowrap;
}

.expand-button {
  display: inline-grid;
  width: 24px;
  height: 24px;
  margin-right: 4px;
  place-items: center;
  border-radius: 6px;
  color: var(--text-3);
  vertical-align: middle;
}

.expand-button:hover {
  background: var(--hover);
  color: var(--accent);
}

.expand-button svg {
  transition: transform 0.18s;
}

.expand-button svg.collapsed {
  transform: rotate(-90deg);
}

.project-no-link {
  padding: 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
}

.parts-row td {
  padding: 0 12px 12px;
  background: var(--panel-2);
}

.parts-panel {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 12px 4px 42px;
  border-left: 2px solid var(--accent-soft);
}

.part-link {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0;
  padding: 7px 8px;
  border-radius: 7px;
  color: var(--text-2);
  text-align: left;
}

.part-link:hover {
  background: var(--hover);
  color: var(--accent);
}

.part-link > svg:first-child {
  flex: none;
  color: var(--accent);
}

.part-link__name {
  min-width: 100px;
  font-weight: 600;
}

.part-link__no {
  min-width: 0;
  overflow: hidden;
  color: var(--text-3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.part-link > svg:last-child {
  flex: none;
  margin-left: auto;
}

.borrow-tag {
  margin-left: 4px;
  padding: 1px 7px;
  font-size: 9.5px;
}

.updated {
  color: var(--text-3);
}

.operation-column {
  width: 130px;
}

.row-actions {
  position: relative;
  white-space: nowrap;
}

.row-menu-wrap {
  position: relative;
  display: inline-block;
}

.row-menu-button {
  display: inline-grid;
  width: 28px;
  height: 28px;
  vertical-align: middle;
}

.row-dropdown {
  top: 34px;
  right: 0;
}

@media (max-width: 1180px) {
  .search-box {
    margin-left: 0;
  }
}

@media (max-width: 760px) {
  .lib-head {
    align-items: stretch;
  }

  .lib-title,
  .lib-count {
    width: auto;
  }

  .search-box {
    width: 100%;
  }

  .library-card {
    overflow-x: auto;
  }

  .tbl {
    min-width: 760px;
  }
}
</style>
