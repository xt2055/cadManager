import type { FileVersionInfo } from '@/types/application.types'

export interface VersioningGateway {
  listFileVersions(storageKey: string, attachmentId?: string): Promise<FileVersionInfo[]>
  downloadFileVersion(versionId: string): Promise<Blob>
  restoreFileVersion(versionId: string): Promise<void>
}

/** 把内部版本标记（如 _converted_、_formal_）转成可读文本。 */
export function versionDisplayLabel(version: string): string {
  if (!version) return '—'
  if (version.startsWith('_converted_')) return '转换稿'
  if (version.startsWith('_formal_')) return '正式版'
  return version
}

export function formalVersionLabel(item: Pick<FileVersionInfo, 'version' | 'releaseNumber' | 'isOriginal'>): string {
  if (item.releaseNumber) return `V${item.releaseNumber}`
  if (item.isOriginal) return '原始文件'
  return versionDisplayLabel(item.version)
}

/** 版本查询、下载和回退的应用层入口；回退由后端生成新工作版本，历史版本不变。 */
export class VersioningService {
  public constructor(private readonly gateway: VersioningGateway) {}

  list(storageKey: string, attachmentId?: string): Promise<FileVersionInfo[]> { return this.gateway.listFileVersions(storageKey, attachmentId) }
  download(versionId: string): Promise<Blob> { return this.gateway.downloadFileVersion(versionId) }
  restore(versionId: string): Promise<void> { return this.gateway.restoreFileVersion(versionId) }
}
