<script setup lang="ts">
import { computed, onMounted, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import BatchDownloadModal from './components/BatchDownloadModal.vue'
import CaxaHelpModal from './components/CaxaHelpModal.vue'
import CreateDrawingModal from './components/CreateDrawingModal.vue'
import ReidentifyDrawingModal from './components/ReidentifyDrawingModal.vue'
import ReplaceDrawingModal from './components/ReplaceDrawingModal.vue'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useDrawingOperationsStore } from '@/stores/drawing-operations.store'
import { useReviewStore } from '@/stores/review.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import { versionDisplayLabel } from '@/modules/versioning/versioning-service'
import { formatFileSize } from './drawing-preview-format'
import { useDrawingBatchDownload } from './composables/useDrawingBatchDownload'
import { useDrawingBorrow } from './composables/useDrawingBorrow'
import { useDrawingCreation } from './composables/useDrawingCreation'
import { useDrawingEditSessions } from './composables/useDrawingEditSessions'
import { useDrawingFileHistory } from './composables/useDrawingFileHistory'
import { useDrawingFileReplacement } from './composables/useDrawingFileReplacement'
import { useDrawingFileUpload } from './composables/useDrawingFileUpload'
import { useDrawingPreviewContext } from './composables/useDrawingPreviewContext'
import { useDrawingReidentify } from './composables/useDrawingReidentify'

defineOptions({
  name: 'DrawingPreviewTab',
})

const router = useRouter()
const route = useRoute()
const drawingOperationsStore = useDrawingOperationsStore()
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const uiStore = useUiStore()
const authStore = useAuthStore()

const {
  currentItem,
  isAssembly,
  rootDrawingNo,
  allFiles,
  hasAssemblyFile,
  archivedProject,
  canCreateDrawing,
  canManageDrawingFiles,
  canEditFile,
  canDeleteFiles,
} = useDrawingPreviewContext()

// 上传与替换的 UI 状态各由自己的 composable 管理
const {
  assemblyInput,
  partInput,
  triggerUploadAssembly,
  triggerUploadPart,
  onAssemblyFileChange,
  onPartFilesChange,
} = useDrawingFileUpload({
  currentItem,
  rootDrawingNo,
  isAssembly,
  hasAssemblyFile,
  canManageDrawingFiles,
  onAssemblyUploaded: openBrowse,
})

const {
  replaceInput,
  isReplacing,
  targetReplaceFile,
  replaceReasonInput,
  selectedReplaceBlob,
  triggerReplace,
  onReplaceFileSelected,
  confirmReplace,
  cancelReplace,
} = useDrawingFileReplacement({ currentItem })

// 借用零件弹窗与多维度智能选型
const {
  isBorrowing,
  borrowSearchMode,
  projectSearchQuery,
  partSearchQuery,
  selectedSourceProjectNo,
  selectedSourcePartNo,
  borrowReasonInput,
  isSubmittingBorrow,
  candidateProjects,
  selectedProjectDetail,
  candidateParts,
  selectedPartDetail,
  isBorrowablePart,
  getProjectPartCount,
  getPartProjectName,
  openBorrowModal,
  cancelBorrowModal,
  selectProject,
  confirmBorrowPart,
} = useDrawingBorrow({
  currentItem,
  canManageDrawingFiles,
  archivedProject,
  onOpenChangeWorkOrder: () => {
    const drawingId = String(route.params.drawingId ?? currentItem.value?.no ?? '')
    if (!drawingId) return
    router.push({ name: 'drawing-changes', params: { drawingId } })
  },
})

function openBrowse(file: DrawingFile) {
  if (!currentItem.value) return
  router.push({
    name: 'drawing-viewer',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

function openOnlineEditor(file: DrawingFile) {
  if (!currentItem.value) return
  if (!file.storageKey) {
    uiStore.toast('该文件尚未保存物理存储，无法使用在线编辑器打开', 'warn')
    return
  }
  router.push({
    name: 'drawing-editor',
    params: { drawingId: currentItem.value.no },
    query: { fileId: file.id },
  })
}

const {
  editingFileId,
  readonlyFileId,
  projectActiveSessions,
  closingSessionIds,
  closedSessions,
  caxaHelpVisible,
  caxaHelpDetail,
  isSavingCaxaPath,
  getFileLockInfo,
  isFileLockedByOther,
  isFileEditingByMe,
  isHeartbeatFresh,
  copyEditLink,
  stopSession,
  relaunchEditor,
  relaunchEditorForFile,
  openReadonly,
  openEditor,
  closeCaxaHelpModal,
  pickAndSaveCaxa,
  openSystemDefaultApps,
} = useDrawingEditSessions({
  currentItem,
  files: allFiles,
  canEditFile,
})

const {
  isCreatingDrawing,
  creatingDrawing,
  drawingCreationError,
  newDrawingName,
  newDrawingNo,
  drawingCreationProgress,
  openCreateDrawing,
  closeCreateDrawing,
  createDrawing,
} = useDrawingCreation({
  canCreateDrawing,
  rootDrawingNo,
  allFiles,
  openEditor,
})

onMounted(() => {
  void Promise.all([drawingStore.load(), reviewStore.load()])
})

const { isHistoryUnread, openHistory } = useDrawingFileHistory({
  currentItem,
  onOpenHistory: (fileId) => {
    const item = currentItem.value
    if (!item) return
    router.push({
      name: 'drawing-file-history',
      params: { drawingId: item.no },
      query: { fileId },
    })
  },
})

async function handleDeleteFile(file: DrawingFile) {
  if (!currentItem.value) return
  uiStore.confirm(
    '删除图纸文件',
    `确定要删除图纸文件「${file.name}」吗？`,
    {
      confirmText: '删除',
      danger: true,
      onConfirm: () => doDeleteFile(file),
    },
  )
}

async function doDeleteFile(file: DrawingFile) {
  if (!currentItem.value) return
  try {
    const targetNo = file.partNo || file.drawingNo || currentItem.value.no
    if (file.role === 'other') {
      await drawingOperationsStore.deleteOtherFile(targetNo, file.id)
    } else {
      await drawingOperationsStore.deleteDrawingFile(targetNo, file.id)
    }
    await drawingStore.refresh()
    uiStore.toast(`已删除文件 ${file.name}`)
  } catch (error) {
    console.error('删除文件失败', error)
    uiStore.toast('删除文件失败，请重试', 'warn')
  }
}

// ===== 批量下载：选择文件与格式（EXB 原始 / DWG / PDF），zip 打包下载 =====
const {
  isDownloadOpen,
  downloadFormat,
  downloadFileIds,
  downloadCandidates,
  allDownloadSelected,
  candidateHasFormat,
  isDownloading,
  downloadProgress,
  openDownloadModal,
  closeDownloadModal,
  onFormatChange,
  toggleDownloadFile,
  toggleAllDownloadFiles,
  executeDownload,
} = useDrawingBatchDownload({
  allFiles,
  currentItem,
})

// 弹窗只接「纯展示数据」：候选行（含当前格式是否可用）在父页面投影，勾选 / 切格式 / 打包都是事件
const downloadRows = computed(() =>
  downloadCandidates.value.map((item) => ({
    id: item.file.id,
    name: item.file.name,
    sizeLabel: item.file.size,
    available: candidateHasFormat(item, downloadFormat.value),
    checked: downloadFileIds.value.has(item.file.id),
  })),
)

// 批量识别校正弹窗状态
const {
  isReidentifyingAll,
  isReidentifyModalOpen,
  reidentifyList,
  reidentifyFailures,
  isExecutingReidentify,
  reidentifyAllPartFiles,
  confirmBatchReidentify,
  closeReidentifyModal,
  toggleReidentifyItem,
  toggleAllReidentifyItems,
} = useDrawingReidentify({
  allFiles,
  rootDrawingNo,
  canManageDrawingFiles,
})

// 弹窗只接「纯展示数据」：投影与格式化留在父页面这一层，勾选 / 输入意图再由事件交回 composable
const reidentifyRows = computed(() =>
  reidentifyList.value.map((item) => ({
    id: item.file.id,
    name: item.file.name,
    oldPartNo: item.oldPartNo,
    newPartNo: item.newPartNo,
    checked: item.checked,
  })),
)

const replaceOriginalVersion = computed(() =>
  targetReplaceFile.value ? versionDisplayLabel(targetReplaceFile.value.version) : 'v1.0',
)

const replaceNewSize = computed(() => formatFileSize(selectedReplaceBlob.value?.size || 0))

</script>

<template>
  <div class="drawing-preview-view">
    <!-- 隐藏式文件选择框 -->
    <input
      ref="assemblyInput"
      type="file"
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onAssemblyFileChange"
    />
    <input
      ref="partInput"
      type="file"
      multiple
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onPartFilesChange"
    />
    <input
      ref="replaceInput"
      type="file"
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onReplaceFileSelected"
    />

    <!-- 顶部操作栏：规范的上传总图 / 零件图入口 -->
    <div class="preview-actions-header card card-pad">
      <div class="header-info">
        <DemoIcon name="layers" :size="18" />
        <div>
          <h3>图纸文件管理与在线浏览</h3>
          <p>先上传总图建立主框架，后上传关联零件图；点击任意文件「浏览」开启矢量画布控制。</p>
        </div>
      </div>

      <div class="header-buttons">
        <button v-if="canCreateDrawing && isAssembly" class="btn primary" type="button" @click="openCreateDrawing">
          <DemoIcon name="plus" :size="14" />新建图纸
        </button>
        <button class="btn" type="button" @click="router.push({ name: 'drawing-models', params: { drawingId: route.params.drawingId } })"><DemoIcon name="box" :size="14" />3D 图纸</button>
        <button v-if="currentItem && reviewStore.getCase(currentItem.no)?.changeSubmissionId" class="btn" type="button" @click="router.push({ name: 'drawing-compare', params: { drawingId: route.params.drawingId } })">图纸对比</button>
        <button class="btn" type="button" title="选择文件与格式（EXB / DWG / PDF），打包为 zip 下载" @click="openDownloadModal">
          <DemoIcon name="download" :size="14" />下载
        </button>
        <button v-if="canManageDrawingFiles" class="btn" type="button" title="从其他工程项目借用零件图及关联文件" @click="openBorrowModal">
          <DemoIcon name="share-2" :size="14" />借用零件
        </button>
        <button v-if="canManageDrawingFiles" class="btn primary" type="button" @click="triggerUploadAssembly">
          <DemoIcon name="upload" :size="14" />上传总图文件
        </button>
        <button
          v-if="canManageDrawingFiles"
          class="btn"
          :class="{ primary: hasAssemblyFile }"
          type="button"
          :disabled="!hasAssemblyFile && isAssembly"
          :title="!hasAssemblyFile && isAssembly ? '请先上传总图' : '上传零件图'"
          @click="triggerUploadPart"
        >
          <DemoIcon name="files" :size="14" />上传零件图
        </button>
      </div>
    </div>

    <!-- 本地 CAD 协同状态面板（支持多开协同编辑，置于表格上方） -->
    <div v-if="projectActiveSessions.length > 0" class="collab-multi-container">
      <div v-for="session in projectActiveSessions" :key="session.sessionId" class="card collab-dock-card">
        <div class="dock-left">
          <div class="dock-status-tag">
            <span class="pulse-dot"></span>
            <strong>本地协同编辑中</strong>
          </div>
          <div class="dock-file-info">
            <span class="file-name" :title="session.fileName">{{ session.fileName }}</span>
            <span class="dock-time">
              开始于 {{ session.startedAt }} ·
              <b :class="isHeartbeatFresh(session) ? 'hb-ok' : 'hb-lost'">{{ isHeartbeatFresh(session) ? '心跳正常' : '心跳检测中' }}</b>
              · {{ isHeartbeatFresh(session) ? '自动落盘与版本保护生效中' : '等待心跳确认...' }}
            </span>
          </div>
        </div>
        <div class="dock-actions">
          <button class="btn sm" type="button" title="在外部 CAD 或资源管理器中打开此共享路径" @click="copyEditLink(session.uncPath, '网络工作路径')">
            <DemoIcon name="copy" :size="13" />复制路径
          </button>
          <button class="btn sm" type="button" title="重新唤醒本地 CAXA CAD 程序" @click="relaunchEditor(session)">
            <DemoIcon name="external-link" :size="13" />呼出 CAXA
          </button>
          <button
            class="btn sm primary danger-tone"
            type="button"
            :disabled="closingSessionIds.has(session.sessionId)"
            title="结束当前编辑：等待图纸落盘后生成新版本并释放文件锁"
            @click="stopSession(session)"
          >
            <span v-if="closingSessionIds.has(session.sessionId)" class="local-edit-spinner" aria-hidden="true"></span>
            <DemoIcon v-else name="square" :size="12" />
            {{ closingSessionIds.has(session.sessionId) ? '正在结束，等待图纸落盘...' : '结束编辑' }}
          </button>
        </div>
      </div>
    </div>

    <!-- 结束成功反馈：后端完成版本捕获后短暂展示 -->
    <div v-if="closedSessions.length > 0" class="collab-multi-container">
      <div v-for="closed in closedSessions" :key="closed.sessionId" class="card collab-dock-card closed-ok">
        <div class="dock-left">
          <div class="dock-status-tag success">
            <DemoIcon name="check-circle-2" :size="15" />
            <strong>结束成功</strong>
          </div>
          <div class="dock-file-info">
            <span class="file-name" :title="closed.fileName">{{ closed.fileName }}</span>
            <span class="dock-time">{{ closed.savedAt }} · 图纸已落盘并生成新版本</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 文件总览列表 -->
    <div class="card files-table-card">
      <div class="card-title files-title-row">
        <DemoIcon name="file-text" :size="16" />
        已关联图纸文件清单 ({{ allFiles.length }})
        <span class="hint">支持 DWG / DXF / EXB / PDF / STEP</span>
        <button v-if="canManageDrawingFiles" class="btn sm" type="button" :disabled="isReidentifyingAll" title="按全部零件 CAD 文件名批量校正图号" @click="reidentifyAllPartFiles">
          <DemoIcon name="scan" :size="13" />
          {{ isReidentifyingAll ? '识别中...' : '全部重新识别图号' }}
        </button>
      </div>

      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>文件类型</th>
              <th>文件名</th>
               <th>版本</th>
               <th>上传人</th>
               <th style="width: 560px; text-align: right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="file in allFiles" :key="file.id" :class="{ 'row-editing': isFileEditingByMe(file), 'row-locked': isFileLockedByOther(file) }">
              <td>
                <span class="tag" :class="file.role === 'assembly' ? 'plain' : file.role === 'other' ? 'mute' : 'info'">
                  {{ file.role === 'assembly' ? '项目总图' : file.role === 'other' ? '其他文件' : '零件图' }}
                </span>
              </td>
              <td class="file-name-cell">
                <DemoIcon name="file-check-2" :size="16" />
                <div class="file-title-wrap">
                  <div class="file-title-text">
                    <b>{{ file.name }}</b>
                    <span class="file-owner">{{ file.role === 'assembly' ? `项目总图 · ${file.ownerNo}` : `所属零件 · ${file.ownerNo}${file.ownerName && file.ownerName !== file.ownerNo ? `（${file.ownerName}）` : ''}` }}</span>
                  </div>
                  <span v-if="isFileEditingByMe(file)" class="badge-collab active">
                    <span class="pulse-dot"></span>我正在编辑
                  </span>
                  <span v-else-if="getFileLockInfo(file)" class="badge-collab locked" :title="`由 ${getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount} 占用${getFileLockInfo(file)?.online === false ? '（离线）' : ''}`">
                    <DemoIcon name="lock" :size="11" />{{ getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount }} {{ getFileLockInfo(file)?.online === false ? '离线占用' : '编辑中' }}
                  </span>
                </div>
              </td>
              <td class="num"><span class="ver-badge" :title="file.version">{{ versionDisplayLabel(file.version) }}</span></td>
              <td>{{ file.uploadedBy }}</td>
              <td class="row-actions" style="text-align: right">
                <button class="btn sm primary" type="button" title="在线 CAD 矢量浏览" @click="openBrowse(file)">
                  <DemoIcon name="eye" :size="13" />浏览
                </button>
                <button
                  class="btn sm"
                  type="button"
                  :disabled="Boolean(readonlyFileId)"
                  title="下载临时只读副本到本机用 CAD 打开，关闭后自动销毁，不影响服务器数据"
                  @click="openReadonly(file)"
                >
                  <span v-if="readonlyFileId === file.id" class="local-edit-spinner" aria-hidden="true"></span>
                  <DemoIcon v-else name="eye" :size="13" />本地查看
                </button>
                <button
                  v-if="false && canEditFile(file)"
                  class="btn sm"
                  type="button"
                  title="使用网页 CAD 编辑器打开并编辑文件"
                  @click="openOnlineEditor(file)"
                >
                  <img class="editor-icon" src="/编辑.svg" alt="" aria-hidden="true" />在线编辑
                </button>

                <!-- 本地 CAD 编辑按钮三种状态：编辑中（绿色高亮）、被他人锁定（禁用锁止）、正常空闲；无权限直接隐藏 -->
                <button
                  v-if="isFileEditingByMe(file)"
                  class="btn sm success-btn"
                  type="button"
                  title="当前已在本地 CAD 中打开，点击呼出/重新聚焦"
                  @click="relaunchEditorForFile(file)"
                >
                  <span class="pulse-dot"></span>编辑中
                </button>
                <template v-else-if="canEditFile(file) && !isFileLockedByOther(file)">
                  <button
                    class="btn sm"
                    type="button"
                    :disabled="Boolean(editingFileId)"
                    title="使用本机 CAD 软件（如 CAXA）打开并协同编辑"
                    @click="openEditor(file)"
                  >
                    <span v-if="editingFileId === file.id" class="local-edit-spinner" aria-hidden="true"></span>
                    <img v-else class="editor-icon" src="/编辑.svg" alt="" aria-hidden="true" />{{ editingFileId === file.id ? '启动中...' : '本地编辑' }}
                  </button>
                </template>
                <button
                  v-else-if="isFileLockedByOther(file)"
                  class="btn sm locked-btn"
                  type="button"
                  :disabled="!getFileLockInfo(file)?.canClose || closingSessionIds.has(getFileLockInfo(file)!.id)"
                  :title="getFileLockInfo(file)?.canClose
                    ? `文件正由「${getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount}」占用，可强制释放（将尝试保存其改动并生成版本）`
                    : `文件正由「${getFileLockInfo(file)?.userName || getFileLockInfo(file)?.userAccount}」独占编辑中`"
                  @click="getFileLockInfo(file) && stopSession(getFileLockInfo(file)!)"
                >
                  <span v-if="getFileLockInfo(file)?.canClose && closingSessionIds.has(getFileLockInfo(file)!.id)" class="local-edit-spinner" aria-hidden="true"></span>
                  <DemoIcon v-else name="lock" :size="12" />
                  {{ getFileLockInfo(file)?.canClose ? (getFileLockInfo(file)?.online === false ? '强制释放(离线)' : '强制释放') : (getFileLockInfo(file)?.online === false ? '已被占用(离线)' : '已被占用') }}
                </button>

                <button
                  v-if="canEditFile(file)"
                  class="btn sm"
                  type="button"
                  title="替换当前图纸文件并生成新版本"
                  @click="triggerReplace(file)"
                >
                  <DemoIcon name="refresh-cw" :size="13" />替换
                </button>
                <button class="btn sm history-action" type="button" title="查看该文件所有历史版本树与演进" @click="openHistory(file)">
                  <DemoIcon name="history" :size="13" />历史
                  <span v-if="isHistoryUnread(file)" class="hist-count">{{ file.history?.length }}</span>
                </button>
                <button
                  v-if="canDeleteFiles"
                  class="btn sm danger"
                  type="button"
                  title="删除文件"
                  @click="handleDeleteFile(file)"
                >
                  <DemoIcon name="trash-2" :size="13" />删除
                </button>
              </td>
            </tr>
            <tr v-if="!allFiles.length">
              <td colspan="6">
                <div class="empty">
                  <DemoIcon name="file-up" :size="36" />
                  <div class="t">尚未上传任何图纸文件</div>
                  <p>请点击上方「上传总图文件」开始建立工程档案</p>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 批量重新识别并校正图号弹窗：DOM 与外壳样式都在组件内，勾选意图回抛给父页面 -->
    <ReidentifyDrawingModal
      v-if="isReidentifyModalOpen"
      :rows="reidentifyRows"
      :failures="reidentifyFailures"
      :executing="isExecutingReidentify"
      @close="closeReidentifyModal"
      @confirm="confirmBatchReidentify"
      @toggle="toggleReidentifyItem"
      @toggle-all="toggleAllReidentifyItems"
    />

    <!-- 本机未找到 CAXA：提供可操作的解决途径，而不是一闪而过的提示 -->
    <CaxaHelpModal
      v-if="caxaHelpVisible"
      :detail="caxaHelpDetail"
      :saving="isSavingCaxaPath"
      @close="closeCaxaHelpModal"
      @pick="pickAndSaveCaxa"
      @open-system-apps="openSystemDefaultApps"
    />

    <!-- 替换图纸确认与说明弹窗：展示串由父页面备好，替换原因经 update:reason 回抛 -->
    <ReplaceDrawingModal
      v-if="isReplacing"
      v-model:reason="replaceReasonInput"
      :original-name="targetReplaceFile?.name ?? ''"
      :original-version="replaceOriginalVersion"
      :new-name="selectedReplaceBlob?.name ?? ''"
      :new-size="replaceNewSize"
      @close="cancelReplace"
      @confirm="confirmReplace"
    />
    <!-- 批量下载弹窗：候选行与格式在父页面投影，勾选 / 切格式 / 打包都是事件 -->
    <BatchDownloadModal
      v-if="isDownloadOpen"
      :format="downloadFormat"
      :rows="downloadRows"
      :all-selected="allDownloadSelected"
      :busy="isDownloading"
      :progress="downloadProgress"
      @close="closeDownloadModal"
      @confirm="executeDownload"
      @format-change="onFormatChange"
      @toggle="toggleDownloadFile"
      @toggle-all="toggleAllDownloadFiles"
    />
    <!-- 新建图纸弹窗：Teleport 已在组件内部，父页面不再需要为 scope id 替它包一层 -->
    <CreateDrawingModal
      v-if="isCreatingDrawing"
      v-model:name="newDrawingName"
      v-model:drawing-no="newDrawingNo"
      :creating="creatingDrawing"
      :progress="drawingCreationProgress"
      :error="drawingCreationError"
      @close="closeCreateDrawing"
      @submit="createDrawing"
    />
    <!-- 借用其他项目零件弹窗 (支持千级项目/海量零件双模智能选型体系) -->
    <div v-if="isBorrowing" class="modal-backdrop">
      <div class="modal card borrow-modal">
        <div class="modal-head">
          <div class="modal-title">
            <DemoIcon name="share-2" :size="18" />
            <span>跨工程项目借用零件与图纸</span>
          </div>
          <div class="modal-mode-tabs">
            <button
              class="mode-tab-btn"
              :class="{ active: borrowSearchMode === 'by-project' }"
              type="button"
              @click="borrowSearchMode = 'by-project'"
            >
              <DemoIcon name="folder" :size="13" />按工程项目选型
            </button>
            <button
              class="mode-tab-btn"
              :class="{ active: borrowSearchMode === 'global-part' }"
              type="button"
              @click="borrowSearchMode = 'global-part'"
            >
              <DemoIcon name="search" :size="13" />全库全局穿透搜索
            </button>
          </div>
          <button class="btn sm close-btn" type="button" @click="cancelBorrowModal">✕</button>
        </div>

        <div class="modal-body borrow-modal-body">
          <!-- 模式 1：按工程项目三栏分级导航与选型（专为几千个项目设计） -->
          <div v-if="borrowSearchMode === 'by-project'" class="borrow-three-grid">
            <!-- 栏 1：项目库快速检索与选择 -->
            <div class="borrow-panel-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="folder" :size="13" />工程项目库 ({{ candidateProjects.length }})</span>
              </div>
              <div class="search-input-wrap compact">
                <DemoIcon name="search" :size="13" />
                <input
                  v-model="projectSearchQuery"
                  type="text"
                  class="inp filter-inp"
                  placeholder="搜索项目名称/图号/厂商..."
                />
              </div>
              <div class="scroll-select-list">
                <div
                  v-for="proj in candidateProjects"
                  :key="proj.no"
                  class="project-item-card"
                  :class="{ active: (selectedSourceProjectNo || candidateProjects[0]?.no) === proj.no }"
                  @click="selectProject(proj.no)"
                >
                  <div class="proj-card-title">{{ proj.name }}</div>
                  <div class="proj-card-meta">
                    <span class="mono">{{ proj.no }}</span>
                    <span class="tag tag-no-dot plain tag-xs">{{ getProjectPartCount(proj.no) }} 个零件</span>
                  </div>
                </div>
                <div v-if="!candidateProjects.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="20" />
                  <span>未找到匹配的项目</span>
                </div>
              </div>
            </div>

            <!-- 栏 2：当前项目下的零件列表 -->
            <div class="borrow-panel-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="file" :size="13" />可选零件清单 ({{ candidateParts.length }})</span>
                <span class="col-badge mono">{{ selectedProjectDetail?.no }}</span>
              </div>
              <div class="search-input-wrap compact">
                <DemoIcon name="filter" :size="13" />
                <input
                  v-model="partSearchQuery"
                  type="text"
                  class="inp filter-inp"
                  placeholder="过滤零件图号/名称/材质..."
                />
              </div>
              <div class="scroll-select-list">
                <div
                  v-for="part in candidateParts"
                  :key="part.no"
                  class="part-candidate-card"
                  :class="{ active: selectedSourcePartNo === part.no }"
                  @click="selectedSourcePartNo = part.no"
                >
                  <div class="part-card-head">
                    <span class="part-name">{{ part.name }}</span>
                    <span class="tag tag-no-dot mono tag-xs">{{ part.no }}</span>
                  </div>
                  <div class="part-card-sub">
                    <span>材质: {{ part.material || '—' }}</span>
                    <span>数量: {{ part.qty || 1 }}</span>
                    <span class="has-file-badge" :class="{ ok: part.files?.length }">
                      {{ part.files?.length ? `${part.files.length} 份图纸` : '无图纸' }}
                    </span>
                  </div>
                </div>
                <div v-if="!candidateParts.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="20" />
                  <span>该项目暂无匹配零件</span>
                </div>
              </div>
            </div>

            <!-- 栏 3：选中零件档案核对与借用理由 -->
            <div class="borrow-panel-col borrow-detail-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
              </div>

              <div v-if="selectedPartDetail" class="borrow-card-detail-content">
                <div class="preview-hero-card">
                  <DemoIcon name="file-check-2" :size="22" />
                  <div>
                    <h4>{{ selectedPartDetail.name }}</h4>
                    <span class="mono text-accent">{{ selectedPartDetail.no }}</span>
                  </div>
                </div>

                <div class="preview-spec-grid">
                  <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ selectedProjectDetail?.name }}</span></div>
                  <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPartDetail.partType }}</span></div>
                  <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPartDetail.material }} {{ selectedPartDetail.spec }}</span></div>
                  <div class="spec-row"><span class="k">单重/数量：</span><span class="v">{{ selectedPartDetail.weight ? `${selectedPartDetail.weight} kg` : '—' }} / {{ selectedPartDetail.qty }} 件</span></div>
                  <div class="spec-row"><span class="k">表面处理：</span><span class="v">{{ selectedPartDetail.surfaceTreatment || '—' }}</span></div>
                  <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPartDetail.files?.[0]?.name || '（无独立图纸文件）' }}</span></div>
                </div>

                <div class="field" style="margin-top: auto;">
                  <label class="bold">借用备注说明 / 选型原因</label>
                  <input
                    v-model="borrowReasonInput"
                    type="text"
                    class="inp"
                    placeholder="例如：复用成熟导向套设计，缩短加工周期"
                  />
                </div>
              </div>

              <div v-else class="empty-preview-prompt">
                <DemoIcon name="mouse-pointer-click" :size="32" />
                <p>请点击中间列表选择需要借用的零件</p>
              </div>
            </div>
          </div>

          <!-- 模式 2：全库全局穿透搜索（直击数万图纸） -->
          <div v-else class="borrow-two-grid">
            <div class="borrow-panel-col">
              <div class="search-input-wrap">
                <DemoIcon name="search" :size="15" />
                <input
                  v-model="partSearchQuery"
                  type="text"
                  class="inp global-search-inp"
                  placeholder="在企业全库中穿透搜索：输入图号如 04、活塞、HT200、或项目名称..."
                  autofocus
                />
              </div>

              <div class="scroll-select-list global-list" style="margin-top: 10px;">
                <div
                  v-for="part in candidateParts"
                  :key="part.no"
                  class="global-part-card"
                  :class="{ active: selectedSourcePartNo === part.no }"
                  @click="selectedSourcePartNo = part.no; selectedSourceProjectNo = part.parentNo"
                >
                  <div class="gp-top">
                    <span class="gp-name">{{ part.name }}</span>
                    <span class="mono gp-no">{{ part.no }}</span>
                    <span class="tag info tag-xs">{{ getPartProjectName(part) }}</span>
                  </div>
                  <div class="gp-btm">
                    <span>材质: {{ part.material || '—' }}</span>
                    <span>数量: {{ part.qty || 1 }}</span>
                    <span>图纸: {{ part.files?.[0]?.name || '无文件' }}</span>
                  </div>
                </div>
                <div v-if="!candidateParts.length" class="empty-list-prompt">
                  <DemoIcon name="search-x" :size="24" />
                  <span>全库中未找到匹配的零件，请尝试更简短的关键词</span>
                </div>
              </div>
            </div>

            <!-- 右侧详情 -->
            <div class="borrow-panel-col borrow-detail-col">
              <div class="col-header">
                <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
              </div>

              <div v-if="selectedPartDetail" class="borrow-card-detail-content">
                <div class="preview-hero-card">
                  <DemoIcon name="file-check-2" :size="22" />
                  <div>
                    <h4>{{ selectedPartDetail.name }}</h4>
                    <span class="mono text-accent">{{ selectedPartDetail.no }}</span>
                  </div>
                </div>

                <div class="preview-spec-grid">
                  <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ getPartProjectName(selectedPartDetail) }}</span></div>
                  <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPartDetail.partType }}</span></div>
                  <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPartDetail.material }} {{ selectedPartDetail.spec }}</span></div>
                  <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPartDetail.files?.[0]?.name || '（无文件）' }}</span></div>
                </div>

                <div class="field" style="margin-top: auto;">
                  <label class="bold">借用备注说明</label>
                  <input
                    v-model="borrowReasonInput"
                    type="text"
                    class="inp"
                    placeholder="输入借用说明..."
                  />
                </div>
              </div>

              <div v-else class="empty-preview-prompt">
                <DemoIcon name="mouse-pointer-click" :size="32" />
                <p>请在搜索结果中点击选定要借用的零件</p>
              </div>
            </div>
          </div>

          <div class="note info-note" style="margin-top: 12px;">
            <DemoIcon name="shield-check" :size="14" />
            <div>系统将自动克隆图纸零件并挂载至当前工程，在借用记录台账中建立双向可追溯凭据，不污染源工程。</div>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" type="button" :disabled="isSubmittingBorrow" @click="cancelBorrowModal">取消</button>
          <button
            class="btn primary"
            type="button"
            :disabled="!selectedSourcePartNo || isSubmittingBorrow"
            @click="confirmBorrowPart"
          >
            <DemoIcon name="check" :size="14" />{{ isSubmittingBorrow ? '借入中...' : '确认借入此零件' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped src="./styles/modal-chrome.css"></style>

<style scoped>
/* 弹窗外壳（backdrop / head / title / close-btn / body / foot）与 .text-accent / .info-note
   都来自上一行的共享 modal-chrome；本文件不再对子组件内部结构使用 :deep()。
   这里只留父页面自身的布局与「借用零件弹窗」（P5b 再抽成组件）的样式。 */
.ver-badge {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
  font-weight: 700;
  color: var(--accent);
  background: var(--panel-2);
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid var(--line);
}

.hist-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10.5px;
  font-weight: 700;
  min-width: 16px;
  height: 16px;
  border-radius: 8px;
  background: var(--accent);
  color: #fff;
  padding: 0 4px;
  margin-left: 2px;
}

.files-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 16px;
}

.files-title-row .hint {
  flex: 1;
  min-width: 180px;
}

.files-title-row .btn {
  flex: 0 0 auto;
  white-space: nowrap;
}

/* 借用零件高阶选型体系弹窗 */
.borrow-modal {
  width: 960px;
  max-width: 96vw;
  height: 640px;
  max-height: 92vh;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.modal-mode-tabs {
  display: flex;
  gap: 6px;
  background: var(--panel-2);
  padding: 3px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.mode-tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--text-3);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.mode-tab-btn:hover {
  color: var(--text-1);
}

.mode-tab-btn.active {
  background: var(--panel);
  color: var(--accent);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
}

.borrow-modal-body {
  flex: 1;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

/* 模式 1：三栏式自适应布局 */
.borrow-three-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 260px 310px 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

/* 模式 2：双栏穿透搜索布局 */
.borrow-two-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

.borrow-panel-col {
  display: flex;
  flex-direction: column;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 12px;
  min-height: 0;
  overflow: hidden;
}

.col-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.col-title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  display: flex;
  align-items: center;
  gap: 6px;
}

.col-title svg {
  color: var(--accent);
}

.col-badge {
  font-size: 11px;
  color: var(--text-3);
  background: var(--panel);
  padding: 1px 5px;
  border-radius: 3px;
  border: 1px solid var(--line);
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.compact {
  margin-top: 0 !important;
  margin-bottom: 8px;
}

.filter-inp {
  font-size: 12px !important;
  height: 30px !important;
  padding-left: 28px !important;
}

.global-search-inp {
  padding-left: 36px !important;
  height: 38px !important;
  font-size: 13px !important;
}

.scroll-select-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-right: 2px;
}

.project-item-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.project-item-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.project-item-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.proj-card-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.proj-card-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.part-candidate-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.part-candidate-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.part-candidate-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.part-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.part-name {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.part-card-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.has-file-badge {
  color: var(--text-3);
}

.has-file-badge.ok {
  color: var(--accent);
}

.global-part-card {
  padding: 10px 12px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.global-part-card:hover {
  background: var(--hover);
  border-color: var(--accent);
}

.global-part-card.active {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.gp-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.gp-name {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.gp-no {
  font-size: 12px;
  color: var(--text-2);
}

.gp-btm {
  display: flex;
  align-items: center;
  gap: 14px;
  font-size: 11.5px;
  color: var(--text-3);
}

.borrow-detail-col {
  background: var(--panel);
  padding: 14px;
}

.borrow-card-detail-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow-y: auto;
}

.preview-hero-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
}

.preview-hero-card svg {
  color: var(--accent);
}

.preview-hero-card h4 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.preview-spec-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--panel-2);
  padding: 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.spec-row {
  display: flex;
  align-items: center;
  font-size: 12.5px;
  gap: 6px;
}

.spec-row .k {
  color: var(--text-3);
  width: 75px;
  flex-shrink: 0;
}

.spec-row .v {
  color: var(--text-1);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
}

.empty-list-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px 10px;
  color: var(--text-3);
  gap: 6px;
  font-size: 12px;
}

.empty-preview-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--text-3);
  gap: 8px;
  font-size: 13px;
}

@media (max-width: 900px) {
  .borrow-three-grid {
    grid-template-columns: 1fr;
  }
  .borrow-two-grid {
    grid-template-columns: 1fr;
  }
}




.drawing-preview-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}

.drawing-preview-view > * {
  max-width: 100%;
  min-width: 0;
}

.hidden-file-input {
  display: none;
}

.preview-actions-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  flex-wrap: wrap;
}

.header-info {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1 1 auto;
}

.header-info svg {
  color: var(--accent);
}

.header-info h3 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.header-info p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-buttons {
  display: flex;
  flex: 0 1 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  min-width: 0;
}

.files-table-card {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}

.files-table-card .tbl {
  width: 100%;
  min-width: 0;
  table-layout: auto;
}

.files-table-card .tbl th,
.files-table-card .tbl td {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.files-table-card .tbl td.row-actions {
  padding-right: 18px;
  overflow: visible;
  text-overflow: clip;
}

.files-table-card .tbl thead th {
  position: sticky;
  top: 0;
  z-index: 3;
  padding-top: 13px;
  padding-bottom: 11px;
  background: var(--panel-top);
  background-clip: padding-box;
  box-shadow: inset 0 -1px 0 var(--line);
}

@media (max-width: 1280px) {
  .preview-actions-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .header-buttons {
    justify-content: flex-start;
    width: 100%;
  }
}

@media (max-width: 760px) {
  .files-table-card .tbl th:nth-child(2),
  .files-table-card .tbl td:nth-child(2) {
    min-width: 200px;
  }
}

.files-table-card .tbl th:nth-child(1),
.files-table-card .tbl td:nth-child(1) { width: 100px; }
.files-table-card .tbl th:nth-child(2),
.files-table-card .tbl td:nth-child(2) { width: 100%; }
.files-table-card .tbl th:nth-child(3),
.files-table-card .tbl td:nth-child(3) { width: 84px; }
.files-table-card .tbl th:nth-child(4),
.files-table-card .tbl td:nth-child(4) { width: 120px; }
.files-table-card .tbl th:nth-child(5),
.files-table-card .tbl td:nth-child(5) { width: 1%; white-space: nowrap; }

.file-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.file-title-text {
  display: grid;
  min-width: 0;
}

.file-title-text b {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-owner {
  margin-top: 2px;
  color: var(--muted);
  font-size: 12px;
}

.badge-collab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 1px 7px;
  border-radius: 99px;
  font-size: 10px;
  font-weight: 600;
  line-height: 16px;
  white-space: nowrap;
  flex: none;
}

.badge-collab.active {
  background: var(--ok-soft, rgba(34, 197, 94, 0.15));
  color: var(--ok, #16a34a);
  border: 1px solid rgba(34, 197, 94, 0.3);
}

.badge-collab.locked {
  background: var(--warn-soft, rgba(245, 158, 11, 0.15));
  color: var(--warn, #d97706);
  border: 1px solid rgba(245, 158, 11, 0.3);
}

.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ok, #22c55e);
  box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  animation: pulse-ring 1.8s infinite cubic-bezier(0.66, 0, 0, 1);
  flex: none;
}

@keyframes pulse-ring {
  0% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  }
  70% {
    box-shadow: 0 0 0 6px rgba(34, 197, 94, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0);
  }
}

.row-editing {
  background: var(--accent-soft, rgba(59, 130, 246, 0.05)) !important;
}

.row-locked {
  opacity: 0.88;
}

.success-btn {
  background: var(--ok-soft, rgba(34, 197, 94, 0.15)) !important;
  color: var(--ok, #16a34a) !important;
  border-color: rgba(34, 197, 94, 0.35) !important;
  font-weight: 600;
}

.dock-time .hb-ok {
  color: var(--ok, #16a34a);
}

.dock-time .hb-lost {
  color: var(--warn, #f59e0b);
}

.locked-btn {
  opacity: 0.6;
  cursor: not-allowed !important;
}

.collab-multi-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.collab-dock-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 18px;
  border: 1px solid var(--accent);
  background: linear-gradient(135deg, var(--panel) 0%, var(--panel-2) 100%);
  border-radius: 12px;
  box-shadow: 0 4px 16px -4px rgba(0, 0, 0, 0.1);
}

.dock-left {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.dock-status-tag {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 4px 10px;
  background: var(--ok-soft, rgba(34, 197, 94, 0.12));
  color: var(--ok, #16a34a);
  border-radius: 99px;
  font-size: 11.5px;
  font-weight: 700;
  white-space: nowrap;
  flex: none;
}

.dock-file-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.dock-file-info .file-name {
  color: var(--text-1);
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dock-file-info .dock-time {
  color: var(--text-3);
  font-size: 11px;
}

.dock-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

.danger-tone {
  background: var(--danger, #ef4444) !important;
  color: #fff !important;
  border-color: transparent !important;
}

.dock-fade-enter-active,
.dock-fade-leave-active {
  transition: all 0.25s ease;
}

.dock-fade-enter-from,
.dock-fade-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

.table-pad {
  width: 100%;
  min-width: 0;
  max-height: max(280px, calc(100vh - 420px));
  padding: 0 14px 10px;
  overflow: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--line-strong) transparent;
}

.table-pad::-webkit-scrollbar {
  width: 7px;
  height: 7px;
}

.table-pad::-webkit-scrollbar-thumb {
  background: var(--line-strong);
  border-radius: 8px;
}

.table-pad::-webkit-scrollbar-track {
  background: transparent;
}

.actions-heading {
  width: 10%;
  min-width: 0;
  text-align: right !important;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.file-name-cell svg {
  color: var(--accent);
}

.file-name-cell b {
  display: block;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
}

.row-actions {
  position: relative;
  min-width: 0;
  white-space: nowrap;
  text-align: right;
}

.row-actions .btn {
  display: inline-flex;
  min-width: 0;
  height: 28px;
  padding: 5px 8px;
  gap: 5px;
  font-size: 11px;
  white-space: nowrap;
  margin-left: 4px;
}

.row-actions .btn :deep(svg) {
  width: 14px;
  height: 14px;
}

.row-actions .hist-count {
  position: absolute;
  top: -7px;
  right: -5px;
  min-width: 13px;
  padding: 1px 3px;
  border-radius: 99px;
  background: var(--accent);
  color: var(--accent-ink);
  font-size: 9px;
  line-height: 12px;
  text-align: center;
}

.row-actions .history-action {
  position: relative;
}

.editor-icon {
  width: 14px;
  height: 14px;
  flex: none;
}

.local-edit-spinner {
  width: 13px;
  height: 13px;
  flex: none;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: local-edit-spin 0.75s linear infinite;
}

.dock-status-tag.success {
  color: var(--ok);
}

.dock-status-tag.success :deep(svg),
.dock-status-tag.success strong {
  color: var(--ok);
}

.closed-ok {
  border-color: rgb(52 211 153 / 40%);
  background: rgb(52 211 153 / 6%);
}

@keyframes local-edit-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .local-edit-spinner {
    animation-duration: 1.5s;
  }
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  color: var(--text-3);
  gap: 8px;
}

.empty .t {
  font-size: 14px;
  font-weight: 500;
  color: var(--text-2);
}

.empty p {
  font-size: 12px;
  margin: 0;
}
</style>
