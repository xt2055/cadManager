<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import CadVectorViewer from '@/features/drawings/detail-tabs/preview/CadVectorViewer.vue'
import { useDomainStore } from '@/stores/domain.store'
import type { Drawing, DrawingFile, StructurePart } from '@/types/domain.types'

defineOptions({
  name: 'DrawingViewerPage',
})

const route = useRoute()
const router = useRouter()
const domainStore = useDomainStore()

const drawingId = computed(() => String(route.params.drawingId ?? ''))
const fileId = computed(() => String(route.query.fileId ?? ''))

const currentDrawing = computed(() => domainStore.currentDrawing)
const isAssembly = computed(() => !currentDrawing.value || !('parentNo' in currentDrawing.value))

// 汇总所有关联图纸文件
const allFiles = computed<DrawingFile[]>(() => {
  if (!currentDrawing.value) return []
  const files: DrawingFile[] = []

  if (isAssembly.value) {
    const mainDrawing = currentDrawing.value as Drawing
    files.push(...(mainDrawing.files ?? []), ...(mainDrawing.otherFiles ?? []))
    const belongsToDrawing = (part: StructurePart): boolean => {
      if (part.parentNo === mainDrawing.no || part.no.startsWith(`${mainDrawing.no}-`)) return true
      const visited = new Set<string>()
      let parentNo = part.parentNo
      while (parentNo && !visited.has(parentNo)) {
        if (parentNo === mainDrawing.no) return true
        visited.add(parentNo)
        const parent = domainStore.structure.find((candidate) => candidate.no === parentNo)
        if (!parent) return part.no.startsWith(`${mainDrawing.no}-`)
        parentNo = parent.parentNo
      }
      return false
    }

    domainStore.structure
      .filter(belongsToDrawing)
      .forEach((part) => files.push(...(part.files ?? []), ...(part.otherFiles ?? [])))
  } else {
    const part = currentDrawing.value as StructurePart
    files.push(...(part.files ?? []), ...(part.otherFiles ?? []))
  }

  return files.filter((file, index, sourceFiles) => sourceFiles.findIndex((candidate) => candidate.id === file.id) === index)
})

// 当前选中的图纸文件
const activeFile = ref<DrawingFile | null>(null)
const cadDxfUrl = ref<string | null>(null)
const cadViewerRef = ref<InstanceType<typeof CadVectorViewer> | null>(null)

// 视图与图层控制
const layerPanelVisible = ref(true)
const fileListVisible = ref(true)
const dynamicLayers = ref<Array<{ name: string; color: string; visible: boolean }>>([])
const zoomLevel = ref(1)

const zoomText = computed(() => `${Math.round(zoomLevel.value * 100)}%`)

function selectFile(file: DrawingFile) {
  activeFile.value = file
  dynamicLayers.value = []

  const isCad = file.name.toLowerCase().endsWith('.exb') || file.name.toLowerCase().endsWith('.dxf') || file.name.toLowerCase().endsWith('.dwg')
  if (isCad) {
    const storageKey = file.storageKey
    const params = new URLSearchParams()
    if (storageKey) {
      params.set('storageKey', storageKey)
    }
    // 无论是否有 storageKey，都附带 drawingNo/partNo/fileName 作为双重保险，避免因分叉或局部引用丢失 storageKey 导致后端查找失败
    if (file.drawingNo) params.set('drawingNo', file.drawingNo)
    if (file.partNo) params.set('partNo', file.partNo)
    params.set('fileName', file.name)
    // 增加时间戳防缓存
    params.set('_t', String(Date.now()))
    const baseUrl = (import.meta.env.VITE_API_BASE_URL || '/api').replace(/\/$/, '')
    cadDxfUrl.value = `${baseUrl}/exb/preview?${params.toString()}`
  } else {
    cadDxfUrl.value = null
  }

  void domainStore.recordActivityAndPersist({
    drawingNo: file.partNo || file.drawingNo,
    targetType: 'file',
    act: 'view',
    text: `独立浏览 CAD 图纸 <b>${file.name}</b>`,
    detail: { fileId: file.id, fileName: file.name },
  })
}

function handleLayersLoaded(layers: Array<{ name: string; color: string; visible: boolean }>) {
  dynamicLayers.value = layers
}

function toggleDynamicLayer(layer: { name: string; color: string; visible: boolean }) {
  cadViewerRef.value?.setLayerVisibility(layer.name, layer.visible)
}

function handleZoomChange(val: number) {
  zoomLevel.value = val
}

function zoomIn() {
  cadViewerRef.value?.zoomIn()
}

function zoomOut() {
  cadViewerRef.value?.zoomOut()
}

function resetView() {
  cadViewerRef.value?.resetView()
}

function goBack() {
  router.push({ name: 'drawing-preview', params: { drawingId: drawingId.value } })
}

function initCurrentFile() {
  if (drawingId.value) {
    domainStore.openDrawing(drawingId.value)
  }
  if (allFiles.value.length > 0) {
    if (fileId.value) {
      const match = allFiles.value.find((f) => f.id === fileId.value)
      if (match) {
        selectFile(match)
        return
      }
    }
    const firstFile = allFiles.value[0]
    if (firstFile) {
      selectFile(firstFile)
    }
  }
}

watch(
  () => [drawingId.value, allFiles.value.length],
  () => {
    if (!activeFile.value && allFiles.value.length > 0) {
      initCurrentFile()
    }
  },
  { immediate: true },
)

onMounted(() => {
  initCurrentFile()
})
</script>

<template>
  <div class="drawing-standalone-viewer">
    <!-- 顶部导航与操作栏 -->
    <header class="viewer-header">
      <div class="header-left">
        <button class="back-btn" type="button" title="返回图纸详情" @click="goBack">
          <DemoIcon name="arrow-left" :size="16" />
          <span>返回详情</span>
        </button>
        <div class="divider"></div>
        <div class="drawing-meta">
          <span class="tag tag-no-dot" :class="isAssembly ? 'plain' : 'info'">
            {{ isAssembly ? '总图' : '零件图' }}
          </span>
          <h2 class="title">{{ currentDrawing?.name || 'CAD 图纸独立浏览' }}</h2>
          <span class="no">{{ currentDrawing?.no }}</span>
        </div>
      </div>

      <!-- 中间文件选择胶囊（多文件时方便快速切换） -->
      <div v-if="allFiles.length > 1" class="file-selector-capsule">
        <button
          v-for="file in allFiles"
          :key="file.id"
          class="file-pill"
          :class="{ active: activeFile?.id === file.id }"
          type="button"
          @click="selectFile(file)"
        >
          <DemoIcon :name="file.role === 'assembly' ? 'layout' : 'file-code'" :size="13" />
          <span class="file-pill-name">{{ file.name }}</span>
        </button>
      </div>

      <div class="header-right">
        <div class="view-controls">
          <button class="ctrl-btn" type="button" title="缩小" @click="zoomOut">
            <DemoIcon name="zoom-out" :size="15" />
          </button>
          <span class="zoom-value">{{ zoomText }}</span>
          <button class="ctrl-btn" type="button" title="放大" @click="zoomIn">
            <DemoIcon name="zoom-in" :size="15" />
          </button>
          <button class="ctrl-btn" type="button" title="自适应居中" @click="resetView">
            <DemoIcon name="maximize" :size="15" />
          </button>
        </div>

        <div class="divider"></div>

        <button
          class="toggle-btn"
          :class="{ active: layerPanelVisible }"
          type="button"
          title="切换图层面板"
          @click="layerPanelVisible = !layerPanelVisible"
        >
          <DemoIcon name="layers" :size="15" />
          <span>图层</span>
        </button>

        <button
          class="toggle-btn"
          :class="{ active: fileListVisible }"
          type="button"
          title="切换图纸清单"
          @click="fileListVisible = !fileListVisible"
        >
          <DemoIcon name="files" :size="15" />
          <span>图纸列表 ({{ allFiles.length }})</span>
        </button>
      </div>
    </header>

    <!-- 核心画布工作区 -->
    <div class="viewer-body">
      <!-- 左侧图纸列表面板（可收起） -->
      <aside v-if="fileListVisible" class="side-panel file-list-panel">
        <div class="panel-header">
          <DemoIcon name="files" :size="14" />
          <span>关联图纸文件 ({{ allFiles.length }})</span>
        </div>
        <div class="panel-content">
          <div
            v-for="file in allFiles"
            :key="file.id"
            class="file-item-card"
            :class="{ active: activeFile?.id === file.id }"
            @click="selectFile(file)"
          >
            <div class="file-icon-box">
              <DemoIcon :name="file.role === 'assembly' ? 'layout' : 'file-code'" :size="18" />
            </div>
            <div class="file-details">
              <div class="file-title" :title="file.name">{{ file.name }}</div>
              <div class="file-sub">
                <span class="tag-sub">{{ file.role === 'assembly' ? '总图' : '零件' }}</span>
                <span>{{ file.partNo || file.drawingNo }}</span>
              </div>
            </div>
          </div>
        </div>
      </aside>

      <!-- 中间 CAD 矢量图画板 -->
      <main class="viewer-canvas-container">
        <CadVectorViewer
          v-if="cadDxfUrl"
          ref="cadViewerRef"
          :dxf-url="cadDxfUrl"
          :file-name="activeFile?.name"
          @layers-loaded="handleLayersLoaded"
          @zoom-change="handleZoomChange"
        />
        <div v-else class="empty-prompt">
          <DemoIcon name="file-question" :size="48" />
          <p>{{ activeFile ? '该格式暂不支持矢量直接渲染' : '暂无选中的图纸文件' }}</p>
        </div>
      </main>

      <!-- 右侧图层管理器面板（可收起） -->
      <aside v-if="layerPanelVisible" class="side-panel layer-panel">
        <div class="panel-header">
          <DemoIcon name="layers" :size="14" />
          <span>图层控制 ({{ dynamicLayers.length }})</span>
        </div>
        <div class="panel-content">
          <div v-if="dynamicLayers.length === 0" class="no-layers">
            正在读取 CAD 图层拓扑...
          </div>
          <label
            v-for="layer in dynamicLayers"
            :key="layer.name"
            class="layer-row"
          >
            <input
              v-model="layer.visible"
              type="checkbox"
              class="layer-checkbox"
              @change="toggleDynamicLayer(layer)"
            />
            <span class="layer-name" :title="layer.name">{{ layer.name }}</span>
            <span class="layer-color-dot" :style="{ background: layer.color }"></span>
          </label>
        </div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.drawing-standalone-viewer {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: #0d1117;
  color: #f0f6fc;
  overflow: hidden;
  position: relative;
}

/* 顶部导航栏 */
.viewer-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 52px;
  padding: 0 16px;
  background: rgba(22, 27, 34, 0.95);
  border-bottom: 1px solid rgba(48, 54, 61, 0.8);
  backdrop-filter: blur(8px);
  z-index: 20;
  flex-shrink: 0;
  gap: 16px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.back-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.12);
  color: #f0f6fc;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.back-btn:hover {
  background: rgba(255, 255, 255, 0.12);
  border-color: rgba(255, 255, 255, 0.24);
  color: #58a6ff;
}

.divider {
  width: 1px;
  height: 20px;
  background: rgba(48, 54, 61, 0.8);
}

.drawing-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title {
  margin: 0;
  font-size: 14.5px;
  font-weight: 600;
  color: #f0f6fc;
  max-width: 240px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.no {
  font-size: 12.5px;
  color: #8b949e;
  font-family: 'JetBrains Mono', monospace;
}

/* 顶部快速切图胶囊 */
.file-selector-capsule {
  display: flex;
  align-items: center;
  gap: 6px;
  background: rgba(13, 17, 23, 0.8);
  padding: 3px;
  border-radius: 20px;
  border: 1px solid rgba(48, 54, 61, 0.6);
  max-width: 420px;
  overflow-x: auto;
}

.file-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 16px;
  background: transparent;
  border: none;
  color: #8b949e;
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.file-pill:hover {
  color: #f0f6fc;
  background: rgba(255, 255, 255, 0.05);
}

.file-pill.active {
  background: #1f6feb;
  color: #ffffff;
  font-weight: 500;
}

.file-pill-name {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 头部右侧操作区 */
.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.view-controls {
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(13, 17, 23, 0.7);
  padding: 2px 4px;
  border-radius: 6px;
  border: 1px solid rgba(48, 54, 61, 0.6);
}

.ctrl-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 4px;
  background: transparent;
  border: none;
  color: #c9d1d9;
  cursor: pointer;
  transition: all 0.15s ease;
}

.ctrl-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  color: #58a6ff;
}

.zoom-value {
  font-size: 12px;
  font-family: 'JetBrains Mono', monospace;
  padding: 0 6px;
  color: #58a6ff;
  min-width: 44px;
  text-align: center;
}

.toggle-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.1);
  color: #8b949e;
  font-size: 12.5px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.toggle-btn:hover {
  color: #f0f6fc;
  background: rgba(255, 255, 255, 0.08);
}

.toggle-btn.active {
  background: rgba(56, 189, 248, 0.12);
  border-color: rgba(56, 189, 248, 0.35);
  color: #38bdf8;
}

/* 主画布区与左右侧边栏 */
.viewer-body {
  display: flex;
  flex: 1;
  width: 100%;
  height: calc(100% - 52px);
  overflow: hidden;
  position: relative;
}

.side-panel {
  width: 240px;
  height: 100%;
  background: rgba(22, 27, 34, 0.95);
  border-color: rgba(48, 54, 61, 0.8);
  display: flex;
  flex-direction: column;
  z-index: 10;
  flex-shrink: 0;
}

.file-list-panel {
  border-right: 1px solid rgba(48, 54, 61, 0.8);
}

.layer-panel {
  border-left: 1px solid rgba(48, 54, 61, 0.8);
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  font-size: 12.5px;
  font-weight: 600;
  color: #8b949e;
  border-bottom: 1px solid rgba(48, 54, 61, 0.6);
  letter-spacing: 0.5px;
}

.panel-content {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.file-item-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid transparent;
  cursor: pointer;
  transition: all 0.15s ease;
}

.file-item-card:hover {
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(255, 255, 255, 0.1);
}

.file-item-card.active {
  background: rgba(56, 189, 248, 0.12);
  border-color: rgba(56, 189, 248, 0.35);
}

.file-icon-box {
  color: #38bdf8;
  display: flex;
  align-items: center;
  justify-content: center;
}

.file-details {
  flex: 1;
  min-width: 0;
}

.file-title {
  font-size: 13px;
  font-weight: 500;
  color: #f0f6fc;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-sub {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  color: #8b949e;
  margin-top: 2px;
}

.tag-sub {
  font-size: 10.5px;
  padding: 1px 4px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.08);
  color: #c9d1d9;
}

/* 图层列表项 */
.layer-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  font-size: 12.5px;
  color: #c9d1d9;
  cursor: pointer;
  user-select: none;
  transition: background 0.15s ease;
}

.layer-row:hover {
  background: rgba(255, 255, 255, 0.05);
}

.layer-checkbox {
  accent-color: #38bdf8;
  cursor: pointer;
}

.layer-name {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.layer-color-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 1px solid rgba(255, 255, 255, 0.2);
  flex-shrink: 0;
}

.no-layers {
  font-size: 12px;
  color: #8b949e;
  text-align: center;
  padding: 24px 0;
}

/* 画布容器 */
.viewer-canvas-container {
  flex: 1;
  height: 100%;
  background: #12151c;
  position: relative;
  overflow: hidden;
}

.empty-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #484f58;
  gap: 12px;
  font-size: 14px;
}
</style>
