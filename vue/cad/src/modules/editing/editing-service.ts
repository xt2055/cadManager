import type {
  ActiveEditSessionInfo,
  EditSessionControlResult,
  EditSessionOpenResult,
} from '@/services/data-manager/data-provider'

export interface NativeEditOpenPayload {
  sessionId: string
  openUrl: string
  expiresAt: string
}

export interface ReadonlyOpenPayload {
  storageKey: string
}

export interface EditingApiGateway {
  listEditSessions(drawingNo?: string): Promise<ActiveEditSessionInfo[]>
  openEditSession(storageKey: string): Promise<EditSessionOpenResult>
  heartbeatEditSession(sessionId: string): Promise<EditSessionControlResult>
  closeEditSession(sessionId: string): Promise<EditSessionControlResult>
}

export interface EditingDesktopGateway {
  openCadEditSession(payload: NativeEditOpenPayload): Promise<void>
  openCadReadonly(payload: ReadonlyOpenPayload): Promise<void>
  pickCaxaExecutable(): Promise<string>
  saveLocalCaxaPath(path: string): Promise<void>
  openDefaultAppsSettings(): Promise<void>
  ensureSmbCredential(): Promise<void>
}

export const CAXA_NOT_FOUND_PREFIX = 'CAXA_NOT_FOUND:'

/** 编辑会话和版本操作的应用层入口，隔离 API、Tauri 与页面状态。 */
export class EditingService {
  public constructor(
    private readonly api: EditingApiGateway,
    private readonly desktop: EditingDesktopGateway,
  ) {}

  listSessions(drawingNo?: string): Promise<ActiveEditSessionInfo[]> { return this.api.listEditSessions(drawingNo) }
  openSession(storageKey: string): Promise<EditSessionOpenResult> { return this.api.openEditSession(storageKey) }
  heartbeat(sessionId: string): Promise<EditSessionControlResult> { return this.api.heartbeatEditSession(sessionId) }
  closeSession(sessionId: string): Promise<EditSessionControlResult> { return this.api.closeEditSession(sessionId) }
  openCad(payload: NativeEditOpenPayload): Promise<void> { return this.desktop.openCadEditSession(payload) }
  openReadonly(payload: ReadonlyOpenPayload): Promise<void> { return this.desktop.openCadReadonly(payload) }
  pickCaxa(): Promise<string> { return this.desktop.pickCaxaExecutable() }
  saveCaxaPath(path: string): Promise<void> { return this.desktop.saveLocalCaxaPath(path) }
  openDefaultApps(): Promise<void> { return this.desktop.openDefaultAppsSettings() }
  ensureSmbCredential(): Promise<void> { return this.desktop.ensureSmbCredential() }
}
