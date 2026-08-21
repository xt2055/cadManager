<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'

defineOptions({ name: 'OperationLogPage' })

const demoStore = useDemoStore()
const filter = ref('')
const options = ['', '查看图纸', '上传版本', '发起审核', '借用图纸', '版本回退', '禁用分支']
const rows = computed(() => demoStore.adminLogs.filter((item) => !filter.value || item.act === filter.value))
const colors: Record<string, string> = { 查看图纸: 'info', 上传版本: 'warn', 发起审核: 'info', 借用图纸: 'plain', 版本回退: 'danger', 禁用分支: 'danger' }
</script>

<template>
  <div class="page log-page"><div class="section-head"><h3>操作记录</h3><span class="lib-count">只读 · 不可篡改 · 追加式日志</span><select v-model="filter" class="inp log-filter"><option v-for="option in options" :key="option" :value="option">{{ option || '全部操作' }}</option></select></div><div class="card"><table class="tbl"><thead><tr><th>时间</th><th>操作人</th><th>操作</th><th>对象</th><th>来源</th></tr></thead><tbody><tr v-for="item in rows" :key="`${item.time}-${item.obj}`"><td class="num updated">{{ item.time }}</td><td class="operator">{{ item.user }}</td><td><span class="tag" :class="colors[item.act] ?? 'mute'">{{ item.act }}</span></td><td class="num link">{{ item.obj }}</td><td class="source">系统日志</td></tr><tr v-if="!rows.length"><td colspan="5"><div class="empty"><DemoIcon name="scroll-text" :size="34" /><div class="t">暂无操作记录</div></div></td></tr></tbody></table></div></div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 13px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.log-filter { width: 150px; height: 34px; margin-left: auto; }
.operator { font-weight: 500; }
.source { color: var(--text-3); font-size: 11.5px; }
</style>
