import { invoke } from '@tauri-apps/api/core'

const DEBUG_MODE_STORAGE_KEY = 'cad:debug-mode:v1'

function isTauriRuntime(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

export async function readDebugMode(): Promise<boolean> {
  if (isTauriRuntime()) {
    return invoke<boolean>('read_debug_mode')
  }
  return window.localStorage.getItem(DEBUG_MODE_STORAGE_KEY) === 'true'
}

export async function writeDebugMode(enabled: boolean): Promise<void> {
  if (isTauriRuntime()) {
    await invoke('write_debug_mode', { enabled })
    return
  }
  window.localStorage.setItem(DEBUG_MODE_STORAGE_KEY, String(enabled))
}
