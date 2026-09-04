<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { editingService } from '@/app/container'
import type { FileVersionInfo } from '@/services/data-manager/data-provider'
import type { DrawingFile } from '@/types/domain.types'
import { useDomainStore } from '@/stores/domain.store'
import { useAuthStore } from '@/stores/auth.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'DrawingVersionsTab' })

const domainStore = useDomainStore()
const authStore = useAuthStore()
const uiStore = useUiStore()
const router = useRouter()

const isAdmin = computed(() => authStore.hasRole('admin'))

const currentNo = computed(() => domainStore.currentDrawing?.no || '')

// 双向分支关系：我从哪里分叉（forkedFrom）+ 我衍生出哪些分支（branches.from === 当前图号）
const forkedFromNo = computed(() => {
  const target = domainStore.currentDrawing
  return target && 'forkedFrom' in target ? (target as { forkedFrom?: string }).forkedFrom || '' : ''
})

const forkedFromName = computed(() => {
  if (!forkedFromNo.value) return ''
  const source = domainStore.drawings.find((item) => item.no === forkedFromNo.value)
  return source?.name || '源图纸'
})

const derivedBranches = computed(() => {
  const no = currentNo.value
  if (!no) return domainStore.branches
  return domainStore.branches.filter((item) => item.from === no)
})

function openDrawingByNo(no: string) {
  domainStore.openDrawing(no)
}

const cadFiles = computed<DrawingFile[]>(() => {
  const target = domainStore.currentDrawing
  if (!target) return []
  return (target.files || []).filter((file) => /\.(exb|dwg|dxf)$/i.test(file.name))
})

const selectedStorageKey = ref('')
const versions = ref<FileVersionInfo[]>([])
const loading = ref(false)
const busyVersionId = ref('')

const selectedFile = computed(() => cadFiles.value.find((file) => (file.currentStorageKey || file.storageKey) === selectedStorageKey.value))

watch(cadFiles, (files) => {
  const keys = new Set(files.map((file) => file.currentStorageKey || file.storageKey || ''))
  if (!keys.has(selectedStorageKey.value)) {
    selectedStorageKey.value = files[0]?.currentStorageKey || files[0]?.storageKey || ''
  }
}, { immediate: true })

watch(selectedStorageKey, () => { void loadVersions() }, { immediate: true })

async function loadVersions() {
  const key = selectedStorageKey.value
  if (!key) {
    versions.value = []
    return
  }
  loading.value = true
  try {
    versions.value = await editingService.listVersions(key)
  } catch (error) {
    console.warn('加载版本列表失败', error)
    versions.value = []
  } finally {
    loading.value = false
  }
}

function formatSize(size: number): string {
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`
  if (size >= 1024) return `${(size / 1024).toFixed(0)} KB`
  return `${size} B`
}

function formatTime(iso: string): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString('zh-CN', { hour12: false })
}

function kindLabel(kind: string): string {
  if (kind === 'release') return '正式版本'
  if (kind === 'working') return '工作版本'
  if (kind === 'initial') return '初始版本'
  return kind
}

async function downloadVersion(version: FileVersionInfo) {
  if (busyVersionId.value) return
  busyVersionId.value = version.id
  try {
    const blob = await editingService.downloadVersion(version.id)
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = version.storageKey.split('/').pop() || `${version.version}.dwg`
    anchor.click()
    URL.revokeObjectURL(url)
    uiStore.toast(`版本 ${version.version} 已下载`, 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '下载版本文件失败', 'warn')
  } finally {
    busyVersionId.value = ''
  }
}

async function restoreVersion(version: FileVersionInfo) {
  if (busyVersionId.value || !selectedFile.value) return
  const confirmed = window.confirm(
    `确定要将「${selectedFile.value.name}」回退到版本 ${version.version} 吗？\n\n` +
    '系统将以该版本内容生成一个新的工作版本并设为当前内容；\n历史版本原样保留，可随时再次回退。',
  )
  if (!confirmed) return
  busyVersionId.value = version.id
  try {
    await editingService.restoreVersion(version.id)
    uiStore.toast(`已回退到 ${version.version}，当前内容已更新`, 'ok')
    await loadVersions()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '回退版本失败', 'warn')
  } finally {
    busyVersionId.value = ''
  }
}

// 在线浏览该历史版本：按版本 ID 精确读取该版本 DWG，不影响当前版本指针。
function viewVersion(version: FileVersionInfo) {
  const drawingId = currentNo.value
  if (!drawingId || !selectedFile.value) return
  router.push({
    name: 'drawing-viewer',
    params: { drawingId },
    query: { fileId: selectedFile.value.id, versionId: version.id },
  })
}
</script>

<template>
  <div class="ver-grid">
    <div class="card">
      <div class="card-title">
        <DemoIcon name="history" :size="16" />版本时间线
        <span class="hint">所有版本永久保留 · 回退不删除任何版本</span>
      </div>

      <div v-if="cadFiles.length > 1" class="file-picker">
        <label class="picker-label" for="version-file-select">选择文件</label>
        <select id="version-file-select" v-model="selectedStorageKey" class="inp picker-select">
          <option v-for="file in cadFiles" :key="file.id" :value="file.currentStorageKey || file.storageKey">
            {{ file.name }}
          </option>
        </select>
      </div>
      <div v-else-if="cadFiles.length === 1 && cadFiles[0]" class="file-picker single">
        <DemoIcon name="file" :size="14" />
        <span class="picker-name">{{ cadFiles[0].name }}</span>
      </div>

      <div v-if="loading" class="empty"><div class="t">正在加载版本记录...</div></div>
      <div v-else-if="!selectedStorageKey" class="empty"><DemoIcon name="history" :size="34" /><div class="t">当前图纸暂无 CAD 文件</div></div>
      <div v-else-if="versions.length" class="timeline">
        <div
          v-for="version in versions"
          :key="version.id"
          class="tl-item"
          :class="{ cur: version.isCurrentRelease || version.versionKind === 'release' }"
        >
          <div class="tl-dot"></div>
          <div class="tl-head">
            <span class="v">{{ version.version }}</span>
            <span class="tag" :class="version.versionKind === 'release' ? 'ok' : 'mute'">{{ kindLabel(version.versionKind) }}</span>
            <span v-if="version.versionKind === 'release'" class="tag ok">当前正式</span>
            <span class="tl-size">{{ formatSize(version.size) }}</span>
          </div>
          <div class="tl-meta">
            提交：{{ version.createdByName || '未知' }} · {{ formatTime(version.createdAt) }}
          </div>
          <div class="tl-actions">
            <button class="btn sm" type="button" :disabled="busyVersionId === version.id" @click="viewVersion(version)">
              <DemoIcon name="eye" :size="13" />在线浏览
            </button>
            <button class="btn sm" type="button" :disabled="busyVersionId === version.id" @click="downloadVersion(version)">
              <DemoIcon name="download" :size="13" />下载
            </button>
            <button
              v-if="isAdmin && version.versionKind !== 'release'"
              class="btn sm primary"
              type="button"
              :disabled="busyVersionId === version.id"
              title="以该版本内容生成新版本并设为当前内容，历史版本保留"
              @click="restoreVersion(version)"
            >
              <DemoIcon name="undo-2" :size="13" />{{ busyVersionId === version.id ? '回退中...' : '回退到此版本' }}
            </button>
          </div>
        </div>
      </div>
      <div v-else class="empty"><DemoIcon name="history" :size="34" /><div class="t">该文件暂无版本记录</div></div>
    </div>
    <div>
      <div class="card branch-panel">
        <div class="card-title"><DemoIcon name="git-branch" :size="16" />分叉 / 分支<span class="hint">源图与衍生图独立维护 · 可追溯分叉人</span></div>

        <div v-if="forkedFromNo" class="fork-origin-box">
          <div class="fo-head"><DemoIcon name="corner-down-right" :size="14" /><b>我从哪里分叉</b></div>
          <button class="fo-card" type="button" title="打开源图纸" @click="openDrawingByNo(forkedFromNo)">
            <span class="mono">{{ forkedFromNo }}</span>
            <span class="fo-name">{{ forkedFromName }}</span>
            <DemoIcon name="arrow-up-right" :size="13" />
          </button>
        </div>

        <div class="derived-head">
          <DemoIcon name="git-branch" :size="14" />
          <b>我的分支 ({{ derivedBranches.length }})</b>
          <span class="hint">基于当前图纸分叉产生的衍生项目</span>
        </div>
        <div v-if="derivedBranches.length" class="branch-list">
          <div v-for="branch in derivedBranches" :key="branch.name" class="card branch-card" :class="{ disabled: branch.status === '已禁用' }">
            <div class="bh"><DemoIcon name="git-branch" :size="15" /><b>{{ branch.name }}</b><span class="tag" :class="branch.status === '使用中' ? 'ok' : 'mute'">{{ branch.status }}</span><button class="btn sm" type="button" @click="uiStore.toast(`「${branch.status === '已禁用' ? '恢复分支' : '禁用分支'}」已执行 · 分支历史完整保留，可随时恢复`, branch.status === '已禁用' ? 'ok' : 'warn')">{{ branch.status === '已禁用' ? '恢复分支' : '禁用' }}</button></div>
            <div class="bd">分叉自 <span class="mono">{{ branch.from }}</span> · 分叉者 <b>{{ branch.by }}</b> 创建于 {{ branch.date }} — {{ branch.desc }}</div>
          </div>
        </div>
        <div v-else class="empty compact-empty"><DemoIcon name="git-branch" :size="30" /><div class="t">当前图纸暂无衍生分叉分支</div></div>
      </div>
      <div class="note version-note"><DemoIcon name="shield-check" :size="14" /><div>本地编辑保存与在线编辑保存都会自动生成工作版本；仅管理员可执行回退，普通用户可浏览与下载任意版本。</div></div>
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
.tl-size { margin-left: auto; color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.tl-meta { margin-top: 4px; color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.tl-actions { display: flex; gap: 8px; margin-top: 8px; }
.file-picker { display: flex; align-items: center; gap: 10px; margin: 10px 20px 0; }
.file-picker.single { color: var(--text-2); font-size: 12.5px; }
.picker-name { font-family: 'JetBrains Mono', monospace; }
.picker-label { color: var(--text-2); font-size: 12.5px; flex: none; }
.picker-select { flex: 1; }
.branch-panel { margin-bottom: 14px; }
.fork-origin-box { padding: 10px 14px 0; }
.fo-head { display: flex; align-items: center; gap: 7px; margin-bottom: 8px; color: var(--text-2); font-size: 12.5px; }
.fo-head svg { color: var(--accent); }
.fo-card { display: flex; align-items: center; gap: 10px; width: 100%; padding: 11px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel-2); cursor: pointer; font-size: 13px; text-align: left; transition: border-color 0.2s ease; }
.fo-card:hover { border-color: var(--accent); }
.fo-card .mono { font-weight: 700; }
.fo-card .fo-name { flex: 1; overflow: hidden; color: var(--text-2); text-overflow: ellipsis; white-space: nowrap; }
.fo-card svg { color: var(--text-3); }
.derived-head { display: flex; align-items: center; gap: 7px; padding: 14px 14px 4px; font-size: 12.5px; }
.derived-head svg { color: var(--accent); }
.derived-head .hint { margin-left: auto; }
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
.compact-empty { padding: 18px 0 22px; }
@media (max-width: 1180px) { .ver-grid { grid-template-columns: 1fr; } }
</style>
