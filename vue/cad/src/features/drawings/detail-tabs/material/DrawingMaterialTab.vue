<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingMaterialTab' })

const demoStore = useDemoStore()
const uiStore = useUiStore()
</script>

<template>
  <div class="card bom-card">
    <div class="card-title">
      <DemoIcon name="file-spreadsheet" :size="16" />
       备料表
      <span class="hint">随图纸版本绑定 · 支持历史版本</span>
      <span class="bom-actions">
        <button class="btn sm" type="button" @click="uiStore.toast('打开备料表在线编辑器（Demo）', 'info')">
          <DemoIcon name="pencil" :size="14" />编辑
        </button>
        <button class="btn sm" type="button" @click="uiStore.toast('备料表已导出为 Excel（绑定当前图纸版本）')">
          <DemoIcon name="download" :size="14" />导出 Excel
        </button>
      </span>
    </div>

    <div class="table-pad">
      <table class="tbl">
        <thead>
          <tr>
            <th>序号</th>
            <th>图号 / 标准号</th>
            <th>名称</th>
            <th>规格 / 材质</th>
            <th>数量</th>
            <th>单重</th>
            <th>总重</th>
            <th>备注</th>
          </tr>
        </thead>
        <tbody>
           <tr v-for="item in demoStore.bom" :key="item.id">
            <td class="num">{{ item.no }}</td>
            <td class="num">{{ item.id }}</td>
            <td class="material-name">{{ item.name }}</td>
            <td class="spec">{{ item.spec }}</td>
            <td class="num">{{ item.qty }}</td>
            <td class="num">{{ item.weight.toFixed(2) }}</td>
            <td class="num">{{ (item.weight * item.qty).toFixed(2) }}</td>
             <td class="remark">{{ item.remark }}</td>
           </tr>
           <tr v-if="!demoStore.bom.length">
             <td colspan="8">
               <div class="empty"><DemoIcon name="file-spreadsheet" :size="34" /><div class="t">暂无备料数据</div></div>
             </td>
           </tr>
        </tbody>
        <tfoot>
          <tr>
            <td colspan="6">合计（kg）</td>
            <td class="num">{{ demoStore.bom.reduce((total, item) => total + item.weight * item.qty, 0).toFixed(2) }}</td>
            <td></td>
          </tr>
        </tfoot>
      </table>
    </div>
  </div>
</template>

<style scoped>
.bom-card { overflow: visible; }
.bom-actions { display: flex; gap: 8px; margin-left: auto; }
.table-pad { padding: 4px 8px 10px; overflow-x: auto; }
.tbl { min-width: 900px; }
.material-name { font-weight: 500; }
.spec { color: var(--text-2); }
.remark { color: var(--text-3); }
@media (max-width: 760px) { .bom-actions { width: 100%; margin-left: 0; } }
</style>
