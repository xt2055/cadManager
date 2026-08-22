import { invoke } from '@tauri-apps/api/core'

import { createEmptyDataDocument, normalizeDataDocument, type DataDocument } from './data.types'
import type { DataProvider } from './data-provider'

const DATA_STORAGE_KEY = 'cad:data-document:v1'

function isTauriRuntime(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

export class JsonDataProvider implements DataProvider {
  async load(): Promise<DataDocument> {
    if (isTauriRuntime()) {
      const document = await invoke<unknown | null>('read_data_document')
      return normalizeDataDocument(document ?? createEmptyDataDocument())
    }

    const raw = window.localStorage.getItem(DATA_STORAGE_KEY)
    if (!raw) {
      return createEmptyDataDocument()
    }

    try {
      return normalizeDataDocument(JSON.parse(raw))
    } catch (error) {
      throw new Error(`本地 JSON 数据解析失败：${error instanceof Error ? error.message : String(error)}`)
    }
  }

  async save(document: DataDocument): Promise<void> {
    if (isTauriRuntime()) {
      await invoke('write_data_document', { document })
      return
    }

    window.localStorage.setItem(DATA_STORAGE_KEY, JSON.stringify(document, null, 2))
  }
}
