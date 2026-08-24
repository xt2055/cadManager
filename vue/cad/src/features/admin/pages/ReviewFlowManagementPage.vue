<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import AdminTabs from '../components/AdminTabs.vue'
import { useUiStore } from '@/stores/ui.store'
import { reviewFlowService, type ReviewFlowDto } from '@/services/review-flow.service'

defineOptions({ name: 'ReviewFlowManagementPage' })

const uiStore = useUiStore()
const flows = ref<ReviewFlowDto[]>([])
const loading = ref(false)

async function loadFlows() {
  loading.value = true
  try {
    flows.value = await reviewFlowService.list()
  } catch (error) {
    console.error('读取审核流程失败', error)
    uiStore.toast(error instanceof Error ? error.message : '审核流程读取失败', 'warn')
  } finally {
    loading.value = false
  }
}

function editFlow(flow?: ReviewFlowDto) {
  uiStore.openModal('edit-flow', flow ? `编辑审核流程 · ${flow.name}` : '新增审核流程', flow ? { flowId: flow.id } : undefined)
}

async function toggleFlow(flow: ReviewFlowDto) {
  try {
    const updated = await reviewFlowService.toggle(flow.id, !flow.enabled)
    flow.enabled = updated.enabled
    uiStore.toast(flow.enabled ? '审核流程已启用' : '审核流程已停用')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '审核流程状态保存失败', 'warn')
  }
}

onMounted(loadFlows)
watch(() => uiStore.modal, (current, previous) => {
  if (previous && !current) loadFlows()
})
</script>

<template>
  <div class="page admin-page">
    <div class="section-head"><h3>后台管理</h3><span class="lib-count">审核流程模板由数据库统一管理</span><button class="btn primary admin-action" type="button" @click="editFlow()"><DemoIcon name="plus" :size="14" />新增流程</button></div>
    <AdminTabs active="flows" />
      <div v-for="flow in flows" :key="flow.id" class="card flow-card">
      <div class="flow-head"><DemoIcon name="workflow" :size="16" /><b>{{ flow.name }}</b><span class="tag" :class="flow.enabled ? 'ok' : 'mute'">{{ flow.enabled ? '启用中' : '已停用' }}</span><span class="tag info">{{ flow.nodes.length }} 个节点</span><div class="flow-actions"><button class="btn sm" type="button" @click="editFlow(flow)"><DemoIcon name="pencil" :size="14" />编辑节点</button><button class="btn sm" type="button" @click="toggleFlow(flow)">{{ flow.enabled ? '停用' : '启用' }}</button></div></div>
       <div class="flow-nodes"><span v-for="node in flow.nodes" :key="node.id || node.order" class="tag plain">{{ node.name }} · {{ node.candidateRole }}</span></div><div class="flow-desc">{{ flow.description || '未填写流程说明' }} · 创建人：{{ flow.createdBy || '待定' }}</div>
     </div>
      <div v-if="!loading && !flows.length" class="card empty"><DemoIcon name="workflow" :size="34" /><div class="t">暂无审核流程</div></div>
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
