import { dataManager } from '@/services/data-manager'
import { ApiUploadGateway, AttachmentUploader, BrowserUploadRecoveryStore, DrawingUploadCoordinator } from '@/modules/upload'
import { BomCommandService, BorrowCommandService, DrawingCommandService, DrawingFileService, DrawingQueryService, DrawingReadModelMapper, DrawingRelationQueryService, LegacyDrawingModuleService } from '@/modules/drawing'
import { ReviewService } from '@/modules/review'
import { reviewCaseService } from '@/services/review-case.service'
import { reviewFlowService } from '@/services/review-flow.service'
import { EditingService } from '@/modules/editing'
import { VersioningService } from '@/modules/versioning'
import { AttributeCommandService, AttributeQueryService, AttributeService } from '@/modules/attribute'
import { attributeCommandApiGateway } from '@/services/attribute-command-api.service'
import { AuditService } from '@/modules/audit'
import { AdminService } from '@/modules/admin'
import {
  createDrawingOperationLog,
  listAdminOperationLogs,
  listDrawingOperationLogs,
} from '@/services/drawing-operation-log.service'
import { drawingLifecycleService } from '@/services/drawing-lifecycle.service'
import {
  deleteUpdatePackage,
  fetchSystemLogFiles,
  fetchSystemLogs,
  fetchUpdateList,
  systemLogDownloadUrl,
  updatePackageDownloadUrl,
  uploadUpdatePackage,
} from '@/services/admin.service'
import {
  closeAdminEditSession,
  deleteAdminAttachment,
  deleteAdminDrawing,
  downloadAdminVersion,
  getAdminDrawing,
  listAdminDrawings,
  restoreAdminVersion,
  setAdminDrawingStatus,
  setAdminPartStatus,
} from '@/services/admin-drawing.service'
import { ensureSmbCredential, openCadEditSession, openCadReadonly, openDefaultAppsSettings, pickCaxaExecutable, saveLocalCaxaPath } from '@/services/tauri/cad-edit.service'

/** Composition root：页面和 Store 只从这里取得上传能力，不自行创建基础设施。 */
export const appContainer = {
  uploadGateway: new ApiUploadGateway(dataManager),
  uploadRecoveryStore: new BrowserUploadRecoveryStore(),
  resetDataProvider: () => dataManager.resetProvider(),
}

/** 过渡期基础设施适配器：旧模块只能通过应用服务端口访问数据提供者。 */
export const legacyDrawingModuleService = new LegacyDrawingModuleService({
  loadAttributes: () => dataManager.loadAttributes(),
  loadBranches: () => dataManager.loadBranches(),
  loadBorrows: () => dataManager.loadBorrows(),
  loadCrafts: () => dataManager.loadCrafts(),
  saveStructure: (items) => dataManager.saveStructure(items),
  saveAttributes: (items) => dataManager.saveAttributes(items),
  saveBom: (items) => dataManager.saveBom(items),
  readAttachment: (storageKey) => dataManager.readAttachment(storageKey),
  deleteAttachment: (storageKey) => dataManager.deleteAttachment(storageKey),
  identifyDrawingMaterial: (file, name) => dataManager.identifyDrawingMaterial(file, name),
  reidentifyDrawingFile: (storageKey, partNo) => dataManager.reidentifyDrawingFile(storageKey, partNo),
})

export const attachmentUploader = new AttachmentUploader(appContainer.uploadGateway)
export const drawingUploadCoordinator = new DrawingUploadCoordinator(
  appContainer.uploadGateway,
  appContainer.uploadRecoveryStore,
)

export const drawingCommandService = new DrawingCommandService({
  updateDrawing: (drawingId, input) => dataManager.updateDrawing(drawingId, input),
  updatePart: (partId, input) => dataManager.updatePart(partId, input),
  createPart: (drawingId, input) => dataManager.createPart(drawingId, input),
  archive: drawingLifecycleService.archive,
  unarchive: drawingLifecycleService.unarchive,
})

export const bomCommandService = new BomCommandService({
  load: (drawingId) => dataManager.loadDrawingBom(drawingId),
  replace: (drawingId, input) => dataManager.replaceDrawingBom(drawingId, input),
})

export const borrowCommandService = new BorrowCommandService({
  borrow: (drawingId, input) => dataManager.borrowPart(drawingId, input),
})

export const drawingFileService = new DrawingFileService({
  readAttachment: (storageKey) => dataManager.readAttachment(storageKey),
  exportBOM: (drawingNo, storageKey, items) => dataManager.exportBOM(drawingNo, storageKey, items),
  identifyDrawingFile: (file, name, options) => dataManager.identifyDrawingFile(file, name, options),
  identifyDrawingMaterial: (file, name) => dataManager.identifyDrawingMaterial(file, name),
  reidentifyDrawingFile: (storageKey, partNo) => dataManager.reidentifyDrawingFile(storageKey, partNo),
  scanDrawingDesigner: (drawingNo) => dataManager.scanDrawingDesigner(drawingNo),
  deleteAttachment: (storageKey) => dataManager.deleteAttachment(storageKey),
})

export const drawingQueryService = new DrawingQueryService({
  loadDrawings: () => dataManager.loadDrawings(),
  loadStructure: () => dataManager.loadStructure(),
  loadBom: () => dataManager.loadBom(),
  loadAttachments: () => dataManager.loadAttachments(),
})

export const drawingReadModelMapper = new DrawingReadModelMapper()

export const drawingRelationQueryService = new DrawingRelationQueryService({
  loadBranches: () => dataManager.loadBranches(),
  loadBorrows: () => dataManager.loadBorrows(),
})

export const reviewService = new ReviewService(reviewCaseService, reviewFlowService)

export const editingService = new EditingService(dataManager, {
  openCadEditSession,
  openCadReadonly,
  pickCaxaExecutable,
  saveLocalCaxaPath,
  openDefaultAppsSettings,
  ensureSmbCredential,
})

export const versioningService = new VersioningService(dataManager)

export const attributeService = new AttributeService()
export const attributeCommandService = new AttributeCommandService(attributeCommandApiGateway)
export const attributeQueryService = new AttributeQueryService({
  loadAttributes: () => dataManager.loadAttributes(),
})

export const auditService = new AuditService({
  create: createDrawingOperationLog,
  listDrawing: listDrawingOperationLogs,
  listAdmin: listAdminOperationLogs,
})

export const adminService = new AdminService({
  listUsers: () => dataManager.listUsers(),
  createUser: (input) => dataManager.createUser(input),
  updateUser: (userId, input) => dataManager.updateUser(userId, input),
  fetchSystemLogs,
  fetchSystemLogFiles,
  systemLogDownloadUrl,
  fetchUpdateList,
  uploadUpdatePackage,
  deleteUpdatePackage,
  updatePackageDownloadUrl,
  listDrawings: listAdminDrawings,
  getDrawing: getAdminDrawing,
  setDrawingStatus: setAdminDrawingStatus,
  deleteDrawing: deleteAdminDrawing,
  setPartStatus: setAdminPartStatus,
  deleteAttachment: deleteAdminAttachment,
  closeEditSession: closeAdminEditSession,
  downloadVersion: downloadAdminVersion,
  restoreVersion: restoreAdminVersion,
})
