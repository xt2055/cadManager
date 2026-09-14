import type { FileView } from '@/modules/drawing'
import type { FileVersionInfo } from '@/types/application.types'

export interface CompareVersion {
  value: string
  label: string
  name: string
  storageKey?: string
  versionId?: string
  current: boolean
}

export function compareVersionOptions(file: FileView, records: FileVersionInfo[]): CompareVersion[] {
  const currentKey = file.currentStorageKey || file.storageKey
  const current = records.find(record => record.storageKey === currentKey)
  const options: CompareVersion[] = [{ value: 'current', label: `${current?.version || file.version || '最新'}（当前）`, name: file.name, storageKey: currentKey, versionId: current?.id, current: true }]
  const seen = new Set(currentKey ? [currentKey] : [])
  const history = [...records].sort((a, b) => (Date.parse(b.createdAt) || 0) - (Date.parse(a.createdAt) || 0) || b.version.localeCompare(a.version, undefined, { numeric: true }))
  for (const record of history) {
    if (seen.has(record.storageKey)) continue
    seen.add(record.storageKey)
    options.push({ value: `version:${record.id}`, label: `${record.version} · ${record.createdAt ? record.createdAt.slice(0, 10) : '历史版本'}`, name: file.name, versionId: record.id, storageKey: record.storageKey, current: false })
  }
  // Older attachments may only have file history. Keep these exact storage
  // keys instead of silently replacing a historical source with the latest.
  for (const item of [...file.history].sort((a, b) => (Date.parse(b.uploadedAt) || 0) - (Date.parse(a.uploadedAt) || 0))) {
    if (!item.storageKey || seen.has(item.storageKey) || !/\.(exb|dwg|dxf)$/i.test(item.name)) continue
    seen.add(item.storageKey)
    options.push({ value: `key:${item.storageKey}`, label: `${item.version} · ${item.uploadedAt.slice(0, 10)}`, name: item.name, storageKey: item.storageKey, current: false })
  }
  return options
}

// Frozen review submissions are intentionally absent from the public version list.
export function submissionCompareVersion(versionId: string, name: string, label: string): CompareVersion {
  if (!versionId) throw new Error('本次提交缺少对比版本')
  return { value: `version:${versionId}`, versionId, name, label, current: false }
}

export function compareSourcePath(fileId: string, version: CompareVersion): string {
  if (version.versionId) return `/file-versions/${encodeURIComponent(version.versionId)}/source`
  if (!version.current) {
    if (!version.storageKey) throw new Error('该历史版本没有可读取的图纸文件')
    return `/file-versions/source?storageKey=${encodeURIComponent(version.storageKey)}`
  }
  return `/cad/source?attachmentId=${encodeURIComponent(fileId)}`
}
