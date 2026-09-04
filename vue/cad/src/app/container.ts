import { dataManager } from '@/services/data-manager'
import { ApiUploadGateway, AttachmentUploader, BrowserUploadRecoveryStore, DrawingUploadCoordinator } from '@/modules/upload'
import { DrawingCommandService } from '@/modules/drawing'

/** Composition root：页面和 Store 只从这里取得上传能力，不自行创建基础设施。 */
export const appContainer = {
  uploadGateway: new ApiUploadGateway(dataManager),
  uploadRecoveryStore: new BrowserUploadRecoveryStore(),
}

export const attachmentUploader = new AttachmentUploader(appContainer.uploadGateway)
export const drawingUploadCoordinator = new DrawingUploadCoordinator(
  appContainer.uploadGateway,
  appContainer.uploadRecoveryStore,
)

export const drawingCommandService = new DrawingCommandService({
  updateDrawing: (drawingId, input) => dataManager.updateDrawing(drawingId, input),
  updatePart: (partId, input) => dataManager.updatePart(partId, input),
})
