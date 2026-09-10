import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory, RouterView } from 'vue-router'
import Library from './src/features/part-index/pages/PartIndexLibraryPage.vue'
import Detail from './src/features/part-index/pages/PartIndexDetailPage.vue'
import { partIndexService } from './src/services/part-index.service'
import { emptyPartIndexFields } from './src/features/part-index/part-index.helpers'
import { RouteName } from './src/router/route-names'
import './src/styles/index.css'
import './src/styles/themes/juli.css'

const project = { drawingId: 'p1', projectCode: '2000W.02.03d', projectName: '2000W斜撑油缸', drawingNo: '2000W.02.03d', relationTypes: ['direct'] }
const names = ['活塞杆环', '缸筒', '活塞杆', '缸体', '油管']
const rows = names.map((name, index) => ({ attachmentId: `a${index}`, versionId: 'v1', fileName: `2000W.02.03d-0${index + 1}(${name}).dwg`, partId: null, registeredPartNo: '', drawingNo: `2000W.02.03d-0${index + 1}`, partName: name, material: index === 4 ? '20光亮管' : '27SiMn组焊件', designer: '张定', drawingDateRaw: '', drawingDate: null, status: 'recognized', extractionStatus: 'extracted', canWrite: index !== 3, projectCount: 1, projects: [project, project], versionCreatedAt: '' }))
const fields = new Map(rows.map(item => [item.attachmentId, { ...emptyPartIndexFields(), drawingNo: item.drawingNo, partName: item.partName, material: item.material, designer: item.designer, checker: '牟太有', approver: '王超', scale: '1:1', process: '杨世寒', standard: '宋洪钊', company: '泸州市巨力液压有限公司' }]))
partIndexService.list = async filter => {
  const list = rows.filter(item => (!filter.keyword || `${item.partName} ${item.drawingNo} ${item.material} ${item.designer} ${project.projectCode}`.includes(filter.keyword)) && (!filter.material || filter.material === item.material) && (!filter.designer || filter.designer === item.designer))
  return { list, total: list.length, page: 1, pageSize: 20 }
}
partIndexService.options = async (kind, keyword) => ({ list: (kind === 'project' ? [{ value: 'p1', label: '2000W.02.03d / 2000W斜撑油缸' }] : [...new Set(rows.map(item => item[kind]))].map(value => ({ value, label: value }))).filter(option => !keyword || option.label.includes(keyword)), hasMore: false })
partIndexService.detail = async id => ({ ...rows.find(item => item.attachmentId === id), fields: fields.get(id), selectedSpaceId: null, sourcePayload: { spaces: [] }, snapshotRevision: 1, sourceSnapshotRevision: 1, revision: 1, defaultProjectDrawingNo: project.drawingNo })
partIndexService.edit = async (id, input) => { fields.set(id, input.fields); Object.assign(rows.find(item => item.attachmentId === id), input.fields); return partIndexService.detail(id) }
const router = createRouter({ history: createWebHashHistory(), routes: [
  { path: '/', name: RouteName.PartIndexLibrary, component: Library },
  { path: '/detail/:attachmentId', name: RouteName.PartIndexDetail, component: Detail },
  { path: '/viewer/:drawingId', name: RouteName.DrawingViewer, component: { template: '<p>图纸预览路由已打开</p>' } },
  { path: '/project/:drawingId', name: RouteName.DrawingPreview, component: { template: '<p>项目路由已打开</p>' } },
] })
const app = createApp({ render: () => h('div', { style: 'height:100vh;width:100%;display:flex;flex-direction:column' }, [h('div', { style: 'display:flex;gap:12px;padding:6px 28px;background:var(--bg);font-size:12px' }, [h('span', '本地示例数据验收'), h('button', { onClick: () => { document.documentElement.dataset.theme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light' } }, '切换明暗'), h('button', { onClick: () => { const element = document.getElementById('app'); element.style.width = element.style.width ? '' : '390px' } }, '切换窄屏')]), h(RouterView)]) })
app.use(createPinia()).use(router).mount('#app')
