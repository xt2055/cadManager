import { readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(fileURLToPath(new URL('..', import.meta.url)))
const target = resolve(root, 'src/features/drawings/detail-tabs/preview/DrawingPreviewTab.vue')
let source = readFileSync(target, 'utf8')

function replaceOnce(label, pattern, replacement) {
  const next = source.replace(pattern, replacement)
  if (next === source) throw new Error(`未找到待替换区域：${label}`)
  source = next
}

replaceOnce(
  'Vue imports',
  "import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'",
  "import { computed, onBeforeUnmount, onMounted, nextTick, ref, watch } from 'vue'",
)

replaceOnce(
  'editing service imports',
  "import { drawingFileService, editingService } from '@/app/container'\nimport type { ActiveEditSessionInfo, EditSessionOpenResult } from '@/types/application.types'",
  "import { drawingFileService } from '@/app/container'",
)

replaceOnce(
  'legacy editing imports',
  "import { CAXA_NOT_FOUND_PREFIX } from '@/modules/editing'\n",
  '',
)

replaceOnce(
  'session state imports',
  "import { getApiBaseUrl } from '@/services/api-base.service'\nimport { editSessionStorageKey, sessionsForFiles, isExpiredEditSession } from '@/modules/editing/session-state'\n",
  '',
)

replaceOnce(
  'composable import',
  "import { versionDisplayLabel } from '@/modules/versioning/versioning-service'",
  "import { versionDisplayLabel } from '@/modules/versioning/versioning-service'\nimport { useDrawingEditSessions } from './composables/useDrawingEditSessions'",
)

// 第一段：本地 session 缓存。边界卡在 borrowReasonInput 之前——那属于「借用图纸」弹窗状态，与本重构无关，误删会直接报 TS2304。
replaceOnce(
  'local session state block',
  /interface LocalActiveEditSession \{[\s\S]*?(?=const borrowReasonInput = ref\(''\)\n)/,
  '',
)

// 第二段：CAXA 打开失败处理与路径选择。
replaceOnce(
  'CAXA help block',
  /const caxaHelpVisible = ref\(false\)[\s\S]*?async function openSystemDefaultApps\(\) \{[\s\S]*?\n\}\n(?=\n\/\/ 获取除当前项目外的所有可选项目)/,
  '',
)

// 第三段：服务端会话轮询 / 锁 / 停止 / 重新呼出。权限块从“总图清单包含零件文件”开始，保留。
replaceOnce(
  'session commands block',
  /async function refreshActiveSessions\(\) \{[\s\S]*?async function relaunchEditorForFile\(file: DrawingFile\) \{[\s\S]*?\n\}\n\n(?=\/\/ 总图清单包含零件文件)/,
  '',
)

// 第四段：心跳展示 + 本地只读/编辑 + 会话 watch。生命周期由 composable 接管。
replaceOnce(
  'editor open block',
  /\/\/ 心跳新鲜度：[\s\S]*?watch\(\n  \(\) => currentItem\.value\?\.no,[\s\S]*?\n\)\n\n(?=onMounted\(\(\) => \{)/,
  '',
)

// 页面本身仍负责“创建图纸期间禁止关闭”和首次加载；session polling/heartbeat 已搬进 composable。
replaceOnce(
  'mounted session timers',
  /onMounted\(\(\) => \{\n  window\.addEventListener\('beforeunload', guardDrawingCreationUnload\)\n  void Promise\.all\(\[drawingStore\.load\(\), reviewStore\.load\(\)\]\)[\s\S]*?\n\}\)\n\n(?=onBeforeUnmount)/,
  "onMounted(() => {\n  window.addEventListener('beforeunload', guardDrawingCreationUnload)\n  void Promise.all([drawingStore.load(), reviewStore.load()])\n})\n\n",
)

replaceOnce(
  'unmounted session timers',
  /onBeforeUnmount\(\(\) => \{\n  window\.removeEventListener\('beforeunload', guardDrawingCreationUnload\)[\s\S]*?\n\}\)/,
  "onBeforeUnmount(() => {\n  window.removeEventListener('beforeunload', guardDrawingCreationUnload)\n})",
)

// 在权限计算完成后建立会话控制器。createDrawing() 虽定义更早，但只会在用户操作时执行，届时 openEditor 已初始化。
replaceOnce(
  'edit session controller',
  /(const canDeleteFiles = computed\(\(\) => \{[\s\S]*?\n\}\)\n)/,
  `$1\nconst {\n  editingFileId,\n  readonlyFileId,\n  projectActiveSessions,\n  closingSessionIds,\n  closedSessions,\n  caxaHelpVisible,\n  caxaHelpDetail,\n  isSavingCaxaPath,\n  getFileLockInfo,\n  isFileLockedByOther,\n  isFileEditingByMe,\n  isHeartbeatFresh,\n  copyEditLink,\n  stopSession,\n  relaunchEditor,\n  relaunchEditorForFile,\n  openReadonly,\n  openEditor,\n  closeCaxaHelpModal,\n  pickAndSaveCaxa,\n  openSystemDefaultApps,\n} = useDrawingEditSessions({\n  currentItem,\n  files: allFiles,\n  canEditFile,\n})\n\n`,
)

writeFileSync(target, source, 'utf8')
console.log('DrawingPreviewTab 编辑会话逻辑已抽取到 useDrawingEditSessions.ts')
console.log('下一步请执行：npm run type-check && npm run build')
