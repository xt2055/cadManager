<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { loadTitleBlock, savedDesigner } from '@/services/drawing-title-block.service'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS } from '@/constants/drawing-status'
import { changeRequestService } from '@/services/change-request.service'
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
watch(() => drawing.value && 'parentNo' in drawing.value ? '' : drawing.value?.id, (id) => {
  changing.value = false
  if (!id || drawing.value?.status !== 'archived') return
  void changeRequestService.listByDrawing(id).then((requests) => {
    changing.value = requests.some((item) => ['pending_approval', 'executing', 'pending_verify'].includes(item.status))
  }).catch(() => { changing.value = false })
}, { immediate: true })

const creatorLabel = computed(() => {
  const item = drawing.value as { createdBy?: string } | null
  const value = item?.createdBy?.trim() || ''
  return value && value !== '当前用户' ? value : '未知'
})

function openProperties() {
  if (!drawing.value) return
  router.push({ name: 'drawing-properties', params: { drawingId: drawing.value.no }, query: route.query.from === 'review' ? route.query : {} })
}

function openParentDrawing() {
  if (!parentDrawing.value) return
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
           <span v-if="statusMeta" class="tag" :class="statusMeta.c">{{ statusMeta.t }}</span>
           <span v-if="designerName" class="designer-badge">
             <DemoIcon name="pen-tool" :size="13" />
             设计：<b>{{ designerName }}</b>
           </span>
          <span class="dh-no">{{ drawing?.no }}</span>
           <span class="tag mute tag-no-dot">{{ drawing?.version }}</span>
           <span v-if="changing" class="tag warn tag-no-dot">变更中</span>
         </div>
         <div class="dh-meta">
           <span>厂商 <b>{{ ('vendor' in (drawing || {})) ? (drawing as any).vendor : '内部加工' }}</b></span>
          <span>项目 <b>{{ ('project' in (drawing || {})) ? (drawing as any).project : '—' }}</b></span>
          <span v-if="isPart">关联图号 <b class="mono">{{ drawing?.no }}</b></span>
          <span>创建人 <b>{{ creatorLabel }}</b></span>
          <span>更新时间 <b>{{ drawing?.updatedAt ? formatReadableDateTime(drawing.updatedAt, '刚刚') : '刚刚' }}</b></span>
          <span v-if="'forkedFrom' in (drawing || {}) && (drawing as any).forkedFrom" class="tag plain">分叉自·{{ (drawing as any).forkedFrom }}</span>
          <button v-if="isPart && parentDrawing" class="parent-link" type="button" @click="openParentDrawing">
            所属总图 <b>{{ parentDrawing.name }}</b><DemoIcon name="arrow-up-right" :size="12" />
          </button>
        </div>
       </div>

        <div class="dh-acts">
          <button v-if="route.query.from === 'review'" class="btn primary" type="button" @click="router.push({ name: 'review-workspace', params: { drawingNo: String(route.query.reviewNo || drawing?.no || '') } })"><DemoIcon name="arrow-left" :size="14" />返回本次审核</button>
          <button class="btn" type="button" @click="openProperties"><DemoIcon name="info" :size="14" />属性详情</button>
        </div>
    </div>

  </div>
</template>

<style scoped>
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

@media (max-width: 760px) {
.designer-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: var(--bg-card);
  border: 1px solid var(--accent);
  color: var(--accent);
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  margin-left: 2px;
}
.designer-badge b {
  font-weight: 700;
}
.parent-link {
    width: 100%;
  }
}
</style>
