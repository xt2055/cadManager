/** 旧版缓存没有账号信息，不能迁移到任意当前登录用户。 */
export function editSessionStorageKey(apiBaseUrl: string, userId: string): string {
  return `cad:active-edit-sessions:v2:${encodeURIComponent(apiBaseUrl)}:${userId}`
}

export function sessionsForFiles<T extends { fileId: string }>(sessions: T[], files: readonly { id: string }[]): T[] {
  const ids = new Set(files.map((file) => file.id))
  return sessions.filter((session) => ids.has(session.fileId))
}

export function isExpiredEditSession(error: unknown): boolean {
  return error instanceof Error && /会话已失效|会话不存在|会话已过期|票据或会话已失效/.test(error.message)
}
