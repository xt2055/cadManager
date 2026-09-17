import { computed, onBeforeUnmount, onMounted, ref, watch, type ComputedRef } from 'vue'

import { editingService } from '@/app/container'
import { CAXA_NOT_FOUND_PREFIX } from '@/modules/editing'
import { editSessionStorageKey, isExpiredEditSession, sessionsForFiles } from '@/modules/editing/session-state'
import { getApiBaseUrl } from '@/services/api-base.service'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import type { ActiveEditSessionInfo } from '@/types/application.types'
import type { DrawingFile } from '@/types/domain.types'
import type { CadLauncherPort } from './useCaxaLauncher'

export interface LocalActiveEditSession {
  sessionId: string
  fileId: string
  fileName: string
  drawingNo: string
  storageKey?: string
  uncPath: string
  openUrl: string
  startedAt: string
  lastHeartbeatAt?: number
}

interface UseDrawingEditSessionsOptions {
  currentItem: ComputedRef<DrawingSummaryView | PartView | null>
  files: ComputedRef<readonly DrawingFile[]>
  canEditFile: (file: DrawingFile) => boolean
  /** CAXA 启动兜底（未找到本机 CAXA 时的弹窗与重试）：由 useCaxaLauncher 提供，本 composable 不再持有。 */
  launcher: CadLauncherPort
}

export function useDrawingEditSessions(options: UseDrawingEditSessionsOptions) {
  const authStore = useAuthStore()
  const drawingStore = useDrawingStore()
  const uiStore = useUiStore()

  const localSessionsStorageKey = computed(() => editSessionStorageKey(getApiBaseUrl(), authStore.currentUser?.id || ''))

  function loadLocalSessions(): LocalActiveEditSession[] {
    try {
      if (!authStore.currentUser?.id) return []
      const raw = window.localStorage.getItem(localSessionsStorageKey.value)
      const list = raw ? JSON.parse(raw) : []
      if (!Array.isArray(list)) return []
      return list.filter((item): item is LocalActiveEditSession => Boolean(item && item.sessionId && item.fileId))
    } catch {
      return []
    }
  }

  const editingFileId = ref<string | null>(null)
  const readonlyFileId = ref<string | null>(null)
  const activeSessionList = ref<ActiveEditSessionInfo[]>([])
  const myActiveSessions = ref<LocalActiveEditSession[]>(loadLocalSessions())
  const projectActiveSessions = computed(() => sessionsForFiles(myActiveSessions.value, options.files.value))
  const closingSessionIds = ref<Set<string>>(new Set())
  const closedSessions = ref<{ sessionId: string; fileName: string; savedAt: string }[]>([])

  let sessionPollTimer: number | null = null
  let heartbeatTimer: number | null = null

  function persistLocalSessions() {
    try {
      if (!authStore.currentUser?.id) return
      window.localStorage.setItem(localSessionsStorageKey.value, JSON.stringify(myActiveSessions.value))
    } catch {
      // 本地存储不可用时忽略：仅影响状态栏恢复。
    }
  }

  async function refreshActiveSessions() {
    const drawingNo = options.currentItem.value?.no
    const accountKey = localSessionsStorageKey.value
    if (!drawingNo) {
      activeSessionList.value = []
      return
    }

    try {
      const list = await editingService.listSessions(drawingNo)
      if (options.currentItem.value?.no !== drawingNo || localSessionsStorageKey.value !== accountKey) return
      activeSessionList.value = list

      // 只用当前图纸的服务端结果裁决当前图纸的本地缓存，避免切换项目时误删其他项目会话。
      const validIds = new Set(list.filter((session) => session.isCurrent).map((session) => session.id))
      const filtered = myActiveSessions.value.filter((session) => session.drawingNo !== drawingNo || validIds.has(session.sessionId))
      if (filtered.length !== myActiveSessions.value.length) {
        myActiveSessions.value = filtered
        persistLocalSessions()
      }
    } catch {
      // 协同状态轮询允许静默失败；下一轮会自动恢复。
    }
  }

  function getFileLockInfo(file: DrawingFile): ActiveEditSessionInfo | undefined {
    const originalKey = file.rawStorageKey || file.storageKey
    if (!originalKey) return undefined
    return activeSessionList.value.find((session) => session.attachmentId
      ? session.attachmentId === file.id
      : session.storageKey === originalKey || session.storageKey === file.storageKey)
  }

  function isFileLockedByOther(file: DrawingFile): boolean {
    const lock = getFileLockInfo(file)
    return Boolean(lock && !lock.isCurrent)
  }

  function isFileEditingByMe(file: DrawingFile): boolean {
    if (myActiveSessions.value.some((session) => session.fileId === file.id)) return true
    const lock = getFileLockInfo(file)
    return Boolean(lock && lock.isCurrent)
  }

  function isHeartbeatFresh(session: LocalActiveEditSession): boolean {
    return Boolean(session.lastHeartbeatAt && Date.now() - session.lastHeartbeatAt < 90_000)
  }

  async function copyEditLink(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value)
      uiStore.toast(`${label}已复制`, 'ok')
    } catch {
      uiStore.toast(`无法复制${label}，请手动选择文本`, 'warn')
    }
  }

  async function doStopSession(targetId: string, targetFileName: string) {
    if (closingSessionIds.value.has(targetId)) return
    closingSessionIds.value.add(targetId)
    try {
      const result = await editingService.closeSession(targetId)
      myActiveSessions.value = myActiveSessions.value.filter((session) => session.sessionId !== targetId)
      persistLocalSessions()
      await refreshActiveSessions()

      const successRecord = {
        sessionId: targetId,
        fileName: targetFileName,
        savedAt: new Date().toLocaleTimeString(),
      }
      closedSessions.value = [...closedSessions.value, successRecord]
      window.setTimeout(() => {
        closedSessions.value = closedSessions.value.filter((item) => item.sessionId !== targetId)
      }, 5000)

      if (result.changed && result.version) uiStore.toast(`编辑已结束，已保存新版本 ${result.version}`, 'ok')
      else uiStore.toast('编辑已结束，图纸无改动', 'ok')

      await drawingStore.refresh()
    } catch (error) {
      // 捕获失败时后端保留编辑会话；过期会话则清掉本地缓存，避免页面一直显示伪占用。
      if (isExpiredEditSession(error)) {
        myActiveSessions.value = myActiveSessions.value.filter((item) => item.sessionId !== targetId)
        persistLocalSessions()
        await refreshActiveSessions()
      }
      uiStore.toast(error instanceof Error ? error.message : '释放编辑会话失败', 'warn')
    } finally {
      closingSessionIds.value.delete(targetId)
    }
  }

  async function stopSession(session: ActiveEditSessionInfo | { sessionId: string }) {
    const targetId = 'id' in session ? session.id : session.sessionId
    if (!targetId || closingSessionIds.value.has(targetId)) return

    const targetFileName = ('fileName' in session && session.fileName)
      || myActiveSessions.value.find((item) => item.sessionId === targetId)?.fileName
      || '当前文件'

    uiStore.confirm(
      '结束本地编辑',
      `确定要结束「${targetFileName}」的编辑吗？\n结束后端会等待图纸落盘并自动生成新版本，通常需要数秒，请耐心等待。`,
      {
        confirmText: '结束编辑',
        danger: true,
        onConfirm: () => doStopSession(targetId, targetFileName),
      },
    )
  }

  async function relaunchEditor(session: LocalActiveEditSession) {
    const file = options.files.value.find((item) => item.id === session.fileId)
    const storageKey = session.storageKey || file?.rawStorageKey || file?.storageKey
    if (!storageKey) {
      uiStore.toast('本地会话缺少存储键，无法重新呼出；请改点文件行的「本地编辑」重新认领', 'warn')
      return
    }
    if (editingFileId.value) return

    editingFileId.value = session.fileId
    try {
      // 打开票据一次性消费；重新呼出时重新认领会话并签发新票据。
      const result = await editingService.openSession(storageKey, session.fileId)
      session.sessionId = result.sessionId
      session.openUrl = result.openUrl
      session.uncPath = result.uncPath
      session.lastHeartbeatAt = Date.now()
      persistLocalSessions()
      await editingService.openCad(result)
      uiStore.toast('已重新呼出本地 CAD', 'ok')
    } catch (error) {
      options.launcher.handleCadOpenError(error, () => relaunchEditor(session))
    } finally {
      editingFileId.value = null
    }
  }

  async function relaunchEditorForFile(file: DrawingFile) {
    const localSession = myActiveSessions.value.find((session) => session.fileId === file.id)
    if (localSession) {
      await relaunchEditor(localSession)
      return
    }

    const storageKey = file.rawStorageKey || file.storageKey
    if (!storageKey || editingFileId.value) return

    editingFileId.value = file.id
    try {
      const result = await editingService.openSession(storageKey, file.id)
      myActiveSessions.value = [
        ...myActiveSessions.value.filter((session) => session.sessionId !== result.sessionId),
        {
          sessionId: result.sessionId,
          fileId: file.id,
          fileName: file.name,
          drawingNo: options.currentItem.value?.no || '',
          storageKey,
          uncPath: result.uncPath,
          openUrl: result.openUrl,
          startedAt: new Date().toLocaleTimeString(),
          lastHeartbeatAt: Date.now(),
        },
      ]
      persistLocalSessions()
      await editingService.openCad(result)
      await refreshActiveSessions()
      uiStore.toast('已重新认领编辑会话并呼出本地 CAD', 'ok')
    } catch (error) {
      options.launcher.handleCadOpenError(error, () => relaunchEditorForFile(file))
    } finally {
      editingFileId.value = null
    }
  }

  async function openReadonly(file: DrawingFile) {
    const storageKey = file.rawStorageKey || file.storageKey
    if (!storageKey) {
      uiStore.toast('该文件尚未保存物理存储，无法本地查看', 'warn')
      return
    }
    if (readonlyFileId.value) return

    readonlyFileId.value = file.id
    try {
      await editingService.openReadonly({ storageKey })
      uiStore.toast(`已用本机 CAD 打开「${file.name}」（只读副本，关闭后自动销毁）`, 'ok')
    } catch (error) {
      options.launcher.handleCadOpenError(error, () => openReadonly(file))
    } finally {
      readonlyFileId.value = null
    }
  }

  async function openEditor(file: DrawingFile) {
    if (!options.currentItem.value || !options.canEditFile(file) || editingFileId.value) return
    if (!file.storageKey) {
      uiStore.toast('该文件尚未保存物理存储，无法使用本地 CAD 打开', 'warn')
      return
    }

    const lock = getFileLockInfo(file)
    if (lock && !lock.isCurrent) {
      uiStore.toast(`该图纸正由「${lock.userName || lock.userAccount}」编辑中，已被协同锁定`, 'warn')
      return
    }

    const existingLocalSession = myActiveSessions.value.find((session) => session.fileId === file.id)
    if (existingLocalSession) {
      await relaunchEditor(existingLocalSession)
      return
    }

    const ownServerSession = activeSessionList.value.find((session) => session.isCurrent && (session.attachmentId
      ? session.attachmentId === file.id
      : session.storageKey === file.rawStorageKey || session.storageKey === file.storageKey))
    if (ownServerSession) {
      await relaunchEditorForFile(file)
      return
    }

    editingFileId.value = file.id
    let sessionId: string | null = null
    try {
      const storageKey = file.rawStorageKey || file.storageKey
      const session = await editingService.openSession(storageKey, file.id)
      sessionId = session.sessionId
      window.localStorage.setItem('cad_last_edit_url', session.openUrl)

      myActiveSessions.value = [
        ...myActiveSessions.value.filter((item) => item.sessionId !== session.sessionId && item.fileId !== file.id),
        {
          sessionId: session.sessionId,
          fileId: file.id,
          fileName: file.name,
          drawingNo: options.currentItem.value?.no || '',
          storageKey,
          uncPath: session.uncPath,
          openUrl: session.openUrl,
          startedAt: new Date().toLocaleTimeString(),
          lastHeartbeatAt: Date.now(),
        },
      ]
      persistLocalSessions()

      await editingService.openCad(session)
      await refreshActiveSessions()
      uiStore.toast(`已在本地 CAD 中打开「${file.name}」，支持多开协同编辑`, 'ok')
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      // 找不到本机 CAXA 时保留服务端会话；其余启动失败尝试关闭会话，避免残留锁。
      if (sessionId && !message.startsWith(CAXA_NOT_FOUND_PREFIX)) {
        try {
          await editingService.closeSession(sessionId)
          myActiveSessions.value = myActiveSessions.value.filter((item) => item.sessionId !== sessionId)
          persistLocalSessions()
        } catch {
          // 保存失败时保留会话供用户重试。
        }
      }
      options.launcher.handleCadOpenError(error, () => openEditor(file))
    } finally {
      editingFileId.value = null
    }
  }

  watch(localSessionsStorageKey, () => {
    myActiveSessions.value = loadLocalSessions()
    activeSessionList.value = []
    void refreshActiveSessions()
  })

  watch(
    () => options.currentItem.value?.no,
    () => {
      activeSessionList.value = []
      void refreshActiveSessions()
    },
    { immediate: true },
  )

  onMounted(() => {
    sessionPollTimer = window.setInterval(() => {
      void refreshActiveSessions()
    }, 10_000)

    heartbeatTimer = window.setInterval(() => {
      const accountKey = localSessionsStorageKey.value
      for (const session of myActiveSessions.value) {
        void editingService.heartbeat(session.sessionId)
          .then(() => {
            session.lastHeartbeatAt = Date.now()
          })
          .catch((error) => {
            if (accountKey !== localSessionsStorageKey.value) return
            if (isExpiredEditSession(error)) {
              myActiveSessions.value = myActiveSessions.value.filter((item) => item.sessionId !== session.sessionId)
              persistLocalSessions()
              void refreshActiveSessions()
              return
            }
            console.warn(`会话 ${session.sessionId} 心跳失败`, error)
          })
      }
    }, 30_000)
  })

  onBeforeUnmount(() => {
    if (heartbeatTimer) {
      window.clearInterval(heartbeatTimer)
      heartbeatTimer = null
    }
    if (sessionPollTimer) {
      window.clearInterval(sessionPollTimer)
      sessionPollTimer = null
    }
  })

  return {
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
  }
}
