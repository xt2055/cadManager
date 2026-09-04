<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useAuditStore } from '@/stores/audit.store'
import { ACTIVITY_LABELS } from '@/types/domain.types'

defineOptions({ name: 'OperationLogPage' })

const auditStore = useAuditStore()
const filterAct = ref('')
const filterDrawing = ref('')
const filterUser = ref('')
const keyword = ref('')
const options = ['', ...Object.values(ACTIVITY_LABELS)]

const drawingOptions = computed(() => Array.from(new Set(auditStore.drawingLogs.list.map((item) => item.drawingNo).filter(Boolean))).sort())
const userOptions = computed(() => Array.from(new Set(auditStore.drawingLogs.list.map((item) => item.user).filter(Boolean))).sort())

const rows = computed(() =>
  auditStore.drawingLogs.list.filter((item) => {
    if (filterAct.value && ACTIVITY_LABELS[item.act] !== filterAct.value) return false
    if (filterDrawing.value && item.drawingNo !== filterDrawing.value) return false
    if (filterUser.value && item.user !== filterUser.value) return false
    const kw = keyword.value.trim().toLowerCase()
    if (kw) {
      const haystack = `${item.txt || ''} ${item.drawingNo || ''} ${item.drawingName || ''} ${item.user || ''}`.toLowerCase()
      if (!haystack.includes(kw)) return false
    }
    return true
  }),
)

onMounted(() => { void auditStore.loadDrawing({ page: 1, pageSize: 200 }).catch(() => undefined) })

function resetFilters() {
  filterAct.value = ''
  filterDrawing.value = ''
  filterUser.value = ''
  keyword.value = ''
}

const colors: Record<string, string> = { 查看图纸: 'info', 新建图纸: 'ok', 修改图纸: 'warn', 创建分支: 'info', 上传文件: 'warn', 下载文件: 'plain', 删除文件: 'danger', 审核操作: 'info', '解析 EXB': 'plain' }
</script>

<template>
  <div class="page log-page">
    <div class="section-head">
      <h3>操作记录</h3>
      <span class="lib-count">只读 · 追加式图纸操作记录 · 共 {{ rows.length }} 条</span>
    </div>

    <div class="card log-filter-bar">
      <label class="filter-item">
        <span>图纸</span>
        <select v-model="filterDrawing" class="inp">
          <option value="">全部图纸</option>
          <option v-for="no in drawingOptions" :key="no" :value="no">{{ no }}</option>
        </select>
      </label>
      <label class="filter-item">
        <span>操作人</span>
        <select v-model="filterUser" class="inp">
          <option value="">全部人员</option>
          <option v-for="user in userOptions" :key="user" :value="user">{{ user }}</option>
        </select>
      </label>
      <label class="filter-item">
        <span>操作类型</span>
        <select v-model="filterAct" class="inp">
          <option v-for="option in options" :key="option" :value="option">{{ option || '全部操作' }}</option>
        </select>
      </label>
      <label class="filter-item grow">
        <span>关键字</span>
        <input v-model="keyword" class="inp" type="text" placeholder="搜索操作内容 / 图号 / 人员" />
      </label>
      <button class="btn sm" type="button" @click="resetFilters">
        <DemoIcon name="rotate-ccw" :size="13" />重置
      </button>
    </div>

    <div class="card">
      <table class="tbl">
        <thead>
          <tr><th>时间</th><th>操作人</th><th>操作</th><th>图纸</th><th>结果</th></tr>
        </thead>
        <tbody>
          <tr v-for="item in rows" :key="item.id">
            <td class="num updated">{{ item.time }}</td>
            <td class="operator">{{ item.user }}</td>
            <td><span class="tag" :class="colors[ACTIVITY_LABELS[item.act]] ?? 'mute'">{{ ACTIVITY_LABELS[item.act] }}</span></td>
            <td class="num link">{{ item.drawingNo }}</td>
            <td class="source">{{ item.result === 'success' ? '成功' : '失败' }}</td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="5"><div class="empty"><DemoIcon name="scroll-text" :size="34" /><div class="t">暂无符合条件的操作记录</div></div></td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 13px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.log-filter-bar { display: flex; align-items: flex-end; gap: 12px; flex-wrap: wrap; padding: 12px 14px; margin-bottom: 13px; }
.filter-item { display: flex; flex-direction: column; gap: 4px; min-width: 150px; }
.filter-item.grow { flex: 1; min-width: 200px; }
.filter-item span { color: var(--text-3); font-size: 11px; }
.log-filter-bar .btn { height: 34px; }
.operator { font-weight: 500; }
.source { color: var(--text-3); font-size: 11.5px; }
</style>
