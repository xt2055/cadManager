import { test } from 'node:test'
import assert from 'node:assert/strict'
import { selectAllFiles, selectedFiles, toggleFileSelection } from '../src/features/drawings/detail-tabs/file-bulk-selection.ts'

test('文件多选支持单项切换和全选反选', () => {
  const files = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]
  let selected = new Set()
  selected = toggleFileSelection(selected, 'a')
  assert.deepEqual([...selected], ['a'])
  selected = selectAllFiles(files.map(file => file.id), selected)
  assert.deepEqual([...selected], ['a', 'b', 'c'])
  selected = selectAllFiles(files.map(file => file.id), selected)
  assert.equal(selected.size, 0)
})

test('批量操作只返回已选文件', () => {
  assert.deepEqual(selectedFiles([{ id: 'a' }, { id: 'b' }], new Set(['b'])), [{ id: 'b' }])
})
