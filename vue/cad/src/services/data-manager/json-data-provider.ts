import { invoke } from '@tauri-apps/api/core'

import { normalizeDataDocument, type DataDocument } from './data.types'
import { deleteBrowserAttachment, readBrowserAttachment, saveBrowserAttachment } from './attachment-storage'
import type { AttachmentMetadata, AttachmentResult, DataProvider } from './data-provider'
import seedDocument from './data.seed.json'

const DATA_STORAGE_KEY = 'cad:data-document:v2'
const LEGACY_DATA_STORAGE_KEY = 'cad:data-document:v1'

function safePathSegment(value: string): string {
  return value.replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'attachment'
}

function createStorageKey(name: string, metadata: AttachmentMetadata): string {
  const safeName = name.replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'attachment'
  const fileKey = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}-${safeName}`
  if (metadata.role === 'craft' && metadata.drawingNo) {
    return `${safePathSegment(metadata.drawingNo)}/工艺文件/${fileKey}`
  }
  return fileKey
}

function isTauriRuntime(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

function mergeSeedUsers(document: DataDocument): DataDocument {
  const seed = normalizeDataDocument(seedDocument)
  const users = [...document.users]
  for (const seedUser of seed.users) {
    const existingIndex = users.findIndex((user) => user.account.toLowerCase() === seedUser.account.toLowerCase())
    if (existingIndex < 0) {
      users.unshift(seedUser)
      continue
    }

    const existingUser = users[existingIndex]
    if (existingUser && !existingUser.password && seedUser.password) {
      users[existingIndex] = { ...existingUser, password: seedUser.password }
    }
  }
  return { ...document, users }
}

export class JsonDataProvider implements DataProvider {
  async load(): Promise<DataDocument> {
    if (isTauriRuntime()) {
      const document = await invoke<unknown | null>('read_data_document')
      return mergeSeedUsers(normalizeDataDocument(document ?? seedDocument))
    }

    const raw = window.localStorage.getItem(DATA_STORAGE_KEY) ?? window.localStorage.getItem(LEGACY_DATA_STORAGE_KEY)
    if (!raw) {
      return normalizeDataDocument(seedDocument)
    }

    try {
      return mergeSeedUsers(normalizeDataDocument(JSON.parse(raw)))
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

  async uploadAttachment(file: Blob, metadata: AttachmentMetadata): Promise<AttachmentResult> {
    const storageKey = metadata.storageKey || createStorageKey(metadata.name, metadata)
    const mimeType = metadata.mimeType || file.type || 'application/octet-stream'

    if (isTauriRuntime()) {
      const bytes = Array.from(new Uint8Array(await file.arrayBuffer()))
      await invoke('write_attachment', { storageKey, bytes })
    } else {
      await saveBrowserAttachment({ storageKey, blob: file, name: metadata.name, mimeType })
    }

    return { storageKey, size: file.size, mimeType }
  }

  async deleteAttachment(storageKey: string): Promise<void> {
    if (isTauriRuntime()) {
      await invoke('delete_attachment', { storageKey })
      return
    }
    await deleteBrowserAttachment(storageKey)
  }

  async readAttachment(storageKey: string): Promise<Blob> {
    if (isTauriRuntime()) {
      const bytes = await invoke<number[]>('read_attachment', { storageKey })
      return new Blob([new Uint8Array(bytes)], { type: 'application/octet-stream' })
    }
    return readBrowserAttachment(storageKey)
  }

  async exportBOM(drawingNo: string, _storageKey: string, items: unknown[]): Promise<Blob> {
    const XLSX = await import('xlsx')
    const workbook = XLSX.utils.book_new()
    const rows = (items as Array<Record<string, unknown>>).map((item, index) => ({
      序号: item.no ?? index + 1,
      '图号/标准号': item.id ?? '',
      名称: item.name ?? '',
      '规格/材质': item.spec ?? '',
      数量: item.qty ?? 1,
      '单重(kg)': item.weight ?? 0,
      备注: item.remark ?? '',
    }))
    const worksheet = XLSX.utils.json_to_sheet(rows)
    XLSX.utils.book_append_sheet(workbook, worksheet, '备料明细')
    const buffer = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' })
    return new Blob([buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  }

  async scanDrawingDesigner(_drawingNo: string): Promise<string> {
    return ''
  }
}
