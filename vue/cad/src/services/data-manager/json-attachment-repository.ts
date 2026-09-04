import { invoke } from '@tauri-apps/api/core'
import { deleteBrowserAttachment, readBrowserAttachment } from './attachment-storage'
import type { StoredAttachment } from './data-provider'

const MODULE_STORAGE_PREFIX = 'cad:data-module:v1:'

function isTauriRuntime(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

/** JSON 调试模式的附件 Repository；浏览器与桌面文件细节留在这里。 */
export class JsonAttachmentRepository {
  public constructor(
    private readonly storage: Storage = window.localStorage,
  ) {}

  list(): StoredAttachment[] {
    return this.readModule('attachments', [])
  }

  async delete(storageKey: string): Promise<void> {
    if (isTauriRuntime()) {
      await invoke('delete_attachment', { storageKey })
      return
    }
    await deleteBrowserAttachment(storageKey)
  }

  async read(storageKey: string): Promise<Blob> {
    if (isTauriRuntime()) {
      const bytes = await invoke<number[]>('read_attachment', { storageKey })
      return new Blob([new Uint8Array(bytes)], { type: 'application/octet-stream' })
    }
    return readBrowserAttachment(storageKey)
  }

  private readModule<T>(name: string, fallback: T): T {
    const raw = this.storage.getItem(`${MODULE_STORAGE_PREFIX}${name}`)
    if (!raw) return fallback
    try {
      return JSON.parse(raw) as T
    } catch (error) {
      throw new Error(`本地附件模块解析失败：${error instanceof Error ? error.message : String(error)}`)
    }
  }
}
