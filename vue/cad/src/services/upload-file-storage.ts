const DB_NAME = 'cad-upload-recovery'
const DB_VERSION = 1
const STORE_NAME = 'files'

interface StoredUploadFile {
  key: string
  sessionId: string
  clientRef: string
  name: string
  type: string
  lastModified: number
  blob: Blob
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION)
    request.onerror = () => reject(request.error || new Error('打开上传恢复存储失败'))
    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(STORE_NAME)) database.createObjectStore(STORE_NAME, { keyPath: 'key' })
    }
    request.onsuccess = () => resolve(request.result)
  })
}

function fileKey(sessionId: string, clientRef: string): string {
  return `${sessionId}:${clientRef}`
}

export async function saveUploadRecoveryFile(sessionId: string, clientRef: string, file: Blob, name: string): Promise<void> {
  if (typeof indexedDB === 'undefined') return
  const database = await openDatabase()
  await new Promise<void>((resolve, reject) => {
    const transaction = database.transaction(STORE_NAME, 'readwrite')
    transaction.objectStore(STORE_NAME).put({
      key: fileKey(sessionId, clientRef), sessionId, clientRef, name,
      type: file.type || 'application/octet-stream',
		lastModified: typeof File !== 'undefined' && file instanceof File ? file.lastModified : Date.now(), blob: file,
    } satisfies StoredUploadFile)
    transaction.onerror = () => reject(transaction.error || new Error('保存上传恢复文件失败'))
    transaction.oncomplete = () => resolve()
  }).finally(() => database.close())
}

export async function loadUploadRecoveryFile(sessionId: string, clientRef: string): Promise<File | Blob | null> {
  if (typeof indexedDB === 'undefined') return null
  const database = await openDatabase()
  const stored = await new Promise<StoredUploadFile | undefined>((resolve, reject) => {
    const request = database.transaction(STORE_NAME, 'readonly').objectStore(STORE_NAME).get(fileKey(sessionId, clientRef))
    request.onerror = () => reject(request.error || new Error('读取上传恢复文件失败'))
    request.onsuccess = () => resolve(request.result as StoredUploadFile | undefined)
  }).finally(() => database.close())
  if (!stored) return null
  if (typeof File !== 'undefined') return new File([stored.blob], stored.name, { type: stored.type, lastModified: stored.lastModified })
  return stored.blob
}

export async function deleteUploadRecoverySession(sessionId: string): Promise<void> {
  if (typeof indexedDB === 'undefined') return
  const database = await openDatabase()
  await new Promise<void>((resolve, reject) => {
    const transaction = database.transaction(STORE_NAME, 'readwrite')
    const store = transaction.objectStore(STORE_NAME)
    const request = store.openCursor()
    request.onerror = () => reject(request.error || new Error('清理上传恢复文件失败'))
    request.onsuccess = () => {
      const cursor = request.result
      if (!cursor) return
      if ((cursor.value as StoredUploadFile).sessionId === sessionId) cursor.delete()
      cursor.continue()
    }
    transaction.onerror = () => reject(transaction.error || new Error('清理上传恢复文件失败'))
    transaction.oncomplete = () => resolve()
  }).finally(() => database.close())
}
