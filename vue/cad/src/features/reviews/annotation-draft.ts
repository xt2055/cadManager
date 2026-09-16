import type { AnnotationMark, AnnotationWorkspace } from './annotation-model'

export interface AnnotationDraft { revision: number; marks: AnnotationMark[]; savedAt: string }
const memory = new Map<string, string>()
export function annotationDraftKey(workspace: AnnotationWorkspace, userId: string) {
  return `cad:annotation-draft:v1:${JSON.stringify([userId, workspace.caseId, workspace.attachmentId, workspace.versionId, workspace.nodeId])}`
}
export function readAnnotationDraft(key: string): AnnotationDraft | null {
  try {
    const raw = memory.get(key) ?? window.localStorage.getItem(key)
    if (!raw) return null
    const draft = JSON.parse(raw)
    if (!Number.isSafeInteger(draft.revision) || draft.revision < 0 || !Array.isArray(draft.marks) || draft.marks.length > 1000) return null
    if (!draft.marks.every((mark: AnnotationMark) => typeof mark.id === 'string' && typeof mark.layout === 'string' && typeof mark.text === 'string' && Array.isArray(mark.points) && mark.points.length > 0 && mark.points.length <= 10000 && mark.points.every(p => Number.isFinite(p.x) && Number.isFinite(p.y)))) return null
    return draft as AnnotationDraft
  } catch { return null }
}
/** 远端确认前保留本地副本；存储不可用时仍保留当前应用内的草稿。 */
export function writeAnnotationDraft(key: string, draft: AnnotationDraft | null): boolean {
  if (!key) return false
  const raw = draft ? JSON.stringify(draft) : ''
  if (raw) memory.set(key, raw); else memory.delete(key)
  try { if (raw) window.localStorage.setItem(key, raw); else window.localStorage.removeItem(key); return true }
  catch { return false }
}
