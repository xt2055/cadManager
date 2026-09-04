const DATABASE_NAME = 'cad-attachments'
const DATABASE_VERSION = 1
const STORE_NAME = 'files'

interface AttachmentRecord {
  storageKey: string
  blob: Blob
  name: string
  mimeType: string
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = window.indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(STORE_NAME)) {
        request.result.createObjectStore(STORE_NAME, { keyPath: 'storageKey' })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error ?? new Error('打开附件数据库失败'))
  })
}

function runTransaction<T>(
  mode: IDBTransactionMode,
  operation: (store: IDBObjectStore, resolve: (value: T) => void, reject: (reason?: unknown) => void) => void,
): Promise<T> {
  return openDatabase().then((database) => new Promise<T>((resolve, reject) => {
    const transaction = database.transaction(STORE_NAME, mode)
    const store = transaction.objectStore(STORE_NAME)
    operation(store, resolve, reject)
    transaction.oncomplete = () => database.close()
    transaction.onerror = () => reject(transaction.error ?? new Error('附件数据库操作失败'))
    transaction.onabort = () => reject(transaction.error ?? new Error('附件数据库事务已中止'))
  }))
}

export async function saveBrowserAttachment(record: AttachmentRecord): Promise<void> {
  await runTransaction<void>('readwrite', (store, resolve, reject) => {
    const request = store.put(record)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error ?? new Error('保存附件失败'))
  })
}

export async function readBrowserAttachment(storageKey: string): Promise<Blob> {
  return runTransaction<Blob>('readonly', (store, resolve, reject) => {
    const request = store.get(storageKey)
    request.onsuccess = () => {
      const record = request.result as AttachmentRecord | undefined
      if (!record) {
        reject(new Error(`附件不存在：${storageKey}`))
        return
      }
      resolve(record.blob)
    }
    request.onerror = () => reject(request.error ?? new Error('读取附件失败'))
  })
}

export async function deleteBrowserAttachment(storageKey: string): Promise<void> {
  await runTransaction<void>('readwrite', (store, resolve, reject) => {
    const request = store.delete(storageKey)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error ?? new Error('删除附件失败'))
  })
}
