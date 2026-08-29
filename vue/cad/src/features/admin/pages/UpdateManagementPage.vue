<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import {
  deleteUpdatePackage,
  fetchUpdateList,
  updatePackageDownloadUrl,
  uploadUpdatePackage,
  type UpdateRecord,
} from '@/services/admin.service'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'UpdateManagementPage' })

const uiStore = useUiStore()
const records = ref<UpdateRecord[]>([])
const loading = ref(false)
const uploading = ref(false)
const version = ref('')
const notes = ref('')
const mandatory = ref(false)
const file = ref<File | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

const latest = computed(() => {
  const enabled = records.value.filter((item) => item.enabled)
  if (!enabled.length) return null
  return enabled.reduce((best, item) => (compareVersion(item.version, best.version) > 0 ? item : best))
})

function compareVersion(left: string, right: string): number {
  const parse = (value: string) => value.replace(/^v/i, '').split(/[.+-]/).map((part) => Number.parseInt(part, 10) || 0)
  const a = parse(left)
  const b = parse(right)
  for (let index = 0; index < 3; index += 1) {
    if ((a[index] || 0) !== (b[index] || 0)) return (a[index] || 0) - (b[index] || 0)
  }
  return 0
}

function formatSize(size?: number): string {
  if (!size) return '—'
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  return `${(size / 1024).toFixed(0)} KB`
}

function pickFile() {
  fileInput.value?.click()
}

function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  file.value = input.files?.[0] ?? null
}

async function load(showError = false) {
  loading.value = true
  try {
    records.value = await fetchUpdateList()
  } catch (error) {
    if (showError) uiStore.toast(error instanceof Error ? error.message : '读取更新列表失败', 'warn')
  } finally {
    loading.value = false
  }
}

async function submit() {
  if (!file.value) { uiStore.toast('请选择安装包文件', 'warn'); return }
  if (!version.value.trim()) { uiStore.toast('请填写版本号', 'warn'); return }
  uploading.value = true
  try {
    await uploadUpdatePackage({ file: file.value, version: version.value.trim(), notes: notes.value.trim(), mandatory: mandatory.value })
    uiStore.toast(`版本 ${version.value.trim()} 已发布，客户端将提示更新`, 'ok')
    version.value = ''
    notes.value = ''
    mandatory.value = false
    file.value = null
    if (fileInput.value) fileInput.value.value = ''
    await load()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '发布失败', 'warn')
  } finally {
    uploading.value = false
  }
}

async function remove(record: UpdateRecord) {
  uiStore.confirm(`删除版本 ${record.version}`, '将同时删除已上传的安装包文件，客户端将不再收到该版本更新。', {
    danger: true,
    confirmText: '删除',
    onConfirm: async () => {
      try {
        await deleteUpdatePackage(record.id)
        uiStore.toast('已删除', 'ok')
        await load()
      } catch (error) {
        uiStore.toast(error instanceof Error ? error.message : '删除失败', 'warn')
      }
    },
  })
}

onMounted(() => load(true))
</script>

<template>
  <div class="update-manage">
    <header class="page-head">
      <div>
        <h1>更新管理</h1>
        <p>发布客户端安装包：上传新版本后，所有客户端将自动检查并提示更新。</p>
      </div>
      <button class="ghost-btn" type="button" @click="load(true)"><DemoIcon name="refresh-cw" :size="14" /><span>刷新</span></button>
    </header>

    <section class="card latest-card">
      <div class="latest-icon"><DemoIcon name="rocket" :size="22" /></div>
      <div class="latest-info">
        <div class="latest-label">当前最新版本</div>
        <div class="latest-version mono">{{ latest?.version || '尚未发布' }}</div>
        <div class="latest-meta">
          <template v-if="latest">{{ latest.publishedAt || latest.createdAt }} · {{ formatSize(latest.sizeBytes) }} · {{ latest.mandatory ? '强制更新' : '可选更新' }}</template>
          <template v-else>上传第一个版本后，客户端启动时会自动检查更新</template>
        </div>
      </div>
      <a v-if="latest" class="ghost-btn" :href="updatePackageDownloadUrl(latest)"><DemoIcon name="download" :size="14" /><span>下载安装包</span></a>
    </section>

    <section class="card upload-card">
      <h2 class="section-title">发布新版本</h2>
      <div class="upload-row">
        <button class="pick-btn" type="button" @click="pickFile">
          <DemoIcon name="upload" :size="16" />
          <span>{{ file ? file.name : '选择安装包（.exe，最大 1GB）' }}</span>
        </button>
        <input ref="fileInput" type="file" accept=".exe,.zip" hidden @change="onFileChange" />
        <input v-model="version" class="version-input mono" placeholder="版本号，如 0.2.0" />
      </div>
      <textarea v-model="notes" class="notes-input" rows="3" placeholder="更新说明（将展示在客户端更新提示中）"></textarea>
      <div class="upload-foot">
        <label class="check-line">
          <input v-model="mandatory" type="checkbox" />
          <span>标记为强制更新</span>
        </label>
        <button class="primary-btn" type="button" :disabled="uploading" @click="submit">
          <DemoIcon name="upload" :size="14" /><span>{{ uploading ? '发布中…' : '发布版本' }}</span>
        </button>
      </div>
    </section>

    <section class="card">
      <h2 class="section-title">历史版本</h2>
      <table v-if="records.length" class="record-table">
        <thead>
          <tr><th>版本</th><th>发布时间</th><th>大小</th><th>状态</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-for="record in records" :key="record.id">
            <td class="mono">{{ record.version }}</td>
            <td>{{ record.publishedAt || record.createdAt || '—' }}</td>
            <td>{{ formatSize(record.sizeBytes) }}</td>
            <td>
              <span class="tag" :class="record.enabled ? (latest && latest.id === record.id ? 'tag--latest' : 'tag--ok') : 'tag--off'">
                {{ record.enabled ? (latest && latest.id === record.id ? '最新' : '可用') : '已停用' }}
              </span>
              <span v-if="record.mandatory" class="tag tag--mandatory">强制</span>
            </td>
            <td class="row-actions">
              <a class="ghost-btn" :href="updatePackageDownloadUrl(record)"><DemoIcon name="download" :size="13" /></a>
              <button class="ghost-btn danger" type="button" @click="remove(record)"><DemoIcon name="trash-2" :size="13" /></button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else class="empty-tip">{{ loading ? '加载中…' : '暂无已发布版本' }}</p>
    </section>
  </div>
</template>

<style scoped>
.update-manage { display: flex; flex-direction: column; gap: 16px; padding: 22px 26px; }
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
.page-head h1 { font-size: 18px; }
.page-head p { margin-top: 4px; color: var(--text-3); font-size: 12px; }
.ghost-btn { display: inline-flex; align-items: center; gap: 6px; padding: 7px 12px; border: 1px solid var(--line); border-radius: 8px; color: var(--text-2); font-size: 12px; background: transparent; text-decoration: none; cursor: pointer; }
.ghost-btn:hover { color: var(--accent); border-color: var(--accent); }
.ghost-btn.danger:hover { color: var(--danger); border-color: var(--danger); }
.latest-card { display: flex; align-items: center; gap: 16px; padding: 18px 20px; }
.latest-icon { display: grid; width: 48px; height: 48px; flex: none; place-items: center; border: 1px solid var(--accent); border-radius: 12px; color: var(--accent); background: var(--accent-soft); }
.latest-info { flex: 1; min-width: 0; }
.latest-label { color: var(--text-3); font-size: 11px; }
.latest-version { margin: 2px 0 4px; font-size: 20px; font-weight: 700; }
.latest-meta { color: var(--text-3); font-size: 12px; }
.section-title { margin-bottom: 14px; font-size: 14px; }
.upload-card { display: flex; flex-direction: column; gap: 12px; }
.upload-row { display: flex; gap: 10px; flex-wrap: wrap; }
.pick-btn { display: inline-flex; flex: 1; min-width: 240px; align-items: center; gap: 10px; padding: 12px 14px; border: 1px dashed var(--line); border-radius: 10px; color: var(--text-2); font-size: 12.5px; background: var(--panel); cursor: pointer; text-align: left; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.pick-btn:hover { border-color: var(--accent); color: var(--accent); }
.version-input { width: 200px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); color: var(--text-1); font-size: 13px; outline: none; }
.version-input:focus { border-color: var(--accent); }
.notes-input { width: 100%; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); color: var(--text-1); font-size: 12.5px; resize: vertical; outline: none; }
.notes-input:focus { border-color: var(--accent); }
.upload-foot { display: flex; align-items: center; justify-content: space-between; }
.check-line { display: inline-flex; align-items: center; gap: 6px; color: var(--text-2); font-size: 12px; cursor: pointer; }
.check-line input { accent-color: var(--accent); }
.primary-btn { display: inline-flex; align-items: center; gap: 8px; padding: 9px 20px; border: none; border-radius: 9px; background: var(--accent); color: var(--accent-ink, #fff); font-size: 13px; cursor: pointer; }
.primary-btn:disabled { opacity: .55; cursor: not-allowed; }
.record-table { width: 100%; border-collapse: collapse; font-size: 12.5px; }
.record-table th { padding: 8px 10px; color: var(--text-3); text-align: left; font-weight: 500; font-size: 11.5px; border-bottom: 1px solid var(--line); }
.record-table td { padding: 9px 10px; border-bottom: 1px solid var(--line); color: var(--text-2); }
.row-actions { display: flex; gap: 6px; }
.tag { display: inline-block; margin-right: 6px; padding: 2px 9px; border-radius: 999px; font-size: 10.5px; }
.tag--latest { color: var(--ok, #34c77b); background: rgba(52, 199, 123, 0.12); }
.tag--ok { color: var(--accent); background: var(--active); }
.tag--off { color: var(--text-3); background: var(--hover); }
.tag--mandatory { color: var(--warning, #f0a020); background: rgba(240, 160, 32, 0.12); }
.empty-tip { color: var(--text-3); font-size: 12px; }
.mono { font-family: 'JetBrains Mono', monospace; }
</style>
