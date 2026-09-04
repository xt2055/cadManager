<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import * as XLSX from 'xlsx'
import { invoke } from '@tauri-apps/api/core'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { drawingFileService } from '@/app/container'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { BomItem, MaterialFile } from '@/types/domain.types'
import { formatReadableDateTime } from '@/utils/date-time'

defineOptions({ name: 'DrawingMaterialTab' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

const currentItem = computed(() => domainStore.currentDrawing)
const fileInput = ref<HTMLInputElement | null>(null)
const replaceInput = ref<HTMLInputElement | null>(null)
const replacingFileId = ref<string | null>(null)

const isEditing = ref(false)
const isSaving = ref(false)
const localBom = ref<BomItem[]>([])

const materialFiles = computed<MaterialFile[]>(() => {
  return currentItem.value?.materialFiles ?? []
})

const materialAuthor = computed(() => {
	if (!materialFiles.value.length) return ''
	const fromFile = materialFiles.value.find((f) => f.author && f.author.trim())?.author?.trim()
	if (fromFile && fromFile !== '待定') return fromFile
  const designer = (currentItem.value?.signers as Record<string, string> | undefined)?.['设计']
  if (designer && designer.trim() && designer.trim() !== '待定') return designer.trim()
  return ''
})

watch(
  () => domainStore.bom,
  (items) => {
    if (!isEditing.value) {
      localBom.value = items.map((item) => ({ ...item }))
    }
  },
  { immediate: true, deep: true },
)

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function triggerUpload() {
  fileInput.value?.click()
}

function triggerReplace(file: MaterialFile) {
  replacingFileId.value = file.id
  replaceInput.value?.click()
}

async function onReplaceChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  const fileId = replacingFileId.value
  if (!file || !fileId || !currentItem.value) return
  try {
    const result = await domainStore.replaceMaterialFile(currentItem.value.no, fileId, file)
    uiStore.toast(`备料表已替换，并解析出 ${result.importedCount} 条物料明细`, 'ok')
  } catch (error) {
    console.error('替换备料表失败', error)
    uiStore.toast('替换备料表失败', 'warn')
  } finally {
    target.value = ''
    replacingFileId.value = null
  }
}

function startEdit() {
  localBom.value = domainStore.bom.map((item) => ({ ...item }))
  isEditing.value = true
}

function cancelEdit() {
  localBom.value = domainStore.bom.map((item) => ({ ...item }))
  isEditing.value = false
}

function addRow() {
  const drawingNo = currentItem.value?.no ?? ''
  const newRow: BomItem = {
    no: localBom.value.length + 1,
    id: `${drawingNo}-BOM-${String(localBom.value.length + 1).padStart(3, '0')}`,
    drawingNo,
    name: '新增物料',
    spec: '—',
    qty: 1,
    weight: 0,
    remark: '',
  }
  localBom.value.push(newRow)
}

function removeRow(index: number) {
  localBom.value.splice(index, 1)
  localBom.value.forEach((row, idx) => {
    row.no = idx + 1
  })
}

async function saveEdit() {
  if (!currentItem.value) return
  isSaving.value = true
  try {
    await domainStore.saveDrawingBom(currentItem.value.no, localBom.value)
    isEditing.value = false
    uiStore.toast(`备料明细已成功保存，共 ${localBom.value.length} 项`, 'ok')
  } catch (error) {
    console.error('保存备料明细失败', error)
    uiStore.toast('保存备料明细失败', 'warn')
  } finally {
    isSaving.value = false
  }
}

function createFallbackWorkbook() {
  if (!localBom.value.length) {
    return null
  }

  const drawingNo = currentItem.value?.no || '图纸'
  const drawingName = currentItem.value?.name || '物料明细'

  const headers = ['序号', '图号 / 标准号', '名称', '规格 / 材质', '数量', '单重 (kg)', '总重 (kg)', '备注']
  const dataRows = localBom.value.map((item, index) => [
    index + 1,
    item.id,
    item.name,
    item.spec,
    item.qty,
    Number(item.weight.toFixed(2)),
    Number((item.weight * item.qty).toFixed(2)),
    item.remark,
  ])

  const totalWeight = localBom.value.reduce((total, item) => total + item.weight * item.qty, 0)
  dataRows.push(['', '', '', '合计总重量（kg）', '', '', Number(totalWeight.toFixed(2)), ''])

  const worksheet = XLSX.utils.aoa_to_sheet([
    [`${drawingNo} ${drawingName} 备料清单`],
    [],
    headers,
    ...dataRows,
  ])

  worksheet['!cols'] = [
    { wch: 8 },
    { wch: 24 },
    { wch: 20 },
    { wch: 28 },
    { wch: 10 },
    { wch: 12 },
    { wch: 12 },
    { wch: 28 },
  ]
  worksheet['!freeze'] = { xSplit: 0, ySplit: 3 }
  worksheet['!autofilter'] = { ref: `A3:H${dataRows.length + 3}` }
  worksheet['!printOptions'] = { horizontalCentered: true, verticalCentered: false }
  worksheet['!pageSetup'] = {
    orientation: 'landscape',
    paperSize: 9,
    fitToWidth: 1,
    fitToHeight: 0,
    scale: 100,
  }
  worksheet['!margins'] = {
    left: 0.25,
    right: 0.25,
    top: 0.5,
    bottom: 0.5,
    header: 0.2,
    footer: 0.2,
  }

  const workbook = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(workbook, worksheet, '备料明细')

  return { workbook, drawingNo }
}

function createExcelBytes(workbook: XLSX.WorkBook): Uint8Array {
  return new Uint8Array(XLSX.write(workbook, { bookType: 'xlsx', type: 'array' }))
}

async function createPrintableFile(): Promise<{ bytes: Uint8Array; drawingNo: string }> {
  const drawingNo = currentItem.value?.no || '图纸'
  const source = materialFiles.value.find((file) => file.storageKey && /\.xlsx$/i.test(file.name))
  const storageKey = source?.storageKey || ''

  try {
    const blob = await drawingFileService.exportBOM(drawingNo, storageKey, localBom.value)
    const arrayBuffer = await blob.arrayBuffer()
    return {
      bytes: new Uint8Array(arrayBuffer),
      drawingNo,
    }
  } catch (backendError) {
    console.warn('服务端导出 Excel 异常，尝试本地回退', backendError)
    const fallback = createFallbackWorkbook()
    if (!fallback) throw new Error('暂无备料明细可导出')
    return { bytes: createExcelBytes(fallback.workbook), drawingNo: fallback.drawingNo }
  }
}

function downloadExcel(bytes: Uint8Array, fileName: string) {
  const buffer = new ArrayBuffer(bytes.byteLength)
  new Uint8Array(buffer).set(bytes)
  const blob = new Blob([buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = fileName
  anchor.click()
  URL.revokeObjectURL(url)
}

async function exportExcel() {
  try {
    const result = await createPrintableFile()
    const fileName = `${result.drawingNo}_备料明细表_${new Date().toISOString().slice(0, 10)}.xlsx`
    downloadExcel(result.bytes, fileName)
    uiStore.toast(`已导出 Excel：${fileName}`, 'ok')
  } catch (error) {
    console.error('导出 Excel 失败', error)
    uiStore.toast('导出 Excel 失败：请确认原始文件格式有效', 'warn')
  }
}

async function handlePrint() {
  try {
    const result = await createPrintableFile()
    const fileName = `${result.drawingNo}_备料明细表_${new Date().toISOString().slice(0, 10)}.xlsx`
    const bytes = Array.from(result.bytes)
    const isTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window

    if (isTauri) {
      await invoke('open_generated_excel', { fileName, bytes })
      uiStore.toast('已用系统默认 Excel/WPS 打开，请在其中执行打印', 'ok')
      return
    }

    downloadExcel(result.bytes, fileName)
    uiStore.toast('已下载 Excel 文件，请用 Excel/WPS 打开后打印', 'info')
  } catch (error) {
    console.error('打开 Excel 打印文件失败', error)
    uiStore.toast('打印文件生成失败：请确认原始文件格式有效', 'warn')
  }
}

function formatCurrentTime(): string {
  const d = new Date()
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const hours = String(d.getHours()).padStart(2, '0')
  const minutes = String(d.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}`
}

async function onFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file || !currentItem.value) return

  const newFile: MaterialFile = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    drawingNo: currentItem.value.no,
    name: file.name,
    size: formatFileSize(file.size),
    version: currentItem.value.ver || 'v1.0',
    uploadedBy: '张工',
    uploadedAt: formatCurrentTime(),
  }

  try {
    const result = await domainStore.uploadMaterialFile(currentItem.value.no, newFile, file)
    if (result.importedCount > 0) {
      uiStore.toast(`备料表「${file.name}」已保存并成功解析导入 ${result.importedCount} 条物料明细`, 'ok')
    } else {
      uiStore.toast(`备料表「${file.name}」已保存`, 'ok')
    }
  } catch (error) {
    console.error('上传备料表失败', error)
    uiStore.toast('上传备料表失败', 'warn')
  } finally {
    target.value = ''
  }
}

async function handleDeleteFile(file: MaterialFile) {
  if (!currentItem.value) return
  if (!window.confirm(`确定要移除备料表文件「${file.name}」吗？`)) return

  try {
    await domainStore.deleteMaterialFile(currentItem.value.no, file.id)
    uiStore.toast(`已移除备料表文件 ${file.name}`)
  } catch (error) {
    console.error('删除备料表失败', error)
    uiStore.toast('删除备料表失败', 'warn')
  }
}

async function handleDownloadFile(file: MaterialFile) {
  try {
    await domainStore.downloadAttachment(file)
    uiStore.toast(`已开始下载 ${file.name}`, 'ok')
  } catch (error) {
    console.error('下载备料表失败', error)
    uiStore.toast('下载备料表失败：附件可能尚未保存', 'warn')
  }
}

async function handleParseFile(file: MaterialFile) {
  if (!currentItem.value) return

  try {
    const result = await domainStore.parseMaterialFile(currentItem.value.no, file.id)
    if (result.importedCount > 0) {
      uiStore.toast(`已从「${file.name}」解析出 ${result.importedCount} 条物料明细`, 'ok')
    } else {
      uiStore.toast(`未从「${file.name}」识别到物料明细`, 'warn')
    }
  } catch (error) {
    console.error('解析备料表明细失败', error)
    uiStore.toast('解析备料表明细失败：请确认文件内容和格式', 'warn')
  }
}
</script>

<template>
  <div class="material-page-view">
    <input
      ref="fileInput"
      type="file"
      accept=".xlsx,.xls,.csv"
      class="hidden-file-input"
      @change="onFileChange"
    />
    <input
      ref="replaceInput"
      type="file"
      accept=".xlsx,.xls,.csv"
      class="hidden-file-input"
      @change="onReplaceChange"
    />

    <!-- 顶部操作栏 -->
    <div class="card card-pad mat-top-bar">
      <div class="mat-info">
        <DemoIcon name="file-spreadsheet" :size="18" />
        <div>
          <div class="mat-title-row">
            <h3>备料清单与物资定额</h3>
            <span v-if="materialAuthor" class="author-badge">
              <DemoIcon name="user-check" :size="13" />
              编制：<b>{{ materialAuthor }}</b>
            </span>
          </div>
          <p>支持上传 Excel / CSV 格式备料表，版本与当前图纸生命周期完全绑定。</p>
        </div>
      </div>
      <div class="mat-actions">
        <template v-if="!isEditing">
          <button class="btn primary" type="button" @click="triggerUpload">
            <DemoIcon name="upload" :size="14" />上传备料表 (Excel/CSV)
          </button>
          <button class="btn" type="button" @click="startEdit">
            <DemoIcon name="edit" :size="14" />编辑明细
          </button>
          <button class="btn" type="button" @click="exportExcel">
            <DemoIcon name="download" :size="14" />导出 Excel
          </button>
          <button class="btn" type="button" @click="handlePrint">
            <DemoIcon name="printer" :size="14" />打印明细
          </button>
        </template>
        <template v-else>
          <button class="btn primary" type="button" :disabled="isSaving" @click="saveEdit">
            <DemoIcon name="check" :size="14" />{{ isSaving ? '保存中...' : '保存更改' }}
          </button>
          <button class="btn" type="button" :disabled="isSaving" @click="addRow">
            <DemoIcon name="plus" :size="14" />添加行
          </button>
          <button class="btn" type="button" :disabled="isSaving" @click="cancelEdit">
            <DemoIcon name="x" :size="14" />取消编辑
          </button>
        </template>
      </div>
    </div>

    <!-- 已上传的备料表文件档案卡片 -->
    <div v-if="materialFiles.length" class="card card-pad file-att-card">
      <div class="card-title small-title">
        <DemoIcon name="paperclip" :size="15" />已绑定备料表源文件
      </div>
      <div class="att-list">
        <div v-for="f in materialFiles" :key="f.id" class="att-item">
          <DemoIcon name="file-spreadsheet" :size="20" />
           <div class="att-meta">
            <b>{{ f.name }}</b>
            <span>
              {{ f.size }} · {{ f.version }} · 由 {{ f.uploadedBy }} 上传于 {{ formatReadableDateTime(f.uploadedAt, formatCurrentTime()) }}
              <template v-if="f.author || materialAuthor"> · 编制：<b class="author-tag">{{ f.author || materialAuthor }}</b></template>
            </span>
           </div>
            <button class="btn sm" type="button" @click="handleDownloadFile(f)">
             <DemoIcon name="download" :size="13" />下载
            </button>
            <button class="btn sm" type="button" @click="triggerReplace(f)">
             <DemoIcon name="refresh-cw" :size="13" />替换
            </button>
            <button class="btn sm" type="button" @click="handleParseFile(f)">
             <DemoIcon name="refresh-cw" :size="13" />解析明细
            </button>
            <button class="btn sm danger" type="button" @click="handleDeleteFile(f)">
            <DemoIcon name="trash-2" :size="13" />删除
          </button>
        </div>
      </div>
    </div>

    <!-- 备料明细数据表 -->
    <div class="card bom-card print-target">
      <div class="card-title">
        <DemoIcon name="table" :size="16" />
        备料项目清单
        <span class="hint">共 {{ localBom.length }} 项物料</span>
        <span v-if="materialAuthor" class="author-badge-sm">编制：<b>{{ materialAuthor }}</b></span>
        <span v-if="isEditing" class="tag warn">编辑模式中</span>
      </div>

      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>序号</th>
              <th>图号 / 标准号</th>
              <th>名称</th>
              <th>规格 / 材质</th>
              <th>数量</th>
              <th>单重 (kg)</th>
              <th>总重 (kg)</th>
              <th>备注</th>
              <th v-if="isEditing" class="no-print">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(item, index) in localBom" :key="item.id">
              <td class="num">{{ index + 1 }}</td>
              <td v-if="!isEditing" class="num">{{ item.id }}</td>
              <td v-else><input v-model="item.id" class="input-cell" /></td>

              <td v-if="!isEditing" class="material-name">{{ item.name }}</td>
              <td v-else><input v-model="item.name" class="input-cell" /></td>

              <td v-if="!isEditing" class="spec">{{ item.spec }}</td>
              <td v-else><input v-model="item.spec" class="input-cell" /></td>

              <td v-if="!isEditing" class="num">{{ item.qty }}</td>
              <td v-else><input v-model.number="item.qty" type="number" step="1" class="input-cell num-input" /></td>

              <td v-if="!isEditing" class="num">{{ item.weight.toFixed(2) }}</td>
              <td v-else><input v-model.number="item.weight" type="number" step="0.01" class="input-cell num-input" /></td>

              <td class="num">{{ (item.weight * item.qty).toFixed(2) }}</td>

              <td v-if="!isEditing" class="remark">{{ item.remark }}</td>
              <td v-else><input v-model="item.remark" class="input-cell" /></td>

              <td v-if="isEditing" class="no-print">
                <button class="btn sm danger" type="button" @click="removeRow(index)">
                  <DemoIcon name="trash-2" :size="12" />删除
                </button>
              </td>
            </tr>
            <tr v-if="!localBom.length">
              <td :colspan="isEditing ? 9 : 8">
                <div class="empty">
                  <DemoIcon name="file-spreadsheet" :size="34" />
                  <div class="t">暂无备料明细数据</div>
                  <p>请点击上方「上传备料表」导入物料清单或点击「编辑明细」手动添加</p>
                </div>
              </td>
            </tr>
          </tbody>
          <tfoot v-if="localBom.length">
            <tr>
              <td colspan="6">合计总重量（kg）</td>
              <td class="num">{{ localBom.reduce((total, item) => total + item.weight * item.qty, 0).toFixed(2) }}</td>
              <td :colspan="isEditing ? 2 : 1"></td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.material-page-view {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.hidden-file-input {
  display: none;
}
.mat-top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.mat-info {
  display: flex;
  align-items: center;
  gap: 12px;
}
.mat-info svg {
  color: var(--accent);
}
.mat-info h3 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}
.mat-info p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}
.mat-actions {
  display: flex;
  gap: 10px;
}
.file-att-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.small-title {
  padding: 0;
  font-size: 13px;
}
.att-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.att-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--panel-2);
}
.att-item svg {
  color: var(--accent);
}
.att-meta {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}
.att-meta span {
  color: var(--text-3);
  font-size: 11px;
}
.bom-card {
  overflow: visible;
}
.table-pad {
  padding: 10px 14px;
  overflow-x: auto;
}
.tbl {
  min-width: 900px;
}
.material-name {
  font-weight: 500;
}
.spec {
  color: var(--text-2);
}
.remark {
  color: var(--text-3);
}
.input-cell {
  width: 100%;
  padding: 4px 8px;
  border-radius: 4px;
  border: 1px solid var(--line);
  background: var(--panel);
  color: var(--text-1);
  font-size: 12px;
  outline: none;
}
.input-cell:focus {
  border-color: var(--accent);
}
.num-input {
  text-align: right;
}
.mat-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.author-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  background: var(--bg-card);
  border: 1px solid var(--accent);
  color: var(--accent);
  padding: 2px 10px;
  border-radius: 12px;
  font-size: 13px;
  font-weight: 500;
}
.author-badge b {
  font-weight: 700;
}
.author-badge-sm {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: rgba(var(--accent-rgb, 59, 130, 246), 0.1);
  border: 1px solid var(--accent);
  color: var(--accent);
  padding: 1px 8px;
  border-radius: 6px;
  font-size: 12px;
  margin-left: 8px;
}
.author-badge-sm b {
  font-weight: 700;
}
.author-tag {
  color: var(--accent);
  font-weight: 600;
}
@media print {
  :global(body *) {
    visibility: hidden;
  }
  .print-target,
  .print-target * {
    visibility: visible;
  }
  .print-target {
    position: absolute;
    left: 0;
    top: 0;
    width: 100%;
    border: none !important;
    box-shadow: none !important;
    background: transparent !important;
  }
  .no-print {
    display: none !important;
  }
}
@media (max-width: 760px) {
  .mat-top-bar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
