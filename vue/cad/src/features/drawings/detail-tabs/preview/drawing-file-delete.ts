/**
 * 图纸文件删除权限（前端显隐；后端 attachmentDeletionDecision 同步强校验）。
 * 控制权判定不在本函数内重复实现：调用方传入 isDrawingDecider 的结果，
 * 保证「有负责人时归负责人、无负责人时回落创建人」只有一处规则。
 */
export function canDeleteDrawingFiles(input: { status: string; decides: boolean }): boolean {
  if (input.status === 'archived') return false
  return input.decides
}
