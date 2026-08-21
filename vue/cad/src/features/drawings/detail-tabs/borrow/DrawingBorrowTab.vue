<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingBorrowTab' })

const demoStore = useDemoStore()
const uiStore = useUiStore()
</script>

<template>
  <div class="borrow-page">
    <div class="note borrow-note"><DemoIcon name="share-2" :size="14" /><div><b>借用同步原则：</b>修改借用零件图时必须选择同步策略 ——<div class="demo-sync"><span class="tag plain">○ 仅更新当前项目副本（默认，不影响原图）</span><span class="tag warn">○ 同步至原借用零件图（影响引用项目，需重新审核）</span></div></div></div>
    <div class="card borrow-card">
      <div class="card-title"><DemoIcon name="share-2" :size="16" />借用关系<span class="hint">谁借用 · 借用哪个版本 · 是否同步</span></div>
       <div class="table-pad"><table class="tbl"><thead><tr><th>方向</th><th>项目</th><th>零件 / 版本</th><th>借用人</th><th>日期</th><th>同步策略</th><th>状态</th><th>操作</th></tr></thead><tbody><tr v-for="record in demoStore.borrows" :key="`${record.project}-${record.part}`"><td><span class="tag" :class="record.dir === 'out' ? 'info' : 'plain'">{{ record.dir === 'out' ? '借出' : '借入' }}</span></td><td>{{ record.project }}</td><td class="num">{{ record.part }}</td><td>{{ record.user }}</td><td class="num">{{ record.date }}</td><td class="sync-text">{{ record.sync }}</td><td><span class="tag" :class="record.status === '使用中' ? 'ok' : 'mute'">{{ record.status }}</span></td><td><button class="btn sm" type="button" @click="uiStore.openModal('sync-original', '同步至原借用零件图')"><DemoIcon name="refresh-cw" :size="14" />同步</button></td></tr><tr v-if="!demoStore.borrows.length"><td colspan="8"><div class="empty"><DemoIcon name="share-2" :size="34" /><div class="t">暂无借用关系</div></div></td></tr></tbody></table></div>
    </div>
  </div>
</template>

<style scoped>
.borrow-note { margin-bottom: 14px; }
.demo-sync { display: flex; gap: 9px; align-items: flex-start; margin-top: 11px; flex-wrap: wrap; }
.borrow-card { overflow: visible; }
.table-pad { padding: 4px 8px 10px; overflow-x: auto; }
.sync-text { color: var(--text-2); font-size: 11.5px; }
.tbl { min-width: 940px; }
</style>
