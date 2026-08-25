<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { ACTIVITY_LABELS } from '@/types/domain.types'

defineOptions({ name: 'OperationLogPage' })

const domainStore = useDomainStore()
const filter = ref('')
const options = ['', ...Object.values(ACTIVITY_LABELS)]
const rows = computed(() => domainStore.logs.filter((item) => !filter.value || ACTIVITY_LABELS[item.act] === filter.value))
const colors: Record<string, string> = { 查看图纸: 'info', 新建图纸: 'ok', 修改图纸: 'warn', 创建分支: 'info', 上传文件: 'warn', 下载文件: 'plain', 删除文件: 'danger', 审核操作: 'info', '解析 EXB': 'plain' }
</script>

<template>
  <div class="page log-page"><div class="section-head"><h3>操作记录</h3><span class="lib-count">只读 · 追加式图纸操作记录</span><select v-model="filter" class="inp log-filter"><option v-for="option in options" :key="option" :value="option">{{ option || '全部操作' }}</option></select></div><div class="card"><table class="tbl"><thead><tr><th>时间</th><th>操作人</th><th>操作</th><th>图纸</th><th>结果</th></tr></thead><tbody><tr v-for="item in rows" :key="item.id"><td class="num updated">{{ item.time }}</td><td class="operator">{{ item.user }}</td><td><span class="tag" :class="colors[ACTIVITY_LABELS[item.act]] ?? 'mute'">{{ ACTIVITY_LABELS[item.act] }}</span></td><td class="num link">{{ item.drawingNo }}</td><td class="source">{{ item.result === 'success' ? '成功' : '失败' }}</td></tr><tr v-if="!rows.length"><td colspan="5"><div class="empty"><DemoIcon name="scroll-text" :size="34" /><div class="t">暂无操作记录</div></div></td></tr></tbody></table></div></div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 13px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.log-filter { width: 150px; height: 34px; margin-left: auto; }
.operator { font-weight: 500; }
.source { color: var(--text-3); font-size: 11.5px; }
</style>
