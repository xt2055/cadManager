import { test } from 'node:test'
import assert from 'node:assert/strict'
import { AnnotationHistory, AnnotationSaveQueue, labelLayout, planTemplateText, translatedMark } from '../src/features/reviews/annotation-model.ts'

const mark = { id: 'one', kind: 'rect', layout: 'Model', points: [{ x: 10, y: 20 }, { x: 30, y: 40 }], text: '', color: '#FF6868', width: 3 }
test('移动批注保持图纸坐标尺寸和原始数据', () => {
  const moved = translatedMark(mark, { x: 10, y: 20 }, { x: -100, y: 200 })
  assert.deepEqual(moved.points, [{ x: -100, y: 200 }, { x: -80, y: 220 }])
  assert.deepEqual(mark.points[0], { x: 10, y: 20 })
})
test('撤销后新操作丢弃重做分支，历史保存快照', () => {
  const h = new AnnotationHistory()
  const start = [structuredClone(mark)]; h.record(start); start[0].text = '已修改'
  assert.equal(h.undo(start)[0].text, '')
  assert.equal(h.redo([mark])[0].text, '已修改')
  h.undo(start); h.record([mark]); assert.equal(h.canRedo, false)
})
test('发送中的新修改串行保存并使用更新后的修订号', async () => {
  const calls = []; let release
  const gate = new Promise(resolve => { release = resolve })
  const q = new AnnotationSaveQueue(4, async (marks, revision) => { calls.push({ marks, revision }); if (calls.length === 1) await gate; return revision + 1 })
  q.set([mark]); const first = q.flush()
  q.set([{ ...mark, text: '第二次' }]); const second = q.flush(); release()
  await Promise.all([first, second])
  assert.deepEqual(calls.map(c => c.revision), [4, 5]); assert.equal(calls[1].marks[0].text, '第二次'); assert.equal(q.dirty, false)
})
test('保存失败保留最新修改，重试不跳过 CAS 修订号', async () => {
  let fail = true
  const calls = []
  const q = new AnnotationSaveQueue(2, async (marks, revision) => { calls.push({ marks, revision }); if (fail) throw Error('offline'); return 3 })
  q.set([mark]); await assert.rejects(q.flush(), /offline/); assert.equal(q.dirty, true)
  q.set([{ ...mark, text: '离线修改' }]); fail = false; await q.flush()
  assert.equal(calls[1].revision, 2); assert.equal(calls[1].marks[0].text, '离线修改'); assert.equal(q.dirty, false)
})

test('点话术：刚圈出、还没有说明的标记直接附加，已有说明的标记不被覆盖', () => {
  // 圈框/勾选刚放出、还没有文字时，点话术直接附加说明（既有流程）。
  assert.equal(planTemplateText('select', '', '尺寸公差标注不清晰').mode, 'attach')
  assert.equal(planTemplateText('select', '  ', '尺寸公差标注不清晰').mode, 'attach')
  // 回归：已有说明的标记不再被下一次话术改掉，否则新批注放不下、图上还留着上一条的文字。
  assert.equal(planTemplateText('select', '上一条说明', '孔距与工艺基准不一致').mode, 'prepare')
})

test('点话术：准备新批注时沿用话术文字，侧栏按钮仍可强制附加', () => {
  assert.deepEqual(planTemplateText('select', '上一条说明', '孔距与工艺基准不一致'), { mode: 'prepare', text: '孔距与工艺基准不一致' })
  // 侧栏“附加到所选标记”按标签语义执行，明确要求附加时不受上面那条限制。
  assert.equal(planTemplateText('select', '上一条说明', '新说明', true).mode, 'attach')
  // 没有选中标记，或正在连续放置其他工具时，只能准备新批注。
  assert.equal(planTemplateText('select', null, '话术').mode, 'prepare')
  assert.equal(planTemplateText('text', '', '话术').mode, 'prepare')
  assert.equal(planTemplateText('rect', '', '话术').mode, 'prepare')
})

test('圈框说明文字与图形居中，框太小才贴到框的正上方', () => {
  const text = '请补充技术要求'
  // 图形放得下文字：文字块中心与框中心重合，不再贴在拖动起点的角上。
  for (const points of [[{ x: 100, y: 200 }, { x: 500, y: 400 }], [{ x: 500, y: 400 }, { x: 100, y: 200 }]]) {
    const inside = labelLayout('rect', points, text)
    assert.equal(inside.align, 'center')
    assert.equal(inside.x + inside.width / 2, 300)
    assert.equal(inside.y + inside.height / 2, 300)
  }
  // 框比文字还小：横向仍与框中心对齐，纵向贴到框的正上方留 8px，避免盖住框里的图形。
  const above = labelLayout('rect', [{ x: 100, y: 200 }, { x: 170, y: 260 }], text)
  assert.equal(above.x + above.width / 2, 135)
  assert.equal(above.y + above.height, 192)
  // 框放得下短话术时仍然居中。
  const short = labelLayout('rect', [{ x: 100, y: 200 }, { x: 170, y: 260 }], '公差')
  assert.equal(short.y + short.height / 2, 230)
  const ellipse = labelLayout('ellipse', [{ x: 0, y: 0 }, { x: 400, y: 300 }], '公差')
  assert.equal(ellipse.y + ellipse.height / 2, 150)
})

test('文字意见居中在放置点，勾选/叉号说明贴在标记右侧并垂直居中', () => {
  const point = [{ x: 300, y: 200 }]
  const text = labelLayout('text', point, '请补充技术要求')
  assert.equal(text.align, 'center')
  assert.equal(text.x + text.width / 2, 300)
  assert.equal(text.y + text.height / 2, 200)
  for (const kind of ['check', 'cross']) {
    const stamp = labelLayout(kind, point, '已核对')
    assert.equal(stamp.align, 'left')
    assert.equal(stamp.x, 316)
    assert.equal(stamp.y + stamp.height / 2, 200)
  }
})
