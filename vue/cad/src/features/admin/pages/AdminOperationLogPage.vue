<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import AdminTabs from '../components/AdminTabs.vue'
import { useDomainStore } from '@/stores/domain.store'
import { ACTIVITY_LABELS } from '@/types/domain.types'

defineOptions({ name: 'AdminOperationLogPage' })

const domainStore = useDomainStore()
const filter = ref('')
const options = Object.entries(ACTIVITY_LABELS)
const rows = computed(() => domainStore.logs.filter((item) => !filter.value || item.act === filter.value))
</script>

<template>
  <div class="page admin-page"><div class="section-head"><h3>后台管理</h3><span class="lib-count">管理员账号同时继承普通用户全部功能</span></div><AdminTabs active="logs" /><div class="section-head log-section-head"><h3 class="small-heading">图纸操作日志</h3><select v-model="filter" class="inp log-filter"><option value="">全部操作</option><option v-for="[code, label] in options" :key="code" :value="code">{{ label }}</option></select></div><div class="card"><table class="tbl"><thead><tr><th>时间</th><th>操作人</th><th>操作</th><th>图纸</th><th>结果</th></tr></thead><tbody><tr v-for="item in rows" :key="item.id"><td class="num updated">{{ item.time }}</td><td class="operator">{{ item.user }}</td><td><span class="tag info">{{ ACTIVITY_LABELS[item.act] }}</span></td><td class="num link">{{ item.drawingNo }}</td><td class="source">{{ item.result === 'success' ? '成功' : '失败' }}</td></tr><tr v-if="!rows.length"><td colspan="5"><div class="empty"><DemoIcon name="scroll-text" :size="34" /><div class="t">暂无操作日志</div></div></td></tr></tbody></table></div></div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 6px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.log-section-head { margin-top: 4px; }
.small-heading { font-size: 14px !important; }
.log-filter { width: 150px; height: 34px; margin-left: auto; }
.operator { font-weight: 500; }
.source { color: var(--text-3); font-size: 11.5px; }
</style>
