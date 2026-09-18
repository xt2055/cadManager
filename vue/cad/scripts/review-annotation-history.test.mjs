import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  historyReadOnlyWorkspace,
  historyViewerQuery,
  isCadFile,
  opinionLines,
  previewableFile,
  recordGroups,
  recordNote,
  recordSummary,
  reviewAnnotationScope,
  roundContextLabel,
  roundLabel,
  roundStatusLabel,
} from '../src/features/reviews/annotation-history.ts'

const round = {
  caseId: 'case-2',
  drawingNo: 'D-1',
  round: 2,
  status: 'rejected',
  flow: '完整审核',
  initiator: '设计员',
  startedAt: '2026-01-02 09:00',
  completedAt: '2026-01-03 10:00',
  changeRequestNo: 'CR-2026-014',
  submissionRound: 2,
  files: [
    { attachmentId: 'a1', versionId: 'v1', name: 'P-1.dwg', version: 'v1.0' },
    { attachmentId: 'a2', versionId: 'v2', name: 'P-2.dwg', version: 'v1.0' },
  ],
  records: [
    {
      documentId: 'd1', attachmentId: 'a1', versionId: 'v1', fileName: 'P-1.dwg', fileVersion: 'v1.0',
      nodeId: 'n1', nodeName: '专业审核', nodeStatus: 'rejected', nodeOpinion: '尺寸体系不完整',
      authorId: 'u2', authorName: '审核员', revision: 2, updatedAt: '2026-01-03 09:30', markCount: 3,
      texts: [{ kind: 'text', text: '缺少尺寸' }, { kind: 'text', text: '此处公差不明确' }],
    },
    {
      documentId: 'd2', attachmentId: 'a2', versionId: 'v2', fileName: 'P-2.dwg', fileVersion: 'v1.0',
      nodeId: 'n1', nodeName: '专业审核', nodeStatus: 'rejected', nodeOpinion: '尺寸体系不完整',
      authorId: 'u2', authorName: '审核员', revision: 1, updatedAt: '2026-01-03 09:40', markCount: 1,
      texts: [{ kind: 'text', text: '材料错误' }],
    },
  ],
}

test('轮次标签与上下文：变更按提交轮次，常规送审单独标注', () => {
  assert.equal(roundLabel(round), '第 2 轮')
  assert.equal(roundContextLabel(round), '变更 CR-2026-014 第 2 次提交')
  assert.equal(roundContextLabel({ changeRequestNo: '', submissionRound: 0 }), '常规送审')
  assert.equal(roundStatusLabel('rejected'), '已驳回')
  assert.equal(roundStatusLabel('reviewing'), '审核中')
  assert.equal(roundStatusLabel('published'), '已通过')
  assert.equal(roundStatusLabel('weird'), '已结束')
})

test('审核入口不猜案例：from=review 缺 reviewCaseId 一律拦下', () => {
  assert.deepEqual(reviewAnnotationScope({ from: 'review' }), { caseId: '', history: false, blocked: true })
  assert.deepEqual(reviewAnnotationScope({ from: 'review', reviewCaseId: '   ' }), { caseId: '', history: false, blocked: true })
  assert.deepEqual(reviewAnnotationScope({ from: 'review', reviewCaseId: 'case-1' }), { caseId: 'case-1', history: false, blocked: false })
  // 普通浏览不受影响，也不会去加载批注。
  assert.deepEqual(reviewAnnotationScope({ fileId: 'f1' }), { caseId: '', history: false, blocked: false })
  // 历史模式必须带案例，且被识别为只读回放。
  assert.deepEqual(reviewAnnotationScope({ from: 'review', reviewCaseId: 'case-1', reviewHistory: '1' }), { caseId: 'case-1', history: true, blocked: false })
  assert.deepEqual(reviewAnnotationScope({ from: 'review', reviewHistory: '1' }), { caseId: '', history: true, blocked: true })
})

test('历史入口无条件只读：即使打开的是当前活跃轮次也不能编辑', () => {
  const workspace = { caseId: 'case-1', canEdit: true, documents: [] }
  assert.equal(historyReadOnlyWorkspace(workspace, true)?.canEdit, false)
  assert.equal(workspace.canEdit, true, '不得改写后端返回的对象')
  assert.equal(historyReadOnlyWorkspace(workspace, false), workspace)
  assert.equal(historyReadOnlyWorkspace(null, true), null)
})

test('记录按文件分组：一张图的意见不会挂到另一张图上', () => {
  const groups = recordGroups(round)
  assert.deepEqual(groups.map((group) => group.name), ['P-1.dwg', 'P-2.dwg'])
  assert.deepEqual(groups[0].records.map((record) => record.documentId), ['d1'])
  assert.deepEqual(groups[1].records.map((record) => record.documentId), ['d2'])
})

test('没有批注的文件仍然出现在历史里', () => {
  const groups = recordGroups({ ...round, records: [] })
  assert.equal(groups.length, 2)
  assert.deepEqual(groups.map((group) => group.records.length), [0, 0])
})

test('记录摘要与圈画提示：无文字意见时不留空白', () => {
  assert.equal(recordSummary(round.records[0]), '专业审核 · 审核员 · 批注 3 条 · 文字 2 条 · 2026-01-03 09:30')
  assert.equal(recordNote(round.records[0]), '另有 1 处圈画/标记')
  assert.equal(recordNote(round.records[1]), '')
  const bare = { ...round.records[1], texts: [], markCount: 0, updatedAt: '' }
  assert.equal(recordSummary(bare), '专业审核 · 审核员 · 批注 0 条')
  assert.equal(recordNote(bare), '')
})

test('意见清单含节点意见、文件分组与圈画提示', () => {
  const lines = opinionLines(round)
  assert.equal(lines[0], '第 2 轮 · 变更 CR-2026-014 第 2 次提交 · 已驳回')
  assert.deepEqual(lines.slice(1, 7), [
    '【P-1.dwg】',
    '专业审核 · 审核员',
    '节点意见：尺寸体系不完整',
    '- 缺少尺寸',
    '- 此处公差不明确',
    '另有 1 处圈画/标记',
  ])
  assert.ok(lines.includes('【P-2.dwg】'))
  assert.ok(lines.includes('- 材料错误'))
})

test('本轮没有批注时给出可读兜底', () => {
  assert.deepEqual(opinionLines({ ...round, records: [] }), ['第 2 轮 · 变更 CR-2026-014 第 2 次提交 · 已驳回', '本轮暂无批注。'])
})

test('图上回放必须显式锁定轮次与固定版本', () => {
  assert.deepEqual(historyViewerQuery(round, round.files[1]), {
    from: 'review',
    reviewHistory: '1',
    reviewCaseId: 'case-2',
    fileId: 'a2',
    versionId: 'v2',
  })
})

test('非 CAD 文件不提供画布回放，历史里仍可看意见', () => {
  assert.equal(isCadFile('P-1.dwg'), true)
  assert.equal(isCadFile('P-1.DXF'), true)
  assert.equal(isCadFile(' 总装.exb '), true)
  assert.equal(isCadFile('技术要求.pdf'), false)
  assert.equal(previewableFile(round)?.attachmentId, 'a1')
  assert.equal(previewableFile({ ...round, files: [{ attachmentId: 'x', versionId: 'v', name: '说明.docx', version: 'v1' }] }), null)
  assert.equal(previewableFile({ ...round, files: [] }), null)
})
