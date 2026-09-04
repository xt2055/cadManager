import type { UserAccount } from '@/types/domain.types'
import type { UserManagementInput } from '@/services/data-manager/data-provider'
import type { SystemLogFile, SystemLogLine, UpdateRecord } from '@/services/admin.service'
import type {
  AdminAttachment,
  AdminDrawingDetail,
  AdminDrawingPage,
  AdminDrawingSummary,
  AdminFileVersion,
  AdminPartSummary,
} from '@/services/admin-drawing.service'

export type { SystemLogFile, SystemLogLine, UpdateRecord }
export type { AdminAttachment, AdminDrawingDetail, AdminDrawingPage, AdminDrawingSummary, AdminFileVersion, AdminPartSummary }

export interface AdminServiceGateway {
  listUsers(): Promise<UserAccount[]>
  createUser(input: UserManagementInput): Promise<UserAccount>
  updateUser(userId: string, input: UserManagementInput): Promise<UserAccount>
  fetchSystemLogs(lines?: number, keyword?: string): Promise<SystemLogLine[]>
  fetchSystemLogFiles(): Promise<SystemLogFile[]>
  systemLogDownloadUrl(file: string): string
  fetchUpdateList(): Promise<UpdateRecord[]>
  uploadUpdatePackage(input: { file: File; version: string; notes: string; mandatory: boolean; platform?: string }): Promise<UpdateRecord>
  deleteUpdatePackage(id: string): Promise<void>
  updatePackageDownloadUrl(record: UpdateRecord): string
  listDrawings(options?: { page?: number; pageSize?: number; keyword?: string; status?: string; kind?: 'drawing' | 'part' }): Promise<AdminDrawingPage>
  getDrawing(id: string): Promise<AdminDrawingDetail>
  setDrawingStatus(id: string, status: 'disabled' | 'draft'): Promise<{ id: string; status: string }>
  deleteDrawing(id: string): Promise<{ no: string }>
  setPartStatus(id: string, status: 'disabled' | 'draft'): Promise<{ id: string; status: string }>
  deleteAttachment(id: string): Promise<{ id: string; name: string }>
  closeEditSession(id: string): Promise<Record<string, unknown>>
  downloadVersion(id: string): Promise<Blob>
  restoreVersion(id: string): Promise<Record<string, unknown>>
}

/** 管理后台应用服务：账号、系统运维和图纸资产管理的统一入口。 */
export class AdminService {
  public constructor(private readonly gateway: AdminServiceGateway) {}

  listUsers() { return this.gateway.listUsers() }
  createUser(input: UserManagementInput) { return this.gateway.createUser(input) }
  updateUser(userId: string, input: UserManagementInput) { return this.gateway.updateUser(userId, input) }
  fetchSystemLogs(lines?: number, keyword?: string) { return this.gateway.fetchSystemLogs(lines, keyword) }
  fetchSystemLogFiles() { return this.gateway.fetchSystemLogFiles() }
  systemLogDownloadUrl(file: string) { return this.gateway.systemLogDownloadUrl(file) }
  fetchUpdateList() { return this.gateway.fetchUpdateList() }
  uploadUpdatePackage(input: Parameters<AdminServiceGateway['uploadUpdatePackage']>[0]) { return this.gateway.uploadUpdatePackage(input) }
  deleteUpdatePackage(id: string) { return this.gateway.deleteUpdatePackage(id) }
  updatePackageDownloadUrl(record: UpdateRecord) { return this.gateway.updatePackageDownloadUrl(record) }
  listDrawings(options?: Parameters<AdminServiceGateway['listDrawings']>[0]) { return this.gateway.listDrawings(options) }
  getDrawing(id: string) { return this.gateway.getDrawing(id) }
  setDrawingStatus(id: string, status: 'disabled' | 'draft') { return this.gateway.setDrawingStatus(id, status) }
  deleteDrawing(id: string) { return this.gateway.deleteDrawing(id) }
  setPartStatus(id: string, status: 'disabled' | 'draft') { return this.gateway.setPartStatus(id, status) }
  deleteAttachment(id: string) { return this.gateway.deleteAttachment(id) }
  closeEditSession(id: string) { return this.gateway.closeEditSession(id) }
  downloadVersion(id: string) { return this.gateway.downloadVersion(id) }
  restoreVersion(id: string) { return this.gateway.restoreVersion(id) }
}
