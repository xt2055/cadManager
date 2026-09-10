import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory, RouterView } from 'vue-router'
import Dashboard from './src/features/dashboard/pages/DashboardPage.vue'
import { useAuthStore } from './src/stores/auth.store'
import { useReviewStore } from './src/stores/review.store'
import { useSystemStatusStore } from './src/stores/system-status.store'
import { drawingQueryService, drawingReadModelMapper } from './src/app/container'
import { RouteName } from './src/router/route-names'
import './src/styles/index.css'
import './src/styles/themes/juli.css'

const drawings = Array.from({ length: 12 }, (_, i) => ({ id: `d${i}`, no: `2000W.02.03${i % 2 ? 'c' : 'd'}-0${i + 1}`, name: ['2000W 斜撑油缸', '主液压缸总成', '导向套组件', '举升油缸', '活塞杆组件'][i % 5], project: i % 2 ? '2000W.02.03c' : '2000W.02.03d', status: ['draft', 'reviewing', 'published', 'published'][i % 4], version: `A.${i % 3}`, updatedAt: `2026-09-${String(10 - i % 7).padStart(2, '0')}T08:30:00Z`, createdBy: i === 10 ? 'other' : 'u1', designer: i === 10 ? '李工' : '张工', kind: '总图', fileCount: i + 3, attributeValues: {}, files: [], otherFiles: [], materialFiles: [], craftFiles: [] }))
let empty = false
drawingQueryService.loadSnapshot = async () => ({})
drawingReadModelMapper.map = () => ({ drawings: empty ? [] : drawings, parts: Array.from({ length: 83 }, (_, i) => ({ id: `p${i}` })), structureByDrawing: {}, bom: [] })
const pinia = createPinia()
const auth = useAuthStore(pinia)
auth.currentUser = { id: 'u1', account: 'zhanggong', displayName: '张工', roles: ['designer'], status: 'active' }
const review = useReviewStore(pinia)
review.load = async () => {
  review.cases = empty ? [] : drawings.slice(0, 5).map((drawing, i) => ({ id: `r${i}`, drawingNo: drawing.no, drawingName: drawing.name, status: 'reviewing', initiator: '李工', startedAt: drawing.updatedAt, nodes: [{ name: ['校对复核', '专业审核', '工艺会签'][i % 3], assignedUserId: 'u1', assignedName: '张工', order: 1, status: 'pending' }] }))
  review.completed = empty ? [] : drawings.slice(5, 8).map((drawing, i) => ({ id: `c${i}`, no: drawing.no, name: drawing.name, node: '专业审核', reviewer: '张工', by: '李工', time: drawing.updatedAt, result: i === 1 ? 'rejected' : 'pass', opinion: i === 1 ? '请补充安装尺寸与表面粗糙度。' : '图纸尺寸和技术要求已核对。' }))
}
const system = useSystemStatusStore(pinia)
system.fetchStatus = async () => { system.status = { service: { status: 'ok' }, database: { status: 'ok' }, storage: { status: 'ok', formattedUsed: '12.6 GB', fileCount: 1384, backupStatus: '未配置' }, onlineUsers: { count: 8 } }; system.isOnline = true; system.lastChecked = new Date() }
const originalFetch = window.fetch
window.fetch = (input, init) => String(input).includes('/admin/audit-logs') ? Promise.resolve(Response.json({ data: { list: empty ? [] : drawings.slice(0, 4).map((drawing, i) => ({ id: String(i), drawingNo: drawing.no, drawingName: drawing.name, user: i % 2 ? '李工' : '张工', act: ['upload', 'check', 'edit', 'create'][i], result: 'success', occurredAt: drawing.updatedAt })) } })) : originalFetch(input, init)
const router = createRouter({ history: createWebHashHistory(), routes: [{ path: '/', name: RouteName.Dashboard, component: Dashboard }, ...[...new Set(Object.values(RouteName))].filter(name => name !== RouteName.Dashboard).map(name => ({ path: `/${name}/:drawingId?`, name, component: { template: '<p>功能入口已打开</p>' } }))] })
const app = createApp({ render: () => h('div', { style: 'height:100vh;width:100%;display:flex;flex-direction:column' }, [h('div', { style: 'display:flex;gap:16px;padding:6px 28px;background:var(--bg);font-size:12px' }, [h('span', '本地示例数据验收'), ...['designer', 'reviewer', 'admin'].map(role => h('button', { onClick: () => { auth.currentUser = { ...auth.currentUser, roles: [role] } } }, role)), h('button', { onClick: () => { auth.currentUser = { ...auth.currentUser, roles: ['admin', 'designer', 'reviewer'] } } }, '多角色'), h('button', { onClick: () => { document.documentElement.dataset.theme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light' } }, '切换明暗'), h('button', { onClick: () => { empty = !empty; auth.currentUser = { ...auth.currentUser, id: empty ? 'empty' : 'u1' } } }, '空数据')]), h(RouterView)]) })
app.use(pinia).use(router).mount('#app')
