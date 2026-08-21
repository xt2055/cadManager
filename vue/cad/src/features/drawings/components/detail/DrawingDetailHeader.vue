<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({
  name: 'DrawingDetailHeader',
})

const route = useRoute()
const router = useRouter()
const demoStore = useDemoStore()
const uiStore = useUiStore()

const drawing = computed(() => demoStore.currentDrawing)

function openModal(type: 'revert' | 'borrow-drawing' | 'upload-version', title: string) {
  uiStore.openModal(type, title)
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
          <span class="tag plain tag-no-dot">{{ drawing?.kind }}</span>
          <span v-if="drawing" class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span>
          <span class="tag mute tag-no-dot">{{ drawing?.ver }}</span>
        </div>
        <div class="dh-meta">
          <span>材料 <b>{{ drawing?.material }}</b></span>
          <span>厂商 <b>{{ drawing?.vendor }}</b></span>
          <span>项目 <b>{{ drawing?.project }}</b></span>
          <span>更新时间 <b>{{ drawing?.updated }}</b></span>
          <span>维护人 <b>{{ drawing?.by }}</b></span>
        </div>
      </div>

      <div class="dh-acts">
        <button class="btn" type="button" @click="openModal('revert', '版本回退')"><DemoIcon name="undo-2" :size="14" />版本回退</button>
        <button class="btn" type="button" @click="openModal('borrow-drawing', '借用图纸')"><DemoIcon name="share-2" :size="14" />借用此图</button>
        <button class="btn primary" type="button" @click="openModal('upload-version', '上传新版本')"><DemoIcon name="upload" :size="14" />上传新版本</button>
      </div>
    </div>
  </div>
</template>
