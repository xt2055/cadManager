/**
 * 审核批注历史：按「轮次 → 文件 → 审核员」回看每一轮留下的批注。
 *
 * 两条契约在这里落地，且都有单元测试：
 * 1. 轮次是批注的隔离边界：新一轮审核不继承上一轮的批注，上一轮的批注只从历史入口看；
 * 2. 审核上下文绝不猜案例、历史入口无条件只读。
 */
export interface AnnotationHistoryText { kind: string; text: string }
export interface AnnotationHistoryRecord {
  documentId: string
  attachmentId: string
  versionId: string
  fileName: string
  fileVersion: string
  nodeId: string
  nodeName: string
  signerRole?: string
  nodeStatus: string
  nodeOpinion?: string
  authorId: string
  authorName: string
  revision: number
  updatedAt: string
  markCount: number
  texts: AnnotationHistoryText[]
}
export interface AnnotationHistoryFile {
  attachmentId: string
  versionId: string
  name: string
  version: string
}
export interface AnnotationHistoryRound {
  caseId: string
  drawingNo: string
  round: number
  status: string
  flow: string
  initiator: string
  startedAt: string
  completedAt?: string
  changeSubmissionId?: string
  changeRequestNo?: string
  submissionRound?: number
  files: AnnotationHistoryFile[]
  records: AnnotationHistoryRecord[]
}

export interface AnnotationHistoryGroup {
  attachmentId: string
  name: string
  records: AnnotationHistoryRecord[]
}

export interface ReviewAnnotationScope {
  /** 本次查看要锁定的审核案例；普通浏览为空串。 */
  caseId: string
  /** 是否从「标注历史」进入：只读回放。 */
  history: boolean
  /** 审核链路缺少案例上下文：只提示，不加载批注，也不猜最新案例。 */
  blocked: boolean
}

/**
 * 解析查看页的审核上下文。
 *
 * `from=review` 一律要求显式携带 `reviewCaseId`：多轮审核的图纸上，任何「猜最新案例」
 * 的逻辑都会把别的轮次（乃至已经归档的轮次）的批注显示给当前查看人。
 */
export function reviewAnnotationScope(query: Record<string, unknown>): ReviewAnnotationScope {
  if (query.from !== 'review') return { caseId: '', history: false, blocked: false }
  const caseId = typeof query.reviewCaseId === 'string' ? query.reviewCaseId.trim() : ''
  return { caseId, history: query.reviewHistory === '1', blocked: caseId === '' }
}

/**
 * 历史入口无条件只读：即使打开的恰好是当前正在审核的轮次，也不能编辑，
 * 否则「历史回放」会变成一条绕过审核工作台的批注写入口。
 */
export function historyReadOnlyWorkspace<T extends { canEdit: boolean }>(workspace: T | null, isHistory: boolean): T | null {
  if (!workspace || !isHistory) return workspace
  return { ...workspace, canEdit: false }
}

export function roundLabel(round: Pick<AnnotationHistoryRound, 'round'>): string {
  return `第 ${round.round} 轮`
}

/** 轮次上下文：变更工单第几次提交，或常规送审。 */
export function roundContextLabel(round: Pick<AnnotationHistoryRound, 'changeRequestNo' | 'submissionRound'>): string {
  if (round.changeRequestNo) return `变更 ${round.changeRequestNo} 第 ${round.submissionRound || 1} 次提交`
  return '常规送审'
}

export function roundStatusLabel(status: string): string {
  return ({ reviewing: '审核中', rejected: '已驳回', published: '已通过' } as Record<string, string>)[status] ?? '已结束'
}

/** 一条批注记录的摘要：节点、审核人、条数、最后更新时间。 */
export function recordSummary(record: AnnotationHistoryRecord): string {
  const written = record.texts.filter(text => text.text.trim()).length
  const parts = [record.nodeName || '审核节点', record.authorName || '未知用户', `批注 ${record.markCount} 条`]
  if (written) parts.push(`文字 ${written} 条`)
  if (record.updatedAt) parts.push(record.updatedAt)
  return parts.join(' · ')
}

/** 只有圈画、没有文字的批注，也要让人知道这处被标记过。 */
export function recordNote(record: AnnotationHistoryRecord): string {
  const graphic = record.markCount - record.texts.filter(text => text.text.trim()).length
  return graphic > 0 ? `另有 ${graphic} 处圈画/标记` : ''
}

/** 按文件分组：一条记录只属于它自己的那份文件。 */
export function recordGroups(round: AnnotationHistoryRound): AnnotationHistoryGroup[] {
  const groups = new Map<string, AnnotationHistoryGroup>()
  for (const file of round.files ?? []) {
    groups.set(file.attachmentId, { attachmentId: file.attachmentId, name: file.name, records: [] })
  }
  for (const record of round.records ?? []) {
    const group = groups.get(record.attachmentId) ?? { attachmentId: record.attachmentId, name: record.fileName, records: [] }
    group.records.push(record)
    groups.set(record.attachmentId, group)
  }
  return [...groups.values()]
}

/**
 * 本轮意见清单（供复制到审核意见里）。
 * 节点意见必须在前：真正的驳回原因常常写在节点意见里，而不是图上的文字标记。
 */
export function opinionLines(round: AnnotationHistoryRound): string[] {
  const lines = [`${roundLabel(round)} · ${roundContextLabel(round)} · ${roundStatusLabel(round.status)}`]
  for (const group of recordGroups(round)) {
    const rows = group.records.filter(record => record.markCount > 0 || record.texts.some(text => text.text.trim()))
    if (!rows.length) continue
    lines.push(`【${group.name || '审核文件'}】`)
    for (const record of rows) {
      lines.push(`${record.nodeName || '审核节点'} · ${record.authorName || '未知用户'}`)
      if (record.nodeOpinion?.trim()) lines.push(`节点意见：${record.nodeOpinion.trim()}`)
      for (const text of record.texts) {
        if (text.text.trim()) lines.push(`- ${text.text.trim()}`)
      }
      const note = recordNote(record)
      if (note) lines.push(note)
    }
  }
  if (lines.length === 1) lines.push('本轮暂无批注。')
  return lines
}

/** 只读回放某一轮的批注：必须显式带上轮次，查看页不接受「猜最新案例」。 */
export function historyViewerQuery(round: AnnotationHistoryRound, file: AnnotationHistoryFile): Record<string, string> {
  return {
    from: 'review',
    reviewHistory: '1',
    reviewCaseId: round.caseId,
    fileId: file.attachmentId,
    versionId: file.versionId,
  }
}

const cadFilePattern = /\.(exb|dwg|dxf)$/i

/** 只有 CAD 文件能在查看页里只读回放；其他文件仍列出意见，但不提供跳转。 */
export function isCadFile(name: string): boolean {
  return cadFilePattern.test(name.trim())
}

/** 本轮第一个可回放的 CAD 文件；没有就说明这轮没有能画批注的图纸。 */
export function previewableFile(round: AnnotationHistoryRound): AnnotationHistoryFile | null {
  return (round.files ?? []).find(file => isCadFile(file.name)) ?? null
}

/**
 * 批注工作区请求参数。
 * 历史回放必须显式声明 `history=1`：服务端据此放行已被取代的轮次并强制只读；
 * 缺省时服务端只服务当前轮次，任何带错轮次 id 的入口都拿不到上一轮批注。
 */
export function annotationWorkspaceQuery(caseId: string, attachmentId: string, history: boolean): string {
  const query = new URLSearchParams({ caseId, attachmentId })
  if (history) query.set('history', '1')
  return query.toString()
}
