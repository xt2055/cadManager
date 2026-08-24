<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { MaterialFile } from '@/types/domain.types'

defineOptions({ name: 'DrawingMaterialTab' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

const currentItem = computed(() => domainStore.currentDrawing)
const fileInput = ref<HTMLInputElement | null>(null)

const materialFiles = computed<MaterialFile[]>(() => {
  return currentItem.value?.materialFiles ?? []
})

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function triggerUpload() {
  fileInput.value?.click()
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
    uploadedAt: '刚刚',
  }

  try {
    const result = await domainStore.uploadMaterialFile(currentItem.value.no, newFile, file)
    if (result.importedCount > 0) {
      uiStore.toast(`备料表「${file.name}」已保存并导入 ${result.importedCount} 项物料`, 'ok')
    } else {
      uiStore.toast(`备料表「${file.name}」已保存；当前仅自动解析 CSV，Excel 明细需后续导入`, 'info')
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

    <!-- 顶部操作栏 -->
    <div class="card card-pad mat-top-bar">
      <div class="mat-info">
        <DemoIcon name="file-spreadsheet" :size="18" />
        <div>
          <h3>备料清单与物资定额</h3>
          <p>支持上传 Excel / CSV 格式备料表，版本与当前图纸生命周期完全绑定。</p>
        </div>
      </div>
      <div class="mat-actions">
        <button class="btn primary" type="button" @click="triggerUpload">
          <DemoIcon name="upload" :size="14" />上传备料表 (Excel/CSV)
        </button>
        <button class="btn" type="button" @click="uiStore.toast('已导出最新备料表为 Excel')">
          <DemoIcon name="download" :size="14" />导出 Excel
        </button>
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
            <span>{{ f.size }} · {{ f.version }} · 由 {{ f.uploadedBy }} 上传于 {{ f.uploadedAt }}</span>
           </div>
           <button class="btn sm" type="button" @click="handleDownloadFile(f)">
             <DemoIcon name="download" :size="13" />下载
           </button>
           <button class="btn sm danger" type="button" @click="handleDeleteFile(f)">
            <DemoIcon name="trash-2" :size="13" />删除
          </button>
        </div>
      </div>
    </div>

    <!-- 备料明细数据表 -->
    <div class="card bom-card">
      <div class="card-title">
        <DemoIcon name="table" :size="16" />
        备料项目清单
        <span class="hint">共 {{ domainStore.bom.length }} 项物料</span>
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
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in domainStore.bom" :key="item.id">
              <td class="num">{{ item.no }}</td>
              <td class="num">{{ item.id }}</td>
              <td class="material-name">{{ item.name }}</td>
              <td class="spec">{{ item.spec }}</td>
              <td class="num">{{ item.qty }}</td>
              <td class="num">{{ item.weight.toFixed(2) }}</td>
              <td class="num">{{ (item.weight * item.qty).toFixed(2) }}</td>
              <td class="remark">{{ item.remark }}</td>
            </tr>
            <tr v-if="!domainStore.bom.length">
              <td colspan="8">
                <div class="empty">
                  <DemoIcon name="file-spreadsheet" :size="34" />
                  <div class="t">暂无备料明细数据</div>
                  <p>请点击上方「上传备料表」导入物料清单</p>
                </div>
              </td>
            </tr>
          </tbody>
          <tfoot v-if="domainStore.bom.length">
            <tr>
              <td colspan="6">合计总重量（kg）</td>
              <td class="num">{{ domainStore.bom.reduce((total, item) => total + item.weight * item.qty, 0).toFixed(2) }}</td>
              <td></td>
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
@media (max-width: 760px) {
  .mat-top-bar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
