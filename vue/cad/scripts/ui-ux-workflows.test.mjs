import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import ts from 'typescript'
import { parse, compileScript } from '@vue/compiler-sfc'
import { compile } from '@vue/compiler-dom'
import * as Vue from 'vue'
import { renderToString } from 'vue/server-renderer'
import { createPinia, setActivePinia } from 'pinia'
import { useUiStore } from '../src/stores/ui.store.ts'
import { useDrawingLibraryUiStore } from '../src/stores/drawing-library-ui.store.ts'
import * as drafts from '../src/features/reviews/review-opinion-draft.ts'
import * as reviewRules from '../src/features/reviews/review-workspace.ts'
import * as drawingAuthority from '../src/modules/drawing/drawing-authority.ts'
import { STATUS } from '../src/constants/drawing-status.ts'
import * as createModes from '../src/features/drawings/create/drawing-create-modes.ts'

// Run the real SFC setup functions with Vue reactivity and lifecycle hooks.
// All network/writes are replaced here; these tests never create or sign real drawings.
const renderer = Vue.createRenderer({
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  setText() {}, setElementText() {}, parentNode: () => null, nextSibling: () => null,
  insert() {}, remove() {}, patchProp() {},
})
const flush = async () => { for (let i = 0; i < 15; i++) await Promise.resolve(); await Vue.nextTick() }
const storage = new Map()
const browserWindow = {
  sessionStorage: { getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key) },
  localStorage: { getItem: () => null },
  addEventListener() {}, removeEventListener() {}, setTimeout: callback => setTimeout(callback, 0),
}
globalThis.window = browserWindow

function fixture() {
  setActivePinia(createPinia())
  const user = Vue.reactive({ id: 'user-a', displayName: '设计员', roles: ['designer'] })
  const drawing = { id: 'd1', no: 'D1', name: '测试图纸', status: 'draft', project: 'P1', vendor: '', files: [{ id: 'f1', name: 'D1.dwg' }], otherFiles: [] }
  const f = {
    user, drawing, route: Vue.reactive({ path: '/drawings/D1/preview', params: { drawingId: 'D1' }, query: {}, name: 'drawing-preview' }),
    pushes: [], replaced: [], created: [], toasts: [], errors: [], leave: null, focused: '', fetch: async () => ({ ok: true, json: async () => ({ data: { status: 'ready' } }) }),
    ui: useUiStore(), library: useDrawingLibraryUiStore(),
    drawings: Vue.reactive({ drawings: [drawing], parts: [], loading: false, error: null,
      load: async () => {}, refresh: async () => {}, refreshDesigner: async () => {}, invalidate() {},
      getDrawing: no => no === 'D1' ? drawing : null, getPart: () => null, getStructure: () => [],
    }),
    attributes: Vue.reactive({ sortedAttributes: [], loading: false, error: null, load: async () => {}, validate: () => [], fieldName: () => '' }),
    operations: Vue.reactive({ pendingUploadSessionId: '', uploadProgress: {}, cancelPendingUploadSession: async () => {}, addDrawing: async (drawing, parts, attachments) => { f.created.push({ drawing, parts, attachments }) } }),
    review: Vue.reactive({ current: null, getCase() { return this.current }, load: async () => {}, myPendingReviews: () => [] }),
  }
  f.ui.toast = (message, type) => f.toasts.push({ message, type })
  f.router = {
    push: async target => { if (!f.leave || await f.leave()) f.pushes.push(target) },
    replace: async target => { f.replaced.push(target) },
  }
  return f
}

function mountFile(relative, f, props = {}) {
  const source = readFileSync(new URL(`../src/${relative}`, import.meta.url), 'utf8')
  const { descriptor } = parse(source)
  const script = compileScript(descriptor, { id: relative })
  const modules = {
    vue: Vue,
    'vue-router': { useRoute: () => f.route, useRouter: () => f.router, onBeforeRouteLeave: callback => { f.leave = callback } },
    '@/stores/auth.store': { useAuthStore: () => ({ currentUser: f.user, hasRole: role => f.user.roles.includes(role) }) },
    '@/stores/ui.store': { useUiStore: () => f.ui },
    '@/stores/drawing.store': { useDrawingStore: () => f.drawings },
    '@/stores/attribute.store': { useAttributeStore: () => f.attributes },
    '@/stores/drawing-operations.store': { useDrawingOperationsStore: () => f.operations },
    '@/stores/drawing-library-ui.store': { useDrawingLibraryUiStore: () => f.library },
    '@/stores/workspace.store': { useWorkspaceStore: () => ({ selectPart() {}, selectDrawing() {}, clearSelection() {} }) },
    '@/stores/review.store': { useReviewStore: () => f.review },
    '@/services/tauri/window.service': { windowService: { guardClose: async () => () => {} } },
    '@/app/container': { appContainer: { uploadGateway: {} }, drawingFileService: {}, drawingCommandService: {} },
    '@/services/api-base.service': { getApiBaseUrl: () => 'http://test.invalid' },
    '@/services/lifecycle.service': { lifecycleApi: async () => ({}) },
    '@/services/drawing-title-block.service': { extractCreationTitleBlocks: async () => [] },
    '@/services/change-request.service': { changeRequestService: { listByDrawing: async () => [] } },
    '@/utils/model-formats': { drawingMediaLabel: () => '2D' },
    '@/features/drawings/create/drawing-create-modes': createModes,
    '@/utils/drawing-number-parser': {},
    '@/utils/date-time': { formatReadableDateTime: () => '' },
    '@/constants/drawing-status': { STATUS },
    '@/router/route-names': { RouteName: { DrawingPreview: 'drawing-preview', ReviewPending: 'review-pending' } },
    '@/features/reviews/review-workspace': reviewRules,
    // 控制权判定用真实实现，避免测试里再写一份「谁算负责人」的假规则。
    '@/modules/drawing/drawing-authority': drawingAuthority,
    '../review-opinion-draft': drafts,
  }
  const context = { exports: {}, console: { ...console, error: (...args) => f.errors.push(args) }, Error, AbortSignal, FormData,
    window: browserWindow, document: {
      getElementById: id => ({ scrollIntoView() {}, focus() { f.focused = id } }),
      addEventListener() {}, removeEventListener() {},
    },
    fetch: (...args) => f.fetch(...args),
    require: id => {
      if (id.endsWith('.vue')) return { default: { render: () => null } }
      if (!(id in modules)) throw new Error(`Missing test dependency: ${id}`)
      return modules[id]
    },
  }
  vm.runInNewContext(ts.transpileModule(script.content, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, context)
  let state
  const component = context.exports.default
  const app = renderer.createApp({ ...component, setup(p, ctx) { state = component.setup(p, ctx); return () => null } }, props)
  app.mount({})
  return { state, unmount: () => app.unmount() }
}

async function renderState(relative, state) {
  const { descriptor } = parse(readFileSync(new URL(`../src/${relative}`, import.meta.url), 'utf8'))
  const { code } = compile(descriptor.template.content, { mode: 'function', prefixIdentifiers: true, expressionPlugins: ['typescript'] })
  const render = new Function('Vue', ts.transpileModule(code, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText)(Vue)
  const app = Vue.createSSRApp({ setup: () => ({ ...state }), render })
  for (const name of ['DemoIcon', 'DrawingAttributesForm', 'DrawingDetailHeader', 'DrawingDetailSubnav', 'RouterView']) app.component(name, { render: () => null })
  return renderToString(app)
}

const createPage = 'features/drawings/pages/DrawingCreatePage.vue'
test('创建向导：空基础信息不能前进或跳步，保存定位字段；必填属性不能跳过', async () => {
  const f = fixture(); const app = mountFile(createPage, f); const s = app.state
  await flush()
  s.nextStep(); assert.equal(s.currentStep.value, 2)
  s.nextStep(); await flush(); assert.equal(s.currentStep.value, 2); assert.equal(f.focused, 'create-project-name')
  s.goToStep(4); assert.equal(s.currentStep.value, 2)
  s.formProject.value = '项目'; s.formProjectNo.value = 'P1'; s.formDrawingNo.value = 'D1'; await flush()
  s.nextStep(); assert.equal(s.currentStep.value, 3)
  f.attributes.sortedAttributes = [{ id: 'material', enabled: true, required: true, fields: [] }]
  f.attributes.validate = () => ['请选择材料']
  s.nextStep(); await flush(); assert.equal(s.currentStep.value, 3); assert.equal(f.focused, 'drawing-attr-material')
  s.handleSubmit(); assert.equal(s.isCreating.value, false); assert.equal(s.currentStep.value, 3)
  app.unmount()
})

test('离开保护：空白直接离开，取消保留输入，确认后才清理；保存中禁止离开', async () => {
  const f = fixture(); let cleaned = 0; f.operations.cancelPendingUploadSession = async () => { cleaned++ }
  const app = mountFile(createPage, f); const s = app.state; await flush()
  assert.equal(await f.leave(), true)
  s.formProject.value = '未保存项目'
  const canceled = f.leave(); assert.equal(f.ui.modal.payload.cancelText, '继续编辑')
  f.ui.closeModal(); assert.equal(await canceled, false); assert.equal(cleaned, 0); assert.equal(s.formProject.value, '未保存项目')
  s.isCreating.value = true; assert.equal(await f.leave(), false); s.isCreating.value = false
  const accepted = f.leave(); const confirm = f.ui.modal.onConfirm; f.ui.closeModal(true); await confirm()
  assert.equal(await accepted, true); assert.equal(cleaned, 1)
  app.unmount()
})

test('转换失败仍可进入已保存图纸；重新提交只重试后续，不重复创建', async () => {
  const f = fixture(); f.fetch = async () => ({ ok: true, json: async () => ({ data: { status: 'failed', error: '转换失败' } }) })
  const app = mountFile(createPage, f); const s = app.state; await flush(); s.savedDrawingNo.value = 'D1'
  await s.completeSavedDrawing()
  assert.equal(s.conversionModalVisible.value, true); assert.equal(s.conversionFailedCount.value, 1); assert.equal(f.pushes.length, 0)
  await s.openSavedDrawing(); assert.equal(f.pushes[0].params.drawingId, 'D1')
  f.fetch = async () => ({ ok: true, json: async () => ({ data: { status: 'ready' } }) })
  await s.retryFailedConversions(); assert.equal(f.pushes.length, 2)
  app.unmount()
})

test('进度断网提供错误并允许恢复；离开后迟到响应不再跳转', async () => {
  const f = fixture(); f.fetch = async () => { throw new Error('网络断开') }
  const app = mountFile(createPage, f); const s = app.state; await flush(); s.savedDrawingNo.value = 'D1'
  await s.completeSavedDrawing(); assert.match(s.conversionError.value, /网络断开/); assert.equal(s.conversionBusy.value, false)
  f.fetch = async () => ({ ok: true, json: async () => ({ data: { status: 'ready' } }) })
  await s.resumeConversionProgress(); assert.equal(f.pushes.length, 1)
  let respond; f.fetch = () => new Promise(resolve => { respond = resolve })
  const pending = s.resumeConversionProgress(); app.unmount()
  respond({ ok: true, json: async () => ({ data: { status: 'ready' } }) }); await pending
  assert.equal(f.pushes.length, 1)
})

test('项目已提交但刷新失败时保留保存成功状态，重试成功后可进入详情', async () => {
  const f = fixture(); f.drawings.refresh = async () => { throw new Error('刷新失败') }
  const app = mountFile(createPage, f); const s = app.state; await flush(); s.savedDrawingNo.value = 'D1'
  await s.completeSavedDrawing(); assert.match(s.createError.value, /图纸已创建/); assert.equal(f.pushes.length, 0)
  f.drawings.refresh = async () => {}; await s.completeSavedDrawing(); assert.equal(f.pushes.length, 1)
  app.unmount()
})

test('图纸库：返回保留条件和位置，清空涵盖所有筛选，命中零件仍可收起，换账号不串条件', async () => {
  const f = fixture(); f.drawings.parts = [{ no: 'PART1', parentNo: 'D1', fileNames: ['零件.dwg'], status: 'draft' }]
  let app = mountFile('features/drawings/pages/DrawingLibraryPage.vue', f); let s = app.state; await flush()
  s.query.value = 'PART1'; s.status.value = 'draft'; s.media.value = '2D'; s.attributeFilters.value = { material: 'steel' }; await flush()
  assert.equal(s.isProjectExpanded('D1'), true); s.toggleExpanded('D1'); assert.equal(s.isProjectExpanded('D1'), false)
  s.pageElement.value = { scrollTop: 240 }; s.tableElement.value = { scrollLeft: 100 }; await f.leave(); app.unmount()
  app = mountFile('features/drawings/pages/DrawingLibraryPage.vue', f); s = app.state; await flush()
  assert.equal(s.query.value, 'PART1'); assert.equal(s.viewState.scrollTop, 240); assert.equal(s.viewState.tableScrollLeft, 100)
  assert.equal(s.availableStatuses.some(([key]) => key === 'disabled'), false)
  s.clearFilters(); assert.equal(s.query.value, ''); assert.equal(s.status.value, ''); assert.equal(s.media.value, ''); assert.equal(Object.keys(s.attributeFilters.value).length, 0)
  s.query.value = '原账号'; app.unmount(); assert.equal(f.library.forUser('another-user').query, '')
})

test('图纸库与详情：慢网保持加载态，断网显示错误，重试恢复', async () => {
  for (const path of ['features/drawings/pages/DrawingLibraryPage.vue', 'features/drawings/layouts/DrawingDetailLayout.vue']) {
    const f = fixture(); let rejectLoad
    f.drawings.load = () => new Promise((_, reject) => { rejectLoad = reject })
    const app = mountFile(path, f); const s = app.state
    assert.equal(s.loading.value, true); rejectLoad(new Error('离线')); await flush()
    assert.equal(s.loading.value, false); assert.ok(s.loadError.value)
    f.drawings.load = async () => {}; await (s.loadLibrary || s.syncDrawing)(true)
    assert.equal(s.loadError.value, ''); app.unmount()
  }
})

test('审核意见：查图和组件重建后恢复，刷新同节点不清空，不同轮次隔离，提交成功清理', async () => {
  const f = fixture(); f.user.id = 'reviewer-a'
  f.review.current = { id: 'case-1', drawingNo: 'D1', startedAt: 'round-1', status: 'reviewing', nodes: [{ name: '校对', order: 1, status: 'pending', assignedUserId: f.user.id }] }
  let app = mountFile('features/reviews/components/ReviewWorkspacePanel.vue', f, { drawingNo: 'D1' }); let s = app.state; await flush()
  s.opinionText.value = '检查尺寸与公差'; const key = s.draftKey.value
  s.openDrawingFiles(); await flush(); assert.equal(f.pushes[0].query.from, 'review'); app.unmount()
  app = mountFile('features/reviews/components/ReviewWorkspacePanel.vue', f, { drawingNo: 'D1' }); s = app.state; await flush()
  assert.equal(s.opinionText.value, '检查尺寸与公差')
  f.review.current = { ...f.review.current, nodes: [...f.review.current.nodes] }; await flush(); assert.equal(s.opinionText.value, '检查尺寸与公差')
  f.review.current.startedAt = 'round-2'; assert.equal(s.opinionText.value, '')
  f.review.current.startedAt = 'round-1'; assert.equal(s.opinionText.value, '检查尺寸与公差')
  f.review.submitNode = async () => { throw new Error('提交失败') }
  await s.handleDecision('pass'); assert.equal(drafts.readOpinionDraft(key), '检查尺寸与公差')
  f.review.submitNode = async () => { f.review.current.status = 'published'; return f.review.current }
  await s.handleDecision('pass'); assert.equal(drafts.readOpinionDraft(key), '')
  app.unmount()
})

test('图纸详情所有子路由均选中图纸库，审核中心仍选中审核', () => {
  const f = fixture(); const app = mountFile('layouts/components/DesktopSidebar/SidebarNavigation.vue', f); const s = app.state
  for (const tab of ['preview', 'models', 'structure', 'review', 'versions', 'properties']) {
    f.route.path = `/drawings/D1/${tab}`; f.route.name = `drawing-${tab}`; assert.equal(s.activeId.value, 'library')
  }
  f.route.path = '/reviews/task/D1'; f.route.name = 'review-workspace'; assert.equal(s.activeId.value, 'review'); app.unmount()
})

test('创建模板：首步只有下一步，保存成功后隐藏创建表单，预览异常始终有恢复和详情入口', async () => {
  const f = fixture(); const app = mountFile(createPage, f); const s = app.state; await flush()
  let html = await renderState(createPage, s)
  assert.match(html, /下一步：基础档案/); assert.doesNotMatch(html, /跳过后续，直接保存|保存并创建|创建草稿/)
  s.savedDrawingNo.value = 'D1'; s.conversionModalVisible.value = true; s.conversionError.value = '离线'
  html = await renderState(createPage, s)
  assert.match(html, /图纸已创建，正在生成预览/); assert.match(html, /重新读取进度/); assert.match(html, /进入图纸详情/)
  assert.doesNotMatch(html, /create-project-name|wizard-stepper/)
  app.unmount()
})

test('空状态模板：网络失败提供重试，真实无结果提供清空，空库提供创建', async () => {
  const f = fixture(); const path = 'features/drawings/pages/DrawingLibraryPage.vue'
  const app = mountFile(path, f); const s = app.state; await flush()
  s.loadError.value = '网络断开'; let html = await renderState(path, s)
  assert.match(html, /重新加载/); assert.doesNotMatch(html, /没有找到符合条件/)
  s.loadError.value = ''; f.drawings.drawings = []; s.query.value = '无结果'; await flush()
  html = await renderState(path, s); assert.match(html, /没有找到符合条件的图纸/); assert.match(html, /清空全部筛选/)
  s.clearFilters(); await flush(); html = await renderState(path, s); assert.match(html, /图纸库还没有图纸/); assert.match(html, /创建第一份图纸/)
  app.unmount()
})

test('创建新图纸：只输入图纸名称与图号，材料不上传也能建档且项目号取图号', async () => {
  const f = fixture(); f.route.query.mode = 'new'
  const app = mountFile(createPage, f); const s = app.state; await flush()
  assert.equal(s.mode.value, 'new')
  assert.equal(s.createTitle.value, '创建新图纸')
  assert.equal(s.steps.value.length, 1)

  s.handleSubmit(); await flush()
  assert.equal(f.focused, 'create-new-name'); assert.equal(f.created.length, 0)
  s.formNewName.value = '主轴回转机构装配图'
  s.handleSubmit(); await flush()
  assert.equal(f.focused, 'create-new-no'); assert.equal(f.created.length, 0)

  s.formDrawingNo.value = 'JG-2026-01-00'
  s.handleSubmit(); await flush()
  assert.equal(f.created.length, 1, '一次提交只创建一条图纸')
  const [call] = f.created
  assert.equal(call.drawing.no, 'JG-2026-01-00')
  assert.equal(call.drawing.name, '主轴回转机构装配图')
  assert.equal(call.drawing.project, 'JG-2026-01-00', '项目号留空时取图号')
  assert.equal(call.drawing.kind, '总图')
  assert.equal(call.drawing.hasFile, false)
  assert.equal(call.parts.length, 0)
  assert.equal(call.attachments.length, 0)
  assert.equal(call.drawing.files.length, 0)
  assert.equal(call.drawing.otherFiles.length, 0)
  assert.equal(s.savedDrawingNo.value, 'JG-2026-01-00')
  app.unmount()
})

test('创建新图纸：图号重复拦截，图纸材料按扩展名归入图纸文件、3D 模型与其他文件', async () => {
  const f = fixture(); f.route.query.mode = 'new'
  const app = mountFile(createPage, f); const s = app.state; await flush()

  s.formNewName.value = '重复图号图纸'; s.formDrawingNo.value = 'D1'
  s.handleSubmit()
  assert.match(Object.values(s.validationErrors.value)[0], /已存在/, '库中已有该图号时先本地拦截')
  await flush()
  assert.equal(f.created.length, 0); assert.equal(f.focused, 'create-new-no')

  s.formDrawingNo.value = 'JG-2026-02-00'
  s.formProjectNo.value = 'PRJ-2026-002'
  const material = (id, name, label, size) => ({ id, name, size: `${size} KB`, file: { name, size: size * 1024 }, label })
  s.newMaterialFiles.value = [
    material('m1', 'JG-2026-02-00.dwg', '图纸文件', 1),
    material('m2', '回转装配体.step', '3D 模型', 2),
    material('m3', '技术要求.pdf', '图纸文件', 3),
    material('m4', '评审纪要.docx', '其他文件', 4),
  ]
  await s.performCreateNew()
  assert.equal(f.created.length, 1)
  const [call] = f.created
  assert.equal(call.drawing.files.map(file => file.name).join('|'), 'JG-2026-02-00.dwg|技术要求.pdf')
  assert.equal(call.drawing.files.every(file => file.role === 'assembly' && file.fileCategory === 'drawing2d'), true)
  assert.equal(call.drawing.otherFiles.map(file => file.name).join('|'), '回转装配体.step|评审纪要.docx')
  assert.equal(call.drawing.otherFiles.map(file => file.fileCategory).join('|'), 'model3d|other')
  assert.equal(call.drawing.project, 'PRJ-2026-002', '填写的项目号优先于图号')
  assert.equal(call.drawing.hasFile, true)
  assert.equal(call.attachments.length, 4, '四份材料的文件内容都要随会话提交')
  app.unmount()
})

test('从老图纸分叉：先在第一步选源图纸，新图号必须与源图号不同', async () => {
  const f = fixture(); f.route.query.mode = 'fork'
  const app = mountFile(createPage, f); const s = app.state; await flush()
  assert.equal(s.mode.value, 'fork')
  assert.equal(s.createTitle.value, '从老图纸分叉')
  assert.equal(s.steps.value.map(step => step.title).join(','), '源图纸,基础档案,图纸属性,相关资料')

  s.nextStep(); await flush()
  assert.equal(s.currentStep.value, 1); assert.equal(f.focused, 'fork-source-select')

  s.selectedForkSourceNo.value = 'D1'; s.onForkSourceChange()
  assert.equal(s.formProject.value, '测试图纸 (改进版)')
  assert.equal(s.formDrawingNo.value, 'D1')
  s.nextStep(); await flush()
  assert.equal(s.currentStep.value, 2)

  s.nextStep(); await flush()
  assert.equal(s.currentStep.value, 2); assert.equal(f.focused, 'create-drawing-no')
  s.formDrawingNo.value = 'D1-2'; await flush()
  s.nextStep(); assert.equal(s.currentStep.value, 3)
  app.unmount()
})

test('创建方式可切换：切到创建新图纸会清空 2D 已选文件并同步地址栏', async () => {
  const f = fixture(); const app = mountFile(createPage, f); const s = app.state; await flush()
  assert.equal(s.mode.value, 'legacy', '无 mode 参数时保持上传老图纸向导')
  assert.equal(s.createTitle.value, '新建项目图纸')
  assert.equal(s.currentStep.value, 1)

  s.formProject.value = '待建项目'
  s.assemblyFile.value = { name: 'A.dwg', size: '1.0 KB' }
  s.currentStep.value = 4
  s.setMode('new')
  assert.equal(s.mode.value, 'new')
  assert.equal(s.currentStep.value, 1, '切换创建方式后回到第一步')
  assert.equal(s.assemblyFile.value, null)
  assert.equal(s.formNewName.value, '待建项目', '沿用已填写的名称作为图纸名称')
  assert.equal(f.replaced.at(-1).query.mode, 'new', '地址栏同步创建方式')
  app.unmount()
})
