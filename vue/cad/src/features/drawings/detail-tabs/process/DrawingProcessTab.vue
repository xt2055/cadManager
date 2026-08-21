<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingProcessTab' })

const demoStore = useDemoStore()
const uiStore = useUiStore()
</script>

<template>
  <div class="craft-grid">
    <div v-for="file in demoStore.crafts" :key="file.name" class="card card-pad craft-card">
      <div class="feed-ic file-icon">
        <DemoIcon name="file-text" :size="20" />
      </div>
      <div class="craft-info">
        <b>{{ file.name }}</b>
        <div>{{ file.op }} · {{ file.ver }} · {{ file.by }} · {{ file.date }} · {{ file.size }}</div>
      </div>
      <div class="craft-actions">
          <button class="btn sm" type="button" @click="uiStore.toast('工艺文件预览功能待接入', 'info')">
          <DemoIcon name="eye" :size="14" />预览
        </button>
        <button class="btn sm" type="button" @click="uiStore.toast('该工艺文件共 4 个历史版本，均可查看 / 对比 / 回退', 'info')">
          <DemoIcon name="history" :size="14" />历史版本
        </button>
        <button class="btn sm" type="button" @click="uiStore.toast('工艺文件已下载 · 已计入操作日志')">
          <DemoIcon name="download" :size="14" />下载
        </button>
      </div>
    </div>
    <div v-if="!demoStore.crafts.length" class="card empty">
      <DemoIcon name="file-text" :size="34" />
      <div class="t">暂无工艺文件</div>
    </div>
  </div>
</template>

<style scoped>
.craft-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(430px, 1fr)); gap: 13px; }
.craft-card { display: flex; align-items: center; gap: 15px; }
.file-icon { width: 42px; height: 42px; border-radius: 12px; }
.file-icon svg { color: var(--accent); }
.craft-info { flex: 1; min-width: 0; }
.craft-info b { display: block; overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.craft-info div { margin-top: 4px; color: var(--text-3); font-size: 11px; }
.craft-actions { display: flex; flex: none; gap: 7px; }
@media (max-width: 760px) { .craft-grid { grid-template-columns: 1fr; } .craft-card { align-items: flex-start; flex-wrap: wrap; } .craft-actions { width: 100%; margin-left: 57px; } }
</style>
