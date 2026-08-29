<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'

defineOptions({
  name: 'DrawingDetailHeader',
})

const router = useRouter()
const domainStore = useDomainStore()

const drawing = computed(() => domainStore.currentDrawing)
const isPart = computed(() => Boolean(drawing.value && 'parentNo' in drawing.value))
const parentDrawing = computed(() => {
  const parentNo = isPart.value ? (drawing.value as { parentNo: string }).parentNo : ''
  return domainStore.drawings.find((item) => item.no === parentNo) ?? null
})

const designerName = computed(() => {
  if (!drawing.value) return ''
  if ('parentNo' in drawing.value) {
    let parentNo = drawing.value.parentNo
    const visited = new Set<string>()
    while (parentNo && !visited.has(parentNo)) {
      visited.add(parentNo)
      const parent = domainStore.drawings.find((item) => item.no === parentNo)
      if (parent) return parent.designer || ''
      const parentPart = domainStore.structure.find((item) => item.no === parentNo)
      if (!parentPart) break
      parentNo = parentPart.parentNo
    }
  }
  return 'designer' in drawing.value ? drawing.value.designer || '' : ''
})

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
          <span v-if="designerName" class="designer-badge">
            <DemoIcon name="pen-tool" :size="13" />
            设计：<b>{{ designerName }}</b>
          </span>
          <span class="dh-no">{{ drawing?.no }}</span>
           <span class="tag mute tag-no-dot">{{ drawing?.ver }}</span>
         </div>
         <div class="dh-meta">
           <span>厂商 <b>{{ ('vendor' in (drawing || {})) ? (drawing as any).vendor : '内部加工' }}</b></span>
          <span>项目 <b>{{ ('project' in (drawing || {})) ? (drawing as any).project : '—' }}</b></span>
          <span v-if="isPart">关联图号 <b class="mono">{{ drawing?.no }}</b></span>
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
