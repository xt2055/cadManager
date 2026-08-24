<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({
  name: 'DrawingDetailHeader',
})

const route = useRoute()
const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()

const drawing = computed(() => domainStore.currentDrawing)
const isPart = computed(() => Boolean(drawing.value && 'parentNo' in drawing.value))
const parentDrawing = computed(() => {
  const parentNo = isPart.value ? (drawing.value as { parentNo: string }).parentNo : ''
  return domainStore.drawings.find((item) => item.no === parentNo) ?? null
})

function openModal(type: 'revert' | 'borrow-drawing', title: string) {
  uiStore.openModal(type, title)
}

function openProperties() {
  if (!drawing.value) return
  router.push({ name: 'drawing-properties', params: { drawingId: drawing.value.no } })
}

function openParentDrawing() {
  if (!parentDrawing.value) return
  domainStore.openDrawing(parentDrawing.value.no)
  router.push({ name: 'drawing-preview', params: { drawingId: parentDrawing.value.no } })
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
          <span class="tag plain tag-no-dot">{{ 'parentNo' in (drawing || {}) ? '零件图' : '总图' }}</span>
          <span v-if="drawing" class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span>
          <span class="tag mute tag-no-dot">{{ drawing?.ver }}</span>
        </div>
        <div class="dh-meta">
          <span>材料 <b>{{ drawing?.material }}</b></span>
          <span>厂商 <b>{{ ('vendor' in (drawing || {})) ? (drawing as any).vendor : '内部加工' }}</b></span>
          <span>项目 <b>{{ ('project' in (drawing || {})) ? (drawing as any).project : '—' }}</b></span>
          <span>创建人 <b>{{ ('createdBy' in (drawing || {}) && (drawing as any).createdBy) ? (drawing as any).createdBy : ('by' in (drawing || {})) ? (drawing as any).by : '待定' }}</b></span>
          <span>更新时间 <b>{{ ('updated' in (drawing || {})) ? (drawing as any).updated : '刚刚' }}</b></span>
          <span v-if="'forkedFrom' in (drawing || {}) && (drawing as any).forkedFrom" class="tag plain">分叉自·{{ (drawing as any).forkedFrom }}</span>
          <button v-if="isPart && parentDrawing" class="parent-link" type="button" @click="openParentDrawing">
            所属总图 <b>{{ parentDrawing.name }}</b><DemoIcon name="arrow-up-right" :size="12" />
          </button>
        </div>
       </div>

       <div class="dh-acts">
         <button class="btn" type="button" @click="openProperties"><DemoIcon name="info" :size="14" />属性详情</button>
         <button class="btn" type="button" @click="openModal('revert', '版本回退')"><DemoIcon name="undo-2" :size="14" />版本回退</button>
        <button class="btn" type="button" @click="openModal('borrow-drawing', '借用图纸')"><DemoIcon name="share-2" :size="14" />借用此图</button>
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
  .parent-link {
    width: 100%;
  }
}
</style>
