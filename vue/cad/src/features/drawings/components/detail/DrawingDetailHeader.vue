<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { loadTitleBlock, savedDesigner } from '@/services/drawing-title-block.service'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS } from '@/constants/drawing-status'
import { changeRequestService } from '@/services/change-request.service'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { useWorkspaceStore } from '@/stores/workspace.store'
import { formatReadableDateTime } from '@/utils/date-time'
import { useRoute } from 'vue-router'

defineOptions({
  name: 'DrawingDetailHeader',
})

const router = useRouter()
const route = useRoute()
const drawingStore = useDrawingStore()
const workspaceStore = useWorkspaceStore()
const authStore = useAuthStore()

const drawing = computed(() => {
  const id = String(route.params.drawingId ?? '')
  return drawingStore.getDrawing(id) ?? drawingStore.getPart(id)
})
const isPart = computed(() => Boolean(drawing.value && 'parentNo' in drawing.value))
const parentDrawing = computed(() => {
  const parentNo = isPart.value ? (drawing.value as { parentNo: string }).parentNo : ''
  return drawingStore.getDrawing(parentNo) ?? drawingStore.getPart(parentNo)
})

const statusMeta = computed(() => (drawing.value ? STATUS[drawing.value.status] ?? null : null))

const titleFiles = computed(() => [...(drawing.value?.files ?? []), ...(drawing.value?.otherFiles ?? [])].filter(file => /\.(exb|dwg|dxf)$/i.test(file.name)))
watch(() => titleFiles.value.map(file => `${file.id}:${file.version}`).join('|'), () => {
  for (const file of titleFiles.value) void loadTitleBlock(file.id).catch(() => undefined)
}, { immediate: true })
const designerName = computed(() => savedDesigner(titleFiles.value.map(file => file.id)))
const changing = ref(false)
watch(
  [() => (drawing.value && 'parentNo' in drawing.value ? '' : drawing.value?.id), () => authStore.isAuthenticated],
  ([id]) => {
    changing.value = false
    // 未登录（会话还没恢复）时不要发请求：无鉴权请求只会拿回 401，把「变更中」误判成无变更。
    if (!id || drawing.value?.status !== 'archived' || !authStore.isAuthenticated) return
    void changeRequestService.listByDrawing(id).then((requests) => {
      changing.value = requests.some((item) => ['pending_approval', 'executing', 'pending_verify'].includes(item.status))
    }).catch(() => { changing.value = false })
  },
  { immediate: true },
)

const creatorLabel = computed(() => {
  const item = drawing.value as { createdBy?: string } | null
  const value = item?.createdBy?.trim() || ''
  return value && value !== '当前用户' ? value : '未知'
})

const assigneeLabel = computed(() => {
  const list = (drawing.value as { assignees?: Array<{ name?: string }> } | null)?.assignees ?? []
  return list.map(item => item.name?.trim()).filter(Boolean).join('、')
})

const drawerOpen = ref(false)

const projectLabel = computed(() => (drawing.value as { project?: string } | null)?.project?.trim() || '—')
const vendorLabel = computed(() => (drawing.value as { vendor?: string } | null)?.vendor?.trim() || '内部项目部')
const updatedLabel = computed(() => drawing.value?.updatedAt ? formatReadableDateTime(drawing.value.updatedAt, '刚刚', false) : '刚刚')

function openProperties() {
  if (!drawing.value) return
  drawerOpen.value = true
}

function openParentDrawing() {
  if (!parentDrawing.value) return
  drawerOpen.value = false
  workspaceStore.selectDrawing(parentDrawing.value.id)
  router.push({ name: 'drawing-preview', params: { drawingId: parentDrawing.value.no }, query: route.query.from === 'review' ? route.query : {} })
}
</script>

<template>
  <div class="detail-head">
    <div class="crumb">
      <button type="button" @click="router.push({ name: 'dashboard' })">工作台</button>
      <DemoIcon name="chevron-right" :size="12" />
      <button type="button" @click="router.push({ name: 'drawing-library' })">图纸库</button>
      <DemoIcon name="chevron-right" :size="12" />
      <span>{{ drawing?.no }}</span>
    </div>

    <div class="dh-row">
      <div class="dh-main">
        <div class="dh-title">
          <h2>{{ drawing?.name }}</h2>
          <span class="dh-no">{{ drawing?.no }}</span>
          <span class="tag mute tag-no-dot">{{ drawing?.version }}</span>
          <span v-if="statusMeta" class="tag" :class="statusMeta.c">{{ statusMeta.t }}</span>
          <span v-if="changing" class="tag warn tag-no-dot">变更中</span>
          <span v-if="assigneeLabel" class="assignee-badge">
            <DemoIcon name="user-check" :size="13" />
            负责人 <b>{{ assigneeLabel }}</b>
          </span>
        </div>
      </div>

        <div class="dh-acts">
          <button v-if="route.query.from === 'review'" class="btn primary" type="button" @click="router.push({ name: 'review-workspace', params: { drawingNo: String(route.query.reviewNo || drawing?.no || '') } })"><DemoIcon name="arrow-left" :size="14" />返回本次审核</button>
          <button class="btn" type="button" @click="openProperties"><DemoIcon name="info" :size="14" />属性详情</button>
        </div>
    </div>

    <Teleport to="body">
      <Transition name="dh-drawer">
        <div v-if="drawerOpen" class="dh-overlay" @click="drawerOpen = false">
          <aside class="dh-drawer" role="dialog" aria-modal="true" aria-label="属性详情" @click.stop>
            <header class="dh-drawer-head">
              <div>
                <span class="dh-drawer-eyebrow">属性详情</span>
                <h3>{{ drawing?.name }}</h3>
                <span class="dh-drawer-sub mono">{{ drawing?.no }}</span>
              </div>
              <button class="dh-drawer-close" type="button" aria-label="关闭" @click="drawerOpen = false"><DemoIcon name="x" :size="16" /></button>
            </header>
            <div class="dh-drawer-body">
              <div class="dh-kv"><span>负责人</span><b>{{ assigneeLabel || '待指派' }}</b></div>
              <div class="dh-kv"><span>创建人</span><b>{{ creatorLabel }}</b></div>
              <div class="dh-kv"><span>更新时间</span><b class="mono">{{ updatedLabel }}</b></div>
              <div class="dh-kv"><span>所属项目</span><b>{{ projectLabel }}</b></div>
              <div class="dh-kv"><span>责任单位</span><b>{{ vendorLabel }}</b></div>
              <div class="dh-kv"><span>设计</span><b>{{ designerName || '—' }}</b></div>
              <div class="dh-kv"><span>发布版本</span><b class="mono">{{ drawing?.version }}</b></div>
              <div class="dh-kv"><span>生命周期状态</span><b>{{ statusMeta?.t || '—' }}</b></div>
              <div class="dh-kv"><span>对象类型</span><b>{{ isPart ? '零件图' : '项目总图' }}</b></div>
              <div v-if="isPart" class="dh-kv"><span>关联图号</span><b class="mono">{{ drawing?.no }}</b></div>
              <div v-if="isPart && parentDrawing" class="dh-kv"><span>所属总图</span><button class="parent-link" type="button" @click="openParentDrawing">{{ parentDrawing.name }}<DemoIcon name="arrow-up-right" :size="12" /></button></div>
              <div v-if="'forkedFrom' in (drawing || {}) && (drawing as any).forkedFrom" class="dh-kv"><span>分叉自</span><b class="mono">{{ (drawing as any).forkedFrom }}</b></div>
            </div>
          </aside>
        </div>
      </Transition>
    </Teleport>

  </div>
</template>

<style scoped>
.dh-title .dh-no {
  border-color: var(--line);
  background: var(--panel-2);
  color: var(--text-2);
}

.dh-title .tag {
  padding: 2px 9px;
  border-color: var(--line);
  background: var(--panel-2);
  color: var(--text-2);
  font-weight: 500;
}

.dh-title .tag::before {
  width: 6px;
  height: 6px;
  background: var(--text-3);
}

.dh-title .tag.ok::before { background: var(--ok); }
.dh-title .tag.warn::before { background: var(--warn); }
.dh-title .tag.info::before { background: var(--info); }
.dh-title .tag.danger::before { background: var(--danger); }

.assignee-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2.5px 10px;
  border-radius: 99px;
  background: var(--accent);
  color: var(--accent-ink);
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
}

.assignee-badge b {
  font-weight: 700;
}

.parent-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--accent);
  font-size: inherit;
}

.parent-link:hover {
  text-decoration: underline;
}

.parent-link b {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dh-overlay {
  position: fixed;
  z-index: 60;
  inset: 0;
  display: flex;
  justify-content: flex-end;
  background: rgb(0 0 0 / 42%);
}

.dh-drawer {
  display: flex;
  flex-direction: column;
  width: min(400px, 92vw);
  height: 100%;
  border-left: 1px solid var(--line);
  background: var(--panel-top);
  box-shadow: -20px 0 50px rgb(0 0 0 / 25%);
}

.dh-drawer-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 14px;
  padding: 20px 22px;
  border-bottom: 1px solid var(--line);
}

.dh-drawer-eyebrow {
  color: var(--accent);
  font-size: 10px;
  letter-spacing: .08em;
}

.dh-drawer-head h3 {
  margin-top: 6px;
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 800;
}

.dh-drawer-sub {
  display: block;
  margin-top: 5px;
  color: var(--text-3);
  font-size: 11px;
}

.dh-drawer-close {
  display: grid;
  flex: none;
  width: 30px;
  height: 30px;
  place-items: center;
  border-radius: 8px;
  color: var(--text-3);
}

.dh-drawer-close:hover {
  background: var(--hover);
  color: var(--text-1);
}

.dh-drawer-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 14px 22px 30px;
}

.dh-kv {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 11px 0;
  border-bottom: 1px dashed var(--line);
}

.dh-kv span {
  flex: none;
  color: var(--text-3);
  font-size: 12px;
}

.dh-kv b {
  min-width: 0;
  color: var(--text-1);
  font-size: 13px;
  font-weight: 600;
  text-align: right;
  word-break: break-all;
}

.dh-drawer-enter-active,
.dh-drawer-leave-active { transition: opacity .25s ease; }

.dh-drawer-enter-active .dh-drawer,
.dh-drawer-leave-active .dh-drawer { transition: transform .3s cubic-bezier(.22, .8, .3, 1); }

.dh-drawer-enter-from,
.dh-drawer-leave-to { opacity: 0; }

.dh-drawer-enter-from .dh-drawer,
.dh-drawer-leave-to .dh-drawer { transform: translateX(100%); }

@media (max-width: 760px) {
  .parent-link {
    width: 100%;
  }
}
</style>
