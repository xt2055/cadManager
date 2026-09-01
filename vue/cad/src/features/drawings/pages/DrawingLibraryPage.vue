<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingStatus } from '@/types/domain.types'

defineOptions({ name: 'DrawingLibraryPage' })

const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()
const query = ref('')
const status = ref<DrawingStatus | ''>('')
const attributeFilters = ref<Record<string, string>>({})
const expandedProjects = ref<Set<string>>(new Set())
const menuFor = ref<string | null>(null)

const rows = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return domainStore.drawings.filter((drawing) => {
    const attributeText = domainStore.sortedAttributes.flatMap((attribute) => [
      attribute.name,
      domainStore.attributeFieldName(attribute.id, drawing.attributeValues?.[attribute.id]),
    ])
    const searchable = [drawing.no, drawing.name, drawing.vendor, drawing.project, ...attributeText, ...partsForDrawing(drawing.no).flatMap((part) => [part.no, part.name])]
      .join(' ').toLowerCase()
    const matchesAttributes = domainStore.sortedAttributes.every((attribute) => {
      const selected = attributeFilters.value[attribute.id]
      return !selected || drawing.attributeValues?.[attribute.id] === selected
    })
    return (!keyword || searchable.includes(keyword)) && (!status.value || drawing.status === status.value) && matchesAttributes
  })
})

const activeFilterCount = computed(() => Object.values(attributeFilters.value).filter(Boolean).length)

function clearFilters() { attributeFilters.value = {} }
function partsForDrawing(drawingNo: string) {
  return domainStore.structure.filter((part) => part.no.startsWith(`${drawingNo}-`) || part.parentNo === drawingNo).sort((a, b) => a.no.localeCompare(b.no, undefined, { numeric: true }))
}
function toggleExpanded(no: string) {
  const next = new Set(expandedProjects.value)
  if (next.has(no)) next.delete(no); else next.add(no)
  expandedProjects.value = next
}
function openDetail(no: string) {
  domainStore.openDrawing(no)
  router.push({ name: 'drawing-preview', params: { drawingId: no } })
}
function toggleMenu(no: string) { menuFor.value = menuFor.value === no ? null : no }
function menuAction(action: string, no: string) {
  menuFor.value = null
  if (action === 'detail') openDetail(no)
  if (action === 'hide') uiStore.toast('图纸已隐藏：用户不可见，管理员可随时恢复，历史完整保留', 'warn')
}
</script>

<template>
  <div class="page drawing-library-page">
    <header class="library-head">
      <div>
        <div class="eyebrow"><DemoIcon name="layers" :size="14" />工程图纸资产</div>
        <h1>图纸库</h1>
        <p>按图纸属性查找和管理工程资料，共 {{ rows.length }} 个总图</p>
      </div>
      <div class="library-actions">
        <button class="btn" type="button" @click="router.push({ name: 'admin-attributes' })"><DemoIcon name="sliders-horizontal" :size="14" />属性管理</button>
        <button class="btn primary" type="button" @click="router.push({ name: 'drawing-create' })"><DemoIcon name="plus" :size="14" />创建图纸</button>
      </div>
    </header>

    <section v-if="domainStore.sortedAttributes.length" class="attribute-filter-panel card">
      <div class="filter-panel-head">
        <strong><DemoIcon name="filter" :size="14" />属性筛选 <span v-if="activeFilterCount">已选 {{ activeFilterCount }} 项</span></strong>
        <button v-if="activeFilterCount" class="btn-clear" type="button" @click="clearFilters">清除全部</button>
      </div>
      <div class="filter-grid">
        <label v-for="attribute in domainStore.sortedAttributes" :key="attribute.id" class="filter-field">
          <span>{{ attribute.name }}<em v-if="attribute.required">必填</em></span>
          <select v-model="attributeFilters[attribute.id]" class="inp">
            <option value="">全部字段</option>
            <option v-for="field in attribute.fields.filter((item) => item.enabled)" :key="field.id" :value="field.id">{{ field.name }}</option>
          </select>
        </label>
      </div>
    </section>

    <section class="card library-table-card">
      <div class="table-toolbar">
        <label class="search-box"><DemoIcon name="search" :size="15" /><input v-model="query" placeholder="搜索图号、名称、项目、属性字段..." /><button v-if="query" class="clear-search" type="button" @click="query = ''"><DemoIcon name="x" :size="12" /></button></label>
        <select v-model="status" class="inp status-select"><option value="">全部状态</option><option v-for="(item, key) in STATUS" :key="key" :value="key">{{ item.t }}</option></select>
      </div>
      <div class="table-scroll">
        <table class="tbl library-table">
          <thead><tr><th>项目图号</th><th>图纸名称</th><th>属性摘要</th><th>责任单位</th><th>状态</th><th>版本</th><th>更新时间</th><th>操作</th></tr></thead>
          <tbody>
            <template v-for="drawing in rows" :key="drawing.no">
              <tr>
                <td class="mono no-cell"><button v-if="partsForDrawing(drawing.no).length" class="expand-btn" type="button" @click="toggleExpanded(drawing.no)"><DemoIcon name="chevron-down" :size="13" :class="{ collapsed: !expandedProjects.has(drawing.no) }" /></button><button class="link" type="button" @click="openDetail(drawing.no)">{{ drawing.no }}</button></td>
                <td><strong>{{ drawing.name }}</strong><small v-if="drawing.remark">{{ drawing.remark }}</small></td>
                <td class="attribute-summary"><span v-if="domainStore.sortedAttributes.some((attribute) => drawing.attributeValues?.[attribute.id])">{{ domainStore.sortedAttributes.filter((attribute) => drawing.attributeValues?.[attribute.id]).map((attribute) => `${attribute.name}: ${domainStore.attributeFieldName(attribute.id, drawing.attributeValues?.[attribute.id])}`).join(' · ') }}</span><span v-else class="tag plain">未填写属性</span></td>
                <td>{{ drawing.vendor }}</td>
                <td><span class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span></td>
                <td class="mono">{{ drawing.ver }}</td><td class="mono">{{ drawing.updated }}</td>
                <td class="row-actions"><button class="btn sm" type="button" @click="openDetail(drawing.no)"><DemoIcon name="eye" :size="13" />详情</button><button class="icon-btn" type="button" @click="toggleMenu(drawing.no)"><DemoIcon name="ellipsis" :size="15" /></button><div v-if="menuFor === drawing.no" class="dropdown row-dropdown"><button class="dd-item" type="button" @click="menuAction('detail', drawing.no)">查看详情</button><button class="dd-item" type="button" @click="menuAction('hide', drawing.no)">隐藏图纸</button></div></td>
              </tr>
              <tr v-if="expandedProjects.has(drawing.no)" class="parts-row"><td colspan="8"><div class="parts-panel"><span class="parts-title">零件明细（{{ partsForDrawing(drawing.no).length }} 项）</span><button v-for="part in partsForDrawing(drawing.no)" :key="part.no" class="part-link" type="button" @click="openDetail(part.no)"><DemoIcon name="file" :size="13" />{{ part.name }} <span class="mono">{{ part.no }}</span></button></div></td></tr>
            </template>
            <tr v-if="!rows.length"><td colspan="8"><div class="empty"><DemoIcon name="search-x" :size="34" /><div class="t">没有符合条件的图纸</div></div></td></tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<style scoped>
.drawing-library-page { display:flex; flex-direction:column; gap:16px; min-height:100%; padding:22px 26px; }
.library-head { display:flex; justify-content:space-between; align-items:flex-start; gap:20px; }
.library-head h1 { margin:5px 0 4px; font-family:var(--font-display); font-size:22px; font-weight:900; }
.library-head p { margin:0; color:var(--text-3); font-size:12px; }
.eyebrow { display:inline-flex; align-items:center; gap:6px; color:var(--accent); font:11px 'JetBrains Mono', monospace; letter-spacing:1px; }
.library-actions { display:flex; gap:8px; }
.attribute-filter-panel { padding:14px 16px; }
.filter-panel-head { display:flex; justify-content:space-between; margin-bottom:12px; }
.filter-panel-head strong { display:inline-flex; align-items:center; gap:6px; font-size:13px; }
.filter-panel-head strong span { margin-left:5px; color:var(--text-3); font-size:11px; font-weight:400; }
.btn-clear { border:0; background:transparent; color:var(--accent); cursor:pointer; font-size:11px; }
.filter-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(180px,1fr)); gap:10px 14px; }
.filter-field { display:flex; flex-direction:column; gap:5px; }
.filter-field>span { color:var(--text-2); font-size:11.5px; font-weight:600; }
.filter-field em { margin-left:5px; color:var(--warn); font-size:10px; font-style:normal; font-weight:400; }
.table-toolbar { display:flex; align-items:center; gap:10px; padding:14px 16px; border-bottom:1px solid var(--line); }
.search-box { display:flex; align-items:center; gap:8px; flex:1; max-width:440px; padding:7px 10px; border:1px solid var(--line); border-radius:8px; color:var(--text-3); }
.search-box input { width:100%; border:0; outline:0; background:transparent; color:var(--text-1); font-size:12px; }
.clear-search { border:0; background:transparent; color:var(--text-3); cursor:pointer; }
.status-select { width:120px; }
.library-table-card { padding:0; overflow:hidden; }
.table-scroll { overflow-x:auto; }
.library-table { min-width:920px; width:100%; border-collapse:collapse; }
.library-table th { padding:10px 14px; background:var(--panel-2); color:var(--text-3); font-size:11px; font-weight:600; text-align:left; border-bottom:1px solid var(--line); }
.library-table td { padding:10px 14px; color:var(--text-2); border-bottom:1px solid var(--line); vertical-align:middle; }
.library-table td strong { display:block; color:var(--text-1); font-size:12.5px; }
.library-table td small { display:block; max-width:220px; margin-top:3px; overflow:hidden; color:var(--text-3); font-size:10.5px; text-overflow:ellipsis; white-space:nowrap; }
.no-cell { white-space:nowrap; color:var(--accent)!important; font-weight:600; }
.expand-btn { margin-right:5px; padding:2px; border:0; background:transparent; color:var(--text-3); cursor:pointer; }
.expand-btn .collapsed { transform:rotate(-90deg); }
.attribute-summary { max-width:310px; color:var(--text-2)!important; font-size:11.5px; }
.attribute-summary>span:not(.tag) { display:block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.row-actions { position:relative; display:flex; align-items:center; gap:5px; white-space:nowrap; }
.dropdown { position:absolute; right:0; top:calc(100% + 4px); z-index:20; min-width:120px; padding:5px; background:var(--panel); border:1px solid var(--line-strong); border-radius:var(--radius); box-shadow:var(--shadow); }
.dd-item { display:block; width:100%; padding:7px 9px; border:0; border-radius:5px; background:transparent; color:var(--text-2); text-align:left; cursor:pointer; font-size:11.5px; }
.dd-item:hover { background:var(--hover); color:var(--text-1); }
.parts-row { background:var(--panel-2); }
.parts-panel { display:flex; align-items:center; flex-wrap:wrap; gap:7px; padding:12px 20px; }
.parts-title { width:100%; color:var(--text-3); font-size:11px; }
.part-link { display:inline-flex; align-items:center; gap:6px; padding:6px 9px; border:1px solid var(--line); border-radius:6px; background:var(--panel); color:var(--text-2); cursor:pointer; font-size:11px; }
.part-link:hover { border-color:var(--accent); color:var(--accent); }
@media (max-width:680px) { .drawing-library-page{padding:16px}.library-head{flex-direction:column}.library-actions{width:100%}.library-actions .btn{flex:1;justify-content:center}.table-toolbar{flex-wrap:wrap}.search-box{max-width:none;flex-basis:100%} }
</style>
