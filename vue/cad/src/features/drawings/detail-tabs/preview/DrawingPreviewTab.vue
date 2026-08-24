<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { Drawing, DrawingFile, StructurePart } from '@/types/domain.types'
import { parseDrawingFileName, parseStandaloneDrawingFileName } from '@/utils/drawing-number-parser'

defineOptions({
  name: 'DrawingPreviewTab',
})

const domainStore = useDomainStore()
const uiStore = useUiStore()

const currentItem = computed(() => domainStore.currentDrawing)
const isAssembly = computed(() => !currentItem.value || !('parentNo' in currentItem.value))
const rootDrawingNo = computed(() => {
  if (!currentItem.value) return ''
  if (!('parentNo' in currentItem.value)) return currentItem.value.no

  let currentNo = currentItem.value.no
  const visited = new Set<string>()
  while (!visited.has(currentNo)) {
    visited.add(currentNo)
    const part = domainStore.structure.find((item) => item.no === currentNo)
    if (!part) return currentItem.value.parentNo
    if (domainStore.drawings.some((drawing) => drawing.no === part.parentNo)) return part.parentNo
    currentNo = part.parentNo
  }
  return currentItem.value.parentNo
})

// 汇聚当前对象关联的所有图纸文件
const allFiles = computed<DrawingFile[]>(() => {
  if (!currentItem.value) return []
  const files: DrawingFile[] = []

  // 如果是总图，递归汇总总图、零件和其他文件。
  if (isAssembly.value) {
    const mainDrawing = currentItem.value as Drawing
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
    // 如果当前选中的就是零件图
    const part = currentItem.value as StructurePart
    files.push(...(part.files ?? []), ...(part.otherFiles ?? []))
  }

  return files.filter((file, index, sourceFiles) => sourceFiles.findIndex((candidate) => candidate.id === file.id) === index)
})

const hasAssemblyFile = computed(() => {
  if (isAssembly.value) {
    const mainDrawing = currentItem.value as Drawing
    return Boolean(mainDrawing?.files?.some((f) => f.role === 'assembly'))
  }
  return true // 零件图单独查看时
})

// 当前正在全屏/画布预览的文件
const activePreviewFile = ref<DrawingFile | null>(null)

// CAD 画布控制状态
const canvasRef = ref<HTMLElement | null>(null)
const layerPanel = ref(true)
const zoom = ref(1)
const rotation = ref(0)
const offset = ref({ x: 0, y: 0 })
const measuring = ref(false)
const measureStart = ref<{ x: number; y: number } | null>(null)
const measureEnd = ref<{ x: number; y: number } | null>(null)
const dragging = ref(false)
const lastPointer = ref({ x: 0, y: 0 })
const moved = ref(false)
const visibleLayers = ref<Record<string, boolean>>({
  'ly-outline': true,
  'ly-center': true,
  'ly-dim': true,
  'ly-hatch': true,
  'ly-frame': true,
  'ly-title': true,
})

const zoomText = computed(() => `${Math.round(zoom.value * 100)}%`)
const transform = computed(
  () => `translate(${offset.value.x}px, ${offset.value.y}px) rotate(${rotation.value}deg) scale(${zoom.value})`,
)

// 文件上传 input ref
const assemblyInput = ref<HTMLInputElement | null>(null)
const partInput = ref<HTMLInputElement | null>(null)

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function openBrowse(file: DrawingFile) {
  activePreviewFile.value = file
  resetView()
}

function closeBrowse() {
  activePreviewFile.value = null
}

function resetView() {
  zoom.value = 1
  rotation.value = 0
  offset.value = { x: 0, y: 0 }
  measuring.value = false
  measureStart.value = null
  measureEnd.value = null
}

async function handleDeleteFile(file: DrawingFile) {
  if (!currentItem.value) return
  if (!window.confirm(`确定要删除图纸文件「${file.name}」吗？`)) return

  try {
    const targetNo = file.partNo || file.drawingNo || currentItem.value.no
    if (file.role === 'other') {
      await domainStore.deleteOtherFile(targetNo, file.id)
    } else {
      await domainStore.deleteDrawingFile(targetNo, file.id)
    }
    if (activePreviewFile.value?.id === file.id) {
      activePreviewFile.value = null
    }
    uiStore.toast(`已删除文件 ${file.name}`)
  } catch (error) {
    console.error('删除文件失败', error)
    uiStore.toast('删除文件失败，请重试', 'warn')
  }
}

function triggerUploadAssembly() {
  assemblyInput.value?.click()
}

function triggerUploadPart() {
  if (!hasAssemblyFile.value && isAssembly.value) {
    uiStore.toast('请先上传总图文件，再进行零件图上传', 'warn')
    return
  }
  partInput.value?.click()
}

async function onAssemblyFileChange(event: Event) {
  const target = event.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file || !currentItem.value) return

  const newFile: DrawingFile = {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    name: file.name,
    size: formatFileSize(file.size),
    role: 'assembly',
    drawingNo: currentItem.value.no,
    version: currentItem.value.ver || 'v1.0',
        uploadedBy: ('by' in (currentItem.value || {})) ? (currentItem.value as any).by : '当前用户',
    uploadedAt: '刚刚',
    previewable: true,
  }

  try {
    await domainStore.uploadDrawingFile(currentItem.value.no, newFile, file)
    uiStore.toast(`总图文件「${file.name}」上传成功`, 'ok')
    activePreviewFile.value = newFile
  } catch (error) {
    console.error('上传总图失败', error)
    uiStore.toast('上传总图失败', 'warn')
  } finally {
    target.value = ''
  }
}

async function onPartFilesChange(event: Event) {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (!files || !files.length || !currentItem.value) return

  let createdCount = 0
  let otherCount = 0
  try {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      if (!file) continue
      const cleanName = file.name.replace(/\.[^/.]+$/, '')
      const rootNo = rootDrawingNo.value
      const currentProjectParsed = parseDrawingFileName(file.name, rootNo)
      const standaloneParsed = parseStandaloneDrawingFileName(file.name)
      const parsed = currentProjectParsed.isStandard ? currentProjectParsed : standaloneParsed
      const isBorrowed = standaloneParsed.isStandard && standaloneParsed.rootNo !== rootNo
      const isStructuredPart = parsed.isStandard && (parsed.level > 0 && parsed.rootNo === rootNo || isBorrowed)
      const parentNo = isBorrowed ? rootNo : parsed.parentNo ?? rootNo
      const partNo = parsed.no

      const newFile: DrawingFile = {
        id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        name: file.name,
        size: formatFileSize(file.size),
        role: isStructuredPart ? 'part' : 'other',
        drawingNo: rootNo,
        ...(isStructuredPart ? { partNo } : {}),
        version: 'v1.0',
        uploadedBy: ('by' in currentItem.value ? currentItem.value.by : '当前用户') || '当前用户',
        uploadedAt: '刚刚',
        previewable: true,
      }

      const parentExists = Boolean(
        domainStore.drawings.some((drawing) => drawing.no === parentNo)
        || domainStore.structure.some((part) => part.no === parentNo),
      )
      const existingPart = partNo
        ? domainStore.structure.find((part) => part.no === partNo && part.parentNo === rootNo)
        : undefined

      if (!isStructuredPart || !parentExists) {
        await domainStore.uploadOtherFile(rootNo, newFile, file)
        otherCount += 1
      } else if (existingPart) {
        await domainStore.uploadDrawingFile(partNo, newFile, file)
        createdCount += 1
      } else {
        await domainStore.createPartWithFile(parentNo, {
          no: partNo,
          name: parsed.name || cleanName,
          parentNo,
          project: '',
          material: 'HT200',
          spec: '',
          weight: 0,
          surfaceTreatment: '',
          partType: '自制件',
          qty: 1,
          status: 'draft',
          ver: 'v1.0',
          hasFile: true,
           files: [newFile],
           ...(isBorrowed ? { borrowFrom: standaloneParsed.rootNo ?? standaloneParsed.no } : {}),
         }, newFile, file)
        createdCount += 1
      }
    }
    uiStore.toast(`已整理 ${createdCount} 个规范零件文件${otherCount ? `，${otherCount} 个文件归入其他文件` : ''}`, 'ok')
  } catch (error) {
    console.error('上传零件图失败', error)
    uiStore.toast('上传零件图失败', 'warn')
  } finally {
    target.value = ''
  }
}

// CAD 操作工具
function zoomAtCenter(factor: number) {
  zoom.value = Math.min(8, Math.max(0.2, zoom.value * factor))
}

function zoomAt(event: WheelEvent) {
  const canvas = canvasRef.value
  if (!canvas) return
  const rect = canvas.getBoundingClientRect()
  const nextZoom = Math.min(8, Math.max(0.2, zoom.value * (event.deltaY < 0 ? 1.15 : 1 / 1.15)))
  if (nextZoom === zoom.value) return
  const radians = (rotation.value * Math.PI) / 180
  const dx = event.clientX - rect.left - offset.value.x
  const dy = event.clientY - rect.top - offset.value.y
  const ux = (dx * Math.cos(radians) + dy * Math.sin(radians)) / zoom.value
  const uy = (-dx * Math.sin(radians) + dy * Math.cos(radians)) / zoom.value
  const sx = nextZoom * ux
  const sy = nextZoom * uy
  offset.value = {
    x: event.clientX - rect.left - (sx * Math.cos(radians) - sy * Math.sin(radians)),
    y: event.clientY - rect.top - (sx * Math.sin(radians) + sy * Math.cos(radians)),
  }
  zoom.value = nextZoom
}

function rotate() {
  rotation.value = (rotation.value + 90) % 360
}

function screenToSvg(event: MouseEvent) {
  const canvas = canvasRef.value
  if (!canvas) return { x: 0, y: 0 }
  const rect = canvas.getBoundingClientRect()
  const radians = (rotation.value * Math.PI) / 180
  const dx = event.clientX - rect.left - offset.value.x
  const dy = event.clientY - rect.top - offset.value.y
  return {
    x: (dx * Math.cos(radians) + dy * Math.sin(radians)) / zoom.value,
    y: (-dx * Math.sin(radians) + dy * Math.cos(radians)) / zoom.value,
  }
}

function toggleMeasure() {
  measuring.value = !measuring.value
  if (!measuring.value) {
    measureStart.value = null
    measureEnd.value = null
  } else {
    uiStore.toast('测量模式：在图纸上点击两点（Esc 退出）· 按 1:2 比例换算', 'info')
  }
}

function handleCanvasClick(event: MouseEvent) {
  if (!measuring.value || moved.value) return
  const point = screenToSvg(event)
  if (!measureStart.value || measureEnd.value) {
    measureStart.value = point
    measureEnd.value = null
  } else {
    measureEnd.value = point
  }
}

function handlePointerDown(event: PointerEvent) {
  dragging.value = true
  moved.value = false
  lastPointer.value = { x: event.clientX, y: event.clientY }
  canvasRef.value?.setPointerCapture(event.pointerId)
}

function handlePointerMove(event: PointerEvent) {
  if (!dragging.value) return
  const dx = event.clientX - lastPointer.value.x
  const dy = event.clientY - lastPointer.value.y
  if (Math.abs(dx) + Math.abs(dy) > 3) moved.value = true
  offset.value.x += dx
  offset.value.y += dy
  lastPointer.value = { x: event.clientX, y: event.clientY }
}

function handlePointerUp(event: PointerEvent) {
  dragging.value = false
  canvasRef.value?.releasePointerCapture(event.pointerId)
}

function toggleFullscreen() {
  const viewer = document.querySelector('.viewer-wrapper')
  if (!viewer) return
  if (!document.fullscreenElement) {
    void viewer.requestFullscreen?.()
  } else {
    void document.exitFullscreen?.()
  }
}
</script>

<template>
  <div class="drawing-preview-view">
    <!-- 隐藏式文件选择框 -->
    <input
      ref="assemblyInput"
      type="file"
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onAssemblyFileChange"
    />
    <input
      ref="partInput"
      type="file"
      multiple
      accept=".exb,.dwg,.dxf,.pdf,.step,.stp"
      class="hidden-file-input"
      @change="onPartFilesChange"
    />

    <!-- 顶部操作栏：规范的上传总图 / 零件图入口 -->
    <div class="preview-actions-header card card-pad">
      <div class="header-info">
        <DemoIcon name="layers" :size="18" />
        <div>
          <h3>图纸文件管理与在线浏览</h3>
          <p>先上传总图建立主框架，后上传关联零件图；点击任意文件「浏览」开启矢量画布控制。</p>
        </div>
      </div>

      <div class="header-buttons">
        <button class="btn primary" type="button" @click="triggerUploadAssembly">
          <DemoIcon name="upload" :size="14" />上传总图文件
        </button>
        <button
          class="btn"
          :class="{ primary: hasAssemblyFile }"
          type="button"
          :disabled="!hasAssemblyFile && isAssembly"
          :title="!hasAssemblyFile && isAssembly ? '请先上传总图' : '上传零件图'"
          @click="triggerUploadPart"
        >
          <DemoIcon name="files" :size="14" />上传零件图
        </button>
      </div>
    </div>

    <!-- 主展示区：若处于“浏览模式”，优先展示 CAD 画布，并附带侧边/顶部收起条 -->
    <div v-if="activePreviewFile" class="viewer-wrapper card">
      <div class="viewer-top-bar">
        <div class="current-browsing-meta">
          <span class="tag" :class="activePreviewFile.role === 'assembly' ? 'plain' : activePreviewFile.role === 'other' ? 'mute' : 'info'">
            {{ activePreviewFile.role === 'assembly' ? '总图' : activePreviewFile.role === 'other' ? '其他文件' : '零件图' }}
          </span>
          <b>{{ activePreviewFile.name }}</b>
          <span class="file-dim">图号：{{ activePreviewFile.partNo || activePreviewFile.drawingNo }}</span>
          <span class="file-dim">大小：{{ activePreviewFile.size }}</span>
        </div>
        <div class="viewer-top-tools">
          <button class="btn sm" type="button" @click="closeBrowse">
            <DemoIcon name="arrow-left" :size="13" />返回文件列表
          </button>
        </div>
      </div>

      <!-- 浏览工具栏 -->
      <div class="v-toolbar">
        <div class="v-group">
          <button
            class="v-btn"
            :class="{ on: layerPanel }"
            type="button"
            title="图层面板"
            @click="layerPanel = !layerPanel"
          >
            <DemoIcon name="layers" :size="15" />
          </button>
        </div>
        <div class="v-group">
          <button class="v-btn" type="button" title="缩小" @click="zoomAtCenter(1 / 1.3)">
            <DemoIcon name="zoom-out" :size="15" />
          </button>
          <span class="v-zoomlbl">{{ zoomText }}</span>
          <button class="v-btn" type="button" title="放大" @click="zoomAtCenter(1.3)">
            <DemoIcon name="zoom-in" :size="15" />
          </button>
          <button class="v-btn" type="button" title="重置视图" @click="resetView">
            <DemoIcon name="maximize" :size="15" />
          </button>
          <button class="v-btn" type="button" title="旋转 90°" @click="rotate">
            <DemoIcon name="rotate-cw" :size="15" />
          </button>
        </div>
        <div class="v-group">
          <button class="v-btn" :class="{ on: measuring }" type="button" title="测量模式" @click="toggleMeasure">
            <DemoIcon name="ruler" :size="15" />
          </button>
        </div>
        <div class="v-group last-group">
          <button class="v-btn" type="button" title="全屏" @click="toggleFullscreen">
            <DemoIcon name="maximize-2" :size="15" />
          </button>
        </div>
      </div>

      <div class="v-main">
        <aside class="layer-panel" :class="{ hidden: !layerPanel }">
          <div class="lp-title">LAYERS · 图层</div>
          <label
            v-for="item in [
              ['ly-outline', '轮廓线', 'var(--cad-line)'],
              ['ly-center', '中心线', 'var(--cad-center)'],
              ['ly-dim', '标注', 'var(--cad-dim)'],
              ['ly-hatch', '剖面线', 'var(--cad-hatch)'],
              ['ly-frame', '图框', 'var(--cad-frame)'],
            ]"
            :key="item[0]"
            class="layer-item"
          >
            <input v-model="visibleLayers[item[0]]" type="checkbox" />
            <span>{{ item[1] }}</span>
            <span class="ly-swatch" :style="{ background: item[2] }"></span>
          </label>
        </aside>

        <div
          ref="canvasRef"
          class="v-canvas"
          :class="{ dragging, measuring }"
          @wheel.prevent="zoomAt"
          @pointerdown="handlePointerDown"
          @pointermove="handlePointerMove"
          @pointerup="handlePointerUp"
          @pointercancel="handlePointerUp"
          @click="handleCanvasClick"
        >
          <div class="cad-stage" :style="{ transform }">
            <svg id="cadSvg" width="1000" height="660" viewBox="0 0 1000 660">
              <defs>
                <marker
                  id="arr"
                  viewBox="0 0 10 10"
                  refX="9.5"
                  refY="5"
                  markerWidth="7.5"
                  markerHeight="7.5"
                  orient="auto-start-reverse"
                >
                  <path class="m-arr" d="M0,0 L10,5 L0,10 z" />
                </marker>
                <pattern
                  id="hatch"
                  width="8"
                  height="8"
                  patternTransform="rotate(45)"
                  patternUnits="userSpaceOnUse"
                >
                  <line class="h-line" x1="0" y1="0" x2="0" y2="8" />
                </pattern>
              </defs>

              <g id="ly-frame" :style="{ display: visibleLayers['ly-frame'] ? '' : 'none' }">
                <rect class="fline" x="8" y="8" width="984" height="644" stroke-width="1" />
                <rect class="fline" x="26" y="26" width="948" height="608" stroke-width="2.2" />
                <g class="fline" stroke-width="1">
                  <rect x="560" y="494" width="414" height="120" />
                  <line x1="560" y1="524" x2="974" y2="524" />
                  <line x1="560" y1="554" x2="974" y2="554" />
                  <line x1="560" y1="584" x2="974" y2="584" />
                  <line x1="740" y1="524" x2="740" y2="614" />
                </g>
                <text
                  class="txt"
                  x="767"
                  y="515"
                  font-size="15"
                  text-anchor="middle"
                  font-weight="700"
                  style="font-family: 'Noto Sans SC'"
                >
                  {{ activePreviewFile.name }}
                </text>
                <text class="txt-lbl" x="570" y="543">图号</text>
                <text class="txt" x="598" y="543">{{ activePreviewFile.partNo || activePreviewFile.drawingNo }}</text>
                <text class="txt-lbl" x="750" y="543">比例</text>
                <text class="txt" x="778" y="543">1:2</text>
                <text class="txt-lbl" x="570" y="573">设计</text>
                <text class="txt" x="598" y="573">{{ currentItem?.signers?.['设计'] || '待定' }}</text>
                <text class="txt-lbl" x="750" y="573">校对</text>
                <text class="txt" x="778" y="573">{{ currentItem?.signers?.['校对'] || '待定' }}</text>
                <text class="txt-lbl" x="570" y="603">材料</text>
                <text class="txt" x="598" y="603">{{ currentItem?.material || 'HT200' }}</text>
                <text class="txt-lbl" x="750" y="603">批准</text>
                <text class="txt" x="778" y="603">{{ currentItem?.signers?.['批准'] || '待定' }}</text>
              </g>

              <g id="ly-hatch" :style="{ display: visibleLayers['ly-hatch'] ? '' : 'none' }">
                <path d="M550,140 H830 V420 H550 Z M590,180 H790 V420 H590 Z" fill="url(#hatch)" fill-rule="evenodd" />
              </g>

              <g id="ly-center" :style="{ display: visibleLayers['ly-center'] ? '' : 'none' }">
                <line class="ctr" x1="115" y1="280" x2="425" y2="280" />
                <line class="ctr" x1="250" y1="115" x2="250" y2="445" />
                <circle class="ctr" cx="250" cy="280" r="85" />
                <line class="ctr" x1="535" y1="280" x2="845" y2="280" />
                <line class="ctr" x1="690" y1="125" x2="690" y2="435" />
              </g>

              <g id="ly-outline" :style="{ display: visibleLayers['ly-outline'] ? '' : 'none' }">
                <circle class="ln thick" cx="250" cy="280" r="140" />
                <circle class="ln thick" cx="250" cy="280" r="115" />
                <circle class="ln thick" cx="250" cy="280" r="55" />
                <rect class="ln thick" x="550" y="140" width="280" height="280" />
                <path class="ln thick" d="M590,180 H790 V420" />
              </g>

              <g id="ly-dim" :style="{ display: visibleLayers['ly-dim'] ? '' : 'none' }">
                <line class="dim" x1="151" y1="181" x2="100" y2="128" marker-end="url(#arr)" />
                <line class="dim" x1="100" y1="128" x2="62" y2="128" />
                <text class="txt" x="62" y="121">Φ280</text>
                <line class="dim" x1="550" y1="118" x2="830" y2="118" marker-start="url(#arr)" marker-end="url(#arr)" />
                <text class="txt" x="690" y="111" text-anchor="middle">280</text>
              </g>

              <g v-if="measureStart" class="measure-layer">
                <circle :cx="measureStart.x" :cy="measureStart.y" r="5" fill="var(--accent)" stroke="var(--cad-bg)" stroke-width="2" />
                <g v-if="measureEnd">
                  <line
                    :x1="measureStart.x"
                    :y1="measureStart.y"
                    :x2="measureEnd.x"
                    :y2="measureEnd.y"
                    stroke="var(--accent)"
                    stroke-width="1.6"
                    stroke-dasharray="7 4"
                  />
                  <circle :cx="measureEnd.x" :cy="measureEnd.y" r="5" fill="var(--accent)" stroke="var(--cad-bg)" stroke-width="2" />
                </g>
              </g>
            </svg>
          </div>
          <div class="v-hud">{{ activePreviewFile.name }} · 比例 1:2 · 矢量图框</div>
        </div>
      </div>
    </div>

    <!-- 文件总览列表 -->
    <div class="card files-table-card">
      <div class="card-title">
        <DemoIcon name="file-text" :size="16" />
        已关联图纸文件清单 ({{ allFiles.length }})
        <span class="hint">支持 DWG / DXF / EXB / PDF / STEP</span>
      </div>

      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>文件类型</th>
              <th>文件名</th>
              <th>关联图号</th>
              <th>文件大小</th>
              <th>版本</th>
              <th>上传人</th>
              <th>上传时间</th>
              <th style="width: 140px; text-align: right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="file in allFiles" :key="file.id" :class="{ 'active-row': activePreviewFile?.id === file.id }">
              <td>
                <span class="tag" :class="file.role === 'assembly' ? 'plain' : file.role === 'other' ? 'mute' : 'info'">
                  {{ file.role === 'assembly' ? '项目总图' : file.role === 'other' ? '其他文件' : '零件图' }}
                </span>
              </td>
              <td class="file-name-cell">
                <DemoIcon name="file-check-2" :size="16" />
                <b>{{ file.name }}</b>
              </td>
              <td class="num mono">{{ file.partNo || file.drawingNo }}</td>
              <td class="num">{{ file.size }}</td>
              <td class="num">{{ file.version }}</td>
              <td>{{ file.uploadedBy }}</td>
              <td class="num updated">{{ file.uploadedAt }}</td>
              <td class="row-actions" style="text-align: right">
                <button class="btn sm primary" type="button" @click="openBrowse(file)">
                  <DemoIcon name="eye" :size="13" />浏览
                </button>
                <button class="btn sm danger" type="button" @click="handleDeleteFile(file)">
                  <DemoIcon name="trash-2" :size="13" />删除
                </button>
              </td>
            </tr>
            <tr v-if="!allFiles.length">
              <td colspan="8">
                <div class="empty">
                  <DemoIcon name="file-up" :size="36" />
                  <div class="t">尚未上传任何图纸文件</div>
                  <p>请点击上方「上传总图文件」开始建立工程档案</p>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.drawing-preview-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.hidden-file-input {
  display: none;
}

.preview-actions-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.header-info {
  display: flex;
  align-items: center;
  gap: 12px;
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
}

.header-buttons {
  display: flex;
  gap: 10px;
}

.files-table-card {
  overflow: visible;
}

.table-pad {
  padding: 10px 14px;
  max-height: none;
  overflow: auto;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 260px;
}

.file-name-cell svg {
  color: var(--accent);
}

.file-name-cell b {
  overflow-wrap: anywhere;
  line-height: 1.4;
}

.active-row {
  background: var(--active);
}

.viewer-wrapper {
  display: flex;
  flex-direction: column;
  height: min(600px, calc(100vh - 180px));
  min-height: 360px;
  overflow: hidden;
  border-radius: var(--radius);
}

.viewer-top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-bottom: 1px solid var(--line);
  background: var(--panel-2);
}

.current-browsing-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}

.file-dim {
  color: var(--text-3);
  font-size: 11.5px;
  font-family: 'JetBrains Mono', monospace;
}

.v-toolbar {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border-bottom: 1px solid var(--line);
  background: var(--panel);
}

.v-group {
  display: flex;
  align-items: center;
  gap: 4px;
  padding-right: 8px;
  border-right: 1px solid var(--line);
}

.v-group.last-group {
  margin-left: auto;
  padding-right: 0;
  border-right: none;
}

.v-btn {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  color: var(--text-2);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all 0.2s;
}

.v-btn:hover,
.v-btn.on {
  background: var(--active);
  color: var(--accent);
}

.v-zoomlbl {
  min-width: 44px;
  text-align: center;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
  color: var(--text-2);
}

.v-main {
  display: flex;
  flex: 1;
  min-height: 0;
  position: relative;
}

.layer-panel {
  width: 160px;
  padding: 12px;
  border-right: 1px solid var(--line);
  background: var(--panel);
  overflow-y: auto;
}

.layer-panel.hidden {
  display: none;
}

.lp-title {
  padding-bottom: 8px;
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
}

.layer-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 12px;
}

.layer-item:hover {
  background: var(--hover);
}

.layer-item span:nth-child(2) {
  flex: 1;
}

.ly-swatch {
  width: 14px;
  height: 4px;
  border-radius: 2px;
}

.v-canvas {
  position: relative;
  flex: 1;
  overflow: hidden;
  background: var(--cad-bg);
  cursor: grab;
  touch-action: none;
}

.v-canvas.dragging {
  cursor: grabbing;
}

.cad-stage {
  position: absolute;
  top: 0;
  left: 0;
  width: 1000px;
  height: 660px;
  transform-origin: 0 0;
}

.v-hud {
  position: absolute;
  bottom: 10px;
  left: 12px;
  color: var(--cad-dim);
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  pointer-events: none;
}

:deep(#cadSvg) {
  display: block;
}

:deep(#cadSvg .ln) { stroke: var(--cad-line); fill: none; stroke-linecap: round; }
:deep(#cadSvg .thick) { stroke-width: 2.1; }
:deep(#cadSvg .ctr) { stroke: var(--cad-center); fill: none; stroke-width: 0.9; stroke-dasharray: 15 4 3 4; }
:deep(#cadSvg .dim) { stroke: var(--cad-dim); fill: none; stroke-width: 0.8; }
:deep(#cadSvg .txt) { fill: var(--cad-text); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
:deep(#cadSvg .txt-lbl) { fill: var(--cad-dim); font-family: 'Noto Sans SC', sans-serif; font-size: 9.5px; }
:deep(#cadSvg .fline) { stroke: var(--cad-frame); fill: none; }
:deep(#cadSvg .m-arr) { fill: var(--cad-dim); stroke: none; }
:deep(#cadSvg .h-line) { stroke: var(--cad-hatch); stroke-width: 0.75; }

@media (max-width: 800px) {
  .preview-actions-header {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
