<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
import { MousePointer2, Hand, Pencil, ArrowUpRight, Square, Circle, Type, Check, X, Undo2, Redo2, Trash2, Eye, EyeOff, PanelRightClose, PanelRightOpen, Save } from 'lucide-vue-next'
import DrawingAnnotationLayer from './DrawingAnnotationLayer.vue'
import { AnnotationHistory, AnnotationSaveQueue, type AnnotationMark, type AnnotationTemplate, type AnnotationTool, type AnnotationViewport, type AnnotationWorkspace } from '../annotation-model'
import { reviewAnnotationService as api } from '@/services/review-annotation.service'
import { useAuthStore } from '@/stores/auth.store'
import { annotationDraftKey, readAnnotationDraft, writeAnnotationDraft, type AnnotationDraft } from '../annotation-draft'

const props = defineProps<{ workspace: AnnotationWorkspace | null; viewport: AnnotationViewport | null; enabled: boolean; error: string }>()
const emit = defineEmits<{ reload: [] }>()
const auth = useAuthStore()
const tool = ref<AnnotationTool>('browse')
const color = ref('#FF6868')
const width = ref(3)
const text = ref('')
const selected = ref('')
const visible = ref(true)
const marks = ref<AnnotationMark[]>([])
const templates = ref<AnnotationTemplate[]>([])
const templateError = ref('')
const templateBusy = ref(false)
const templatesLoading = ref(false)
const editingTemplate = ref<AnnotationTemplate | null>(null)
const templateEditorOpen = ref(false)
const templateCategory = ref('全部')
const category = ref('常用')
const publicTemplate = ref(false)
const search = ref('')
const status = ref('')
const saveError = ref('')
const hint = ref('')
const saving = ref(false)
const dirty = ref(false)
const authorFilter = ref('')
const panelOpen = ref(true)
const panelTab = ref<'templates' | 'marks'>('templates')
const repeatPlacement = ref(false)
const localWarning = ref('')
const recovery = ref<AnnotationDraft | null>(null)
const selectedText = ref('')
const summaryMessage = ref('')
let activeDraftKey = ''
let scopeGeneration = 0
const history = shallowRef(new AnnotationHistory())
const historyTick = ref(0)
let queue: AnnotationSaveQueue | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let spaceHeld = ref(false)
const editable = computed(() => Boolean(props.workspace?.canEdit && props.viewport))
const ownDocument = computed(() => props.workspace?.documents.find(d => d.nodeId === props.workspace?.nodeId && d.authorId === auth.currentUser?.id))
const otherDocuments = computed(() => props.workspace?.documents.filter(d => d !== ownDocument.value) ?? [])
const allMarks = computed(() => [...otherDocuments.value.flatMap(d => d.content.marks), ...marks.value])
const filteredMarks = computed(() => authorFilter.value ? (authorFilter.value === auth.currentUser?.id ? marks.value.concat(otherDocuments.value.filter(d => d.authorId === authorFilter.value).flatMap(d => d.content.marks)) : otherDocuments.value.filter(d => d.authorId === authorFilter.value).flatMap(d => d.content.marks)) : allMarks.value)
const authors = computed(() => [...new Map([...(props.workspace?.documents ?? []).map(d => [d.authorId, d.authorName] as const), ...(marks.value.length ? [[auth.currentUser?.id ?? '', auth.currentUser?.displayName ?? '我'] as const] : [])]).entries()])
const selectedMark = computed(() => marks.value.find(m => m.id === selected.value))
const canUndo = computed(() => { historyTick.value; return history.value.canUndo })
const canRedo = computed(() => { historyTick.value; return history.value.canRedo })
const activeTool = computed(() => spaceHeld.value || !props.enabled || !props.viewport ? 'browse' : !editable.value && tool.value !== 'select' ? 'browse' : tool.value)
const filteredTemplates = computed(() => templates.value.filter(t => (templateCategory.value === '全部' || (templateCategory.value === '我的' ? t.ownerId === auth.currentUser?.id : t.category === templateCategory.value)) && `${t.category} ${t.text}`.includes(search.value.trim())))
const templateCategories = computed(() => ['全部', '我的', ...new Set(templates.value.map(t => t.category))])
const selectedInfo = computed(() => allMarks.value.find(m => m.id === selected.value))
const markLabels = { pen: '手绘标记', rect: '矩形圈框', ellipse: '椭圆圈框', arrow: '箭头', check: '已核对标记', cross: '叉号标记', text: '文字意见' }
const toolHint = computed(() => ({ browse: '浏览图纸；选择画笔或话术开始批注', select: '点击标记可编辑，拖动可移动位置', pen: '按住左键画线，松开自动保存', arrow: '按住左键拖出箭头', rect: '拖动圈出问题区域，再点话术添加说明', ellipse: '拖动圈出问题区域，再点话术添加说明', text: text.value.trim() ? `点击放置：${text.value.trim()}` : '先点击右侧话术，或填写文字', check: '点击图纸放置核对标记', cross: '点击图纸放置叉号' })[activeTool.value])
const isAdmin = computed(() => auth.currentUser?.roles.includes('admin'))
const tools = [
  { id: 'browse', label: '浏览', icon: Hand }, { id: 'select', label: '选择', icon: MousePointer2 },
  { id: 'pen', label: '画笔', icon: Pencil }, { id: 'arrow', label: '箭头', icon: ArrowUpRight },
  { id: 'rect', label: '圈框', icon: Square }, { id: 'ellipse', label: '椭圆', icon: Circle },
  { id: 'text', label: '文字', icon: Type }, { id: 'check', label: '勾选', icon: Check }, { id: 'cross', label: '叉号', icon: X },
] as const

watch(() => props.workspace, workspace => {
  scopeGeneration++
  clearTimeout(timer)
  marks.value = JSON.parse(JSON.stringify(ownDocument.value?.content.marks ?? []))
  selected.value = ''; tool.value = 'browse'; history.value = new AnnotationHistory(); historyTick.value++
  saveError.value = ''; status.value = ''; dirty.value = false; hint.value = ''; recovery.value = null; localWarning.value = ''
  authorFilter.value = ''; panelTab.value = workspace?.canEdit ? 'templates' : 'marks'
  activeDraftKey = workspace && auth.currentUser ? annotationDraftKey(workspace, auth.currentUser.id) : ''
  queue = workspace ? new AnnotationSaveQueue(ownDocument.value?.revision ?? 0, async (snapshot, revision) => {
    const saved = await api.save(workspace, revision, { schemaVersion: 1, marks: snapshot })
    return saved.revision
  }) : null
  if (activeDraftKey && workspace?.canEdit) {
    const draft = readAnnotationDraft(activeDraftKey)
    if (draft && JSON.stringify(draft.marks) !== JSON.stringify(marks.value)) recovery.value = draft
    else if (draft) writeAnnotationDraft(activeDraftKey, null)
  }
}, { immediate: true })

watch(selectedInfo, mark => { selectedText.value = mark?.text ?? '' })
watch(() => props.enabled, enabled => { if (!enabled) { tool.value = 'browse'; spaceHeld.value = false; void flush() } })

function persistDraft() {
  if (!queue || !activeDraftKey) return
  if (!writeAnnotationDraft(activeDraftKey, { revision: queue.revision, marks: marks.value, savedAt: new Date().toISOString() })) localWarning.value = '浏览器无法保留本地草稿，请确认显示“已保存”后再关闭页面。'
}
function restoreDraft() {
  const draft = recovery.value
  if (!draft || !queue || !editable.value) return
  const diverged = draft.revision !== queue.revision
  const next = diverged ? [...marks.value, ...draft.marks.filter(local => !marks.value.some(remote => JSON.stringify(local) === JSON.stringify(remote))).map(mark => ({ ...mark, id: crypto.randomUUID() }))] : draft.marks
  if (next.length > 1000 || next.reduce((n, mark) => n + mark.points.length, 0) > 100000) { hint.value = '恢复后的批注超过数量上限，请先导出草稿备份'; return }
  recovery.value = null; commit(next); hint.value = diverged ? '已将本地草稿另存为新标记，服务端已有意见保持原样' : '已恢复上次未保存的批注'; panelTab.value = 'marks'
}
function discardRecovery() { recovery.value = null; writeAnnotationDraft(activeDraftKey, null) }

function commit(next: AnnotationMark[]) {
  if (!editable.value || !queue) return
  if (recovery.value) { hint.value = '请先恢复或舍弃上次的本地草稿，再继续批注'; return }
  if (next.length > 1000 || next.reduce((n, m) => n + m.points.length, 0) > 100000) { hint.value = '批注数量达到上限，请先整理已有批注'; return }
  history.value.record(marks.value); historyTick.value++; marks.value = next; changed()
}
function changed() {
  queue?.set(marks.value); dirty.value = true; status.value = '待保存'
  persistDraft()
  clearTimeout(timer)
  if (!saveError.value) timer = setTimeout(() => { void flush() }, 650)
}
async function flush(): Promise<boolean> {
  clearTimeout(timer)
  if (!queue?.dirty) return !saveError.value
  const activeQueue = queue, generation = scopeGeneration, key = activeDraftKey
  saving.value = true; status.value = '保存中…'
  try {
    await activeQueue.flush()
    if (generation !== scopeGeneration) return true
    dirty.value = activeQueue.dirty; saveError.value = ''; status.value = '已保存'
    if (!dirty.value) writeAnnotationDraft(key, null)
    return true
  }
  catch (e) { if (generation === scopeGeneration) { saveError.value = e instanceof Error ? e.message : '保存失败，请重试'; status.value = '未保存'; persistDraft(); panelOpen.value = true }; return false }
  finally { if (generation === scopeGeneration) saving.value = false }
}
function add(mark: AnnotationMark) {
  authorFilter.value = ''; visible.value = true; commit([...marks.value, mark])
  if (!marks.value.some(m => m.id === mark.id)) return
  selected.value = mark.id; hint.value = ''
  if (mark.kind !== 'pen' && !repeatPlacement.value) { tool.value = 'select'; hint.value = ['rect', 'ellipse', 'arrow'].includes(mark.kind) ? '已选中新标记，点击话术即可附加说明' : '已放置，可拖动调整位置' }
}
function replace(mark: AnnotationMark) { commit(marks.value.map(m => m.id === mark.id ? mark : m)) }
function removeSelected() { if (selectedMark.value) { commit(marks.value.filter(m => m.id !== selected.value)); selected.value = ''; hint.value = '已删除，可按 Ctrl+Z 撤销' } }
function undo() { if (!editable.value || !canUndo.value) return; marks.value = history.value.undo(marks.value); historyTick.value++; selected.value = ''; changed() }
function redo() { if (!editable.value || !canRedo.value) return; marks.value = history.value.redo(marks.value); historyTick.value++; selected.value = ''; changed() }
function useText(value: string) {
  if (!editable.value) return
  text.value = value
  if (selectedMark.value && tool.value === 'select') { replace({ ...selectedMark.value, text: value }); hint.value = '话术已附加到选中的标记' }
  else { selected.value = ''; tool.value = 'text'; hint.value = '在图纸上点击放置这条意见' }
}
function pickTool(value: AnnotationTool) { tool.value = value; selected.value = ''; hint.value = ''; visible.value = true }
function updateSelectedText() { if (!selectedMark.value) return; if (selectedMark.value.kind === 'text' && !selectedText.value.trim()) { hint.value = '文字批注不能为空；如不需要，可删除该批注'; return }; replace({ ...selectedMark.value, text: selectedText.value.trim() }); hint.value = '意见已更新' }
function applyStyle() { if (selectedMark.value && tool.value === 'select') replace({ ...selectedMark.value, color: color.value, width: width.value }) }
function chooseColor(value: string) { color.value = value; applyStyle() }
function describeMark(mark: AnnotationMark) {
  const document = otherDocuments.value.find(d => d.content.marks.some(m => m.id === mark.id))
  return document ? `${document.authorName} · ${document.nodeName}` : `我 · ${props.workspace?.nodeName ?? ''}`
}
function focus(mark: AnnotationMark) {
  if (mark.layout !== props.viewport?.layout()) { hint.value = '这条批注属于其他布局，请在图纸中切换到对应布局后查看'; return }
  selected.value = mark.id; tool.value = 'select'; visible.value = true; props.viewport?.focus(mark.points)
}
async function loadTemplates() {
  templateError.value = ''; templatesLoading.value = true
  try { templates.value = await api.templates() } catch (e) { templateError.value = e instanceof Error ? e.message : '话术加载失败' } finally { templatesLoading.value = false }
}
async function saveTemplate() {
  if (!text.value.trim() || templateBusy.value) return
  templateBusy.value = true; templateError.value = ''
  try {
    if (editingTemplate.value) await api.updateTemplate({ ...editingTemplate.value, text: text.value, category: category.value })
    else await api.createTemplate(text.value, category.value, publicTemplate.value && isAdmin.value ? '' : auth.currentUser!.id)
    editingTemplate.value = null; templateEditorOpen.value = false; hint.value = '话术已保存，下次可直接点击使用'; await loadTemplates()
  }
  catch (e) { templateError.value = e instanceof Error ? e.message : '保存话术失败' }
  finally { templateBusy.value = false }
}
function editTemplate(item: AnnotationTemplate) { editingTemplate.value = item; text.value = item.text; category.value = item.category; publicTemplate.value = !item.ownerId; templateEditorOpen.value = true }
function cancelTemplateEdit() { editingTemplate.value = null; templateEditorOpen.value = false }
async function deleteTemplate(id: string) {
  if (templateBusy.value) return; templateBusy.value = true
  try { await api.deleteTemplate(id); await loadTemplates() } catch (e) { templateError.value = e instanceof Error ? e.message : '删除失败' } finally { templateBusy.value = false }
}
function exportDraft() {
  const blob = new Blob([JSON.stringify({ caseId: props.workspace?.caseId, attachmentId: props.workspace?.attachmentId, versionId: props.workspace?.versionId, nodeId: props.workspace?.nodeId, content: { schemaVersion: 1, marks: recovery.value?.marks ?? marks.value } }, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob), a = document.createElement('a'); a.href = url; a.download = '审核批注备份.json'; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
}
async function copySummary() {
  const rows = filteredMarks.value.filter(m => m.text.trim()).map((mark, index) => `${index + 1}. ${mark.text.trim()}（${describeMark(mark)}）`)
  if (!rows.length) { summaryMessage.value = '暂无文字意见，给圈框附加话术后即可复制'; return }
  try { await navigator.clipboard.writeText(rows.join('\n')); summaryMessage.value = '已复制，可粘贴到审核意见中' }
  catch { summaryMessage.value = '无法自动复制，请从下面的意见清单手动复制' }
}
function reload() { if (dirty.value) { hint.value = '还有未保存的批注，请先重试保存，或导出备份后刷新页面'; return }; emit('reload') }
function keydown(e: KeyboardEvent) {
  if (!props.enabled || (e.target as HTMLElement)?.closest('input, textarea, select, [contenteditable]')) return
  if (e.code === 'Space') { e.preventDefault(); spaceHeld.value = true }
  if (e.key === 'Escape') { tool.value = 'browse'; selected.value = '' }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); void flush() }
  if (editable.value && (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z') { e.preventDefault(); e.shiftKey ? redo() : undo() }
  if (editable.value && e.key === 'Delete') { e.preventDefault(); removeSelected() }
  if (editable.value && !e.ctrlKey && !e.metaKey && !e.altKey) {
    const shortcuts: Record<string, AnnotationTool> = { v: 'select', h: 'browse', p: 'pen', a: 'arrow', r: 'rect', o: 'ellipse', t: 'text' }
    const next = shortcuts[e.key.toLowerCase()]; if (next) { e.preventDefault(); pickTool(next) }
  }
}
function keyup(e: KeyboardEvent) { if (e.code === 'Space') spaceHeld.value = false }
function blur() { spaceHeld.value = false }
function beforeUnload(e: BeforeUnloadEvent) { if (dirty.value) { e.preventDefault(); e.returnValue = '' } }
function online() { if (dirty.value && saveError.value && !saveError.value.includes('其他窗口') && !saveError.value.includes('已签署')) void flush() }
onBeforeRouteLeave(async () => await flush())
onBeforeRouteUpdate(async () => await flush())
onMounted(() => { void loadTemplates(); window.addEventListener('keydown', keydown); window.addEventListener('keyup', keyup); window.addEventListener('blur', blur); window.addEventListener('beforeunload', beforeUnload); window.addEventListener('online', online) })
onBeforeUnmount(() => { clearTimeout(timer); window.removeEventListener('keydown', keydown); window.removeEventListener('keyup', keyup); window.removeEventListener('blur', blur); window.removeEventListener('beforeunload', beforeUnload); window.removeEventListener('online', online) })
</script>

<template>
  <div class="annotation-board">
    <div v-if="enabled" class="annotation-toolbar" role="toolbar" aria-label="图纸批注工具">
      <button v-for="item in tools" :key="item.id" type="button" class="tool" :class="{ active: activeTool === item.id }" :aria-pressed="activeTool === item.id" :disabled="item.id === 'select' ? !viewport : item.id !== 'browse' && (!editable || !!recovery)" @click="pickTool(item.id)"><component :is="item.icon" :size="16" />{{ item.label }}</button>
      <span class="tool-separator" aria-hidden="true"></span>
      <div class="swatches" aria-label="常用颜色"><button v-for="swatch in [{ color: '#FF6868', name: '红色' }, { color: '#FFD166', name: '黄色' }, { color: '#60D394', name: '绿色' }, { color: '#72B7FF', name: '蓝色' }]" :key="swatch.color" class="swatch" :style="{ '--swatch': swatch.color }" :class="{ chosen: color === swatch.color }" :title="swatch.name" :aria-label="swatch.name" :aria-pressed="color === swatch.color" :disabled="!editable" @click="chooseColor(swatch.color)"></button></div>
      <label class="color-control">自选<input v-model="color" type="color" aria-label="批注颜色" :disabled="!editable" @change="applyStyle" /></label>
      <label>线宽<select v-model.number="width" :disabled="!editable" aria-label="批注线宽" @change="applyStyle"><option :value="2">细</option><option :value="3">中</option><option :value="5">粗</option></select></label>
      <button class="tool icon" type="button" title="撤销 Ctrl+Z" aria-label="撤销" :disabled="!editable || !canUndo" @click="undo"><Undo2 :size="16" /></button>
      <button class="tool icon" type="button" title="重做 Ctrl+Shift+Z" aria-label="重做" :disabled="!editable || !canRedo" @click="redo"><Redo2 :size="16" /></button>
      <button class="tool icon" type="button" title="删除选中批注" aria-label="删除选中批注" :disabled="!editable || !selectedMark" @click="removeSelected"><Trash2 :size="16" /></button>
      <button class="tool icon" type="button" :aria-label="visible ? '隐藏批注' : '显示批注'" :title="visible ? '隐藏批注' : '显示批注'" @click="visible = !visible"><component :is="visible ? Eye : EyeOff" :size="16" /></button>
      <button class="tool icon" type="button" title="保存 Ctrl+S" aria-label="保存批注" :disabled="!dirty || saving" @click="flush"><Save :size="16" /></button>
      <button class="tool icon" type="button" :title="panelOpen ? '收起侧栏，扩大画图区域' : '展开话术与意见'" :aria-label="panelOpen ? '收起批注侧栏' : '展开批注侧栏'" :aria-expanded="panelOpen" @click="panelOpen = !panelOpen"><component :is="panelOpen ? PanelRightClose : PanelRightOpen" :size="16" /></button>
      <button class="save-state" type="button" role="status" :class="{ failed: saveError }" @click="saveError ? panelOpen = true : flush()">{{ status || (workspace?.canEdit ? '自动保存' : '只读批注') }}</button>
    </div>
    <div class="annotation-body">
      <div class="annotation-viewport">
        <slot />
        <DrawingAnnotationLayer v-if="viewport && workspace && enabled" :viewport="viewport" :marks="filteredMarks" :editable-ids="editable ? marks.map(m => m.id) : []" :tool="activeTool" :color="color" :width="width" :text="text" :selected="selected" :visible="visible" @add="add" @replace="replace" @select="selected = $event" @hint="hint = $event" />
        <div v-if="enabled && viewport" class="canvas-guidance"><span>{{ toolHint }}</span><span v-if="!visible">批注已隐藏</span></div>
      </div>
      <aside v-if="enabled && panelOpen" class="annotation-panel" aria-label="批注意见与快捷话术">
        <div class="panel-heading"><strong>审核批注</strong><button class="text-button" type="button" :disabled="saving" @click="reload">重新读取</button></div>
        <p class="note">{{ workspace?.canEdit ? `当前节点：${workspace.nodeName}。批注独立保存，原图保持不变。` : '可查看已有意见；仅当前节点责任人可添加批注。' }}</p>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div v-if="saveError" class="error" role="alert">{{ saveError }}<div class="actions"><button class="btn sm" :disabled="saving" @click="flush">重试保存</button><button class="btn sm" @click="exportDraft">导出备份</button></div></div>
        <p v-if="localWarning" class="error" role="alert">{{ localWarning }}</p>
        <div v-if="recovery" class="recovery-note" role="status"><strong>发现未保存的本地草稿</strong><p>{{ recovery.revision === (ownDocument?.revision ?? 0) ? '可以恢复上次的圈画和意见。' : '服务端已有新修改，可将草稿另存为一组新标记，不覆盖他人的修改。' }}</p><div class="actions"><button class="btn sm primary" :disabled="!editable" @click="restoreDraft">{{ recovery.revision === (ownDocument?.revision ?? 0) ? '恢复草稿' : '另存为新标记' }}</button><button class="btn sm" @click="exportDraft">导出草稿</button><button class="text-button" @click="discardRecovery">舍弃草稿</button></div></div>
        <p v-if="hint" class="hint" role="status">{{ hint }}</p>
        <section v-if="selectedInfo" class="selected-editor" aria-label="选中批注">
          <div class="panel-heading"><strong>{{ markLabels[selectedInfo.kind] }}</strong><button class="text-button" @click="selected = ''" aria-label="取消选择"><X :size="14" /></button></div>
          <p class="note">{{ describeMark(selectedInfo) }}</p>
          <template v-if="selectedMark && editable"><label class="text-label" for="selected-annotation-text">这处的意见</label><textarea id="selected-annotation-text" v-model="selectedText" class="field" maxlength="2000" rows="3" placeholder="也可直接点击下方话术附加说明" /><div class="actions"><button class="btn sm primary" :disabled="selectedText === selectedMark.text" @click="updateSelectedText">更新意见</button><button class="btn sm" @click="removeSelected">删除标记</button></div></template>
          <p v-else class="selected-readonly">{{ selectedInfo.text || '此标记未附加文字' }}</p>
        </section>
        <div class="panel-tabs" role="tablist" aria-label="批注面板内容"><button v-if="workspace?.canEdit" role="tab" :aria-selected="panelTab === 'templates'" @click="panelTab = 'templates'">快捷话术</button><button role="tab" :aria-selected="panelTab === 'marks'" @click="panelTab = 'marks'">图上意见 <span>{{ allMarks.length }}</span></button></div>
        <section v-if="workspace?.canEdit && panelTab === 'templates'" class="template-section">
          <input v-model="search" class="field" aria-label="搜索话术" placeholder="搜索尺寸、公差、工艺…" />
          <div class="category-tabs" aria-label="话术分类"><button v-for="item in templateCategories" :key="item" :aria-pressed="templateCategory === item" :class="{ active: templateCategory === item }" @click="templateCategory = item">{{ item }}</button></div>
          <p v-if="templatesLoading" class="note" role="status">正在读取常用话术…</p>
          <p v-if="templateError" class="error" role="alert">{{ templateError }} <button class="text-button" @click="loadTemplates">重试</button></p>
          <div class="template-list">
            <div v-for="item in filteredTemplates" :key="item.id" class="template-row">
              <button class="template-choice" :disabled="!editable" @click="useText(item.text)"><span>{{ item.category }} · {{ item.ownerId ? '个人' : '公共' }}</span>{{ item.text }}</button>
              <button v-if="item.ownerId === auth.currentUser?.id || (!item.ownerId && isAdmin)" class="text-button template-delete" :disabled="templateBusy" :aria-label="`编辑话术：${item.text}`" title="编辑话术" @click="editTemplate(item)"><Pencil :size="13" /></button>
              <button v-if="item.ownerId === auth.currentUser?.id || (!item.ownerId && isAdmin)" class="text-button template-delete" :disabled="templateBusy" :aria-label="`删除话术：${item.text}`" title="删除话术" @click="deleteTemplate(item.id)"><Trash2 :size="13" /></button>
            </div>
            <p v-if="!filteredTemplates.length && !templateError && !templatesLoading" class="note">暂无匹配话术，可在下方添加。</p>
          </div>
          <label class="text-label" for="annotation-text">批注文字</label>
          <textarea id="annotation-text" v-model="text" class="field" maxlength="500" rows="3" placeholder="点击话术后在图纸上放置，也可自行填写" />
          <div class="actions"><button class="btn sm primary" :disabled="!editable || !text.trim()" @click="useText(text)">{{ selectedMark && tool === 'select' ? '附加到所选标记' : '放到图纸上' }}</button></div>
          <label class="repeat-check"><input v-model="repeatPlacement" type="checkbox" />连续放置，适合多处使用同一标记</label>
          <button v-if="!templateEditorOpen" class="text-button template-settings" @click="templateEditorOpen = true">存为常用话术</button>
          <div v-else class="template-settings"><strong>{{ editingTemplate ? '编辑话术' : '保存为常用话术' }}</strong><label>分类<input v-model="category" class="field" maxlength="40" /></label><label v-if="isAdmin && !editingTemplate" class="public-check"><input v-model="publicTemplate" type="checkbox" />设为公共话术</label><div class="actions"><button class="btn sm" :disabled="templateBusy || !text.trim() || !category.trim()" @click="saveTemplate">{{ templateBusy ? '保存中…' : '保存话术' }}</button><button class="text-button" @click="cancelTemplateEdit">取消</button></div></div>
        </section>
        <section v-if="panelTab === 'marks'" class="marks-section">
          <select v-model="authorFilter" class="field" aria-label="筛选批注人"><option value="">全部审核人</option><option v-for="[id, name] in authors" :key="id" :value="id">{{ name }}</option></select>
          <div class="actions"><button class="text-button" :disabled="!filteredMarks.some(m => m.text.trim())" @click="copySummary">复制意见清单</button></div>
          <p v-if="summaryMessage" class="note" role="status">{{ summaryMessage }}</p>
          <p v-if="!allMarks.length" class="note">{{ editable ? '暂无批注。选择画笔圈出问题，或点击话术直接贴到图上。' : '本轮审核暂未留下图上批注。' }}</p>
          <button v-for="(mark, index) in filteredMarks" :key="mark.id" class="mark-row" :class="{ selected: selected === mark.id }" @click="focus(mark)"><span class="mark-number">{{ index + 1 }}</span><span class="mark-description">{{ mark.text || markLabels[mark.kind] }}<small>{{ describeMark(mark) }}</small></span></button>
        </section>
        <details class="keyboard-note"><summary>快捷键与操作帮助</summary><p>空格：临时浏览 · Esc：结束当前工具<br />P：画笔 · R：圈框 · A：箭头 · T：文字<br />V：选择 · H：浏览 · O：椭圆<br />Ctrl+Z：撤销 · Ctrl+Shift+Z：重做<br />Ctrl+S：保存 · Delete：删除所选批注</p><p>圈框后点击话术，直接附加说明。图上的勾号仅为标记，正式结论请返回审核工作台签署。</p></details>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.annotation-board { display: flex; flex-direction: column; width: 100%; height: 100%; min-width: 0; min-height: 0; }
.annotation-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 4px; padding: 7px 10px; flex: none; background: var(--panel); color: var(--text-1); border-bottom: 1px solid var(--line); }
.tool { display: inline-flex; align-items: center; gap: 5px; border: 1px solid transparent; border-radius: 5px; padding: 7px; background: transparent; color: var(--text-2); cursor: pointer; font: inherit; font-size: 12px; }
.tool:hover:not(:disabled), .tool.active { color: var(--accent); background: var(--accent-soft); border-color: var(--line); }
button:disabled { opacity: .45; cursor: not-allowed; }
.annotation-toolbar label { display: inline-flex; align-items: center; gap: 5px; margin: 0 5px; font-size: 12px; }
.color-control input { width: 28px; height: 26px; padding: 1px; background: transparent; border: 1px solid var(--line); }
select { color: var(--text-1); background: var(--panel); border: 1px solid var(--line); padding: 4px; border-radius: 4px; }
.save-state { margin-left: auto; font-size: 12px; color: var(--text-2); background: none; border: 0; cursor: pointer; padding: 7px; }
.annotation-body { display: flex; flex: 1; min-height: 0; min-width: 0; }
.annotation-viewport { position: relative; flex: 1; min-width: 0; min-height: 0; overflow: hidden; }
.annotation-panel { flex: 0 0 300px; width: 300px; padding: 16px; overflow-y: auto; background: var(--panel); color: var(--text-1); border-left: 1px solid var(--line); scrollbar-color: var(--line) transparent; }
.panel-heading { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.panel-heading strong { font-size: 15px; }
h3 { font-size: 13px; margin: 0 0 10px; }
.note, .keyboard-note { font-size: 12px; line-height: 1.65; color: var(--text-2); margin: 10px 0; }
.template-section, .marks-section { padding-top: 14px; }
.field { display: block; width: 100%; box-sizing: border-box; border: 1px solid var(--line); border-radius: 5px; background: var(--panel-2); color: var(--text-1); font: inherit; font-size: 12px; line-height: 1.5; padding: 8px 10px; caret-color: var(--accent); }
.field::placeholder { color: var(--text-3); }
.template-list { max-height: 280px; overflow-y: auto; margin: 8px 0 14px; }
.template-row { display: flex; border-bottom: 1px solid var(--line); }
.template-choice { flex: 1; min-width: 0; padding: 9px 4px; text-align: left; border: 0; background: transparent; color: var(--text-1); cursor: pointer; font-size: 12px; line-height: 1.6; }
.template-choice span { display: block; color: var(--text-2); font-size: 11px; }
.template-choice:hover { color: var(--accent); }
.text-button { background: transparent; border: 0; color: var(--accent); cursor: pointer; font-size: 12px; padding: 4px; }
.template-delete { align-self: center; }
.text-label { display: block; margin: 8px 0 6px; font-size: 12px; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.template-settings { font-size: 12px; margin-top: 12px; }
.template-settings summary { color: var(--text-2); cursor: pointer; }
.template-settings label { display: block; margin: 8px 0; }
.public-check { display: flex !important; align-items: center; gap: 6px; }
.mark-row { display: flex; width: 100%; gap: 8px; text-align: left; padding: 10px 6px; border: 0; border-bottom: 1px solid var(--line); background: transparent; color: var(--text-1); cursor: pointer; line-height: 1.6; font-size: 12px; overflow-wrap: anywhere; }
.mark-row:hover, .mark-row.selected { background: var(--accent-soft); }
.mark-number { color: var(--text-2); min-width: 18px; font-variant-numeric: tabular-nums; }
.hint { background: var(--accent-soft); color: var(--text-1); padding: 8px; font-size: 12px; line-height: 1.6; }
.error, .failed { color: var(--danger); font-size: 12px; line-height: 1.6; overflow-wrap: anywhere; }
.keyboard-note { padding-top: 14px; border-top: 1px solid var(--line); }
.keyboard-note summary { cursor: pointer; }
.tool-separator { height: 20px; width: 1px; background: var(--line); margin: 0 5px; }
.swatches { display: flex; gap: 5px; padding: 0 3px; }
.swatch { width: 18px; height: 18px; padding: 0; border-radius: 50%; border: 2px solid var(--panel); background: var(--swatch); cursor: pointer; }
.swatch.chosen { outline: 1px solid var(--text-1); outline-offset: 1px; }
.canvas-guidance { position: absolute; bottom: 10px; left: 12px; right: 12px; display: flex; gap: 12px; pointer-events: none; color: #fff; font-size: 12px; }
.canvas-guidance span { max-width: min(600px, 100%); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; background: #202936; padding: 6px 10px; border-radius: 4px; }
.selected-editor { border: 1px solid var(--line); padding: 12px; border-radius: 6px; margin: 14px 0; }
.selected-editor strong { font-size: 13px; }
.selected-editor .note { margin: 6px 0; }
.selected-readonly { font-size: 13px; line-height: 1.65; white-space: pre-wrap; overflow-wrap: anywhere; }
.panel-tabs { display: flex; border-bottom: 1px solid var(--line); margin-top: 18px; }
.panel-tabs button { flex: 1; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--text-2); padding: 10px 4px; font: inherit; font-size: 13px; cursor: pointer; }
.panel-tabs button[aria-selected='true'] { color: var(--accent); border-bottom-color: var(--accent); font-weight: 600; }
.panel-tabs span { font-variant-numeric: tabular-nums; }
.category-tabs { display: flex; gap: 4px; flex-wrap: wrap; margin-top: 10px; }
.category-tabs button { border: 0; border-radius: 4px; padding: 4px 8px; background: transparent; color: var(--text-2); font-size: 12px; cursor: pointer; }
.category-tabs button.active, .category-tabs button:hover { color: var(--accent); background: var(--accent-soft); }
.repeat-check { display: flex; gap: 5px; align-items: flex-start; font-size: 12px; line-height: 1.6; margin-top: 12px; color: var(--text-2); }
.repeat-check input { margin-top: 3px; }
.mark-description { flex: 1; min-width: 0; white-space: pre-wrap; }
.mark-description small { display: block; color: var(--text-2); font-size: 11px; margin-top: 4px; }
.recovery-note { background: var(--accent-soft); color: var(--text-1); padding: 12px; font-size: 12px; line-height: 1.65; }
.recovery-note p { margin: 6px 0; }
button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible, summary:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
::selection { background: var(--accent-soft); color: var(--text-1); }
@media (max-width: 1100px) { .annotation-panel { flex-basis: 260px; width: 260px; padding: 12px; } .tool { padding: 6px; } }
</style>
