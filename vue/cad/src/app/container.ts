import { dataManager } from '@/services/data-manager'
import { ApiUploadGateway, AttachmentUploader, BrowserUploadRecoveryStore, DrawingUploadCoordinator } from '@/modules/upload'

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
