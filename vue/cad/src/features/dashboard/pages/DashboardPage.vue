<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useSystemStatusStore } from '@/stores/system-status.store'

defineOptions({
  name: 'DashboardPage',
})

const domainStore = useDomainStore()
const systemStore = useSystemStatusStore()
const router = useRouter()

onMounted(() => {
  systemStore.fetchStatus()
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
  const { total, assemblies, parts } = domainStore.drawingStats ?? { total: 0, assemblies: 0, parts: 0 }

  return [
    { label: '图纸总数', value: String(total), icon: 'layers', delta: total ? '当前库内总计' : '暂无图纸数据' },
    { label: '总图 / 零件图', value: `${assemblies} / ${parts}`, icon: 'box', delta: `总图 ${assemblies} · 零件 ${parts}` },
    { label: '待我审核', value: String(domainStore.reviewCount), icon: 'clipboard-check', delta: domainStore.reviewCount ? '请及时处理待办' : '暂无待审核记录' },
  ]
})

function openCreateDrawing() {
  router.push({ name: 'drawing-create' })
}

function openLibrary() {
  router.push({ name: 'drawing-library' })
}

function openReview(no: string) {
  domainStore.openDrawing(no)
  router.push({ name: 'drawing-preview', params: { drawingId: no } })
}
</script>

<template>
  <div class="page dashboard-page">
    <div class="dash-hero">
      <div>
        <h1>{{ greeting }}</h1>
        <p>{{ todayText }} · 有 {{ domainStore.reviewCount }} 份审核任务等待处理</p>
      </div>
      <div class="acts">
        <button class="btn primary" type="button" @click="openCreateDrawing">
          <DemoIcon name="plus" :size="14" />创建图纸
        </button>
      </div>
    </div>

    <div class="stat-grid">
      <div v-for="stat in stats" :key="stat.label" class="card stat-card">
        <div class="lbl"><DemoIcon :name="stat.icon" :size="14" />{{ stat.label }}</div>
        <div class="val">{{ stat.value }}</div>
        <div class="delta">
          {{ stat.delta }}
        </div>
      </div>
    </div>

    <div class="dash-grid">
      <div class="card">
        <div class="card-title">
          <DemoIcon name="activity" :size="16" />最近动态
          <span class="hint">谁查看 · 谁修改 · 谁分叉 · 谁上传</span>
        </div>
        <div class="feed">
          <div v-for="item in domainStore.logs" :key="`${item.user}-${item.time}-${item.txt}`" class="feed-item">
            <div class="feed-ic" :class="item.act"><DemoIcon :name="feedIcons[item.act] ?? 'activity'" :size="14" /></div>
            <div class="feed-txt"><b>{{ item.user }}</b> <span v-html="item.txt"></span></div>
            <div class="feed-time">{{ item.time }}</div>
          </div>
          <div v-if="!domainStore.logs.length" class="empty">
            <DemoIcon name="activity" :size="34" />
            <div class="t">暂无动态记录</div>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="card-title">
          <DemoIcon name="stamp" :size="16" />待我审核
          <span class="hint">{{ domainStore.reviewCount }} 项</span>
        </div>
        <div v-if="domainStore.myReviews.length" class="todo-list">
          <div v-for="item in domainStore.myReviews" :key="`${item.reviewCaseId}-${item.node}`" class="todo-item">
            <div class="todo-info">
              <b>{{ item.name }} · {{ item.node }}</b>
              <span>{{ item.no }} · {{ item.by }} 发起于 {{ item.time }}</span>
            </div>
            <div class="todo-acts">
              <button class="btn sm primary" type="button" @click="openReview(item.no)">
                <DemoIcon name="eye" :size="14" />查看并审核
              </button>
            </div>
          </div>
        </div>
        <div v-else class="empty">
          <DemoIcon name="check-circle-2" :size="34" />
          <div class="t">太棒了，暂无待办审核</div>
        </div>
      </div>
    </div>

    <div class="dash-foot">
      <!-- 转换服务模块：暂不确定是否保留，先作隐藏留痕处理 -->
      <!--
      <div class="card card-pad">
        <div class="card-title compact-title"><DemoIcon name="server" :size="16" />转换服务</div>
        <div class="mini-row"><span>EXB → PDF 队列</span><b>暂无数据</b></div>
        <div class="mini-row"><span>DWG → PDF 队列</span><b>暂无数据</b></div>
        <div class="mini-row"><span>今日转换</span><b>暂无数据</b></div>
      </div>
      -->

      <div class="card card-pad">
        <div class="card-title compact-title"><DemoIcon name="hard-drive" :size="16" />文件存储</div>
        <div class="mini-row"><span>图纸文件</span><b>{{ systemStore.storageSummary.fileCount }} 个 ({{ systemStore.storageSummary.formattedUsed }})</b></div>
        <div class="hbar"><i :style="{ width: systemStore.storageSummary.fileCount ? '18%' : '0%' }"></i></div>
        <div class="mini-row storage-row"><span>冗余备份</span><b>{{ systemStore.storageSummary.backupStatus }}</b></div>
      </div>

      <div class="card card-pad">
        <div class="card-title compact-title"><DemoIcon name="folder-tree" :size="16" />进行中项目</div>
        <div class="empty compact-empty">
          <DemoIcon name="folder-tree" :size="28" />
          <div class="t">暂无进行中项目</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dashboard-page {
  display: flex;
  flex-direction: column;
}

.dash-hero {
  display: flex;
  align-items: flex-end;
  gap: 16px;
  margin-bottom: 20px;
}

.dash-hero h1 {
  font-family: var(--font-display);
  font-size: 23px;
  font-weight: 900;
}

.dash-hero p {
  margin-top: 5px;
  color: var(--text-3);
  font-size: 12px;
}

.acts {
  display: flex;
  gap: 9px;
  margin-left: auto;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 14px;
  margin-bottom: 16px;
}

.stat-card {
  padding: 17px 19px;
}

html[data-skin='tech'] .stat-card::before {
  position: absolute;
  top: 10px;
  right: 12px;
  width: 11px;
  height: 11px;
  border-top: 1.5px solid var(--accent);
  border-right: 1.5px solid var(--accent);
  border-radius: 2px;
  content: '';
  opacity: 0.5;
}

.lbl {
  display: flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 10px;
  color: var(--text-2);
  font-size: 12px;
}

.lbl svg {
  color: var(--accent);
}

.val {
  font-family: 'JetBrains Mono', monospace;
  font-size: 29px;
  font-weight: 700;
  letter-spacing: -0.5px;
  line-height: 1;
}

html[data-skin='tech'] .val {
  text-shadow: 0 0 18px var(--glow);
}

.delta {
  margin-top: 9px;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.delta b {
  color: var(--ok);
  font-weight: 500;
}

.dash-grid {
  display: grid;
  grid-template-columns: 1.55fr 1fr;
  gap: 14px;
  margin-bottom: 14px;
}

.feed {
  display: flex;
  flex-direction: column;
}

.feed-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 20px;
  border-bottom: 1px solid var(--line);
}

.feed-item:last-child {
  border-bottom: none;
}

.feed-ic {
  display: grid;
  width: 29px;
  height: 29px;
  flex: none;
  place-items: center;
  border: 1px solid var(--line);
  border-radius: 9px;
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
  font-size: 12.5px;
  line-height: 1.55;
}

.feed-txt b {
  color: var(--accent);
  font-weight: 500;
}

.feed-time {
  flex: none;
  margin-top: 2px;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
}

.todo-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 13px 20px;
  border-bottom: 1px solid var(--line);
}

.todo-item:last-child {
  border-bottom: none;
}

.todo-info {
  flex: 1;
  min-width: 0;
}

.todo-info b,
.todo-info span {
  display: block;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.todo-info b {
  font-size: 12.5px;
}

.todo-info span {
  color: var(--text-3);
  font-size: 11px;
}

.todo-acts {
  display: flex;
  flex: none;
  gap: 6px;
}

.dash-foot {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 14px;
}

.compact-title {
  padding: 0 0 12px;
}

.hbar {
  height: 7px;
  margin-top: 9px;
  overflow: hidden;
  border-radius: 99px;
  background: var(--panel-2);
}

.hbar i {
  display: block;
  height: 100%;
  border-radius: 99px;
  background: linear-gradient(90deg, var(--accent), var(--accent-2));
  animation: grow-bar 1.1s cubic-bezier(0.2, 0.8, 0.3, 1);
}

@keyframes grow-bar {
  from { width: 0 !important; }
}

.mini-row {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 5px 0;
  color: var(--text-2);
  font-size: 12px;
}

.mini-row b {
  color: var(--text-1);
  font-family: 'JetBrains Mono', monospace;
  font-weight: 500;
  text-align: right;
}

.storage-row {
  margin-top: 10px;
}

.ok-text {
  color: var(--ok) !important;
}

@media (max-width: 1180px) {
  .dash-grid,
  .dash-foot {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 760px) {
  .dash-hero {
    align-items: flex-start;
    flex-direction: column;
  }

  .acts {
    margin-left: 0;
  }

  .acts .btn {
    padding: 7px 9px;
  }

  .todo-item {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
