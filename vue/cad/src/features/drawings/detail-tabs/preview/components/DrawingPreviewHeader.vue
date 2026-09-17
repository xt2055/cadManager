<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'DrawingPreviewHeader' })

/**
 * 图纸文件管理顶部操作栏：只按 props 决定可见性与禁用态，点击一律 emit 意图。
 * 不引入 store / router / composable——「3D 图纸」「图纸对比」的跳转与权限判断都由父页面处理。
 */
defineProps<{
  canCreateDrawing: boolean
  isAssembly: boolean
  /** 是否显示「图纸对比」（原 currentItem && reviewStore.getCase(...)?.changeSubmissionId）。 */
  canCompare: boolean
  /** 文件管理权限（原 canManageDrawingFiles）。 */
  canManageFiles: boolean
  hasAssemblyFile: boolean
}>()

const emit = defineEmits<{
  create: []
  open3d: []
  compare: []
  download: []
  borrow: []
  uploadAssembly: []
  uploadPart: []
}>()
</script>

<template>
  <div class="preview-actions-header card card-pad">
    <div class="header-info">
      <DemoIcon name="layers" :size="18" />
      <div>
        <h3>图纸文件管理与在线浏览</h3>
        <p>先上传总图建立主框架，后上传关联零件图；点击任意文件「浏览」开启矢量画布控制。</p>
      </div>
    </div>

    <div class="header-buttons">
      <button v-if="canCreateDrawing && isAssembly" class="btn primary" type="button" @click="emit('create')">
        <DemoIcon name="plus" :size="14" />新建图纸
      </button>
      <button class="btn" type="button" @click="emit('open3d')"><DemoIcon name="box" :size="14" />3D 图纸</button>
      <button v-if="canCompare" class="btn" type="button" @click="emit('compare')">图纸对比</button>
      <button class="btn" type="button" title="选择文件与格式（EXB / DWG / PDF），打包为 zip 下载" @click="emit('download')">
        <DemoIcon name="download" :size="14" />下载
      </button>
      <button v-if="canManageFiles" class="btn" type="button" title="从其他工程项目借用零件图及关联文件" @click="emit('borrow')">
        <DemoIcon name="share-2" :size="14" />借用零件
      </button>
      <button v-if="canManageFiles" class="btn primary" type="button" @click="emit('uploadAssembly')">
        <DemoIcon name="upload" :size="14" />上传总图文件
      </button>
      <button
        v-if="canManageFiles"
        class="btn"
        :class="{ primary: hasAssemblyFile }"
        type="button"
        :disabled="!hasAssemblyFile && isAssembly"
        :title="!hasAssemblyFile && isAssembly ? '请先上传总图' : '上传零件图'"
        @click="emit('uploadPart')"
      >
        <DemoIcon name="files" :size="14" />上传零件图
      </button>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 顶部操作栏专属样式（含 max-width:1280px 下的换列布局）。 */

.preview-actions-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-width: 0;
  flex-wrap: wrap;
}

.header-info {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  flex: 1 1 auto;
}

.header-info svg {
  color: var(--accent);
}

.header-info h3 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.header-info p {
  margin: 2px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.header-buttons {
  display: flex;
  flex: 0 1 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  min-width: 0;
}

@media (max-width: 1280px) {
  .preview-actions-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .header-buttons {
    justify-content: flex-start;
    width: 100%;
  }
}
</style>
