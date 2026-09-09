<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useSystemStatusStore } from '@/stores/system-status.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useReviewStore } from '@/stores/review.store'
import { useAuditStore } from '@/stores/audit.store'

defineOptions({
  name: 'DashboardPage',
})

const systemStore = useSystemStatusStore()
const drawingStore = useDrawingStore()
const reviewStore = useReviewStore()
const auditStore = useAuditStore()
const router = useRouter()

onMounted(() => {
  systemStore.fetchStatus()
  void Promise.all([
    drawingStore.load(),
    reviewStore.load(),
    auditStore.loadDrawing({ page: 1, pageSize: 20 }),
  ]).catch(() => undefined)
})

const currentHour = new Date().getHours()
const greeting = currentHour < 6 ? '晚上好' : currentHour < 12 ? '早上好' : currentHour < 18 ? '下午好' : '晚上好'
const todayText = new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'short' }).format(new Date())

const feedIcons: Record<string, string> = {
  view: 'eye',
  create: 'plus',
  edit: 'pencil',
  branch: 'git-branch',
  upload: 'upload',
  download: 'download',
  delete: 'trash-2',
  check: 'check-circle-2',
  parse: 'file-search',
}

const stats = computed(() => {
  const assemblies = drawingStore.drawings.filter((item) => item.kind === '总图').length
  const parts = drawingStore.parts.length
  const total = assemblies + parts
  const reviewCount = reviewStore.myPendingReviews().length

  return [
    { label: '图纸总数', value: String(total), icon: 'layers', delta: total ? '库内总计' : '暂无图纸' },
    { label: '总图 / 零件', value: `${assemblies} / ${parts}`, icon: 'box', delta: `总图 ${assemblies} · 零件 ${parts}` },
    { label: '待我审核', value: String(reviewCount), icon: 'clipboard-check', delta: reviewCount ? '待处理' : '暂无待办' },
  ]
})

function openCreateDrawing() {
  router.push({ name: 'drawing-create' })
}

function openLibrary() {
  router.push({ name: 'drawing-library' })
}

function openReview(no: string) {
  router.push({ name: 'drawing-preview', params: { drawingId: no } })
}
</script>

<template>
  <div class="page dashboard-page">
    <!-- 顶部问候与快捷操作行 -->
    <header class="dash-hero">
      <div class="hero-text-wrap">
        <div class="hero-title-row">
          <div class="hero-status-dot"></div>
          <h1>{{ greeting }}，工程师</h1>
        </div>
        <p class="hero-subtitle">{{ todayText }} · 当前有 <b>{{ reviewStore.myPendingReviews().length }}</b> 份审核任务等待处理</p>
      </div>
      <div class="acts">
        <button class="btn secondary sm" type="button" @click="openLibrary">
          <DemoIcon name="folder-kanban" :size="14" />浏览图库
        </button>
        <button class="btn primary sm" type="button" @click="openCreateDrawing">
          <DemoIcon name="plus" :size="14" />新建工程图纸
        </button>
      </div>
    </header>

    <!-- 方案 A：工业仪表盘标准双列布局（满屏高度严丝合缝对齐） -->
    <div class="dash-main-container">
      <!-- 左列：协同业务与实时信息流（待我审核 + 最近动态） -->
      <section class="dash-col-left">
        <!-- 待我审核卡片 -->
        <div class="card panel-card todo-panel">
          <div class="panel-header">
            <div class="panel-title">
              <DemoIcon name="stamp" :size="15" />
              <span>待我审核</span>
              <span class="count-tag" :class="{ highlight: reviewStore.myPendingReviews().length > 0 }">
                {{ reviewStore.myPendingReviews().length }} 项
              </span>
            </div>
            <span class="panel-hint">流程审批与签批流转</span>
          </div>

          <div class="panel-scroll-content">
            <div v-if="reviewStore.myPendingReviews().length" class="todo-list">
              <div
                v-for="item in reviewStore.myPendingReviews()"
                :key="`${item.reviewCaseId}-${item.node}`"
                class="todo-item"
              >
                <div class="todo-info">
                  <div class="todo-name-row">
                    <b>{{ item.name }}</b>
                    <span class="todo-node-tag">{{ item.node }}</span>
                  </div>
                  <span class="todo-meta">图号 {{ item.no }} · 发起人 {{ item.by }} · {{ item.time }}</span>
                </div>
                <button class="btn sm primary" type="button" @click="openReview(item.no)">
                  <DemoIcon name="eye" :size="13" />审核
                </button>
              </div>
            </div>
            <div v-else class="compact-empty">
              <DemoIcon name="check-circle-2" :size="30" />
              <div class="empty-title">当前暂无待审核任务</div>
              <div class="empty-desc">所有发起的图纸流程均已处理完毕</div>
            </div>
          </div>
        </div>

        <!-- 最近动态卡片 -->
        <div class="card panel-card activity-panel">
          <div class="panel-header">
            <div class="panel-title">
              <DemoIcon name="activity" :size="15" />
              <span>最近动态</span>
              <span class="count-tag">{{ auditStore.drawingLogs.list.length }} 条记录</span>
            </div>
            <span class="panel-hint">修改 · 审图 · 借用 · 检出</span>
          </div>

          <div class="panel-scroll-content">
            <div v-if="auditStore.drawingLogs.list.length" class="feed">
              <div
                v-for="item in auditStore.drawingLogs.list"
                :key="item.id"
                class="feed-item"
              >
                <div class="feed-ic" :class="item.act">
                  <DemoIcon :name="feedIcons[item.act] ?? 'activity'" :size="13" />
                </div>
                <div class="feed-txt">
                  <b>{{ item.user }}</b> <span v-html="item.txt"></span>
                </div>
                <div class="feed-time">{{ item.time }}</div>
              </div>
            </div>
            <div v-else class="compact-empty">
              <DemoIcon name="activity" :size="30" />
              <div class="empty-title">暂无图纸动态</div>
              <div class="empty-desc">图纸的检出、借用与修改历史将实时呈现于此</div>
            </div>
          </div>
        </div>
      </section>

      <!-- 右列：宏观指标与系统状态（指标卡 + 文件存储 + 进行中项目） -->
      <section class="dash-col-right">
        <!-- 顶部指标卡组（三等分） -->
        <div class="stat-row-grid">
          <div v-for="stat in stats" :key="stat.label" class="card stat-card">
            <div class="stat-top">
              <span class="stat-label">{{ stat.label }}</span>
              <div class="stat-icon-wrap">
                <DemoIcon :name="stat.icon" :size="14" />
              </div>
            </div>
            <div class="stat-val">{{ stat.value }}</div>
            <div class="stat-sub">{{ stat.delta }}</div>
          </div>
        </div>

        <!-- 中部：文件存储看板 -->
        <div class="card panel-card storage-card">
          <div class="panel-header">
            <div class="panel-title">
              <DemoIcon name="hard-drive" :size="15" />
              <span>文件存储与备份</span>
            </div>
            <span class="status-pill ok">
              <span class="dot"></span>正常运行
            </span>
          </div>

          <div class="storage-content">
            <div class="storage-meta-row">
              <span class="storage-lbl">CAD 图纸物理资产</span>
              <b class="storage-num">{{ systemStore.storageSummary.fileCount }} 份 ({{ systemStore.storageSummary.formattedUsed }})</b>
            </div>
            <div class="hbar">
              <i :style="{ width: systemStore.storageSummary.fileCount ? '24%' : '0%' }"></i>
            </div>
            <div class="storage-meta-row storage-sub-row">
              <span class="storage-lbl">冗余冷备与落盘状态</span>
              <span class="storage-status-text">{{ systemStore.storageSummary.backupStatus }}</span>
            </div>
          </div>
        </div>

        <!-- 下部：进行中项目（占满剩余高度，与左侧对齐） -->
        <div class="card panel-card projects-card">
          <div class="panel-header">
            <div class="panel-title">
              <DemoIcon name="folder-tree" :size="15" />
              <span>进行中工程项目</span>
            </div>
            <button class="btn sm secondary text-btn" type="button" @click="openCreateDrawing">
              <DemoIcon name="plus" :size="12" />新项目
            </button>
          </div>

          <div class="projects-content-box">
            <div class="compact-empty">
              <div class="empty-icon-circle">
                <DemoIcon name="folder-tree" :size="26" />
              </div>
              <div class="empty-title">暂无活跃的研发项目</div>
              <div class="empty-desc">在图纸库中关联新产品线或工程任务后将自动展示进度</div>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
/* 满屏标准仪表盘：高度定死，内部自适应无页面纵向滚动 */
.dashboard-page {
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  max-height: 100%;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: 14px 18px 16px;
  gap: 12px;
}

/* 顶部问候栏 */
.dash-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex: none;
}

.hero-text-wrap {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.hero-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.hero-status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 8px var(--glow);
}

.dash-hero h1 {
  margin: 0;
  font-family: var(--font-display);
  font-size: 19px;
  font-weight: 800;
  letter-spacing: 0.3px;
  color: var(--text-1);
}

.hero-subtitle {
  margin: 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.hero-subtitle b {
  color: var(--accent);
  font-weight: 600;
}

.acts {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

/* 左右两列主体容器：占满屏幕剩余高度 */
.dash-main-container {
  display: grid;
  grid-template-columns: 1.15fr 1fr;
  gap: 12px;
  flex: 1;
  min-height: 0;
}

/* 左列：待我审核 + 最近动态（上下各占 50% 撑满高度） */
.dash-col-left {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  height: 100%;
}

.todo-panel {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.activity-panel {
  flex: 1.15;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 右列：统计指标 + 存储 + 项目（三块垂直排布，项目撑满到底） */
.dash-col-right {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  height: 100%;
}

/* 通用面板卡片规范 */
.panel-card {
  padding: 12px 14px;
  border-radius: 11px;
  border: 1px solid var(--line);
  background: var(--panel);
  display: flex;
  flex-direction: column;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--line);
  flex: none;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-1);
}

.panel-title svg {
  color: var(--accent);
}

.count-tag {
  font-size: 10.5px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 99px;
  background: var(--panel-2);
  color: var(--text-3);
}

.count-tag.highlight {
  background: var(--accent-soft);
  color: var(--accent);
}

.panel-hint {
  font-size: 11px;
  color: var(--text-3);
}

.panel-scroll-content {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  margin-top: 4px;
}

/* 待我审核列表 */
.todo-list {
  display: flex;
  flex-direction: column;
}

.todo-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 4px;
  border-bottom: 1px solid var(--line);
}

.todo-item:last-child {
  border-bottom: none;
}

.todo-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.todo-name-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.todo-name-row b {
  font-size: 12px;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.todo-node-tag {
  font-size: 10px;
  font-weight: 500;
  padding: 0 5px;
  border-radius: 4px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  color: var(--accent);
}

.todo-meta {
  font-size: 10.5px;
  color: var(--text-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 最近动态列表 */
.feed {
  display: flex;
  flex-direction: column;
}

.feed-item {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  padding: 7px 4px;
  border-bottom: 1px solid var(--line);
}

.feed-item:last-child {
  border-bottom: none;
}

.feed-ic {
  display: grid;
  width: 22px;
  height: 22px;
  flex: none;
  place-items: center;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: var(--panel-2);
}

.feed-ic.view svg { color: var(--info); }
.feed-ic.edit svg { color: var(--warn); }
.feed-ic.branch svg { color: var(--accent); }
.feed-ic.borrow svg { color: var(--accent-2); }
.feed-ic.check svg { color: var(--ok); }
.feed-ic.back svg { color: var(--danger); }

.feed-txt {
  flex: 1;
  font-size: 11.5px;
  line-height: 1.45;
  color: var(--text-2);
}

.feed-txt b {
  color: var(--accent);
  font-weight: 600;
}

.feed-time {
  flex: none;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-3);
  margin-top: 1px;
}

/* 右列：指标卡网格（3 列） */
.stat-row-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  flex: none;
}

.stat-card {
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid var(--line);
  background: var(--panel);
  display: flex;
  flex-direction: column;
  gap: 4px;
  position: relative;
  overflow: hidden;
}

.stat-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.stat-label {
  font-size: 11px;
  color: var(--text-3);
  font-weight: 500;
}

.stat-icon-wrap {
  width: 22px;
  height: 22px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--accent);
  display: grid;
  place-items: center;
}

.stat-val {
  font-family: 'JetBrains Mono', monospace;
  font-size: 21px;
  font-weight: 800;
  color: var(--text-1);
  letter-spacing: -0.5px;
  line-height: 1.1;
}

.stat-sub {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 存储卡片 */
.storage-card {
  flex: none;
}

.storage-content {
  display: flex;
  flex-direction: column;
  gap: 7px;
  margin-top: 8px;
}

.storage-meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11.5px;
}

.storage-lbl {
  color: var(--text-2);
}

.storage-num {
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-1);
  font-weight: 600;
}

.storage-status-text {
  font-size: 11px;
  color: var(--ok);
  font-weight: 500;
}

.hbar {
  height: 5px;
  overflow: hidden;
  border-radius: 99px;
  background: var(--panel-2);
}

.hbar i {
  display: block;
  height: 100%;
  border-radius: 99px;
  background: linear-gradient(90deg, var(--accent), var(--accent-2));
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 99px;
}

.status-pill.ok {
  background: var(--ok-soft, rgba(34, 197, 94, 0.12));
  color: var(--ok);
}

.status-pill .dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}

/* 进行中项目卡片（flex: 1 撑到底） */
.projects-card {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.projects-content-box {
  flex: 1;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.text-btn {
  padding: 2px 7px;
  font-size: 11px;
}

/* 精致空状态 */
.compact-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 20px 12px;
  text-align: center;
  height: 100%;
  color: var(--text-3);
}

.compact-empty svg {
  color: var(--text-3);
  opacity: 0.6;
}

.empty-icon-circle {
  width: 44px;
  height: 44px;
  border-radius: 50%;
  background: var(--panel-2);
  display: grid;
  place-items: center;
  margin-bottom: 2px;
}

.empty-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}

.empty-desc {
  font-size: 11px;
  color: var(--text-3);
  max-width: 240px;
  line-height: 1.4;
}

/* 响应式断点（窗口过窄时自然降级） */
@media (max-width: 1080px) {
  .dashboard-page {
    height: auto;
    max-height: none;
    overflow-y: auto;
  }
  .dash-main-container {
    grid-template-columns: 1fr;
  }
}
</style>
