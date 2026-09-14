import assert from 'node:assert/strict'
import { test } from 'node:test'
import { AttachmentUploader } from '../src/modules/upload/attachment-uploader.ts'

function fixture(commitError) {
  const calls = { cancelled: false }
  const uploader = new AttachmentUploader({
    hashCheck: async () => ({ exists: false, sha256: '', size: 6 }),
    createSession: async (input) => { calls.metadata = input.metadata; return { id: 'session' } },
    createItem: async (sessionId, input) => { calls.item = input; return { id: 'item', status: 'ready' } },
    commitSession: async () => { if (commitError) throw commitError; return { attachmentId: 'saved' } },
    cancelSession: async () => { calls.cancelled = true },
  })
  return { uploader, calls }
}

test('项目直属零件提交空父级，保留附件所属项目与零件图号', async () => {
  const { uploader, calls } = fixture()
  await uploader.create('PROJECT', {
    id: 'new', name: 'PART.dwg', role: 'part', partNo: 'PART',
    createPart: { no: 'PART', name: '零件', parentNo: 'PROJECT', status: 'draft' },
  }, new Blob(['AC1015']))
  assert.equal(calls.metadata.createPart.parentNo, '')
  assert.equal(calls.metadata.createPart.status, 'draft')
  assert.equal(calls.item.drawingNo, 'PROJECT')
  assert.equal(calls.item.partNo, 'PART')
})

test('多级零件保留上级零件图号，提交失败取消上传会话并返回原错误', async () => {
  const error = new Error('提交失败')
  const { uploader, calls } = fixture(error)
  await assert.rejects(uploader.create('PROJECT', {
    id: 'new', name: 'CHILD.dwg', role: 'part', partNo: 'CHILD',
    createPart: { no: 'CHILD', parentNo: 'PARENT' },
  }, new Blob(['AC1015'])), (actual) => actual === error)
  assert.equal(calls.metadata.createPart.parentNo, 'PARENT')
  assert.equal(calls.cancelled, true)
})
