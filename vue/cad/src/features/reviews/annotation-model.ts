export interface Point { x: number; y: number }
export type MarkKind = 'pen' | 'rect' | 'ellipse' | 'arrow' | 'text' | 'check' | 'cross'
export type AnnotationTool = 'browse' | 'select' | MarkKind
export interface AnnotationMark {
  id: string
  kind: MarkKind
  layout: string
  points: Point[]
  text: string
  color: string
  width: number
}
export interface AnnotationContent { schemaVersion: 1; marks: AnnotationMark[] }
export interface AnnotationDocument {
  id: string; nodeId: string; nodeName: string; authorId: string; authorName: string
  revision: number; updatedAt: string; content: AnnotationContent
}
export interface AnnotationWorkspace {
  caseId: string; attachmentId: string; versionId: string; nodeId: string; nodeName: string
  canEdit: boolean; documents: AnnotationDocument[]
}
export interface AnnotationViewport {
  toScreen(point: Point): Point
  toDrawing(point: Point): Point
  layout(): string
  subscribe(callback: () => void): () => void
  focus(points: Point[]): void
  zoom(direction: number): void
}
export interface AnnotationTemplate { id: string; ownerId: string; category: string; text: string }

export function translatedMark(mark: AnnotationMark, from: Point, to: Point): AnnotationMark {
  return { ...mark, points: mark.points.map(p => ({ x: p.x + to.x - from.x, y: p.y + to.y - from.y })) }
}

/** 一次手势记录一次历史，不把鼠标移动帧塞入撤销栈。 */
export class AnnotationHistory {
  private past: string[] = []
  private future: string[] = []
  get canUndo() { return this.past.length > 0 }
  get canRedo() { return this.future.length > 0 }
  record(marks: AnnotationMark[]) {
    this.past.push(JSON.stringify(marks)); this.past = this.past.slice(-50); this.future = []
  }
  undo(current: AnnotationMark[]): AnnotationMark[] {
    const value = this.past.pop(); if (value === undefined) return current
    this.future.push(JSON.stringify(current)); return JSON.parse(value)
  }
  redo(current: AnnotationMark[]): AnnotationMark[] {
    const value = this.future.pop(); if (value === undefined) return current
    this.past.push(JSON.stringify(current)); return JSON.parse(value)
  }
}

/** 串行发送完整快照，发送途中新增的修改留给下一次请求。失败不自动覆盖远端。 */
export class AnnotationSaveQueue {
  private pending: AnnotationMark[] | null = null
  private running: Promise<void> | null = null
  revision: number
  private send: (marks: AnnotationMark[], revision: number) => Promise<number>
  constructor(revision: number, send: (marks: AnnotationMark[], revision: number) => Promise<number>) { this.revision = revision; this.send = send }
  set(marks: AnnotationMark[]) { this.pending = JSON.parse(JSON.stringify(marks)) }
  get dirty() { return this.pending !== null || this.running !== null }
  async flush(): Promise<void> {
    if (this.running) { await this.running; if (this.pending) return this.flush(); return }
    this.running = this.drain()
    try { await this.running } finally { this.running = null }
  }
  private async drain() {
    while (this.pending) {
      const snapshot = this.pending; this.pending = null
      try { this.revision = await this.send(snapshot, this.revision) }
      catch (error) { if (!this.pending) this.pending = snapshot; throw error }
    }
  }
}
