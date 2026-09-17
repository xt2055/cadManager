<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import BatchDownloadModal from './components/BatchDownloadModal.vue'
import BorrowPartModal from './components/BorrowPartModal.vue'
import CaxaHelpModal from './components/CaxaHelpModal.vue'
import CreateDrawingModal from './components/CreateDrawingModal.vue'
import DrawingFileTable from './components/DrawingFileTable.vue'
import DrawingEditSessionPanel from './components/DrawingEditSessionPanel.vue'
import DrawingPreviewHeader from './components/DrawingPreviewHeader.vue'
import ReidentifyDrawingModal from './components/ReidentifyDrawingModal.vue'
import ReplaceDrawingModal from './components/ReplaceDrawingModal.vue'
import { useDrawingStore } from '@/stores/drawing.store'
import { useReviewStore } from '@/stores/review.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile } from '@/types/domain.types'
import { useCaxaLauncher } from './composables/useCaxaLauncher'
import { versionDisplayLabel } from '@/modules/versioning/versioning-service'
import { useDrawingFileDelete } from './composables/useDrawingFileDelete'
import { useDrawingPreviewViewModels } from './composables/useDrawingPreviewViewModels'
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
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const uiStore = useUiStore()

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
  getProjectPartCount,
  getPartProjectName,
  openBorrowModal,
  cancelBorrowModal,
  selectProject,
  selectPart,
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


/** 强制释放文件锁：行内动作只抛文件，占用会话在这里查（与原模板里的守卫一致）。 */
function releaseFileLock(file: DrawingFile) {
  const lock = getFileLockInfo(file)
  if (lock) void stopSession(lock)
}

// 本机 CAXA 启动兜底独立成 composable：会话 composable 只通过它处理「未找到 CAXA」
const {
  caxaHelpVisible,
  caxaHelpDetail,
  isSavingCaxaPath,
  handleCadOpenError,
  closeCaxaHelpModal,
  pickAndSaveCaxa,
  openSystemDefaultApps,
} = useCaxaLauncher()

const {
  editingFileId,
  readonlyFileId,
  projectActiveSessions,
  closingSessionIds,
  closedSessions,
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
} = useDrawingEditSessions({
  currentItem,
  files: allFiles,
  canEditFile,
  launcher: { handleCadOpenError },
})

function relaunchSessionById(sessionId: string) {
  const session = projectActiveSessions.value.find((item) => item.sessionId === sessionId)
  if (session) void relaunchEditor(session)
}

function stopSessionById(sessionId: string) {
  const session = projectActiveSessions.value.find((item) => item.sessionId === sessionId)
  if (session) void stopSession(session)
}


// 顶部操作栏只接可见性布尔与意图：跳转在这里做，权限与工单判断也留在这里
const canCompareCase = computed(() =>
  Boolean(currentItem.value && reviewStore.getCase(currentItem.value.no)?.changeSubmissionId),
)

function openModelsTab() {
  router.push({ name: 'drawing-models', params: { drawingId: route.params.drawingId } })
}

function openCompareTab() {
  router.push({ name: 'drawing-compare', params: { drawingId: route.params.drawingId } })
}
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

// 删除文件（确认 → 调 store → 刷新）与上传 / 替换同层，页面只留一个入口
const { confirmDeleteFile } = useDrawingFileDelete({ currentItem })

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




// 子组件只接展示数据：投影集中在一个纯映射 composable 里，页面只负责「composable → 投影 → 组件」的装配
const {
  borrowProjectCards,
  borrowPartCards,
  borrowSelectedProject,
  borrowSelectedPart,
  sessionRows,
  closedSessionRows,
  downloadRows,
  reidentifyRows,
  replaceOriginalVersion,
  replaceNewSize,
} = useDrawingPreviewViewModels({
  borrow: {
    candidateProjects,
    selectedProjectDetail,
    candidateParts,
    selectedPartDetail,
    getProjectPartCount,
    getPartProjectName,
  },
  sessions: { projectActiveSessions, closingSessionIds, closedSessions, isHeartbeatFresh },
  download: { candidates: downloadCandidates, format: downloadFormat, selectedIds: downloadFileIds, hasFormat: candidateHasFormat },
  reidentify: { list: reidentifyList },
  replacement: { targetFile: targetReplaceFile, selectedBlob: selectedReplaceBlob },
})
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

    <!-- 顶部操作栏：可见性与禁用态走 props，动作只 emit 意图（路由跳转留在父页面） -->
    <DrawingPreviewHeader
      :can-create-drawing="canCreateDrawing"
      :is-assembly="isAssembly"
      :can-compare="canCompareCase"
      :can-manage-files="canManageDrawingFiles"
      :has-assembly-file="hasAssemblyFile"
      @create="openCreateDrawing"
      @open3d="openModelsTab"
      @compare="openCompareTab"
      @download="openDownloadModal"
      @borrow="openBorrowModal"
      @upload-assembly="triggerUploadAssembly"
      @upload-part="triggerUploadPart"
    />

    <!-- 本地 CAD 协同状态面板：会话行与心跳文案由父页面投影，动作只抛 sessionId -->
    <DrawingEditSessionPanel
      v-if="sessionRows.length > 0 || closedSessionRows.length > 0"
      :active-sessions="sessionRows"
      :recently-closed="closedSessionRows"
      @copy-path="copyEditLink($event, '网络工作路径')"
      @relaunch="relaunchSessionById"
      @stop="stopSessionById"
    />

    <!-- 已关联图纸文件清单：权限与行内状态由父页面按 props / callback 提供，动作只 emit 意图 -->
    <DrawingFileTable
      :files="allFiles"
      :can-edit="canEditFile"
      :can-delete="canDeleteFiles"
      :can-manage-files="canManageDrawingFiles"
      :reidentifying="isReidentifyingAll"
      :readonly-file-id="readonlyFileId"
      :editing-file-id="editingFileId"
      :closing-ids="closingSessionIds"
      :is-editing-by-me="isFileEditingByMe"
      :is-locked-by-other="isFileLockedByOther"
      :lock-info="getFileLockInfo"
      :version-label="versionDisplayLabel"
      :has-unread-history="isHistoryUnread"
      @reidentify-all="reidentifyAllPartFiles"
      @browse="openBrowse"
      @readonly="openReadonly"
      @online-edit="openOnlineEditor"
      @edit="openEditor"
      @relaunch="relaunchEditorForFile"
      @force-release="releaseFileLock"
      @replace="triggerReplace"
      @history="openHistory"
      @delete="confirmDeleteFile"
    />

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
    <!-- 借用其他项目零件弹窗：三栏选型 / 全库穿透搜索都在组件内，检索词与选中项经事件回抛 -->
    <BorrowPartModal
      v-if="isBorrowing"
      v-model:mode="borrowSearchMode"
      v-model:project-query="projectSearchQuery"
      v-model:part-query="partSearchQuery"
      v-model:reason="borrowReasonInput"
      :projects="borrowProjectCards"
      :parts="borrowPartCards"
      :selected-project-no="selectedSourceProjectNo"
      :selected-project="borrowSelectedProject"
      :selected-part-no="selectedSourcePartNo"
      :selected-part="borrowSelectedPart"
      :submitting="isSubmittingBorrow"
      @close="cancelBorrowModal"
      @confirm="confirmBorrowPart"
      @select-project="selectProject"
      @select-part="selectPart"
    />
  </div>
</template>

<style scoped>
/* 页面外壳样式：顶部操作栏 / 协同面板 / 文件清单 / 各弹窗的样式都在各自组件里，
   这里只留预览页自身的纵向布局、子元素宽度约束与隐藏式 file input。 */

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
</style>
