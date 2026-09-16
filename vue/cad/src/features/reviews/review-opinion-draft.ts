export interface OpinionDraftContext {
  userId: string
  caseId: string
  startedAt: string
  submissionId?: string
  node: string
  order: number
}

export function opinionDraftKey(context: OpinionDraftContext): string {
  return `cad:review-opinion:v1:${JSON.stringify([context.userId, context.caseId, context.startedAt, context.submissionId || '', context.node, context.order])}`
}

const memory = new Map<string, string>()

/** 浏览器禁用会话存储时，仍能在当前应用内往返查图。 */
export function readOpinionDraft(key: string): string {
  if (memory.has(key)) return memory.get(key) || ''
  try { return window.sessionStorage.getItem(key) || '' } catch { return '' }
}

export function writeOpinionDraft(key: string, text: string) {
  memory.set(key, text)
  try {
    if (text) window.sessionStorage.setItem(key, text)
    else window.sessionStorage.removeItem(key)
  } catch { /* 内存草稿保留到应用关闭。 */ }
}
