<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import AdminTabs from '../components/AdminTabs.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'ReviewFlowManagementPage' })

const demoStore = useDemoStore()
const uiStore = useUiStore()
</script>

<template>
  <div class="page admin-page">
    <div class="section-head"><h3>后台管理</h3><span class="lib-count">管理员账号同时继承普通用户全部功能</span><button class="btn primary admin-action" type="button" @click="uiStore.toast('请先配置流程节点与审核人员', 'info')"><DemoIcon name="plus" :size="14" />新增流程</button></div>
    <AdminTabs active="flows" />
    <div v-for="(flow, index) in demoStore.flows" :key="flow.name" class="card flow-card">
      <div class="flow-head"><DemoIcon name="workflow" :size="16" /><b>{{ flow.name }}</b><span class="tag" :class="flow.on ? 'ok' : 'mute'">{{ flow.on ? '启用中' : '已停用' }}</span><span class="tag info">无序并行</span><div class="flow-actions"><button class="btn sm" type="button" @click="uiStore.openModal('edit-flow', '编辑审核流程')"><DemoIcon name="pencil" :size="14" />编辑节点</button><button class="btn sm" type="button" @click="demoStore.toggleFlow(index)">{{ flow.on ? '停用' : '启用' }}</button><button class="btn sm danger" type="button" @click="uiStore.toast('该流程已有审核记录 · 已转为「停用归档」，不做物理删除', 'warn')"><DemoIcon name="trash-2" :size="14" />删除</button></div></div>
      <div class="flow-nodes"><span class="tag plain">{{ flow.nodes }} 个审核节点</span></div><div class="flow-desc">{{ flow.desc }}</div>
    </div>
    <div v-if="!demoStore.flows.length" class="card empty"><DemoIcon name="workflow" :size="34" /><div class="t">暂无审核流程</div></div>
    <div class="note"><DemoIcon name="shield-check" :size="14" /><div>已产生审核记录的流程执行「删除」时自动转为<b>停用归档</b>，历史审核记录始终可追溯，永不物理删除。</div></div>
  </div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 6px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.admin-action { margin-left: auto; }
.flow-card { margin-bottom: 13px; padding: 18px 20px; }
.flow-head { display: flex; align-items: center; gap: 11px; margin-bottom: 11px; flex-wrap: wrap; }
.flow-head > svg { color: var(--accent); }
.flow-actions { display: flex; gap: 7px; margin-left: auto; }
.flow-nodes { display: flex; align-items: center; gap: 9px; flex-wrap: wrap; }
.flow-desc { margin-top: 10px; color: var(--text-3); font-size: 11.5px; }
@media (max-width: 760px) { .flow-actions { width: 100%; margin-left: 0; } }
</style>
