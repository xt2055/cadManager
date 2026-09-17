<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import type { ProjectDrawingFile } from '../drawing-preview-files'

defineOptions({ name: 'DrawingFileTable' })

/**
 * 已关联图纸文件清单表格：文件类型 / 名称归属 / 版本 / 上传人 + 每行的全部操作入口与空状态。
 * 不引入 store / router / 业务 composable；权限与按行状态通过 props / callback 提供，
 * 行内动作只 emit 意图（父页面决定要不要真的执行）。
 */
interface FileLockView {
  id: string
  userName: string
  userAccount: string
  canClose?: boolean
  online?: boolean
}

defineProps<{
  files: ProjectDrawingFile[]
  /** 是否有编辑权限（原 canEditFile）。 */
  canEdit: (file: ProjectDrawingFile) => boolean
  /** 是否有删除权限（原 canDeleteFiles）。 */
  canDelete: boolean
  /** 是否有文件管理权限（原 canManageDrawingFiles）。 */
  canManageFiles: boolean
  /** 正在批量重新识别图号。 */
  reidentifying: boolean
  /** 正在本地只读打开的文件 id。 */
  readonlyFileId: string | null
  /** 正在启动本地编辑的文件 id。 */
  editingFileId: string | null
  /** 正在结束的会话 id 集合。 */
  closingIds: Set<string>
  isEditingByMe: (file: ProjectDrawingFile) => boolean
  isLockedByOther: (file: ProjectDrawingFile) => boolean
  /** 当前会话对该文件的占用信息（原 getFileLockInfo）。 */
  lockInfo: (file: ProjectDrawingFile) => FileLockView | undefined
  /** 版本展示串（原 versionDisplayLabel）。 */
  versionLabel: (version: string) => string
  /** 是否有未读历史（原 isHistoryUnread）。 */
  hasUnreadHistory: (file: ProjectDrawingFile) => boolean
}>()

const emit = defineEmits<{
  reidentifyAll: []
  browse: [file: ProjectDrawingFile]
  readonly: [file: ProjectDrawingFile]
  onlineEdit: [file: ProjectDrawingFile]
  edit: [file: ProjectDrawingFile]
  relaunch: [file: ProjectDrawingFile]
  forceRelease: [file: ProjectDrawingFile]
  replace: [file: ProjectDrawingFile]
  history: [file: ProjectDrawingFile]
  delete: [file: ProjectDrawingFile]
}>()
</script>

<template>
  <div class="card files-table-card">
    <div class="card-title files-title-row">
      <DemoIcon name="file-text" :size="16" />
      已关联图纸文件清单 ({{ files.length }})
      <span class="hint">支持 DWG / DXF / EXB / PDF / STEP</span>
      <button v-if="canManageFiles" class="btn sm" type="button" :disabled="reidentifying" title="按全部零件 CAD 文件名批量校正图号" @click="emit('reidentifyAll')">
        <DemoIcon name="scan" :size="13" />
        {{ reidentifying ? '识别中...' : '全部重新识别图号' }}
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
          <tr v-for="file in files" :key="file.id" :class="{ 'row-editing': isEditingByMe(file), 'row-locked': isLockedByOther(file) }">
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
                <span v-if="isEditingByMe(file)" class="badge-collab active">
                  <span class="pulse-dot"></span>我正在编辑
                </span>
                <span v-else-if="lockInfo(file)" class="badge-collab locked" :title="`由 ${lockInfo(file)?.userName || lockInfo(file)?.userAccount} 占用${lockInfo(file)?.online === false ? '（离线）' : ''}`">
                  <DemoIcon name="lock" :size="11" />{{ lockInfo(file)?.userName || lockInfo(file)?.userAccount }} {{ lockInfo(file)?.online === false ? '离线占用' : '编辑中' }}
                </span>
              </div>
            </td>
            <td class="num"><span class="ver-badge" :title="file.version">{{ versionLabel(file.version) }}</span></td>
            <td>{{ file.uploadedBy }}</td>
            <td class="row-actions" style="text-align: right">
              <button class="btn sm primary" type="button" title="在线 CAD 矢量浏览" @click="emit('browse', file)">
                <DemoIcon name="eye" :size="13" />浏览
              </button>
              <button
                class="btn sm"
                type="button"
                :disabled="Boolean(readonlyFileId)"
                title="下载临时只读副本到本机用 CAD 打开，关闭后自动销毁，不影响服务器数据"
                @click="emit('readonly', file)"
              >
                <span v-if="readonlyFileId === file.id" class="local-edit-spinner" aria-hidden="true"></span>
                <DemoIcon v-else name="eye" :size="13" />本地查看
              </button>
              <button
                v-if="false && canEdit(file)"
                class="btn sm"
                type="button"
                title="使用网页 CAD 编辑器打开并编辑文件"
                @click="emit('onlineEdit', file)"
              >
                <img class="editor-icon" src="/编辑.svg" alt="" aria-hidden="true" />在线编辑
              </button>

              <!-- 本地 CAD 编辑按钮三种状态：编辑中（绿色高亮）、被他人锁定（禁用锁止）、正常空闲；无权限直接隐藏 -->
              <button
                v-if="isEditingByMe(file)"
                class="btn sm success-btn"
                type="button"
                title="当前已在本地 CAD 中打开，点击呼出/重新聚焦"
                @click="emit('relaunch', file)"
              >
                <span class="pulse-dot"></span>编辑中
              </button>
              <template v-else-if="canEdit(file) && !isLockedByOther(file)">
                <button
                  class="btn sm"
                  type="button"
                  :disabled="Boolean(editingFileId)"
                  title="使用本机 CAD 软件（如 CAXA）打开并协同编辑"
                  @click="emit('edit', file)"
                >
                  <span v-if="editingFileId === file.id" class="local-edit-spinner" aria-hidden="true"></span>
                  <img v-else class="editor-icon" src="/编辑.svg" alt="" aria-hidden="true" />{{ editingFileId === file.id ? '启动中...' : '本地编辑' }}
                </button>
              </template>
              <button
                v-else-if="isLockedByOther(file)"
                class="btn sm locked-btn"
                type="button"
                :disabled="!lockInfo(file)?.canClose || closingIds.has(lockInfo(file)!.id)"
                :title="lockInfo(file)?.canClose
                  ? `文件正由「${lockInfo(file)?.userName || lockInfo(file)?.userAccount}」占用，可强制释放（将尝试保存其改动并生成版本）`
                  : `文件正由「${lockInfo(file)?.userName || lockInfo(file)?.userAccount}」独占编辑中`"
                @click="emit('forceRelease', file)"
              >
                <span v-if="lockInfo(file)?.canClose && closingIds.has(lockInfo(file)!.id)" class="local-edit-spinner" aria-hidden="true"></span>
                <DemoIcon v-else name="lock" :size="12" />
                {{ lockInfo(file)?.canClose ? (lockInfo(file)?.online === false ? '强制释放(离线)' : '强制释放') : (lockInfo(file)?.online === false ? '已被占用(离线)' : '已被占用') }}
              </button>

              <button
                v-if="canEdit(file)"
                class="btn sm"
                type="button"
                title="替换当前图纸文件并生成新版本"
                @click="emit('replace', file)"
              >
                <DemoIcon name="refresh-cw" :size="13" />替换
              </button>
              <button class="btn sm history-action" type="button" title="查看该文件所有历史版本树与演进" @click="emit('history', file)">
                <DemoIcon name="history" :size="13" />历史
                <span v-if="hasUnreadHistory(file)" class="hist-count">{{ file.history?.length }}</span>
              </button>
              <button
                v-if="canDelete"
                class="btn sm danger"
                type="button"
                title="删除文件"
                @click="emit('delete', file)"
              >
                <DemoIcon name="trash-2" :size="13" />删除
              </button>
            </td>
          </tr>
          <tr v-if="!files.length">
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
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 文件清单表格专属样式；全部来自本组件，与弹窗外壳无关。 */

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

.locked-btn {
  opacity: 0.6;
  cursor: not-allowed !important;
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


.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.file-name-cell :deep(svg) {
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
