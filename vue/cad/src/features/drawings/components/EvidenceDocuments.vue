<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { lifecycleApi, downloadEvidence, type EvidenceDocument } from '@/services/lifecycle.service'
import { formatReadableDateTime } from '@/utils/date-time'
import { useUiStore } from '@/stores/ui.store'

const props = defineProps<{
  drawingId?: string
  patentId?: string
  changeRequestId?: string
  submissionId?: string
  readOnly?: boolean
}>()

const uiStore = useUiStore()
const documents = ref<EvidenceDocument[]>([])
const categories = [
  '客户沟通',
  '确认图',
  '技术要求',
  '备料表',
  '工艺资料',
  '变更依据',
  '验收证明',
  '专利证书',
  '缴费凭证',
  '其他资料',
]

const category = ref('客户沟通')
const title = ref('')
const description = ref('')
const search = ref('')
const filter = ref('')
const folderPath = ref('')
const selectedFolder = ref('')
const isUploadModalOpen = ref(false)
const isDragging = ref(false)
const viewMode = ref<'grid' | 'table'>('grid')

const folderListId = computed(() => `evidence-folders-${props.submissionId || props.changeRequestId || props.patentId || props.drawingId || 'root'}`)

const folderOptions = computed(() => [
  ...new Set(
    documents.value
      .filter((d) => !props.changeRequestId || d.changeRequestId === props.changeRequestId)
      .flatMap((d) => {
        const segments = (d.folderPath || '').split('/').filter(Boolean)
        return segments.map((_, index) => segments.slice(0, index + 1).join('/'))
      }),
  ),
].sort())

const file = ref<File | null>(null)
const input = ref<HTMLInputElement | null>(null)
const busy = ref(false)
const loading = ref(false)
const error = ref('')

const visible = computed(() =>
  documents.value.filter(
    (d) =>
      (!props.changeRequestId || d.changeRequestId === props.changeRequestId) &&
      (!selectedFolder.value || d.folderPath === selectedFolder.value || d.folderPath?.startsWith(`${selectedFolder.value}/`)) &&
      (!filter.value || d.category === filter.value) &&
      `${d.title} ${d.fileName} ${d.description || ''} ${d.folderPath || ''}`.toLowerCase().includes(search.value.toLowerCase()),
  ),
)

const totalSizeBytes = computed(() => visible.value.reduce((acc, cur) => acc + (cur.size || 0), 0))

function formatFileSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

function getFileIcon(name: string): string {
  const ext = name.slice(name.lastIndexOf('.')).toLowerCase()
  if (['.jpg', '.jpeg', '.png', '.gif', '.webp', '.svg'].includes(ext)) return 'image'
  if (['.pdf'].includes(ext)) return 'file-text'
  if (['.doc', '.docx'].includes(ext)) return 'file-text'
  if (['.xls', '.xlsx', '.csv'].includes(ext)) return 'file-spreadsheet'
  if (['.dwg', '.dxf', '.exb'].includes(ext)) return 'layers'
  if (['.step', '.stp', '.iges', '.igs', '.z3prt', '.z3asm'].includes(ext)) return 'box'
  return 'file'
}

let generation = 0
async function load() {
  const seq = ++generation
  loading.value = true
  error.value = ''
  try {
    const query = props.patentId
      ? `patentId=${encodeURIComponent(props.patentId)}`
      : `drawingId=${encodeURIComponent(props.drawingId || '')}`
    const result = await lifecycleApi<EvidenceDocument[]>(
      `/lifecycle-documents?${query}${props.submissionId ? `&submissionId=${encodeURIComponent(props.submissionId)}` : ''}`,
    )
    if (seq === generation) documents.value = result
  } catch (e) {
    if (seq === generation) error.value = (e as Error).message
  } finally {
    if (seq === generation) loading.value = false
  }
}

watch(
  () => [props.drawingId, props.patentId, props.submissionId],
  () => {
    documents.value = []
    title.value = ''
    description.value = ''
    file.value = null
    if (input.value) input.value.value = ''
    void load()
  },
  { immediate: true },
)

function onDrop(event: DragEvent) {
  isDragging.value = false
  if (props.readOnly) return
  const dropped = event.dataTransfer?.files?.[0]
  if (dropped) {
    assignFile(dropped)
  }
}

function assignFile(selected: File) {
  file.value = selected
  if (!title.value.trim()) {
    title.value = selected.name.replace(/\.[^/.]+$/, '')
  }
}

function pick(event: Event) {
  const target = event.target as HTMLInputElement
  const chosen = target.files?.[0] || null
  if (chosen) {
    assignFile(chosen)
  }
}

function removeSelectedFile() {
  file.value = null
  if (input.value) input.value.value = ''
}

function openUploadModal(defaultFolder?: string) {
  if (defaultFolder) folderPath.value = defaultFolder
  isUploadModalOpen.value = true
}

function closeUploadModal() {
  isUploadModalOpen.value = false
  removeSelectedFile()
  title.value = ''
  description.value = ''
}

async function upload() {
  if (!file.value || !title.value.trim() || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const form = new FormData()
    form.set('file', file.value)
    form.set('title', title.value.trim())
    form.set('category', category.value)
    form.set('description', description.value.trim())
    form.set('folderPath', folderPath.value.trim())
    if (props.patentId) form.set('patentId', props.patentId)
    else if (props.drawingId) form.set('drawingId', props.drawingId)
    if (props.changeRequestId) form.set('changeRequestId', props.changeRequestId)

    await lifecycleApi('/lifecycle-documents', { method: 'POST', body: form })
    uiStore.toast(`资料「${title.value}」已成功归档入库`, 'ok')
    closeUploadModal()
    await load()
  } catch (e) {
    error.value = (e as Error).message
    uiStore.toast(error.value || '资料上传归档失败', 'warn')
  } finally {
    busy.value = false
  }
}

async function download(d: EvidenceDocument) {
  try {
    await downloadEvidence(`/lifecycle-documents/${d.id}`, d.fileName)
    uiStore.toast(`已开始下载：${d.fileName}`, 'ok')
  } catch (e) {
    error.value = (e as Error).message
    uiStore.toast('下载失败：' + error.value, 'warn')
  }
}

function copySha256(sha: string) {
  navigator.clipboard.writeText(sha)
  uiStore.toast('已复制原件 SHA-256 数字指纹', 'ok')
}

defineExpose({ load, documents })
</script>

<template>
  <div class="evidence-root">
    <!-- 顶部状态栏与工具条 -->
    <header class="evidence-toolbar card">
      <div class="toolbar-main">
        <div class="toolbar-title-group">
          <div class="toolbar-icon">
            <DemoIcon :name="changeRequestId ? 'clipboard-check' : 'archive'" :size="20" />
          </div>
          <div>
            <div class="title-with-badge">
              <h3 class="toolbar-heading">{{ changeRequestId ? '变更证明依据与材料' : '资料档案库' }}</h3>
              <span class="badge muted-badge">{{ visible.length }} 份</span>
              <span v-if="totalSizeBytes" class="size-pill">{{ formatFileSize(totalSizeBytes) }}</span>
            </div>
            <p class="toolbar-sub">按原件永久留存并追加防篡改指纹，支持多级目录与分类索引</p>
          </div>
        </div>

        <div class="toolbar-actions">
          <div class="view-switch" role="tablist" aria-label="展示方式">
            <button
              class="view-btn"
              :class="{ active: viewMode === 'grid' }"
              type="button"
              title="卡片网格"
              @click="viewMode = 'grid'"
            >
              <DemoIcon name="boxes" :size="14" />
            </button>
            <button
              class="view-btn"
              :class="{ active: viewMode === 'table' }"
              type="button"
              title="表格列表"
              @click="viewMode = 'table'"
            >
              <DemoIcon name="file-spreadsheet" :size="14" />
            </button>
          </div>

          <button class="btn sm" type="button" :disabled="loading || busy" title="刷新资料" @click="load">
            <DemoIcon name="rotate-cw" :size="13" />
            刷新
          </button>

          <button
            v-if="!readOnly"
            class="btn sm primary upload-trigger-btn"
            type="button"
            @click="openUploadModal(selectedFolder)"
          >
            <DemoIcon name="upload" :size="13" />
            上传归档资料
          </button>
        </div>
      </div>

      <!-- 搜索与多维度筛选栏 -->
      <div class="filter-strip">
        <div class="search-input-wrap">
          <DemoIcon name="search" :size="14" class="search-icon" />
          <input
            v-model="search"
            class="inp filter-search"
            placeholder="搜索资料标题、文件名、说明或目录…"
            aria-label="搜索资料"
          />
          <button v-if="search" class="clear-search-btn" type="button" @click="search = ''">
            <DemoIcon name="x" :size="12" />
          </button>
        </div>

        <div class="filter-controls">
          <div class="select-chip">
            <DemoIcon name="filter" :size="12" />
            <select v-model="filter" class="inline-select" aria-label="分类筛选">
              <option value="">全部分类 ({{ documents.length }})</option>
              <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
            </select>
          </div>

          <div v-if="folderOptions.length" class="select-chip">
            <DemoIcon name="folder" :size="12" />
            <select v-model="selectedFolder" class="inline-select" aria-label="资料目录筛选">
              <option value="">全部目录 ({{ folderOptions.length }})</option>
              <option v-for="folder in folderOptions" :key="folder" :value="folder">
                📁 {{ folder }}
              </option>
            </select>
          </div>
        </div>
      </div>

      <!-- 快捷目录面包屑/分类标签切换条 -->
      <div v-if="folderOptions.length" class="folder-pills-bar">
        <span class="pills-title">目录导航:</span>
        <div class="pills-scroll">
          <button
            class="folder-pill"
            :class="{ active: !selectedFolder }"
            type="button"
            @click="selectedFolder = ''"
          >
            全部
          </button>
          <button
            v-for="folder in folderOptions"
            :key="folder"
            class="folder-pill"
            :class="{ active: selectedFolder === folder }"
            type="button"
            @click="selectedFolder = selectedFolder === folder ? '' : folder"
          >
            <DemoIcon name="folder" :size="11" />
            {{ folder }}
          </button>
        </div>
      </div>
    </header>

    <!-- 错误警告提示 -->
    <div v-if="error" class="card card-pad error-alert" role="alert">
      <DemoIcon name="alert-triangle" :size="18" />
      <div class="alert-content">
        <b>操作出现错误</b>
        <span>{{ error }}</span>
      </div>
      <button class="btn sm" type="button" @click="load">重试</button>
    </div>

    <!-- 加载中动画 -->
    <div v-if="loading" class="card card-pad loading-card">
      <DemoIcon name="rotate-cw" :size="24" class="spin-icon" />
      <span>正在读取资料档案与原件记录…</span>
    </div>

    <!-- 暂无数据空状态 -->
    <div v-else-if="!visible.length" class="card empty evidence-empty">
      <div class="empty-icon-wrap">
        <DemoIcon name="archive" :size="36" />
      </div>
      <div class="t">暂无符合条件的资料档案</div>
      <p v-if="search || filter || selectedFolder">当前筛选条件未匹配到资料，您可以重置过滤条件</p>
      <p v-else>这里记录与图号关联的合同、客户沟通、技术要求、验收证明等各种原始凭据</p>
      <div class="empty-actions">
        <button v-if="search || filter || selectedFolder" class="btn sm" type="button" @click="search = ''; filter = ''; selectedFolder = ''">
          清除筛选条件
        </button>
        <button v-if="!readOnly" class="btn sm primary" type="button" @click="openUploadModal(selectedFolder)">
          <DemoIcon name="upload" :size="13" />
          立即归档第一份资料
        </button>
      </div>
    </div>

    <!-- 视图：卡片网格 -->
    <div v-else-if="viewMode === 'grid'" class="evidence-grid">
      <article
        v-for="doc in visible"
        :key="doc.id"
        class="card card-pad evidence-card"
      >
        <div class="card-head-row">
          <div class="file-badge-icon" :title="doc.fileName">
            <DemoIcon :name="getFileIcon(doc.fileName)" :size="20" />
          </div>
          <div class="card-title-col">
            <h4 class="doc-title" :title="doc.title">{{ doc.title }}</h4>
            <div class="doc-tags">
              <span class="tag plain cat-tag">{{ doc.category }}</span>
              <span v-if="doc.folderPath" class="tag mute folder-tag" :title="doc.folderPath">
                <DemoIcon name="folder" :size="10" />
                {{ doc.folderPath }}
              </span>
            </div>
          </div>
        </div>

        <p v-if="doc.description" class="doc-desc" :title="doc.description">
          {{ doc.description }}
        </p>
        <p v-else class="doc-desc empty-desc">暂无补充说明</p>

        <!-- 元数据明细区 -->
        <div class="doc-meta-box">
          <div class="meta-line">
            <span class="meta-k">原始文件名</span>
            <span class="meta-v mono file-name-txt" :title="doc.fileName">{{ doc.fileName }}</span>
          </div>
          <div class="meta-row">
            <div class="meta-col">
              <span class="meta-k">文件大小</span>
              <span class="meta-v">{{ formatFileSize(doc.size) }}</span>
            </div>
            <div class="meta-col">
              <span class="meta-k">上传归档人</span>
              <span class="meta-v">{{ doc.createdBy || '系统记录' }}</span>
            </div>
          </div>
          <div class="meta-line">
            <span class="meta-k">归档时间</span>
            <span class="meta-v mono text-time">{{ formatReadableDateTime(doc.createdAt) }}</span>
          </div>
        </div>

        <!-- SHA-256 与 变更关联折叠 -->
        <details class="evidence-details">
          <summary>
            <span class="summary-label">
              <DemoIcon name="shield-check" :size="12" />
              防篡改与证书信息
            </span>
          </summary>
          <div class="details-body">
            <div class="detail-item">
              <span class="detail-label">档案编号</span>
              <span class="mono detail-val">{{ doc.id }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">SHA-256 数字指纹</span>
              <div class="sha-container">
                <code class="sha-code">{{ doc.sha256 }}</code>
                <button
                  class="btn sm icon-only-btn"
                  type="button"
                  title="复制 SHA-256"
                  @click="copySha256(doc.sha256)"
                >
                  <DemoIcon name="copy" :size="12" />
                </button>
              </div>
            </div>
            <div v-if="doc.changeRequestId" class="detail-item">
              <span class="detail-label">关联变更工单</span>
              <span class="mono detail-val">{{ doc.changeRequestId }}</span>
            </div>
          </div>
        </details>

        <!-- 底部下载操作栏 -->
        <footer class="card-footer">
          <button class="btn sm primary download-btn" type="button" @click="download(doc)">
            <DemoIcon name="download" :size="13" />
            下载原始文件
          </button>
        </footer>
      </article>
    </div>

    <!-- 视图：表格列表 -->
    <div v-else class="card table-card">
      <div class="table-scroll">
        <table class="tbl evidence-table">
          <thead>
            <tr>
              <th style="width: 220px;">资料标题</th>
              <th style="width: 110px;">分类</th>
              <th style="width: 130px;">目录路径</th>
              <th>原始文件名</th>
              <th style="width: 90px;">文件大小</th>
              <th style="width: 100px;">归档人</th>
              <th style="width: 150px;">归档时间</th>
              <th style="width: 110px; text-align: center;">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="doc in visible" :key="doc.id">
              <td>
                <div class="table-title-cell">
                  <div class="table-icon">
                    <DemoIcon :name="getFileIcon(doc.fileName)" :size="15" />
                  </div>
                  <div>
                    <strong class="tbl-doc-title" :title="doc.title">{{ doc.title }}</strong>
                    <small v-if="doc.description" class="tbl-desc" :title="doc.description">
                      {{ doc.description }}
                    </small>
                  </div>
                </div>
              </td>
              <td><span class="tag plain">{{ doc.category }}</span></td>
              <td>
                <span v-if="doc.folderPath" class="folder-chip" :title="doc.folderPath">
                  📁 {{ doc.folderPath }}
                </span>
                <span v-else class="empty-muted">—</span>
              </td>
              <td class="mono tbl-filename" :title="doc.fileName">{{ doc.fileName }}</td>
              <td class="mono">{{ formatFileSize(doc.size) }}</td>
              <td>{{ doc.createdBy || '系统' }}</td>
              <td class="mono text-time">{{ formatReadableDateTime(doc.createdAt) }}</td>
              <td class="row-actions">
                <button class="btn sm" type="button" title="下载原件" @click="download(doc)">
                  <DemoIcon name="download" :size="12" />
                  下载
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 现代化上传归档弹窗 Modal -->
    <div v-if="isUploadModalOpen" class="modal-backdrop" @click.self="closeUploadModal">
      <div class="modal-card card">
        <header class="modal-head">
          <div class="modal-title-wrap">
            <div class="modal-title-icon">
              <DemoIcon name="file-up" :size="18" />
            </div>
            <div>
              <h3>上传并归档资料</h3>
              <p>原件上传后将自动计算 SHA-256 数字指纹，永久安全留存</p>
            </div>
          </div>
          <button class="icon-btn" type="button" title="关闭" @click="closeUploadModal">
            <DemoIcon name="x" :size="16" />
          </button>
        </header>

        <form class="modal-body" @submit.prevent="upload">
          <!-- 现代化拖拽上传框 -->
          <div class="upload-zone-wrap">
            <input
              ref="input"
              type="file"
              class="hidden-file-input"
              @change="pick"
            />

            <div
              class="modern-dropzone"
              :class="{ 'is-dragging': isDragging, 'has-file': Boolean(file) }"
              @dragover.prevent="isDragging = true"
              @dragleave.prevent="isDragging = false"
              @drop.prevent="onDrop"
              @click="input?.click()"
            >
              <template v-if="!file">
                <div class="dropzone-icon">
                  <DemoIcon name="file-up" :size="32" />
                </div>
                <div class="dropzone-text">
                  <b>点击选择文件 或 将原始文件拖拽至此</b>
                  <p>支持 PDF、Word、Excel、图纸(DWG/EXB)、3D 模型、图片、压缩包等全部格式，单文件上限 100 MB</p>
                </div>
              </template>

              <template v-else>
                <div class="dropzone-file-card" @click.stop>
                  <div class="file-card-left">
                    <div class="file-icon-box">
                      <DemoIcon :name="getFileIcon(file.name)" :size="24" />
                    </div>
                    <div class="file-info-text">
                      <b class="name" :title="file.name">{{ file.name }}</b>
                      <span class="size">{{ formatFileSize(file.size) }} · 准备就绪</span>
                    </div>
                  </div>
                  <div class="file-card-right">
                    <button class="btn sm" type="button" @click="input?.click()">重新选择</button>
                    <button class="btn sm danger" type="button" @click="removeSelectedFile">移除</button>
                  </div>
                </div>
              </template>
            </div>
          </div>

          <!-- 表单元数据填写区 -->
          <div class="form-grid">
            <div class="form-field">
              <label class="field-label">
                资料分类 <span class="req">*</span>
              </label>
              <select v-model="category" class="inp" required>
                <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
              </select>
            </div>

            <div class="form-field">
              <label class="field-label">
                资料标题 <span class="req">*</span>
              </label>
              <input
                v-model="title"
                class="inp"
                required
                maxlength="200"
                placeholder="例如：客户第二轮设计确认图及签收单"
              />
            </div>

            <div class="form-field full-width">
              <label class="field-label">
                存放目录路径（支持多级，例如：技术方案/评审纪要/2026）
              </label>
              <div class="input-with-datalist">
                <input
                  v-model="folderPath"
                  class="inp"
                  :list="folderListId"
                  placeholder="可输入新目录或从已有目录中选择"
                />
                <datalist :id="folderListId">
                  <option v-for="folder in folderOptions" :key="folder" :value="folder" />
                </datalist>
              </div>
            </div>

            <div class="form-field full-width">
              <label class="field-label">
                说明 / 依据备忘 / 与旧资料的关系
              </label>
              <textarea
                v-model="description"
                class="inp"
                rows="3"
                placeholder="补充说明：本资料来源、会谈关键结论、或本次提交替代的旧版资料…"
              />
            </div>
          </div>

          <footer class="modal-footer">
            <div class="footer-note">
              <DemoIcon name="info" :size="13" />
              <span>归档后文件不可修改或覆盖，确保企业证据链的法律效力与可审计性。</span>
            </div>
            <div class="footer-actions">
              <button class="btn" type="button" :disabled="busy" @click="closeUploadModal">
                取消
              </button>
              <button
                class="btn primary"
                type="submit"
                :disabled="busy || !file || !title.trim()"
              >
                <DemoIcon v-if="busy" name="rotate-cw" :size="13" class="spin-icon" />
                <span>{{ busy ? '正在上传归档…' : '确认上传并归档' }}</span>
              </button>
            </div>
          </footer>
        </form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.evidence-root {
  display: flex;
  flex-direction: column;
  gap: 18px;
  width: 100%;
}

.evidence-toolbar {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px 20px;
  border-radius: var(--radius);
}

.toolbar-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.toolbar-title-group {
  display: flex;
  align-items: center;
  gap: 14px;
}

.toolbar-icon {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  flex-shrink: 0;
  border-radius: 12px;
  background: var(--accent-soft);
  color: var(--accent);
}

.title-with-badge {
  display: flex;
  align-items: center;
  gap: 10px;
}

.toolbar-heading {
  margin: 0;
  font-size: 16px;
  font-weight: 800;
  color: var(--text-1);
}

.size-pill {
  padding: 2px 8px;
  border-radius: 99px;
  background: var(--panel-2);
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.toolbar-sub {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 12px;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.view-switch {
  display: flex;
  gap: 2px;
  padding: 3px;
  border: 1px solid var(--line);
  border-radius: 9px;
  background: var(--panel);
}

.view-btn {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-2);
  cursor: pointer;
  transition: all 0.2s;
}

.view-btn.active {
  background: var(--accent);
  color: var(--accent-ink);
}

.filter-strip {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  padding-top: 12px;
  border-top: 1px solid var(--line);
}

.search-input-wrap {
  position: relative;
  flex: 1;
  min-width: 240px;
  display: flex;
  align-items: center;
}

.search-icon {
  position: absolute;
  left: 12px;
  color: var(--text-3);
  pointer-events: none;
}

.filter-search {
  padding-left: 36px;
  padding-right: 32px;
  height: 34px;
}

.clear-search-btn {
  position: absolute;
  right: 8px;
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
}

.filter-controls {
  display: flex;
  align-items: center;
  gap: 10px;
}

.select-chip {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 12px;
  border: 1px solid var(--line);
  border-radius: 9px;
  background: var(--panel-2);
  color: var(--text-2);
}

.inline-select {
  border: none;
  background: transparent;
  color: var(--text-1);
  font-size: 12.5px;
  outline: none;
  cursor: pointer;
}

.folder-pills-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 10px;
}

.pills-title {
  color: var(--text-3);
  font-size: 11.5px;
  white-space: nowrap;
}

.pills-scroll {
  display: flex;
  align-items: center;
  gap: 6px;
  overflow-x: auto;
  scrollbar-width: thin;
}

.folder-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border: 1px solid var(--line);
  border-radius: 99px;
  background: var(--panel-2);
  color: var(--text-2);
  font-size: 11.5px;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}

.folder-pill:hover {
  border-color: var(--accent);
  color: var(--text-1);
}

.folder-pill.active {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
}

.error-alert {
  display: flex;
  align-items: center;
  gap: 12px;
  border-color: var(--danger);
  background: rgb(248 113 113 / 8%);
  color: var(--danger);
}

.alert-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.loading-card,
.evidence-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 48px 24px;
  text-align: center;
  color: var(--text-3);
}

.empty-icon-wrap {
  display: grid;
  place-items: center;
  width: 64px;
  height: 64px;
  border-radius: 50%;
  background: var(--panel-2);
  color: var(--accent);
}

.empty-actions {
  display: flex;
  gap: 10px;
  margin-top: 8px;
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 网格视图 */
.evidence-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 16px;
}

.evidence-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  border-radius: var(--radius);
  transition: all 0.25s;
}

.evidence-card:hover {
  border-color: var(--accent);
  box-shadow: var(--shadow);
}

.card-head-row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.file-badge-icon {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
}

.card-title-col {
  flex: 1;
  min-width: 0;
}

.doc-title {
  margin: 0 0 6px;
  font-size: 14px;
  font-weight: 700;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.doc-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.cat-tag {
  font-size: 10.5px;
}

.folder-tag {
  font-size: 10.5px;
}

.doc-desc {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-2);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 36px;
}

.empty-desc {
  color: var(--text-3);
  font-style: italic;
}

.doc-meta-box {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--panel-2);
  font-size: 11.5px;
}

.meta-line {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.meta-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.meta-col {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.meta-k {
  color: var(--text-3);
  font-size: 10.5px;
}

.meta-v {
  color: var(--text-1);
  font-weight: 500;
}

.file-name-txt {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.evidence-details {
  border-top: 1px dashed var(--line);
  padding-top: 8px;
  font-size: 11.5px;
}

.evidence-details summary {
  cursor: pointer;
  color: var(--text-3);
  user-select: none;
}

.evidence-details summary:hover {
  color: var(--accent);
}

.summary-label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.details-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 10px;
  padding: 10px;
  border-radius: 8px;
  background: var(--panel);
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.detail-label {
  color: var(--text-3);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.detail-val {
  color: var(--text-2);
  font-size: 11px;
}

.sha-container {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--panel-2);
}

.sha-code {
  font-size: 10.5px;
  overflow-wrap: anywhere;
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-2);
}

.icon-only-btn {
  padding: 4px;
  min-height: auto;
}

.card-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px solid var(--line);
}

.download-btn {
  width: 100%;
  justify-content: center;
}

/* 表格视图 */
.table-card {
  padding: 0;
  overflow: hidden;
}

.table-scroll {
  overflow-x: auto;
}

.table-title-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.table-icon {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--accent);
  flex-shrink: 0;
}

.tbl-doc-title {
  display: block;
  font-size: 13px;
}

.tbl-desc {
  display: block;
  color: var(--text-3);
  font-size: 11px;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.folder-chip {
  display: inline-block;
  font-size: 11.5px;
  color: var(--text-2);
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tbl-filename {
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-muted {
  color: var(--text-3);
}

/* 现代化模态框 Modal */
.modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 2200;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgb(0 0 0 / 50%);
  backdrop-filter: blur(4px);
  animation: modal-fade 0.2s ease;
}

@keyframes modal-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

.modal-card {
  width: min(100%, 640px);
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  border-radius: 16px;
  background: var(--panel);
  box-shadow: 0 24px 60px rgb(0 0 0 / 30%);
  overflow: hidden;
}

.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 24px;
  border-bottom: 1px solid var(--line);
  background: var(--panel-top);
}

.modal-title-wrap {
  display: flex;
  align-items: center;
  gap: 12px;
}

.modal-title-icon {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
}

.modal-head h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
}

.modal-head p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.modal-body {
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 24px;
  overflow-y: auto;
}

.hidden-file-input {
  display: none;
}

.modern-dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 28px 20px;
  border: 2px dashed var(--line);
  border-radius: 12px;
  background: var(--panel-2);
  cursor: pointer;
  transition: all 0.25s;
}

.modern-dropzone:hover,
.modern-dropzone.is-dragging {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.modern-dropzone.has-file {
  padding: 12px;
  border-style: solid;
  border-color: var(--accent);
  background: var(--panel);
}

.dropzone-icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  border-radius: 50%;
  background: var(--panel);
  color: var(--accent);
  margin-bottom: 12px;
}

.dropzone-text {
  text-align: center;
}

.dropzone-text b {
  display: block;
  font-size: 13.5px;
  color: var(--text-1);
}

.dropzone-text p {
  margin: 6px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
  max-width: 440px;
  line-height: 1.5;
}

.dropzone-file-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 12px;
}

.file-card-left {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.file-icon-box {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
  flex-shrink: 0;
}

.file-info-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.file-info-text .name {
  font-size: 13px;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 320px;
}

.file-info-text .size {
  color: var(--text-3);
  font-size: 11px;
}

.file-card-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.full-width {
  grid-column: 1 / -1;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}

.req {
  color: var(--danger);
}

.input-with-datalist {
  position: relative;
}

.modal-footer {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-top: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--line);
}

.footer-note {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--text-3);
  font-size: 11.5px;
}

.footer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .toolbar-main {
    flex-direction: column;
    align-items: flex-start;
  }
  .filter-strip {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
