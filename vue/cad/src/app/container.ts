import { dataManager } from '@/services/data-manager'
import { ApiUploadGateway, BrowserUploadRecoveryStore } from '@/modules/upload'

/** Composition root：页面和 Store 只从这里取得上传能力，不自行创建基础设施。 */
export const appContainer = {
  uploadGateway: new ApiUploadGateway(dataManager),
  uploadRecoveryStore: new BrowserUploadRecoveryStore(),
}
