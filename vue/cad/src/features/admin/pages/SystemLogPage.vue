<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import type { SystemLogFile, SystemLogLine } from '@/modules/admin'
import { useUiStore } from '@/stores/ui.store'
import { useAdminStore } from '@/stores/admin.store'

defineOptions({ name: 'SystemLogPage' })

const uiStore = useUiStore()
const adminStore = useAdminStore()
const lines = computed(() => adminStore.systemLogs)
const files = computed(() => adminStore.systemLogFiles)
const keyword = ref('')
const level = ref<'all' | 'INFO' | 'WARN' | 'ERROR'>('all')
const autoRefresh = ref(true)
const loading = ref(false)
let timer: ReturnType<typeof setInterval> | null = null

const levelClass = (value: string) => `log-line__level log-line__level--${value.toLowerCase()}`

const filtered = computed(() =>
  lines.value.filter((item) => level.value === 'all' || item.level === (level.value as string)))

const latestFile = computed(() => files.value[0])

function formatSize(size: number): string {
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  if (size >= 1024) return `${(size / 1024).toFixed(0)} KB`
  return `${size} B`
}

async function load(showError = false) {
  loading.value = true
  try {
    await Promise.all([
      adminStore.loadSystemLogs(500, keyword.value.trim()),
      adminStore.loadSystemLogFiles(),
    ])
  } catch (error) {
    if (showError) uiStore.toast(error instanceof Error ? error.message : '读取日志失败', 'warn')
  } finally {
    loading.value = false
  }
}

function toggleAuto() {
  if (timer) clearInterval(timer)
  timer = null
  if (autoRefresh.value) timer = setInterval(() => load(false), 2000)
}

onMounted(() => {
  load(true)
  toggleAuto()
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="system-log">
    <header class="page-head">
      <div>
        <h1>系统日志</h1>
        <p>查看服务运行日志（内存缓冲最近 500 条 + 历史日志文件下载）。</p>
      </div>
      <div class="page-head__actions">
        <label class="switch">
          <input v-model="autoRefresh" type="checkbox" @change="toggleAuto" />
          <span>2 秒自动刷新</span>
        </label>
        <a class="ghost-btn" :href="latestFile ? adminStore.systemLogDownloadUrl(latestFile.name) : undefined" :class="{ disabled: !latestFile }">
          <DemoIcon name="download" :size="14" /><span>下载最新日志</span>
        </a>
        <button class="ghost-btn" type="button" @click="load(true)"><DemoIcon name="refresh-cw" :size="14" /><span>刷新</span></button>
      </div>
    </header>

    <section class="card filter-bar">
      <input v-model="keyword" class="filter-input" placeholder="按关键字过滤（发送到服务端）" @keyup.enter="load(true)" />
      <div class="level-tabs">
        <button v-for="item in (['all', 'INFO', 'WARN', 'ERROR'] as const)" :key="item" type="button" class="level-tab" :class="{ active: level === item }" @click="level = item">
          {{ item === 'all' ? '全部' : item }}
        </button>
      </div>
      <span class="filter-count">{{ filtered.length }} 条</span>
    </section>

    <section class="card log-panel">
      <div v-if="loading && !lines.length" class="log-empty">加载中…</div>
      <div v-else-if="!filtered.length" class="log-empty">暂无日志</div>
      <template v-else>
        <div v-for="(item, index) in filtered" :key="index" class="log-line">
          <span class="log-line__time">{{ item.time }}</span>
          <span :class="levelClass(item.level)">{{ item.level }}</span>
          <span class="log-line__text">{{ item.message }}</span>
        </div>
      </template>
    </section>

    <section class="card">
      <h2 class="section-title">历史日志文件</h2>
      <table v-if="files.length" class="file-table">
        <thead>
          <tr><th>文件名</th><th>大小</th><th>最后修改</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="file in files" :key="file.name">
            <td class="mono">{{ file.name }}</td>
            <td>{{ formatSize(file.size) }}</td>
            <td>{{ file.modTime }}</td>
            <td><a class="ghost-btn" :href="adminStore.systemLogDownloadUrl(file.name)"><DemoIcon name="download" :size="13" /><span>下载</span></a></td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty-tip">暂无历史日志文件</p>
    </section>
  </div>
</template>

<style scoped>
.system-log { display: flex; flex-direction: column; gap: 16px; padding: 22px 26px; }
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
.page-head h1 { font-size: 18px; }
.page-head p { margin-top: 4px; color: var(--text-3); font-size: 12px; }
.page-head__actions { display: flex; align-items: center; gap: 10px; }
.switch { display: inline-flex; align-items: center; gap: 6px; color: var(--text-2); font-size: 12px; cursor: pointer; }
.switch input { accent-color: var(--accent); }
.ghost-btn { display: inline-flex; align-items: center; gap: 6px; padding: 7px 12px; border: 1px solid var(--line); border-radius: 8px; color: var(--text-2); font-size: 12px; background: transparent; text-decoration: none; cursor: pointer; }
.ghost-btn:hover { color: var(--accent); border-color: var(--accent); }
.ghost-btn.disabled { opacity: .45; pointer-events: none; }
.filter-bar { display: flex; align-items: center; gap: 12px; padding: 12px 14px; }
.filter-input { flex: 1; min-width: 200px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); color: var(--text-1); font-size: 12.5px; outline: none; }
.filter-input:focus { border-color: var(--accent); }
.level-tabs { display: flex; gap: 4px; }
.level-tab { padding: 6px 12px; border: 1px solid var(--line); border-radius: 7px; background: transparent; color: var(--text-3); font-size: 11.5px; cursor: pointer; }
.level-tab.active { color: var(--accent); border-color: var(--accent); background: var(--active); }
.filter-count { color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.log-panel { padding: 10px 14px; max-height: 46vh; overflow: auto; font-family: 'JetBrains Mono', monospace; }
.log-line { display: flex; gap: 10px; padding: 4px 0; border-bottom: 1px dashed var(--line); font-size: 12px; line-height: 1.55; }
.log-line:last-child { border-bottom: none; }
.log-line__time { flex: none; color: var(--text-3); }
.log-line__level { flex: none; width: 46px; font-weight: 600; }
.log-line__level--info { color: var(--accent); }
.log-line__level--warn { color: var(--warning, #f0a020); }
.log-line__level--error { color: var(--danger); }
.log-line__text { color: var(--text-2); word-break: break-all; white-space: pre-wrap; }
.log-empty { padding: 30px 0; color: var(--text-3); text-align: center; font-size: 12px; }
.section-title { margin-bottom: 12px; font-size: 14px; }
.file-table { width: 100%; border-collapse: collapse; font-size: 12.5px; }
.file-table th { padding: 8px 10px; color: var(--text-3); text-align: left; font-weight: 500; font-size: 11.5px; border-bottom: 1px solid var(--line); }
.file-table td { padding: 8px 10px; border-bottom: 1px solid var(--line); color: var(--text-2); }
.empty-tip { color: var(--text-3); font-size: 12px; }
.mono { font-family: 'JetBrains Mono', monospace; }
</style>
