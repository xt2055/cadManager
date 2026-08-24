<script setup lang="ts">
import { computed } from 'vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingVersionsTab' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

const currentNo = computed(() => domainStore.currentDrawing?.no || '')
const currentBranches = computed(() => {
  const no = currentNo.value
  if (!no) return domainStore.branches
  return domainStore.branches.filter((item) => item.from === no || item.name.includes(no))
})
</script>

<template>
  <div class="ver-grid">
    <div class="card">
      <div class="card-title"><DemoIcon name="history" :size="16" />版本时间线<span class="hint">所有版本永久保留 · 回退不删除任何版本</span></div>
      <div v-if="domainStore.versions.length" class="timeline">
        <div v-for="version in domainStore.versions" :key="version.v" class="tl-item" :class="{ cur: version.cur }">
          <div class="tl-dot"></div><div class="tl-head"><span class="v">{{ version.v }}</span><span v-if="version.cur" class="tag ok">当前版本</span></div><div class="tl-body">{{ version.note }}</div><div class="tl-meta">创建/维护：{{ version.by }} · {{ version.date }}</div>
        </div>
      </div>
      <div v-else class="empty"><DemoIcon name="history" :size="34" /><div class="t">暂无版本记录</div></div>
      </div>
    <div>
      <div class="card branch-panel">
        <div class="card-title"><DemoIcon name="git-branch" :size="16" />分叉 / 分支<span class="hint">源图与衍生图独立维护 · 可追溯分叉人</span></div>
        <div v-if="currentBranches.length" class="branch-list">
          <div v-for="branch in currentBranches" :key="branch.name" class="card branch-card" :class="{ disabled: branch.status === '已禁用' }">
            <div class="bh"><DemoIcon name="git-branch" :size="15" /><b>{{ branch.name }}</b><span class="tag" :class="branch.status === '使用中' ? 'ok' : 'mute'">{{ branch.status }}</span><button class="btn sm" type="button" @click="uiStore.toast(`「${branch.status === '已禁用' ? '恢复分支' : '禁用分支'}」已执行 · 分支历史完整保留，可随时恢复`, branch.status === '已禁用' ? 'ok' : 'warn')">{{ branch.status === '已禁用' ? '恢复分支' : '禁用' }}</button></div>
            <div class="bd">分叉自 <span class="mono">{{ branch.from }}</span> · 分叉者 <b>{{ branch.by }}</b> 创建于 {{ branch.date }} — {{ branch.desc }}</div>
          </div>
        </div>
        <div v-else class="empty"><DemoIcon name="git-branch" :size="34" /><div class="t">当前图纸暂无衍生分叉分支</div></div>
      </div>
      <div class="note version-note"><DemoIcon name="shield-check" :size="14" /><div>分叉操作会自动完整复制图纸元标签、层级结构和关联附件文件，生成独立草稿图号。</div></div>
    </div>
  </div>
</template>

<style scoped>
.ver-grid { display: grid; grid-template-columns: 1.2fr 1fr; gap: 14px; align-items: start; }
.timeline { padding: 6px 20px 14px; }
.tl-item { position: relative; padding: 0 0 22px 26px; }
.tl-item::before { position: absolute; top: 16px; bottom: -2px; left: 5.5px; width: 1.5px; background: var(--line-strong); content: ''; }
.tl-item:last-child::before { display: none; }
.tl-dot { position: absolute; top: 4px; left: 0; width: 12px; height: 12px; border: 2.5px solid var(--line-strong); border-radius: 50%; background: var(--panel); }
.tl-item.cur .tl-dot { border-color: var(--accent); background: var(--accent); }
html[data-skin='tech'] .tl-item.cur .tl-dot { box-shadow: 0 0 12px var(--glow); }
.tl-head { display: flex; align-items: center; gap: 10px; }
.tl-head .v { font-family: 'JetBrains Mono', monospace; font-size: 14px; font-weight: 700; }
.tl-body { margin-top: 5px; color: var(--text-2); font-size: 12px; line-height: 1.6; }
.tl-meta { margin-top: 4px; color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.branch-panel { margin-bottom: 14px; }
.branch-list { padding: 6px 14px 14px; }
.branch-card { margin-bottom: 11px; padding: 15px 17px; }
.branch-card:last-child { margin-bottom: 0; }
.branch-card.disabled { opacity: 0.62; }
.bh { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
.bh > svg { color: var(--accent); }
.bh b { font-family: 'JetBrains Mono', monospace; font-size: 13.5px; }
.bh .btn { margin-left: auto; }
.bd { color: var(--text-2); font-size: 12px; line-height: 1.65; }
.version-note { margin-top: 0; }
@media (max-width: 1180px) { .ver-grid { grid-template-columns: 1fr; } }
</style>
