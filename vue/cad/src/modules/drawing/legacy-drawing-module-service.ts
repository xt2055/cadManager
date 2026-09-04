import type { Branch, BomItem, BorrowRecord, CraftFile, DrawingAttribute, StructurePart } from '@/types/domain.types'
import type { DrawingFileIdentity, ReidentifyDrawingFileResult } from '@/services/data-manager/data-provider'

export interface LegacyDrawingModuleGateway {
  loadAttributes(): Promise<DrawingAttribute[]>
  loadBranches(): Promise<Branch[]>
  loadBorrows(): Promise<BorrowRecord[]>
  loadCrafts(): Promise<CraftFile[]>
  saveStructure(items: StructurePart[]): Promise<void>
  saveAttributes(items: DrawingAttribute[]): Promise<void>
  saveBom(items: BomItem[]): Promise<void>
  readAttachment(storageKey: string): Promise<Blob>
  deleteAttachment(storageKey: string): Promise<void>
  identifyDrawingMaterial(file: Blob, name: string): Promise<DrawingFileIdentity>
  reidentifyDrawingFile(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult>
}

/**
 * 旧模块的过渡边界。业务 Store 只依赖这个应用服务，旧 dataManager 仅在组合根实现该端口。
 * 新的原子命令迁移完成后可逐项删除这里的兼容方法。
 */
export class LegacyDrawingModuleService {
  public constructor(private readonly gateway: LegacyDrawingModuleGateway) {}

  loadAttributes(): Promise<DrawingAttribute[]> { return this.gateway.loadAttributes() }
  loadBranches(): Promise<Branch[]> { return this.gateway.loadBranches() }
  loadBorrows(): Promise<BorrowRecord[]> { return this.gateway.loadBorrows() }
  loadCrafts(): Promise<CraftFile[]> { return this.gateway.loadCrafts() }
  saveStructure(items: StructurePart[]): Promise<void> { return this.gateway.saveStructure(items) }
  saveAttributes(items: DrawingAttribute[]): Promise<void> { return this.gateway.saveAttributes(items) }
  saveBom(items: BomItem[]): Promise<void> { return this.gateway.saveBom(items) }
  readAttachment(storageKey: string): Promise<Blob> { return this.gateway.readAttachment(storageKey) }
  deleteAttachment(storageKey: string): Promise<void> { return this.gateway.deleteAttachment(storageKey) }
  identifyDrawingMaterial(file: Blob, name: string): Promise<DrawingFileIdentity> { return this.gateway.identifyDrawingMaterial(file, name) }
  reidentifyDrawingFile(storageKey: string, partNo: string): Promise<ReidentifyDrawingFileResult> {
    return this.gateway.reidentifyDrawingFile(storageKey, partNo)
  }
}
