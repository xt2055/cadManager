<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { attachmentUploader, drawingFileService, versioningService } from '@/app/container'
import { useDrawingStore } from '@/stores/drawing.store'
import { useAuthStore } from '@/stores/auth.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingSummaryView, FileView, PartView, StructureNodeView } from '@/modules/drawing'
import type { FileVersionInfo } from '@/types/application.types'
import { acceptsModel, fileFormat, isModelFile } from '@/utils/model-formats'
import { saveDownload } from '@/utils/download-file'
import { formatReadableDateTime } from '@/utils/date-time'
import { versionDisplayLabel } from '@/modules/versioning/versioning-service'

defineOptions({ name: 'DrawingModelsTab' })
type Owner = DrawingSummaryView | PartView
type ModelRow = FileView & { owner: Owner }
const route = useRoute()
const store = useDrawingStore()
const auth = useAuthStore()
const ui = useUiStore()
const input = ref<HTMLInputElement | null>(null)
const replacementInput = ref<HTMLInputElement | null>(null)
const replacement = ref<ModelRow | null>(null)
const busy = ref(false)
const progress = ref('')
const error = ref('')
const keyword = ref('')
const selectedOwner = ref('')
const failed = ref<{ file: File; owner: Owner; message: string }[]>([])
const historyFile = ref<ModelRow | null>(null)
const history = ref<FileVersionInfo[]>([])
const historyLoading = ref(false)
const historyError = ref('')
let historyRequest = 0

const current = computed(() => store.getDrawing(String(route.params.drawingId)) ?? store.getPart(String(route.params.drawingId)))
function flatten(nodes: StructureNodeView[]): PartView[] { return nodes.flatMap((node) => [node, ...flatten(node.children)]) }
const owners = computed<Owner[]>(() => {
  const item = current.value
  if (!item) return []
  return 'parentNo' in item ? [item] : [item, ...flatten(store.getStructure(item.no))]
})
function rootFor(owner: Owner): DrawingSummaryView | null {
  if (!('parentNo' in owner)) return owner
  const visited = new Set<string>()
  let no = owner.parentNo
  while (no && !visited.has(no)) {
    visited.add(no)
    const drawing = store.getDrawing(no)
    if (drawing) return drawing
    no = store.getPart(no)?.parentNo ?? ''
  }
  return null
}
function canManage(owner: Owner): boolean {
  const root = rootFor(owner)
  const user = auth.currentUser
  if (!root || !user || ['archived', 'reviewing', 'disabled'].includes(root.status)) return false
  if ('parentNo' in owner && (owner.borrowed || owner.relationType === 'borrowed')) return false
  return auth.hasRole('admin') || root.createdBy === user.displayName
}
const uploadOwner = computed(() => owners.value.find((owner) => owner.id === selectedOwner.value) ?? current.value)
const rows = computed<ModelRow[]>(() => {
  const seen = new Set<string>()
  const result: ModelRow[] = []
  for (const owner of owners.value) {
    for (const file of [...owner.files, ...owner.otherFiles]) {
      if (!isModelFile(file) || seen.has(file.id)) continue
      seen.add(file.id)
      result.push({ ...file, owner })
    }
  }
  return result.sort((a, b) => Number(Boolean(b.isPrimaryModel)) - Number(Boolean(a.isPrimaryModel)) || a.name.localeCompare(b.name))
})
const filtered = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return rows.value.filter((file) => !q || [file.name, file.owner.no, file.owner.name].some((value) => value.toLowerCase().includes(q)))
})

watch(() => route.params.drawingId, () => {
  selectedOwner.value = ''
  keyword.value = ''
  error.value = ''
  historyFile.value = null
  historyRequest++
})
onMounted(() => { void store.load().catch((e: unknown) => { error.value = message(e) }) })
function message(e: unknown): string { return e instanceof Error ? e.message : '操作失败，请重试' }
function storageKey(file: FileView): string {
  const key = file.currentStorageKey || file.storageKey
  if (!key) throw new Error('文件缺少存储信息，请刷新后重试')
  return key
}
async function refreshAfterWrite() {
  try { await store.refresh() } catch (e) { throw new Error(`文件已保存，但列表刷新失败，请刷新页面：${message(e)}`) }
}
async function upload(files: { file: File; owner: Owner }[]) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  failed.value = []
  let count = 0
  try {
    for (const [index, entry] of files.entries()) {
      progress.value = `正在上传 ${index + 1}/${files.length}：${entry.file.name}`
      try {
        if (!canManage(entry.owner)) throw new Error('当前图纸或零件不允许修改')
        if (!acceptsModel(entry.file.name)) throw new Error('不支持的格式，请选择 3D 模型或 ZIP 装配包')
        const root = rootFor(entry.owner)
        if (!root) throw new Error('未找到所属总图')
        await attachmentUploader.create(root.no, {
          id: crypto.randomUUID(), name: entry.file.name, fileCategory: 'model3d',
          role: 'parentNo' in entry.owner ? 'part' : 'assembly',
          ...('parentNo' in entry.owner ? { partNo: entry.owner.no } : {}),
        }, entry.file)
        count++
      } catch (e) { failed.value.push({ ...entry, message: message(e) }) }
    }
    if (count) { await refreshAfterWrite(); ui.toast(`已保存 ${count} 个 3D 文件`, 'ok') }
  } catch (e) { error.value = message(e) }
  finally { busy.value = false; progress.value = '' }
}
function onUpload(event: Event) {
  const target = event.target as HTMLInputElement
  const owner = uploadOwner.value
  const files = Array.from(target.files ?? [])
  target.value = ''
  if (owner && files.length) void upload(files.map((file) => ({ file, owner })))
}
function chooseReplacement(file: ModelRow) {
  replacement.value = file
  replacementInput.value?.click()
}
async function onReplace(event: Event) {
  const target = event.target as HTMLInputElement
  const content = target.files?.[0]
  const file = replacement.value
  target.value = ''
  if (!content || !file || busy.value) return
  busy.value = true
  error.value = ''
  progress.value = `正在保存新版本：${content.name}`
  try {
    if (!acceptsModel(content.name)) throw new Error('只能替换为 3D 模型或 ZIP 装配包')
    if (!canManage(file.owner)) throw new Error('当前图纸或零件不允许修改')
    await attachmentUploader.replace(file.drawingNo, { ...file, fileCategory: 'model3d' }, content)
    await refreshAfterWrite()
    ui.toast('新版本已保存，原版本保留在历史记录中', 'ok')
  } catch (e) { error.value = message(e) }
  finally { busy.value = false; progress.value = ''; replacement.value = null }
}
async function download(file: ModelRow) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try { await saveDownload(file.name, await drawingFileService.read(storageKey(file))) }
  catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
async function setPrimary(file: ModelRow) {
  if (busy.value || !canManage(file.owner)) return
  busy.value = true
  error.value = ''
  try {
    if (!file.revision) throw new Error('缺少附件版本，请刷新后重试')
    await drawingFileService.setPrimaryModel(storageKey(file), file.id, file.revision)
    await refreshAfterWrite()
    ui.toast(`已设为 ${file.owner.no} 的主模型`, 'ok')
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
function remove(file: ModelRow) {
  ui.confirm('删除 3D 文件', `确定删除「${file.name}」及其管理记录？`, {
    confirmText: '删除', danger: true,
    onConfirm: async () => {
      if (busy.value || !canManage(file.owner)) return
      busy.value = true
      error.value = ''
      try {
        await drawingFileService.delete(storageKey(file), file.id)
        await store.refresh()
        ui.toast('3D 文件已删除', 'ok')
      } catch (e) { error.value = message(e) }
      finally { busy.value = false }
    },
  })
}
async function openHistory(file: ModelRow) {
  const request = ++historyRequest
  historyFile.value = file
  history.value = []
  historyLoading.value = true
  historyError.value = ''
  try {
    const versions = await versioningService.list(storageKey(file), file.id)
    if (request === historyRequest) history.value = versions
  } catch (e) { if (request === historyRequest) historyError.value = message(e) }
  finally { if (request === historyRequest) historyLoading.value = false }
}
async function downloadVersion(version: FileVersionInfo) {
  if (busy.value) return
  busy.value = true
  try { await saveDownload(version.originalName || historyFile.value?.name || 'model', await versioningService.download(version.id)) }
  catch (e) { historyError.value = message(e) }
  finally { busy.value = false }
}
</script>

<template>
  <section class="models-view">
    <header class="card card-pad model-header">
      <div>
        <h3><DemoIcon name="box" :size="20" />3D 图纸管理 <span class="tag info">{{ rows.length }} 个文件</span></h3>
        <p>总图和零件可同时关联 2D 图纸与 3D 模型。原文件完整保存，支持下载、替换版本和指定主模型。</p>
      </div>
      <div class="model-upload">
        <label>上传到
          <select v-model="selectedOwner" class="inp" :disabled="busy" aria-label="模型归属">
            <option value="">当前图纸 · {{ current?.no }}</option>
            <option v-for="owner in owners.filter((item) => item.id !== current?.id)" :key="owner.id" :value="owner.id">{{ owner.no }} · {{ owner.name }}</option>
          </select>
        </label>
        <button class="btn primary" :disabled="busy || !uploadOwner || !canManage(uploadOwner)" @click="input?.click()"><DemoIcon name="upload" :size="14" />上传 3D 文件</button>
      </div>
      <!-- Do not use accept: native dialogs otherwise hide Creo's .prt.1 save versions. -->
      <input ref="input" type="file" multiple hidden @change="onUpload" />
      <input ref="replacementInput" type="file" hidden @change="onReplace" />
    </header>
    <div class="card card-pad model-help">
      <strong>格式与预览</strong>
      <p>支持 ZW3D（Z3PRT / Z3）、STEP / IGES、STL / OBJ / GLB、SolidWorks、Creo / NX、CATIA、Inventor、Parasolid、JT 等主流模型格式。</p>
      <p>装配模型请将入口、零件和依赖文件打成 ZIP 后上传；ZIP 完整保存，不自动解包。当前版本暂不支持在线预览，请下载后使用对应 CAD 软件打开。</p>
      <p v-if="uploadOwner && !canManage(uploadOwner)">当前对象仅供查看和下载。模型修改需要所属图纸创建者或管理员操作；审核中、已存档及借用件不能直接修改。</p>
    </div>
    <p v-if="busy" role="status">{{ progress || '正在处理…' }}</p>
    <p v-if="error" class="model-error" role="alert">{{ error }}</p>
    <div v-if="failed.length" class="card card-pad" role="alert">
      <strong>{{ failed.length }} 个文件未保存</strong>
      <p v-for="entry in failed" :key="entry.file.name">{{ entry.file.name }}：{{ entry.message }}</p>
      <button class="btn" :disabled="busy" @click="upload([...failed])">重试失败文件</button>
    </div>
    <div class="card model-table">
      <div class="model-toolbar"><input v-model="keyword" class="inp" placeholder="搜索模型文件名、图号或零件名称" aria-label="搜索 3D 文件" /><span>每个总图或零件可指定一个主模型</span></div>
      <div class="model-scroll">
        <table class="tbl">
          <thead><tr><th>模型文件</th><th>归属</th><th>版本 / 大小</th><th>预览状态</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="file in filtered" :key="file.id">
              <td><strong>{{ file.name }}</strong><div><span class="tag plain">{{ fileFormat(file.name).toUpperCase() }}</span> <span v-if="file.isPrimaryModel" class="tag info">主模型</span></div></td>
              <td>{{ file.owner.no }}<small>{{ file.owner.name }}{{ 'parentNo' in file.owner && file.owner.borrowed ? ' · 借用件' : '' }}</small></td>
              <td>{{ versionDisplayLabel(file.version) }}<small>{{ file.size }} · {{ file.uploadedBy }}</small></td>
              <td><span class="tag mute">原文件已保存</span><small>暂不支持在线预览</small></td>
              <td><div class="model-actions">
                <button class="btn sm" :disabled="busy" @click="download(file)">下载原件</button>
                <button class="btn sm" :disabled="busy" @click="openHistory(file)">历史</button>
                <template v-if="canManage(file.owner)">
                  <button class="btn sm" :disabled="busy" @click="chooseReplacement(file)">替换版本</button>
                  <button v-if="!file.isPrimaryModel" class="btn sm" :disabled="busy" @click="setPrimary(file)">设为主模型</button>
                  <button class="btn sm danger" :disabled="busy" @click="remove(file)">删除</button>
                </template>
              </div></td>
            </tr>
            <tr v-if="!filtered.length"><td colspan="5" class="model-empty">{{ store.loading ? '正在加载…' : keyword ? '没有匹配的 3D 文件' : '暂无 3D 图纸，可直接上传模型，无需先上传 2D 图纸。' }}</td></tr>
          </tbody>
        </table>
      </div>
    </div>
    <div v-if="historyFile" class="model-modal-backdrop" @click.self="historyFile = null">
      <section class="card card-pad model-modal" role="dialog" aria-modal="true" aria-labelledby="model-history-title">
        <header><h3 id="model-history-title">文件历史 · {{ historyFile.name }}</h3><button class="btn" @click="historyFile = null">关闭</button></header>
        <p v-if="historyLoading">正在加载历史版本…</p>
        <p v-if="historyError" class="model-error" role="alert">{{ historyError }}</p>
        <table v-if="history.length" class="tbl"><thead><tr><th>版本</th><th>原文件名</th><th>上传时间</th><th></th></tr></thead><tbody>
          <tr v-for="version in history" :key="version.id"><td>{{ versionDisplayLabel(version.version) }} <span v-if="version.isCurrentRelease" class="tag info">当前</span></td><td>{{ version.originalName || historyFile.name }}</td><td>{{ formatReadableDateTime(version.createdAt) }}<small>{{ version.createdByName }}</small></td><td><button class="btn sm" :disabled="busy" @click="downloadVersion(version)">下载</button></td></tr>
        </tbody></table>
        <p v-else-if="!historyLoading && !historyError">暂无版本记录</p>
      </section>
    </div>
  </section>
</template>

<style scoped>
.models-view { display: grid; gap: 16px; }
.model-header { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 18px; }
.model-header h3 { display: flex; align-items: center; gap: 10px; margin: 0 0 10px; }
.model-header p, .model-help p { color: var(--text-secondary, #667085); line-height: 1.7; margin: 6px 0; }
.model-upload, .model-actions, .model-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.model-upload label { display: grid; gap: 5px; }
.model-upload select { max-width: 320px; }
.model-toolbar { justify-content: space-between; padding: 16px; }
.model-toolbar input { width: min(420px, 100%); }
.model-toolbar span, small { color: var(--text-secondary, #667085); font-size: 12px; }
small { display: block; margin-top: 6px; }
.model-scroll { overflow-x: auto; }
.model-table td { vertical-align: middle; }
.model-table strong { overflow-wrap: anywhere; }
.model-empty { text-align: center; padding: 48px 20px; }
.model-error { color: var(--danger, #b42318); }
.model-modal-backdrop { position: fixed; inset: 0; z-index: 100; background: #0006; display: grid; place-items: center; padding: 24px; }
.model-modal { width: min(1000px, 100%); max-height: 85vh; overflow: auto; }
.model-modal header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
</style>
