import { ref } from 'vue'

import { editingService } from '@/app/container'
import { CAXA_NOT_FOUND_PREFIX } from '@/modules/editing'
import { useUiStore } from '@/stores/ui.store'

/** 「未找到本机 CAXA」的处理入口，供会话 composable 注入使用。 */
export interface CadLauncherPort {
  handleCadOpenError: (error: unknown, retry: () => Promise<void>) => void
}

/**
 * 本机 CAD（CAXA）启动失败的兜底与帮助流程：识别「未找到 CAXA」→ 弹帮助弹窗并记住重试动作，
 * 其余错误直接 toast；帮助弹窗里的「选择程序」「打开系统默认应用」也在这里。
 * 会话本身（file lock / heartbeat / 落盘版本）不在本文件，见 useDrawingEditSessions。
 */
export function useCaxaLauncher() {
  const uiStore = useUiStore()

  const caxaHelpVisible = ref(false)
  const caxaHelpDetail = ref('')
  const isSavingCaxaPath = ref(false)
  let pendingCadRetry: (() => Promise<void>) | null = null

  function handleCadOpenError(error: unknown, retry: () => Promise<void>) {
    const message = error instanceof Error ? error.message : String(error)
    if (message.startsWith(CAXA_NOT_FOUND_PREFIX)) {
      caxaHelpDetail.value = message.slice(CAXA_NOT_FOUND_PREFIX.length)
      pendingCadRetry = retry
      caxaHelpVisible.value = true
      return
    }
    uiStore.toast(message, 'warn')
  }

  function closeCaxaHelpModal() {
    caxaHelpVisible.value = false
    pendingCadRetry = null
  }

  async function pickAndSaveCaxa() {
    if (isSavingCaxaPath.value) return
    isSavingCaxaPath.value = true
    try {
      const picked = await editingService.pickCaxa()
      if (!picked) return
      await editingService.saveCaxaPath(picked)
      uiStore.toast(`已记住本机 CAXA 程序：${picked}`, 'ok')
      caxaHelpVisible.value = false
      const retry = pendingCadRetry
      pendingCadRetry = null
      await retry?.()
    } catch (error) {
      uiStore.toast(error instanceof Error ? error.message : '保存 CAXA 路径失败', 'warn')
    } finally {
      isSavingCaxaPath.value = false
    }
  }

  async function openSystemDefaultApps() {
    try {
      await editingService.openDefaultApps()
      uiStore.toast('已打开系统「默认应用」设置，请为图纸扩展名配置打开方式', 'ok')
    } catch (error) {
      uiStore.toast(error instanceof Error ? error.message : '打开系统设置失败', 'warn')
    }
  }

  return {
    caxaHelpVisible,
    caxaHelpDetail,
    isSavingCaxaPath,
    handleCadOpenError,
    closeCaxaHelpModal,
    pickAndSaveCaxa,
    openSystemDefaultApps,
  }
}
