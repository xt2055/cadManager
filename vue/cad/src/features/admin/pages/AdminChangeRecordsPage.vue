<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { changeRequestService, CHANGE_STATUS_LABELS, type ChangeRequest } from '@/services/change-request.service'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AdminChangeRecordsPage' })

const uiStore = useUiStore()
const loading = ref(false)
const items = ref<ChangeRequest[]>([])
const keyword = ref('')
const expandedId = ref('')

const rows = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return items.value.filter((item) => !q || [item.requestNo, item.drawingNo, item.title, item.applicantName, item.executorName, item.scope].some((value) => value?.toLowerCase().includes(q)))
})

// 创建工单时会默认把执行人写成申请人，待审批阶段应视为尚未指派。
function designated(item: ChangeRequest): string {
  if (item.status === 'pending_approval' && (!item.executorId || item.executorId === item.applicantId)) return ''
  return item.executorName || ''
}

async function load() {
  loading.value = true
  try {
    items.value = await changeRequestService.list()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '变更记录读取失败', 'warn')
  } finally {
    loading.value = false
  }
}

async function toggle(item: ChangeRequest) {
  if (expandedId.value === item.id) {
    expandedId.value = ''
    return
  }
  expandedId.value = item.id
  if (!item.actions) {
    try {
      const detail = await changeRequestService.get(item.id)
      const index = items.value.findIndex((entry) => entry.id === item.id)
      if (index >= 0) items.value[index] = detail
    } catch {
      /* 展开失败不影响列表 */
    }
  }
}

onMounted(() => { void load() })
</script>

<template>
  <div class="page admin-page">
    <div class="section-head">
      <h3>变更记录</h3>
      <span class="lib-count">全部变更工单及操作留痕</span>
      <input v-model="keyword" class="inp" placeholder="搜索图号、工单号、申请人" />
    </div>

    <div class="card">
      <p v-if="loading" class="hint">加载中…</p>
      <table v-else class="tbl">
        <thead>
          <tr>
            <th>工单号</th>
            <th>图号</th>
            <th>状态</th>
            <th>申请人</th>
            <th>指定人</th>
            <th>范围</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="item in rows" :key="item.id">
            <tr class="clickable" @click="toggle(item)">
              <td class="num">{{ item.requestNo }}</td>
              <td>{{ item.drawingNo }}</td>
              <td><span class="tag">{{ CHANGE_STATUS_LABELS[item.status] }}</span></td>
              <td>{{ item.applicantName }}</td>
              <td>{{ designated(item) || '—' }}</td>
              <td class="scope">{{ item.scope }}</td>
              <td class="num">{{ item.createdAt.slice(0, 16).replace('T', ' ') }}</td>
            </tr>
            <tr v-if="expandedId === item.id">
              <td colspan="7">
                <div class="detail">
                  <p><b>原因</b> {{ item.reason }}</p>
                  <p v-for="act in item.actions" :key="act.id"><b>{{ act.actorName || '系统' }}</b> · {{ act.opinion }}</p>
                  <p v-for="(diff, index) in item.diffs" :key="index">{{ diff.field }}：{{ diff.oldValue || '（空）' }} → {{ diff.newValue || '（空）' }}</p>
                </div>
              </td>
            </tr>
          </template>
          <tr v-if="!rows.length">
            <td colspan="6">
              <div class="empty"><DemoIcon name="history" :size="34" /><div class="t">暂无变更记录</div></div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 16px; }
.section-head h3 { font-family: var(--font-display); font-size: 16px; font-weight: 800; }
.lib-count { color: var(--text-3); font-size: 11.5px; margin-right: auto; }
.inp { width: 240px; }
.clickable { cursor: pointer; }
.scope { max-width: 280px; white-space: pre-wrap; color: var(--text-2); font-size: 12px; }
.detail { padding: 8px 4px; color: var(--text-2); font-size: 12px; }
.hint { padding: 16px; color: var(--text-3); }
</style>
