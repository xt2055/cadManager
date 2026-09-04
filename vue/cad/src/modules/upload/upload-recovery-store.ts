import {
  deleteUploadRecoverySession,
  loadUploadRecoveryFile,
  saveUploadRecoveryFile,
} from '@/services/upload-file-storage'

export interface UploadRecoveryStore {
  save(sessionId: string, clientRef: string, file: Blob, name: string): Promise<void>
  load(sessionId: string, clientRef: string): Promise<File | Blob | null>
  clear(sessionId: string): Promise<void>
}

/** 浏览器恢复存储的适配层；IndexedDB 细节只留在 infrastructure service。 */
export class BrowserUploadRecoveryStore implements UploadRecoveryStore {
  save(sessionId: string, clientRef: string, file: Blob, name: string): Promise<void> {
    return saveUploadRecoveryFile(sessionId, clientRef, file, name)
  }

  load(sessionId: string, clientRef: string): Promise<File | Blob | null> {
    return loadUploadRecoveryFile(sessionId, clientRef)
  }

  clear(sessionId: string): Promise<void> {
    return deleteUploadRecoverySession(sessionId)
  }
}
