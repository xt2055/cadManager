import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import ts from 'typescript'

// 执行真实命令 Store；仅替换网络、登录和界面依赖。
function fixture() {
  let attachments = []
  const deleted = []
  const project = { id: 'project', no: 'P-001', name: '测试项目', kind: 'assembly', status: 'draft' }
  const container = {
    appContainer: {},
    drawingQueryService: {
      loadSnapshot: async () => ({ drawings: [{ ...project }], structure: [], bom: [], attachments: [...attachments] }),
      listAttachments: async () => [...attachments],
    },
    attributeQueryService: { list: async () => [] },
    drawingRelationQueryService: { listBranches: async () => [], listBorrows: async () => [] },
    editingService: { ensureSmbCredential: async () => {} },
    auditService: { record: async () => {} },
    drawingFileService: { delete: async (key, id) => { deleted.push({ key, id }); attachments = attachments.filter((file) => file.id !== id) } },
    attachmentUploader: {
      create: async (_drawingNo, file) => {
        attachments.push({ id: 'server-id', drawingNo: project.no, role: 'craft', name: file.name, storageKey: 'blobs/first', size: 1 })
        return { attachmentId: 'server-id', currentStorageKey: 'blobs/first' }
      },
    },
  }
  const module = { exports: {} }
  const code = ts.transpileModule(readFileSync(new URL('../src/stores/drawing-operations.store.ts', import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  vm.runInNewContext(code, {
    module, exports: module.exports, console,
    window: { localStorage: { getItem: () => null } },
    require: (name) => {
      if (name === 'vue') return { ref: (value) => ({ value }), computed: (get) => ({ get value() { return get() } }) }
      if (name === 'pinia') return { defineStore: (_name, setup) => setup }
      if (name === '@/app/container') return container
      if (name === '@/stores/auth.store') return { useAuthStore: () => ({ currentUser: { id: 'creator', displayName: '创建者' }, hasRole: () => false }) }
      if (name === '@/utils/date-time') return { formatReadableDateTime: () => '' }
      return {}
    },
  })
  return { store: module.exports.useDrawingOperationsStore(), deleted, setAttachments: (list) => { attachments = list } }
}

test('非管理员上传后采用数据库 ID，刷新后的页面可以立即删除', async () => {
  const { store, deleted } = fixture()
  const file = { id: 'temporary-id', name: '工艺.pdf', ver: 'v1.0' }
  await store.uploadCraftFile('P-001', file, new Blob(['process']))
  assert.equal(file.id, 'server-id')
  await store.deleteCraftFile('P-001', 'server-id')
  assert.deepEqual(deleted, [{ key: 'blobs/first', id: 'server-id' }])
})

test('命令缓存初始化后，另一客户端新增或替换文件仍可按最新身份删除', async () => {
  const { store, deleted, setAttachments } = fixture()
  await store.initialize()
  setAttachments([{ id: 'remote-id', drawingNo: 'P-001', role: 'craft', name: '工艺.pdf', storageKey: 'blobs/latest', size: 1 }])
  await store.deleteCraftFile('P-001', 'remote-id')
  assert.deepEqual(deleted, [{ key: 'blobs/latest', id: 'remote-id' }])
})
