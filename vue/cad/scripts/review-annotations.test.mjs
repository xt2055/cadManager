import { test } from 'node:test'
import assert from 'node:assert/strict'
import { AnnotationHistory, AnnotationSaveQueue, translatedMark } from '../src/features/reviews/annotation-model.ts'

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
