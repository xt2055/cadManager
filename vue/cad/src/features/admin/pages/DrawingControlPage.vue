<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import AdminTabs from '../components/AdminTabs.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingControlPage' })

const demoStore = useDemoStore()
const uiStore = useUiStore()

async function restoreHidden(index: number) {
  try {
    await demoStore.restoreHidden(index)
  } catch (error) {
    console.error('保存恢复结果失败', error)
    uiStore.toast('恢复结果保存失败，请稍后重试', 'warn')
  }
}
</script>

<template>
  <div class="page admin-page">
    <div class="section-head"><h3>后台管理</h3><span class="lib-count">管理员账号同时继承普通用户全部功能</span></div>
    <AdminTabs active="mgmt" />
    <div class="card control-card"><div class="card-title"><DemoIcon name="folder-lock" :size="16" />隐藏 / 禁用对象<span class="hint">全部可逆 · 历史不丢</span></div><div class="table-pad"><table class="tbl"><thead><tr><th>编号</th><th>名称</th><th>操作</th><th>操作人</th><th>日期</th><th>恢复</th></tr></thead><tbody><tr v-for="(item, index) in demoStore.hiddenList" :key="item.no"><td class="num">{{ item.no }}</td><td class="operator">{{ item.name }}</td><td><span class="tag" :class="item.op === '隐藏' ? 'danger' : 'warn'">{{ item.op }}</span></td><td>{{ item.by }}</td><td class="num updated">{{ item.date }}</td><td><button class="btn sm primary" type="button" @click="restoreHidden(index)"><DemoIcon name="rotate-ccw" :size="14" />恢复</button></td></tr><tr v-if="!demoStore.hiddenList.length"><td colspan="6"><div class="empty"><DemoIcon name="check-circle-2" :size="34" /><div class="t">没有隐藏或禁用的对象</div></div></td></tr></tbody></table></div></div>
    <div class="card revert-card"><div class="card-title"><DemoIcon name="undo-2" :size="16" />版本回退</div><div class="card-pad revert-copy"><div>可将任意图纸的当前版本回退到历史版本。回退<b>不删除、不覆盖</b>任何版本——系统以目标版本内容生成新版本，全程留痕，可再次回退撤销。</div><div class="revert-action"><span>请先选择图纸</span><button class="btn sm primary" type="button" :disabled="!demoStore.drawings.length" @click="uiStore.openModal('revert', '版本回退')"><DemoIcon name="rotate-ccw" :size="14" />回退到历史版本</button></div></div></div>
  </div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 6px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.control-card { margin-bottom: 14px; overflow: visible; }
.table-pad { padding: 4px 8px 10px; overflow-x: auto; }
.revert-copy { padding-top: 12px; color: var(--text-2); font-size: 12.5px; line-height: 1.8; }
.revert-action { display: flex; align-items: center; gap: 10px; margin-top: 12px; color: var(--text-3); font-size: 12px; }
@media (max-width: 760px) { .revert-action { align-items: flex-start; flex-wrap: wrap; } }
</style>
