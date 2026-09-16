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
  /** 缩放：direction 为滚轮档数（>0 放大），anchor 为屏幕坐标锚点，缺省围绕视图中心。 */
  zoom(direction: number, anchor?: Point): void
  /** 平移：dx/dy 为屏幕像素位移，图纸内容跟随手势移动。 */
  pan(dx: number, dy: number): void
}
export interface AnnotationTemplate { id: string; ownerId: string; category: string; text: string }

export function translatedMark(mark: AnnotationMark, from: Point, to: Point): AnnotationMark {
  return { ...mark, points: mark.points.map(p => ({ x: p.x + to.x - from.x, y: p.y + to.y - from.y })) }
}

export interface TemplateTextPlan { mode: 'attach' | 'prepare'; text: string }
/**
 * 决定点击话术后的动作，避免"新批注放不下、上一条批注的文字却被改掉"。
 * - attach：把话术写到当前选中的标记上（刚圈出、还没写说明的框/箭头/勾叉走这条既有流程）。
 * - prepare：只准备文字，等用户在图纸上点击放置一条新批注。
 * 已经有说明的标记不再被下一次话术覆盖，否则连续点话术时新批注根本放不下，
 * 图上留下的还是上一条（乃至上上一条）话术的文字。
 * @param tool 当前工具
 * @param selectedText 当前选中标记的说明文字；没有选中标记时为 null
 * @param value 话术文字
 * @param forceAttach 侧栏"附加到所选标记"这类明确要求附加的入口
 */
export function planTemplateText(tool: AnnotationTool, selectedText: string | null, value: string, forceAttach = false): TemplateTextPlan {
  const attach = tool === 'select' && selectedText !== null && (forceAttach || !selectedText.trim())
  return { mode: attach ? 'attach' : 'prepare', text: value }
}

/** 图形包围盒：矩形的两个角点、箭头两端点、手绘与单点标记都归一到同一个盒子里。 */
export function pointBox(points: Point[]) {
  const a = points[0] ?? { x: 0, y: 0 }, b = points[1] ?? a
  return { x: Math.min(a.x, b.x), y: Math.min(a.y, b.y), width: Math.abs(b.x - a.x), height: Math.abs(b.y - a.y) }
}

/** 批注说明文字的画布样式，标签落位计算与之共用，避免两边尺寸不一致。 */
export const labelStyle = { fontSize: 16, lineHeight: 1.35, padding: 4, width: 280, gap: 8, margin: 16 } as const
export interface LabelLayout { x: number; y: number; width: number; height: number; align: 'left' | 'center' }

/** 估算文字宽度：中日韩字符按字号占满，其余按约一半宽，用来判断图形能否容纳说明文字。 */
export function estimateTextWidth(text: string) {
  let width = 0
  for (const char of text) width += char.charCodeAt(0) > 0x2e80 ? labelStyle.fontSize : labelStyle.fontSize * 0.55
  return width
}

/** 估算文字块高度（含自动换行的行数）；与 Konva 的 verticalAlign=middle 配合即可精确居中。 */
export function estimateLabelHeight(text: string) {
  const perLine = Math.max(1, Math.floor((labelStyle.width - labelStyle.padding * 2) / labelStyle.fontSize))
  const lines = Math.max(1, Math.ceil(estimateTextWidth(text) / labelStyle.fontSize / perLine))
  return lines * labelStyle.fontSize * labelStyle.lineHeight + labelStyle.padding * 2
}

/**
 * 说明文字与图形的对齐方式，横向一律以图形（或标记点）中心为准。
 * - 文字意见：居中在点击位置；
 * - 勾选/叉号：贴在标记右侧并垂直居中，避免压住标记本身；
 * - 圈框/椭圆/箭头/手绘：图形里放得下就居中放在图形内，放不下就紧贴图形上方，
 *   这样既不会像原来那样贴在拖动起点的角上，也不会盖住框内的图形。
 */
export function labelLayout(kind: MarkKind, points: Point[], text: string): LabelLayout {
  const height = estimateLabelHeight(text)
  const first = points[0] ?? { x: 0, y: 0 }
  if (kind === 'text') return { x: first.x - labelStyle.width / 2, y: first.y - height / 2, width: labelStyle.width, height, align: 'center' }
  if (kind === 'check' || kind === 'cross') return { x: first.x + 16, y: first.y - height / 2, width: labelStyle.width, height, align: 'left' }
  const box = pointBox(points)
  const x = box.x + box.width / 2 - labelStyle.width / 2
  const fits = box.width >= estimateTextWidth(text) + labelStyle.margin && box.height >= height + labelStyle.margin
  return fits
    ? { x, y: box.y + box.height / 2 - height / 2, width: labelStyle.width, height, align: 'center' }
    : { x, y: box.y - labelStyle.gap - height, width: labelStyle.width, height, align: 'center' }
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
