import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { parse } from '@vue/compiler-sfc'
import { compile } from '@vue/compiler-dom'
import * as Vue from 'vue'
import { renderToString } from 'vue/server-renderer'

const source = readFileSync(new URL('../src/features/reviews/components/ReviewWorkspacePanel.vue', import.meta.url), 'utf8')
const { descriptor } = parse(source)
const { code } = compile(descriptor.template.content, { mode: 'function', prefixIdentifiers: true })
const render = new Function('Vue', code)(Vue)

async function renderWorkspace(executor, status) {
  const app = Vue.createSSRApp({
    render,
    setup: () => ({
      embedded: true, showOverview: false, drawing: { status: 'archived' },
      drawingNo: 'TEST-001', effectiveNo: 'TEST-001', reviewCase: { status }, reviewing: false,
      nodes: [], currentNode: null, canSign: false, doneCount: 0, percent: 0,
      canArchive: false, canStartReview: false, canRestartChangeReview: executor,
      isRejected: status === 'rejected', missingCase: false, submitting: false,
      changeRequestLoading: false, changeRequestError: false,
      changeRequest: { status: 'executing', executorName: '指定设计员' },
      handleRestartChangeReview() {}, openDrawingFiles() {},
    }),
  })
  app.component('DemoIcon', { render: () => null })
  app.component('ChangeReviewEvidence', { render: () => null })
  return renderToString(app)
}

for (const status of ['published', 'rejected']) {
  test(`旧审核为 ${status} 时，指定修改人仍能直接发起变更审核`, async () => {
    const html = await renderWorkspace(true, status)
    assert.match(html, /发起完整审核<\/button>/)
    assert.doesNotMatch(html, /全部节点已通过|请由图纸创建者/)
  })

  test(`旧审核为 ${status} 时，其他用户等待指定修改人送审`, async () => {
    const html = await renderWorkspace(false, status)
    assert.match(html, /等待指定修改人「指定设计员」/)
    assert.doesNotMatch(html, /发起完整审核<\/button>/)
  })
}
