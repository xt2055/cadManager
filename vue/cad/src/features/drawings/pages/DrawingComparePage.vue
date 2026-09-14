<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import MlightCadViewer from '@/features/drawings/detail-tabs/preview/MlightCadViewer.vue'
import type { DrawingDifference } from '@/features/drawings/detail-tabs/preview/cad-compare'
import { useDrawingStore } from '@/stores/drawing.store'
import { getApiBaseUrl } from '@/services/api-base.service'
import { versioningService } from '@/app/container'
import { compareVersionOptions, compareSourcePath, submissionCompareVersion, type CompareVersion } from '@/features/drawings/detail-tabs/preview/cad-compare-sources'
import { lifecycleApi, type LifecycleTree } from '@/services/lifecycle.service'

const route = useRoute()
const router = useRouter()
const submissionId = computed(() => String(route.query.submissionId || ''))
const isChangeReview = computed(() => !!submissionId.value)
const store = useDrawingStore()
const viewer = ref<InstanceType<typeof MlightCadViewer> | null>(null)
const fileIds = ref<[string, string]>(['', ''])
const mode = ref<'versions' | 'drawings'>('versions')
const searches = ref(['', ''])
const planNos = ref(['', ''])
const plans = computed(() => [...store.drawings].sort((a, b) => a.no.localeCompare(b.no, 'zh-CN')))
const planFiles = computed(() => new Map(plans.value.map(plan => [plan.no, new Set([
  ...plan.files, ...plan.otherFiles,
  ...store.parts.filter(part => part.parentNo === plan.no).flatMap(part => [...part.files, ...part.otherFiles]),
].map(file => file.id))])))
const versionValues = ref(['current', 'current'])
const versionOptions = ref<[CompareVersion[], CompareVersion[]]>([[], []])
const versionLoading = ref([false, false])
const versionErrors = ref(['', ''])
const versionRequests = [0, 0]
let disposed = false
const localFiles = ref<[File | null, File | null]>([null, null])
const busy = ref(false)
const stage = ref('')
const error = ref('')
const baseUrl = ref('')
const baseName = ref('')
const result = ref<DrawingDifference[] | null>(null)
const filter = ref('all')
const active = ref<number | null>(null)
const labels = { added: '新增', deleted: '删除', modified: '修改' }
let pending: { name: string; buffer: ArrayBuffer } | null = null
let request = 0
let controller: AbortController | null = null
const files = computed(() => {
  const unique = new Map([...store.drawings, ...store.parts].flatMap(item => [...item.files, ...item.otherFiles]).map(file => [file.id, file]))
  return [...unique.values()].filter(file => /\.(dwg|dxf|exb)$/i.test(file.name))
})
const visible = computed(() => (result.value ?? []).filter(diff => filter.value === 'all' || diff.kind === filter.value))
const selectedVersions = computed(() => versionOptions.value.map((options, side) => options.find(option => option.value === versionValues.value[side])))
const identical = computed(() => !localFiles.value.some(Boolean) && !!fileIds.value[0] && fileIds.value[0] === fileIds.value[1] && versionValues.value[0] === versionValues.value[1])
const canCompare = computed(() => !busy.value && !identical.value && !versionLoading.value.some(Boolean) && !versionErrors.value.some(Boolean) && [0, 1].every(side => localFiles.value[side] || (fileIds.value[side] && selectedVersions.value[side])))
const sourceLabels = computed(() => [0, 1].map(side => localFiles.value[side]?.name || [files.value.find(file => file.id === fileIds.value[side])?.name, selectedVersions.value[side]?.label].filter(Boolean).join(' · ')))

function filteredFiles(side: number) {
  const search = searches.value[side]?.trim().toLowerCase() || ''
  return files.value.filter(file => (!planNos.value[side] || planFiles.value.get(planNos.value[side]!)?.has(file.id)) &&
    (file.id === fileIds.value[side] || `${file.drawingNo} ${file.partNo || ''} ${file.name}`.toLowerCase().includes(search)))
}
function selectPlan(side: 0 | 1) {
  searches.value[side] = ''
  if (!filteredFiles(side).some(file => file.id === fileIds.value[side])) fileIds.value[side] = ''
  selectStored(side)
}
async function loadVersions(side: 0 | 1, preferred?: { id?: string; key?: string }, usePrevious = false) {
  const generation = ++versionRequests[side]!
  const file = files.value.find(item => item.id === fileIds.value[side])
  versionOptions.value[side] = []
  versionErrors.value[side] = ''
  versionValues.value[side] = 'current'
  versionLoading.value[side] = !!file
  if (!file) return
  try {
    const key = file.currentStorageKey || file.storageKey
    const records = key ? await versioningService.list(key) : []
    if (disposed || generation !== versionRequests[side]) return
    const options = compareVersionOptions(file, records)
    versionOptions.value[side] = options
    if (preferred?.id || preferred?.key) {
      const match = options.find(option => preferred.id ? option.versionId === preferred.id : option.storageKey === preferred.key)
      if (!match) throw new Error('指定版本已不可用，请重新选择版本')
      versionValues.value[side] = match.value
    } else if (usePrevious) versionValues.value[side] = options.find(option => !option.current)?.value || 'current'
  } catch (cause) {
    if (!disposed && generation === versionRequests[side]) versionErrors.value[side] = cause instanceof Error ? cause.message : '版本列表加载失败'
  } finally { if (!disposed && generation === versionRequests[side]) versionLoading.value[side] = false }
}
function changeMode() {
  reset()
  if (mode.value === 'versions') {
    planNos.value[1] = planNos.value[0]!
    localFiles.value = [null, null]
    fileIds.value[1] = fileIds.value[0]
    void Promise.all([loadVersions(0, undefined, true), loadVersions(1)])
  }
}
function swapSources() {
  reset()
  fileIds.value = [fileIds.value[1], fileIds.value[0]]
  localFiles.value = [localFiles.value[1], localFiles.value[0]]
  versionValues.value.reverse()
  versionOptions.value = [versionOptions.value[1], versionOptions.value[0]]
  searches.value.reverse()
  planNos.value.reverse()
  versionErrors.value.reverse()
}

function reset() {
  request++
  controller?.abort()
  pending = null
  busy.value = false
  error.value = ''
  result.value = null
  active.value = null
  viewer.value?.clearComparison()
  if (baseUrl.value) URL.revokeObjectURL(baseUrl.value)
  baseUrl.value = ''
}
function selectLocal(event: Event, side: 0 | 1) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  reset()
  if (!/\.(dwg|dxf)$/i.test(file.name)) { error.value = '本地文件支持 DWG、DXF；EXB 请先上传到图纸库完成转换。'; input.value = ''; return }
  localFiles.value[side] = file
  mode.value = 'drawings'
  fileIds.value[side] = ''
  versionRequests[side]!++
  versionLoading.value[side] = false
  versionErrors.value[side] = ''
  versionOptions.value[side] = []
  input.value = ''
}
function selectStored(side: 0 | 1) {
  localFiles.value[side] = null
  reset()
  if (mode.value === 'versions') {
    planNos.value[1] = planNos.value[0]!
    fileIds.value[1] = fileIds.value[0]
    localFiles.value[1] = null
    void Promise.all([loadVersions(0, undefined, true), loadVersions(1)])
  } else void loadVersions(side)
}
async function readSource(side: 0 | 1, signal: AbortSignal) {
  const local = localFiles.value[side]
  if (local) return { name: local.name, buffer: await local.arrayBuffer() }
  const file = files.value.find(item => item.id === fileIds.value[side])
  if (!file) throw new Error('请选择两张图纸')
  const version = selectedVersions.value[side]
  if (!version) throw new Error('请选择图纸版本')
  const token = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
  const response = await fetch(`${getApiBaseUrl()}${compareSourcePath(file.id, version)}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {}, credentials: 'include', signal,
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(response.status === 409 ? `${sourceLabels.value[side]} 尚未完成转换，请稍后重试` : `读取 ${sourceLabels.value[side]} 失败：${body.message || response.status}`)
  }
  if (/text\/html|application\/json/.test(response.headers.get('content-type') || '')) throw new Error('图纸接口未返回有效 CAD 文件')
  const buffer = await response.arrayBuffer()
  const isDwg = new TextDecoder().decode(buffer.slice(0, 6)).startsWith('AC10')
  return { name: version.name.replace(/\.(exb|dwg|dxf)$/i, isDwg ? '.dwg' : '.dxf'), buffer }
}
async function start() {
  if (!canCompare.value) return
  reset()
  const generation = request
  controller = new AbortController()
  busy.value = true
  stage.value = '正在读取两张图纸…'
  try {
    const [left, right] = await Promise.all([readSource(0, controller.signal), readSource(1, controller.signal)])
    if (generation !== request) return
    pending = right
    baseName.value = left.name
    stage.value = '正在加载基准图纸…'
    baseUrl.value = URL.createObjectURL(new Blob([left.buffer]))
  } catch (cause) {
    if (generation !== request) return
    error.value = cause instanceof Error ? cause.message : String(cause)
    busy.value = false
  }
}
async function onReady() {
  const source = pending
  if (!source || !viewer.value) return
  pending = null
  const generation = request
  stage.value = '正在识别差异并标注…'
  try {
    const differences = await viewer.value.compareDrawing(source.name, source.buffer)
    if (generation !== request) return
    result.value = differences
    filter.value = 'all'
  } catch (cause) {
    if (generation === request) error.value = cause instanceof Error ? cause.message : String(cause)
  } finally { if (generation === request) busy.value = false }
}
function focus(diff: DrawingDifference) { active.value = diff.id; void viewer.value?.focusDifference(diff.id) }
function navigate(direction: number) {
  if (!visible.value.length) return
  const index = visible.value.findIndex(diff => diff.id === active.value)
  focus(visible.value[(index + direction + visible.value.length) % visible.value.length]!)
}
onMounted(async () => {
  try {
    await store.load()
    if (disposed) return
    const current = store.getDrawing(String(route.params.drawingId)) || store.getPart(String(route.params.drawingId))
    fileIds.value[0] = String(route.query.fileId || files.value.find(file => current?.files.some(item => item.id === file.id))?.id || files.value[0]?.id || '')
    fileIds.value[1] = String(route.query.compareFileId || fileIds.value[0])
    mode.value = fileIds.value[0] === fileIds.value[1] ? 'versions' : 'drawings'
    planNos.value = fileIds.value.map(id => plans.value.find(plan => planFiles.value.get(plan.no)?.has(id))?.no || '')
    if (isChangeReview.value) {
      const tree = await lifecycleApi<LifecycleTree>(`/lifecycle-tree?drawingNo=${encodeURIComponent(current?.no || String(route.params.drawingId))}`)
      if (disposed) return
      const submission = tree.changes.flatMap(change => change.submissions).find(item => item.id === submissionId.value)
      const file = submission?.files.find(item => item.attachmentId === fileIds.value[0])
      if (!file?.baseVersionId || !file.submittedVersionId) throw new Error('未找到本次工单的变更前后版本，请返回审核页面重新打开')
      fileIds.value[1] = file.attachmentId
      versionOptions.value = [
        [submissionCompareVersion(file.baseVersionId, file.name, '变更前版本')],
        [submissionCompareVersion(file.submittedVersionId, file.name, `第 ${submission!.round} 轮提交版本`)],
      ]
      versionValues.value = versionOptions.value.map(options => options[0]!.value)
      await start()
      return
    }
    await Promise.all([
      loadVersions(0, { id: String(route.query.versionId || ''), key: String(route.query.versionKey || '') }, mode.value === 'versions'),
      loadVersions(1, { id: String(route.query.compareVersionId || ''), key: String(route.query.compareVersionKey || '') }),
    ])
  } catch (cause) { error.value = cause instanceof Error ? cause.message : String(cause) }
})
onUnmounted(() => { disposed = true; reset() })
</script>

<template>
  <div class="compare-page">
    <header>
      <button class="btn" @click="isChangeReview ? router.back() : router.push({ name: 'drawing-preview', params: { drawingId: route.params.drawingId } })">{{ isChangeReview ? '返回审核' : '返回图纸' }}</button>
      <strong>图纸对比</strong>
      <span class="hint">按原始坐标叠加 · 模型空间 · 精度 0.000001 图纸单位</span>
    </header>
    <div v-if="!isChangeReview" class="mode-bar">
      <label><input v-model="mode" type="radio" value="versions" :disabled="busy || versionLoading.some(Boolean)" @change="changeMode" />同一图纸版本对比</label>
      <label><input v-model="mode" type="radio" value="drawings" :disabled="busy || versionLoading.some(Boolean)" @change="changeMode" />不同图纸对比</label>
      <span class="hint">{{ mode === 'versions' ? '例如：选择 1.0 与 1.1，查看版本变化' : '两侧可选择任意图纸及各自版本' }}</span>
    </div>
    <div class="source-bar">
      <div v-for="side in ([0, 1] as const)" :key="side" class="source">
        <div class="source-heading"><b>{{ side === 0 ? '基准图纸（旧）' : '待对比图纸（新）' }}</b><small>{{ side === 0 ? '对比基线' : '检查变化' }}</small></div>
        <label v-if="!isChangeReview" class="plan-field"><span>计划号 · 总图图号</span>
          <select v-model="planNos[side]" :disabled="isChangeReview || busy || (mode === 'versions' && side === 1)" @change="selectPlan(side)">
            <option value="">全部计划</option>
            <option v-for="plan in plans" :key="plan.id" :value="plan.no">{{ plan.no }} · {{ plan.name }}</option>
          </select>
        </label>
        <input v-model="searches[side]" class="file-search" :aria-label="side === 0 ? '搜索基准图纸' : '搜索待对比图纸'" placeholder="在当前计划内搜索图号或文件名" :disabled="isChangeReview || busy || (mode === 'versions' && side === 1)" />
        <select v-model="fileIds[side]" :aria-label="side === 0 ? '基准图纸' : '待对比图纸'" :disabled="isChangeReview || busy || (mode === 'versions' && side === 1)" @change="selectStored(side)">
          <option value="">{{ localFiles[side]?.name || '选择图纸库文件' }}</option>
          <option v-for="file in filteredFiles(side)" :key="file.id" :value="file.id">{{ file.partNo || file.drawingNo }} · {{ file.name }}</option>
        </select>
        <div class="version-row">
          <span>版本</span>
          <select v-model="versionValues[side]" :aria-label="side === 0 ? '基准版本' : '待对比版本'" :disabled="busy || versionLoading[side] || !versionOptions[side].length || !!versionErrors[side]" @change="reset">
            <option v-if="!versionOptions[side].length" value="current">{{ versionLoading[side] ? '正在加载版本…' : localFiles[side] ? '本地文件' : '请先选择图纸' }}</option>
            <option v-for="version in versionOptions[side]" :key="version.value" :value="version.value">{{ version.label }}</option>
          </select>
          <label v-if="mode === 'drawings'" class="btn local-button">本地文件<input type="file" accept=".dwg,.dxf" :disabled="busy" @change="selectLocal($event, side)" /></label>
        </div>
        <p v-if="versionErrors[side]" class="version-error" role="alert">{{ versionErrors[side] }} <button class="btn" @click="reset(); loadVersions(side)">重试</button></p>
          <small v-else-if="isChangeReview">按本次工单冻结的版本对比。</small>
          <small v-else-if="mode === 'versions' && side === 1">计划与图纸随左侧同步，版本独立选择。</small>
      </div>
      <div class="compare-actions">
        <button v-if="!isChangeReview" class="btn" :disabled="busy || versionLoading.some(Boolean) || versionErrors.some(Boolean)" @click="swapSources">交换两侧</button>
        <button class="btn primary" :disabled="!canCompare" @click="start">{{ busy ? '对比中…' : '开始对比' }}</button>
        <button v-if="busy" class="btn" @click="reset">取消</button>
      </div>
    </div>
    <p v-if="identical && !versionLoading.some(Boolean) && !versionErrors.some(Boolean)" class="selection-hint">{{ isChangeReview ? '本次提交与变更前为同一版本，该文件没有版本变化。' : '两侧选择了同一图纸的相同版本，请选择不同版本或切换“不同图纸对比”。' }}</p>
    <div v-if="baseUrl" class="comparison-caption"><span>基准：{{ sourceLabels[0] }}</span><span>对比：{{ sourceLabels[1] }}</span></div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div class="workspace">
      <main>
        <MlightCadViewer v-if="baseUrl" :key="baseUrl" ref="viewer" :dxf-url="baseUrl" :file-name="baseName" @ready="onReady" @load-error="error = $event; busy = false; pending = null" />
        <div v-else class="empty">选择图纸和版本，自动识别并标注新增、删除和修改。<small>支持同图纸任意版本对比、跨图纸版本对比，以及本地 DWG / DXF。</small></div>
        <div v-if="busy" class="progress" role="status">{{ stage }}</div>
        <div v-if="result" class="canvas-tools"><button class="btn" @click="viewer?.zoomOut()">缩小</button><button class="btn" @click="viewer?.zoomIn()">放大</button><button class="btn" @click="viewer?.resetView()">全图</button></div>
      </main>
      <aside>
        <h3>差异标注 <span v-if="result">{{ result.length }}</span></h3>
        <div class="legend"><span v-for="(label, kind) in labels" :key="kind" :class="kind">{{ label }}<b>{{ result ? result.filter(diff => diff.kind === kind).length : '—' }}</b></span></div>
        <p class="hint">灰色为未变化图形。忽略字体排版、线型线宽、填充与遮罩；保留文字内容和几何差异。</p>
        <template v-if="result">
          <select v-model="filter" aria-label="筛选差异类型"><option value="all">全部差异（{{ result.length }}）</option><option v-for="(label, kind) in labels" :key="kind" :value="kind">{{ label }}（{{ result.filter(d => d.kind === kind).length }}）</option></select>
          <div class="navigation"><button class="btn" :disabled="!visible.length" @click="navigate(-1)">上一处</button><button class="btn" :disabled="!visible.length" @click="navigate(1)">下一处</button></div>
          <p v-if="!result.length" role="status">未发现模型空间中的图形差异。</p>
          <p v-else-if="!visible.length">没有此类型的差异。</p>
          <div class="results"><button v-for="diff in visible" :key="diff.id" class="difference" :class="{ active: active === diff.id }" @click="focus(diff)"><b :class="diff.kind">#{{ diff.id }} {{ labels[diff.kind] }}</b><span>{{ (diff.after || diff.before)?.type }} · {{ (diff.after || diff.before)?.layer }}</span><small v-if="!diff.before?.bounds && !diff.after?.bounds">无可定位边界</small></button></div>
        </template>
        <p v-else class="hint">对比完成后，点击差异可定位到图上。</p>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.compare-page { height: 100%; min-height: 550px; display: flex; flex-direction: column; background: var(--panel); color: var(--text-1); }
header, .source-bar { display: flex; align-items: center; gap: 14px; padding: 14px 18px; border-bottom: 1px solid var(--line); flex-wrap: wrap; }
.source-bar { align-items: stretch; }
.source { flex: 1; min-width: 280px; display: flex; flex-direction: column; align-items: stretch; gap: 10px; padding: 14px; border: 1px solid var(--line); border-radius: 12px; background: var(--panel-2); }
.source-heading { display: flex; align-items: center; justify-content: space-between; padding-bottom: 8px; border-bottom: 1px solid var(--line); }
.plan-field { display: flex; flex-direction: column; gap: 6px; font-size: 12px; color: var(--text-2); }
select, .file-search { min-height: 38px; }
select:focus-visible, input:focus-visible, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
button:disabled, select:disabled, input:disabled { opacity: .55; cursor: not-allowed; }
.source b { font-size: 12px; white-space: nowrap; }
select { min-width: 0; padding: 8px; color: var(--text-1); background: var(--panel-2); border: 1px solid var(--line); border-radius: 6px; }
.source select { width: 100%; }
.mode-bar { display: flex; align-items: center; gap: 20px; padding: 12px 18px 0; flex-wrap: wrap; font-size: 13px; }
.mode-bar label { display: flex; gap: 6px; align-items: center; cursor: pointer; }
.file-search { padding: 8px; color: var(--text-1); background: var(--panel); border: 1px solid var(--line); border-radius: 6px; }
.version-row { display: flex; align-items: center; gap: 8px; font-size: 12px; }
.version-row select { flex: 1; width: 100px; }.version-row span { white-space: nowrap; }
.compare-actions { display: flex; flex-direction: column; justify-content: center; gap: 10px; min-width: 112px; }
.version-error { color: #e11d48; font-size: 12px; }
.selection-hint { padding: 8px 18px; color: var(--text-3); font-size: 12px; }
.comparison-caption { display: flex; gap: 20px; padding: 8px 18px; font-size: 12px; border-bottom: 1px solid var(--line); overflow-wrap: anywhere; }
.local-button { position: relative; overflow: hidden; white-space: nowrap; }
.local-button input { position: absolute; inset: 0; opacity: 0; width: 100%; cursor: pointer; }
.workspace { flex: 1; min-height: 0; display: flex; }
main { position: relative; flex: 1; min-width: 0; background: #000; }
aside { width: 280px; flex-shrink: 0; box-sizing: border-box; padding: 16px; border-left: 1px solid var(--line); display: flex; flex-direction: column; gap: 12px; }
h3, p { margin: 0; }
.hint, small { color: var(--text-3); font-size: 12px; line-height: 1.7; }
.empty { height: 100%; min-height: 400px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 14px; color: #cbd5e1; padding: 20px; text-align: center; }
.progress, .canvas-tools { position: absolute; bottom: 16px; left: 16px; background: var(--panel); padding: 10px; border-radius: 8px; z-index: 6; }
.canvas-tools, .legend, .navigation { display: flex; gap: 10px; }
.legend > span { flex: 1; display: flex; flex-direction: column; gap: 6px; padding: 10px; border: 1px solid var(--line); border-radius: 8px; font-size: 12px; background: var(--panel-2); }
.legend b { font-size: 20px; font-variant-numeric: tabular-nums; }
.navigation > button { flex: 1; }
.difference:hover { border-color: var(--accent); background: var(--panel); }
.added { color: #22c55e; }.deleted { color: #e11d48; }.modified { color: #f59e0b; }
.error { padding: 12px 18px; color: #e11d48; }
.results { overflow-y: auto; flex: 1; min-height: 0; }
.difference { display: flex; flex-direction: column; width: 100%; padding: 12px; gap: 7px; margin-bottom: 8px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel-2); color: var(--text-1); text-align: left; cursor: pointer; }
.difference.active { border-color: var(--accent); }.difference span { font-size: 12px; overflow-wrap: anywhere; }
@media (max-width: 850px) { aside { width: 210px; padding: 10px; }.source { flex-basis: 100%; }header .hint { display: none; } }
@media (max-width: 850px) { .compare-page { height: auto; min-height: 100%; }.compare-actions { flex-direction: row; width: 100%; }.compare-actions .btn { flex: 1; }.workspace { min-height: 480px; }.source { min-width: 0; } }
@media (max-width: 600px) { .workspace { flex-direction: column; }main { min-height: 420px; }aside { width: 100%; border-left: 0; border-top: 1px solid var(--line); }.results { max-height: 280px; }.comparison-caption { flex-direction: column; gap: 4px; }.mode-bar { gap: 10px; }.source-bar { padding: 12px; }.version-row { flex-wrap: wrap; } }
</style>
