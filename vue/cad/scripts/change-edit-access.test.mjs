import assert from 'node:assert/strict'
import test from 'node:test'
import { editableChangeTargets } from '../src/features/drawings/components/detail/change-edit-access.ts'

test('single part authorization does not enable assembly or sibling files', () => {
  const allowed = editableChangeTargets([{ status: 'executing', executorId: 'designer', targets: [{ attachmentId: 'part-1' }] }], 'designer')
  assert.equal(allowed.has('part-1'), true)
  assert.equal(allowed.has('assembly'), false)
  assert.equal(allowed.has('part-2'), false)
})

test('approval, verification and terminal states grant no editing access', () => {
  for (const status of ['pending_approval', 'pending_verify', 'completed', 'cancelled', 'rejected']) {
    assert.equal(editableChangeTargets([{ status, executorId: 'designer', targets: [{ attachmentId: 'part' }] }], 'designer').size, 0)
  }
})

test('missing detail and a different executor fail closed, including administrators', () => {
  assert.equal(editableChangeTargets([{ status: 'executing', executorId: 'designer' }], 'designer').size, 0)
  assert.equal(editableChangeTargets([{ status: 'executing', executorId: 'designer', targets: [{ attachmentId: 'part' }] }], 'admin').size, 0)
})
