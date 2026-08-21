<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'

defineOptions({ name: 'DrawingPropertiesTab' })

const demoStore = useDemoStore()
const drawing = demoStore.currentDrawing
const feedIcons: Record<string, string> = { view: 'eye', edit: 'pencil', branch: 'git-branch', borrow: 'share-2', check: 'check-circle-2', back: 'undo-2' }
</script>

<template>
  <div class="props-section card">
    <div class="card-title"><DemoIcon name="pencil" :size="16" />签署信息</div>
    <div class="props-content"><div class="empty compact-empty"><DemoIcon name="pencil" :size="34" /><div class="t">暂无签署信息</div></div></div>
  </div>
  <div class="props-section card">
    <div class="card-title"><DemoIcon name="info" :size="16" />图纸属性</div>
    <div class="props-content"><div class="kv-grid"><div class="kv"><div class="k">图号</div><div class="v mono">{{ drawing?.no }}</div></div><div class="kv"><div class="k">名称</div><div class="v">{{ drawing?.name }}</div></div><div class="kv"><div class="k">类型</div><div class="v">{{ drawing?.kind }}</div></div><div class="kv"><div class="k">零件材料</div><div class="v">{{ drawing?.material }}</div></div><div class="kv"><div class="k">厂商</div><div class="v">{{ drawing?.vendor }}</div></div><div class="kv"><div class="k">所属项目</div><div class="v">{{ drawing?.project }}</div></div><div class="kv"><div class="k">当前版本</div><div class="v mono">{{ drawing?.ver }}</div></div><div class="kv"><div class="k">图幅 / 比例</div><div class="v mono">待获取</div></div><div class="kv"><div class="k">文件哈希</div><div class="v mono hash">待获取</div></div><div class="kv"><div class="k">状态</div><div class="v"><span class="tag" :class="drawing?.status ? 'info' : 'mute'">{{ drawing?.status ?? '待获取' }}</span></div></div></div></div>
  </div>
  <div class="props-section card">
    <div class="card-title"><DemoIcon name="activity" :size="16" />操作记录<span class="hint">谁查看 · 谁修改 · 谁分叉 · 谁借用</span></div>
    <div class="feed"><div v-for="item in demoStore.logs" :key="`${item.user}-${item.time}`" class="feed-item"><div class="feed-ic" :class="item.act"><DemoIcon :name="feedIcons[item.act] ?? 'activity'" :size="14" /></div><div class="feed-txt"><b>{{ item.user }}</b> <span v-html="item.txt"></span></div><div class="feed-time">{{ item.time }}</div></div><div v-if="!demoStore.logs.length" class="empty"><DemoIcon name="activity" :size="34" /><div class="t">暂无操作记录</div></div></div>
  </div>
</template>

<style scoped>
.props-section { margin-bottom: 16px; }
.props-content { padding: 14px 20px 18px; }
.signature-list { display: grid; grid-template-columns: repeat(5, 1fr); gap: 13px 22px; }
.sign-date { margin-top: 4px; }
.hash { font-size: 11px; }
@media (max-width: 760px) { .signature-list { grid-template-columns: repeat(2, 1fr); } }
</style>
