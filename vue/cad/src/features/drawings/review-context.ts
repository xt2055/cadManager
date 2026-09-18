/**
 * 把当前路由里的审核上下文带到下一个页面。
 *
 * 独立查看页（drawing-viewer）用 `query.from === 'review'` 决定是否启用审核批注工作区：
 * 审核工作台用「查阅图纸」进入图纸详情后，再点某个文件的「浏览」「历史」打开查看页，
 * 如果跳转时丢掉 from=review，画布照旧打开，但标注工具条与图上批注会整块消失，
 * 表现为「从审核打开却没有标注按钮」。
 *
 * 因此所有指向查看页的跳转都应经过这里：普通预览不变，审核链路原样继承。
 */
export function withReviewContext(
  base: Record<string, string | undefined>,
  currentQuery: Record<string, unknown>,
): Record<string, string> {
  const query: Record<string, string> = {}
  for (const [key, value] of Object.entries(base)) {
    if (value !== undefined && value !== '') query[key] = value
  }
  if (currentQuery.from !== 'review') return query

  query.from = 'review'
  for (const key of ['reviewNo', 'reviewCaseId'] as const) {
    const value = currentQuery[key]
    if (typeof value === 'string' && value) query[key] = value
  }
  return query
}
