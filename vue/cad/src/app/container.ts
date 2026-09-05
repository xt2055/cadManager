import { ApiDataProvider } from '@/services/api/api-data-provider'
import { ApiUploadGateway, AttachmentUploader, BrowserUploadRecoveryStore, DrawingUploadCoordinator } from '@/modules/upload'
import { BomCommandService, BorrowCommandService, DrawingCommandService, DrawingFileService, DrawingQueryService, DrawingReadModelMapper, DrawingRelationQueryService } from '@/modules/drawing'
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
  reconvertAdminAttachment,
  restoreAdminVersion,
  setAdminDrawingStatus,
  setAdminPartStatus,
} from '@/services/admin-drawing.service'
import { ensureSmbCredential, openCadEditSession, openCadReadonly, openDefaultAppsSettings, pickCaxaExecutable, saveLocalCaxaPath } from '@/services/tauri/cad-edit.service'

export const apiDataProvider = new ApiDataProvider()

/** Composition root：页面和 Store 只从这里取得上传能力，不自行创建基础设施。 */
export const appContainer = {
  uploadGateway: new ApiUploadGateway(apiDataProvider),
  uploadRecoveryStore: new BrowserUploadRecoveryStore(),
}

export const attachmentUploader = new AttachmentUploader(appContainer.uploadGateway)
export const drawingUploadCoordinator = new DrawingUploadCoordinator(
  appContainer.uploadGateway,
  appContainer.uploadRecoveryStore,
)

export const drawingCommandService = new DrawingCommandService({
  updateDrawing: (drawingId, input) => apiDataProvider.updateDrawing(drawingId, input),
  updatePart: (partId, input) => apiDataProvider.updatePart(partId, input),
  createPart: (drawingId, input) => apiDataProvider.createPart(drawingId, input),
  archive: drawingLifecycleService.archive,
  unarchive: drawingLifecycleService.unarchive,
})

export const bomCommandService = new BomCommandService({
  load: (drawingId) => apiDataProvider.loadDrawingBom(drawingId),
  replace: (drawingId, input) => apiDataProvider.replaceDrawingBom(drawingId, input),
})

export const borrowCommandService = new BorrowCommandService({
  borrow: (drawingId, input) => apiDataProvider.borrowPart(drawingId, input),
})

export const drawingFileService = new DrawingFileService({
  readAttachment: (storageKey) => apiDataProvider.readAttachment(storageKey),
  exportBOM: (drawingNo, storageKey, items) => apiDataProvider.exportBOM(drawingNo, storageKey, items),
  identifyDrawingFile: (file, name, options) => apiDataProvider.identifyDrawingFile(file, name, options),
  identifyDrawingMaterial: (file, name) => apiDataProvider.identifyDrawingMaterial(file, name),
	reidentifyDrawingFile: (storageKey, partNo, attachmentId) => apiDataProvider.reidentifyDrawingFile(storageKey, partNo, attachmentId),
  scanDrawingDesigner: (drawingNo) => apiDataProvider.scanDrawingDesigner(drawingNo),
	deleteAttachment: (storageKey, attachmentId) => apiDataProvider.deleteAttachment(storageKey, attachmentId),
})

export const drawingQueryService = new DrawingQueryService({
  loadDrawings: () => apiDataProvider.loadDrawings(),
  loadStructure: () => apiDataProvider.loadStructure(),
  loadBom: () => apiDataProvider.loadBom(),
  loadAttachments: () => apiDataProvider.loadAttachments(),
})

export const drawingReadModelMapper = new DrawingReadModelMapper()

export const drawingRelationQueryService = new DrawingRelationQueryService({
  loadBranches: () => apiDataProvider.loadBranches(),
  loadBorrows: () => apiDataProvider.loadBorrows(),
})

export const reviewService = new ReviewService(reviewCaseService, reviewFlowService)

export const editingService = new EditingService(apiDataProvider, {
  openCadEditSession,
  openCadReadonly,
  pickCaxaExecutable,
  saveLocalCaxaPath,
  openDefaultAppsSettings,
  ensureSmbCredential,
})

export const versioningService = new VersioningService(apiDataProvider)

export const attributeService = new AttributeService()
export const attributeCommandService = new AttributeCommandService(attributeCommandApiGateway)
export const attributeQueryService = new AttributeQueryService({
  loadAttributes: () => apiDataProvider.loadAttributes(),
})

export const auditService = new AuditService({
  create: createDrawingOperationLog,
  listDrawing: listDrawingOperationLogs,
  listAdmin: listAdminOperationLogs,
})

export const adminService = new AdminService({
  listUsers: () => apiDataProvider.listUsers(),
  createUser: (input) => apiDataProvider.createUser(input),
  updateUser: (userId, input) => apiDataProvider.updateUser(userId, input),
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
  reconvertAttachment: reconvertAdminAttachment,
  closeEditSession: closeAdminEditSession,
  downloadVersion: downloadAdminVersion,
  restoreVersion: restoreAdminVersion,
})
