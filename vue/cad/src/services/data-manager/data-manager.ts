import { ApiDataProvider } from './api-data-provider'
import type { DataProvider } from './data-provider'
import type { DrawingFileIdentity, DrawingFileIdentifyOptions, EditSessionControlResult, EditSessionOpenResult, ActiveEditSessionInfo, FileVersionInfo, ReidentifyDrawingFileResult, UserManagementInput, StoredAttachment, CreateUploadSessionInput, CreateUploadSessionItemInput, UploadSession, UploadSessionItem, UploadSessionSnapshot, UploadHashCheckResult, UploadChunkManifest, UploadChunkSnapshot, UploadChunkInfo, UpdateDrawingInput, UpdatePartInput } from './data-provider'
import type { BomItem, Branch, BorrowRecord, CraftFile, Drawing, DrawingAttribute, DrawingVersion, StructurePart, UserAccount } from '@/types/domain.types'
import { JsonDataProvider } from './json-data-provider'
import { readDebugMode } from '@/services/runtime-config.service'

export interface DataManager {
	loadDrawings(): Promise<Drawing[]>
  loadStructure(): Promise<StructurePart[]>
  saveStructure(items: StructurePart[]): Promise<void>
  loadAttributes(): Promise<DrawingAttribute[]>
  saveAttributes(items: DrawingAttribute[]): Promise<void>
  loadVersions(): Promise<DrawingVersion[]>
  saveVersions(items: DrawingVersion[]): Promise<void>
  loadBranches(): Promise<Branch[]>
  saveBranches(items: Branch[]): Promise<void>
  loadBorrows(): Promise<BorrowRecord[]>
  saveBorrows(items: BorrowRecord[]): Promise<void>
  loadBom(): Promise<BomItem[]>
  saveBom(items: BomItem[]): Promise<void>
  loadCrafts(): Promise<CraftFile[]>
  saveCrafts(items: CraftFile[]): Promise<void>
  loadAttachments(): Promise<StoredAttachment[]>
	updateDrawing(drawingId: string, input: UpdateDrawingInput): Promise<Drawing>
	updatePart(partId: string, input: UpdatePartInput): Promise<StructurePart>
  createUploadSession(input: CreateUploadSessionInput): Promise<UploadSession>
	createUploadSessionItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem>
	checkUploadHash(file: Blob): Promise<UploadHashCheckResult>
	initUploadChunks(sessionId: string, itemId: string, manifest: UploadChunkManifest): Promise<UploadChunkSnapshot>
	listUploadChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot>
	uploadSessionChunk(sessionId: string, itemId: string, partNumber: number, chunk: Blob): Promise<UploadChunkInfo>
	completeUploadChunks(sessionId: string, itemId: string): Promise<UploadSessionItem>
  uploadSessionItem(sessionId: string, itemId: string, file: Blob, name?: string): Promise<UploadSessionItem>
  retryUploadSessionItem(sessionId: string, itemId: string): Promise<UploadSessionItem>
  getUploadSession(sessionId: string): Promise<UploadSessionSnapshot>
  commitUploadSession(sessionId: string): Promise<Record<string, unknown>>
  cancelUploadSession(sessionId: string): Promise<void>
  deleteAttachment(storageKey: string): Promise<void>
  readAttachment(storageKey: string): Promise<Blob>
  exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob>
  scanDrawingDesigner(drawingNo: string): Promise<string>
  identifyDrawingFile(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity>
  identifyDrawingMaterial(file: Blob, name: string): Promise<DrawingFileIdentity>
  reidentifyDrawingFile(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult>
  listUsers(): Promise<UserAccount[]>
  createUser(input: UserManagementInput): Promise<UserAccount>
  updateUser(userId: string, input: UserManagementInput): Promise<UserAccount>
  listEditSessions(drawingNo?: string): Promise<ActiveEditSessionInfo[]>
  openEditSession(storageKey: string): Promise<EditSessionOpenResult>
  heartbeatEditSession(sessionId: string): Promise<EditSessionControlResult>
  closeEditSession(sessionId: string): Promise<EditSessionControlResult>
  listFileVersions(storageKey: string): Promise<FileVersionInfo[]>
  downloadFileVersion(versionId: string): Promise<Blob>
  restoreFileVersion(versionId: string): Promise<void>
  resetProvider(): void
}

export async function createDataProvider(): Promise<DataProvider> {
  const configuredProvider = import.meta.env.VITE_DATA_PROVIDER
  if (configuredProvider === 'json') return new JsonDataProvider()
  if (configuredProvider === 'api') return new ApiDataProvider()
  return (await readDebugMode()) ? new JsonDataProvider() : new ApiDataProvider()
}

export class DefaultDataManager implements DataManager {
  private provider: DataProvider | null
  private providerPromise: Promise<DataProvider> | null

  constructor(provider?: DataProvider) {
    this.provider = provider ?? null
    this.providerPromise = provider ? Promise.resolve(provider) : null
  }

  private async getProvider(): Promise<DataProvider> {
    if (this.provider) return this.provider
    if (!this.providerPromise) {
      this.providerPromise = createDataProvider()
    }
    this.provider = await this.providerPromise
    return this.provider
  }

	loadDrawings(): Promise<Drawing[]> { return this.getProvider().then((provider) => provider.loadDrawings()) }
  loadStructure(): Promise<StructurePart[]> { return this.getProvider().then((provider) => provider.loadStructure()) }
  saveStructure(items: StructurePart[]): Promise<void> { return this.getProvider().then((provider) => provider.saveStructure(items)) }
  loadAttributes(): Promise<DrawingAttribute[]> { return this.getProvider().then((provider) => provider.loadAttributes()) }
  saveAttributes(items: DrawingAttribute[]): Promise<void> { return this.getProvider().then((provider) => provider.saveAttributes(items)) }
  loadVersions(): Promise<DrawingVersion[]> { return this.getProvider().then((provider) => provider.loadVersions()) }
  saveVersions(items: DrawingVersion[]): Promise<void> { return this.getProvider().then((provider) => provider.saveVersions(items)) }
  loadBranches(): Promise<Branch[]> { return this.getProvider().then((provider) => provider.loadBranches()) }
  saveBranches(items: Branch[]): Promise<void> { return this.getProvider().then((provider) => provider.saveBranches(items)) }
  loadBorrows(): Promise<BorrowRecord[]> { return this.getProvider().then((provider) => provider.loadBorrows()) }
  saveBorrows(items: BorrowRecord[]): Promise<void> { return this.getProvider().then((provider) => provider.saveBorrows(items)) }
  loadBom(): Promise<BomItem[]> { return this.getProvider().then((provider) => provider.loadBom()) }
  saveBom(items: BomItem[]): Promise<void> { return this.getProvider().then((provider) => provider.saveBom(items)) }
  loadCrafts(): Promise<CraftFile[]> { return this.getProvider().then((provider) => provider.loadCrafts()) }
  saveCrafts(items: CraftFile[]): Promise<void> { return this.getProvider().then((provider) => provider.saveCrafts(items)) }
  loadAttachments(): Promise<StoredAttachment[]> { return this.getProvider().then((provider) => provider.loadAttachments()) }

  async updateDrawing(drawingId: string, input: UpdateDrawingInput): Promise<Drawing> {
    const provider = await this.getProvider()
    if (!provider.updateDrawing) throw new Error('当前存储模式不支持图纸原子更新')
    return provider.updateDrawing(drawingId, input)
  }

  async updatePart(partId: string, input: UpdatePartInput): Promise<StructurePart> {
    const provider = await this.getProvider()
    if (!provider.updatePart) throw new Error('当前存储模式不支持零件原子更新')
    return provider.updatePart(partId, input)
  }

  createUploadSession(input: CreateUploadSessionInput): Promise<UploadSession> {
    return this.getProvider().then((provider) => provider.createUploadSession(input))
  }
	createUploadSessionItem(sessionId: string, input: CreateUploadSessionItemInput): Promise<UploadSessionItem> {
		return this.getProvider().then((provider) => provider.createUploadSessionItem(sessionId, input))
	}
	checkUploadHash(file: Blob): Promise<UploadHashCheckResult> {
		return this.getProvider().then((provider) => provider.checkUploadHash(file))
	}
	initUploadChunks(sessionId: string, itemId: string, manifest: UploadChunkManifest): Promise<UploadChunkSnapshot> {
		return this.getProvider().then((provider) => provider.initUploadChunks(sessionId, itemId, manifest))
	}
	listUploadChunks(sessionId: string, itemId: string): Promise<UploadChunkSnapshot> {
		return this.getProvider().then((provider) => provider.listUploadChunks(sessionId, itemId))
	}
	uploadSessionChunk(sessionId: string, itemId: string, partNumber: number, chunk: Blob): Promise<UploadChunkInfo> {
		return this.getProvider().then((provider) => provider.uploadSessionChunk(sessionId, itemId, partNumber, chunk))
	}
	completeUploadChunks(sessionId: string, itemId: string): Promise<UploadSessionItem> {
		return this.getProvider().then((provider) => provider.completeUploadChunks(sessionId, itemId))
	}
  uploadSessionItem(sessionId: string, itemId: string, file: Blob, name?: string): Promise<UploadSessionItem> {
    return this.getProvider().then((provider) => provider.uploadSessionItem(sessionId, itemId, file, name))
  }
  retryUploadSessionItem(sessionId: string, itemId: string): Promise<UploadSessionItem> {
    return this.getProvider().then((provider) => provider.retryUploadSessionItem(sessionId, itemId))
  }
  getUploadSession(sessionId: string): Promise<UploadSessionSnapshot> {
    return this.getProvider().then((provider) => provider.getUploadSession(sessionId))
  }
  commitUploadSession(sessionId: string): Promise<Record<string, unknown>> {
    return this.getProvider().then((provider) => provider.commitUploadSession(sessionId))
  }
  cancelUploadSession(sessionId: string): Promise<void> {
    return this.getProvider().then((provider) => provider.cancelUploadSession(sessionId))
  }

  deleteAttachment(storageKey: string): Promise<void> {
    return this.getProvider().then((provider) => provider.deleteAttachment(storageKey))
  }

  readAttachment(storageKey: string): Promise<Blob> {
    return this.getProvider().then((provider) => provider.readAttachment(storageKey))
  }

  async exportBOM(drawingNo: string, storageKey: string, items: unknown[]): Promise<Blob> {
    const provider = await this.getProvider()
    if (provider.exportBOM) {
      return provider.exportBOM(drawingNo, storageKey, items)
    }
    throw new Error('当前存储模式不支持服务端 BOM 导出')
  }

  async scanDrawingDesigner(drawingNo: string): Promise<string> {
    const provider = await this.getProvider()
    return provider.scanDrawingDesigner ? provider.scanDrawingDesigner(drawingNo) : ''
  }

  async identifyDrawingFile(file: Blob, name: string, options?: DrawingFileIdentifyOptions): Promise<DrawingFileIdentity> {
    const provider = await this.getProvider()
    if (!provider.identifyDrawingFile) {
      throw new Error('当前存储模式不支持从图纸内容读取图号')
    }
    return provider.identifyDrawingFile(file, name, options)
  }

  async identifyDrawingMaterial(file: Blob, name: string): Promise<DrawingFileIdentity> {
    const provider = await this.getProvider()
    if (!provider.identifyDrawingMaterial) {
      throw new Error('当前存储模式不支持从图纸内容读取材料')
    }
    return provider.identifyDrawingMaterial(file, name)
  }

  async reidentifyDrawingFile(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult> {
    const provider = await this.getProvider()
    if (!provider.reidentifyDrawingFile) {
      throw new Error('当前存储模式不支持校正历史图纸图号')
    }
    return provider.reidentifyDrawingFile(storageKey, partNo)
  }

  async listUsers(): Promise<UserAccount[]> {
    const provider = await this.getProvider()
    if (!provider.listUsers) throw new Error('当前存储模式不支持账号管理')
    return provider.listUsers()
  }

  async createUser(input: UserManagementInput): Promise<UserAccount> {
    const provider = await this.getProvider()
    if (!provider.createUser) throw new Error('当前存储模式不支持账号管理')
    return provider.createUser(input)
  }

  async updateUser(userId: string, input: UserManagementInput): Promise<UserAccount> {
    const provider = await this.getProvider()
    if (!provider.updateUser) throw new Error('当前存储模式不支持账号管理')
    return provider.updateUser(userId, input)
  }

  async listEditSessions(drawingNo?: string): Promise<ActiveEditSessionInfo[]> {
    const provider = await this.getProvider()
    if (!provider.listEditSessions) return []
    return provider.listEditSessions(drawingNo)
  }

  async openEditSession(storageKey: string): Promise<EditSessionOpenResult> {
    const provider = await this.getProvider()
    if (!provider.openEditSession) throw new Error('当前存储模式不支持本地 CAD 编辑')
    return provider.openEditSession(storageKey)
  }

  async heartbeatEditSession(sessionId: string): Promise<EditSessionControlResult> {
    const provider = await this.getProvider()
    if (!provider.heartbeatEditSession) throw new Error('当前存储模式不支持编辑会话保活')
    return provider.heartbeatEditSession(sessionId)
  }

  async closeEditSession(sessionId: string): Promise<EditSessionControlResult> {
    const provider = await this.getProvider()
    if (!provider.closeEditSession) throw new Error('当前存储模式不支持关闭编辑会话')
    return provider.closeEditSession(sessionId)
  }

  async listFileVersions(storageKey: string): Promise<FileVersionInfo[]> {
    const provider = await this.getProvider()
    if (!provider.listFileVersions) return []
    return provider.listFileVersions(storageKey)
  }

  async downloadFileVersion(versionId: string): Promise<Blob> {
    const provider = await this.getProvider()
    if (!provider.downloadFileVersion) throw new Error('当前存储模式不支持下载版本文件')
    return provider.downloadFileVersion(versionId)
  }

  async restoreFileVersion(versionId: string): Promise<void> {
    const provider = await this.getProvider()
    if (!provider.restoreFileVersion) throw new Error('当前存储模式不支持回退版本')
    return provider.restoreFileVersion(versionId)
  }

  resetProvider(): void {
    this.provider = null
    this.providerPromise = null
  }
}

export const dataManager: DataManager = new DefaultDataManager()
