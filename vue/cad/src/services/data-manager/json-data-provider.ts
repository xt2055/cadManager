import { invoke } from '@tauri-apps/api/core'

import { normalizeDataDocument, type DataDocument } from './data.types'
import { deleteBrowserAttachment, readBrowserAttachment, saveBrowserAttachment } from './attachment-storage'
import type { AttachmentMetadata, AttachmentResult, DataProvider, UserManagementInput } from './data-provider'
import type { UserAccount } from '@/types/domain.types'
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

  async identifyDrawingFile(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持从图纸内容读取图号，请切换到服务端存储模式')
  }

  async identifyDrawingMaterial(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持从图纸内容读取材料，请切换到服务端存储模式')
  }

  async reidentifyDrawingFile(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持校正历史图纸图号，请切换到服务端存储模式')
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

  async listUsers(): Promise<UserAccount[]> {
    return (await this.load()).users
  }

  async createUser(input: UserManagementInput): Promise<UserAccount> {
    const document = await this.load()
    const account = input.account.trim().toLowerCase()
    if (document.users.some((user) => user.account.toLowerCase() === account)) throw new Error('登录账号已存在')
    const user: UserAccount = {
      id: `user-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      account,
      displayName: input.displayName.trim(),
      password: input.password?.trim() || '',
      roles: [...new Set(input.roles)],
      status: 'active',
      createdAt: new Date().toISOString(),
      lastLoginAt: null,
    }
    document.users.unshift(user)
    await this.save(document)
    return user
  }

  async updateUser(userId: string, input: UserManagementInput): Promise<UserAccount> {
    const document = await this.load()
    const user = document.users.find((item) => item.id === userId)
    if (!user) throw new Error('账号不存在')
    user.account = input.account.trim().toLowerCase()
    user.displayName = input.displayName.trim()
    user.roles = [...new Set(input.roles)]
    user.status = input.status || 'active'
    if (input.password?.trim()) user.password = input.password.trim()
    await this.save(document)
    return user
  }

  async openEditSession(): Promise<never> {
    throw new Error('本地 JSON 存储模式不支持 SMB CAD 编辑，请切换到服务端存储模式')
  }
}
