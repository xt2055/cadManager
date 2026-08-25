<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingFile, DrawingFileHistoryItem } from '@/types/domain.types'

defineOptions({
  name: 'DrawingFileHistoryPage',
})

const route = useRoute()
const router = useRouter()
const domainStore = useDomainStore()
const uiStore = useUiStore()

const drawingId = computed(() => String(route.params.drawingId ?? ''))
const fileId = computed(() => String(route.query.fileId ?? ''))

const targetFile = ref<DrawingFile | null>(null)
const selectedNodeId = ref<string | null>(null)

interface HistoryTreeNode {
  id: string
  version: string
  name: string
  size: string
  uploadedBy: string
  uploadedAt: string
  replacedBy?: string
  replacedAt?: string
  replaceReason?: string
  storageKey?: string
  mimeType?: string
  previewable: boolean
  isCurrent: boolean
  index: number
}

// 加载指定文件
async function loadFile() {
  await domainStore.initialize()
  if (drawingId.value) {
    domainStore.openDrawing(drawingId.value)
  }

  const currentFiles = domainStore.currentDrawing
    ? [...(domainStore.currentDrawing.files ?? []), ...(domainStore.currentDrawing.otherFiles ?? [])]
    : []
  const structureFiles = domainStore.structure.flatMap((part) => [
    ...(part.files ?? []),
    ...(part.otherFiles ?? []),
  ])

  let file = [...currentFiles, ...structureFiles].find((candidate) => candidate.id === fileId.value)
  if (!file && fileId.value) {
    // 递归查找历史项中是否包含该 fileId
    for (const parent of [...currentFiles, ...structureFiles]) {
      if (parent.history?.some((h) => h.id === fileId.value)) {
        file = parent
        break
      }
    }
  }

  targetFile.value = file || null
  if (file && !selectedNodeId.value) {
    selectedNodeId.value = file.id
  }
}

// 构造按时间先后顺序排列的历史演进树节点
const historyTree = computed<HistoryTreeNode[]>(() => {
  if (!targetFile.value) return []
  const nodes: HistoryTreeNode[] = []
  const historyList = targetFile.value.history ?? []

  // 1. 历史版本节点（先出现的版本在前）
  historyList.forEach((item, idx) => {
    nodes.push({
      id: item.id,
      version: item.version,
      name: item.name,
      size: item.size,
      uploadedBy: item.uploadedBy,
      uploadedAt: item.uploadedAt,
      replacedBy: item.replacedBy,
      replacedAt: item.replacedAt,
      replaceReason: item.replaceReason,
      storageKey: item.storageKey,
      mimeType: item.mimeType,
      previewable: item.previewable,
      isCurrent: false,
      index: idx + 1,
    })
  })

  // 2. 当前生效版本节点（位于历史演进树的最前沿/最新）
  nodes.push({
    id: targetFile.value.id,
    version: targetFile.value.version,
    name: targetFile.value.name,
    size: targetFile.value.size,
    uploadedBy: targetFile.value.uploadedBy,
    uploadedAt: targetFile.value.uploadedAt,
    replacedBy: targetFile.value.replacedBy,
    replacedAt: targetFile.value.replacedAt,
    replaceReason: targetFile.value.replaceReason || (nodes.length === 0 ? '初始版本上传' : '最新生效版本'),
    storageKey: targetFile.value.storageKey,
    mimeType: targetFile.value.mimeType,
    previewable: targetFile.value.previewable,
    isCurrent: true,
    index: nodes.length + 1,
  })

  return nodes
})

// 当前选中的版本详情
const selectedNode = computed<HistoryTreeNode | null>(() => {
  if (!selectedNodeId.value) return historyTree.value[historyTree.value.length - 1] || null
  return historyTree.value.find((n) => n.id === selectedNodeId.value) || historyTree.value[historyTree.value.length - 1] || null
})

function selectNode(node: HistoryTreeNode) {
  selectedNodeId.value = node.id
}

function openViewer(node: HistoryTreeNode) {
  if (!targetFile.value) return
  router.push({
    name: 'drawing-viewer',
    params: { drawingId: drawingId.value },
    query: { fileId: node.id },
  })
}

async function downloadFile(node: HistoryTreeNode) {
  if (!node.storageKey) {
    uiStore.toast('该历史文件暂无物理存储路径', 'warn')
    return
  }
  try {
    const dummyFile: DrawingFile = {
      id: node.id,
      name: node.name,
      size: node.size,
      role: targetFile.value?.role || 'part',
      drawingNo: targetFile.value?.drawingNo || drawingId.value,
      version: node.version,
      uploadedBy: node.uploadedBy,
      uploadedAt: node.uploadedAt,
      storageKey: node.storageKey,
      previewable: node.previewable,
    }
    await domainStore.downloadAttachment(dummyFile)
    uiStore.toast(`已触发下载 ${node.name} (${node.version})`, 'ok')
  } catch (err) {
    console.error('下载失败', err)
    uiStore.toast('下载历史文件失败', 'warn')
  }
}

function goBack() {
  router.push({ name: 'drawing-preview', params: { drawingId: drawingId.value } })
}

onMounted(() => {
  void loadFile()
})

watch([drawingId, fileId], () => {
  void loadFile()
})
</script>

<template>
  <div class="file-history-page">
    <!-- 顶部导航栏 -->
    <header class="history-header">
      <div class="header-left">
        <button class="back-btn" type="button" title="返回图纸文件列表" @click="goBack">
          <DemoIcon name="arrow-left" :size="15" />
          <span>返回图纸文件</span>
        </button>
        <div class="divider"></div>
        <div class="history-title-meta">
          <DemoIcon name="history" :size="18" class="title-icon" />
          <span class="file-main-name">{{ targetFile?.name || '图纸文件版本历史树' }}</span>
          <span class="tag info">{{ targetFile?.partNo || targetFile?.drawingNo || drawingId }}</span>
          <span class="tag ok">共 {{ historyTree.length }} 个演进版本</span>
        </div>
      </div>

      <div class="header-right">
        <button class="btn sm" type="button" @click="goBack">
          <DemoIcon name="check" :size="14" />完成查看
        </button>
      </div>
    </header>

    <!-- 核心两栏式工作区 -->
    <main class="history-body">
      <!-- 左侧：版本演进树 -->
      <section class="tree-sidebar card">
        <div class="sidebar-head">
          <DemoIcon name="git-commit" :size="16" />
          <span>版本演进历史树</span>
          <span class="hint">按替换流转时间正序</span>
        </div>

        <div class="tree-timeline-container">
          <div
            v-for="(node, index) in historyTree"
            :key="node.id"
            class="tree-node-item"
            :class="{ active: selectedNode?.id === node.id, 'is-current': node.isCurrent }"
            @click="selectNode(node)"
          >
            <!-- 连线轨道 -->
            <div class="tree-track">
              <div class="node-dot">
                <DemoIcon :name="node.isCurrent ? 'check-circle' : 'circle'" :size="12" />
              </div>
              <div v-if="index < historyTree.length - 1" class="node-line"></div>
            </div>

            <!-- 节点简要卡片 -->
            <div class="node-card">
              <div class="node-card-top">
                <span class="node-ver">{{ node.version }}</span>
                <span v-if="node.isCurrent" class="tag ok tag-xs">当前生效</span>
                <span v-else class="tag mute tag-xs">历史归档</span>
                <span class="node-size mono">{{ node.size }}</span>
              </div>

              <div class="node-card-name" :title="node.name">{{ node.name }}</div>

              <div class="node-card-meta">
                <span>{{ node.uploadedBy }}</span>
                <span class="dot-sep">·</span>
                <span>{{ node.uploadedAt }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 右侧：选中版本的完整详情与操作卡片 -->
      <section class="node-detail-panel card">
        <template v-if="selectedNode">
          <div class="detail-header">
            <div class="detail-header-left">
              <div class="ver-badge-large">
                <span class="v-text">{{ selectedNode.version }}</span>
                <span v-if="selectedNode.isCurrent" class="tag ok">当前生效最新版本</span>
                <span v-else class="tag mute">已归档历史版本</span>
              </div>
              <h2 class="detail-file-name">{{ selectedNode.name }}</h2>
            </div>

            <div class="detail-header-actions">
              <button
                class="btn primary"
                type="button"
                :title="selectedNode.previewable ? '在线浏览 CAD 矢量图' : '该格式暂不支持矢量直接渲染'"
                @click="openViewer(selectedNode)"
              >
                <DemoIcon name="eye" :size="15" />在线浏览
              </button>
              <button class="btn" type="button" title="下载此历史阶段的原始图纸文件" @click="downloadFile(selectedNode)">
                <DemoIcon name="download" :size="15" />下载本版本
              </button>
            </div>
          </div>

          <div class="detail-grid">
            <!-- 基本属性卡片 -->
            <div class="card sub-card">
              <div class="sub-card-title">
                <DemoIcon name="info" :size="15" />
                <span>版本核心属性</span>
              </div>
              <div class="info-table">
                <div class="info-row">
                  <span class="k">版本标识：</span>
                  <span class="v mono bold">{{ selectedNode.version }}</span>
                </div>
                <div class="info-row">
                  <span class="k">文件大小：</span>
                  <span class="v mono">{{ selectedNode.size }}</span>
                </div>
                <div class="info-row">
                  <span class="k">所属图号：</span>
                  <span class="v mono">{{ targetFile?.partNo || targetFile?.drawingNo || drawingId }}</span>
                </div>
                <div class="info-row">
                  <span class="k">文件格式：</span>
                  <span class="v mono uppercase">{{ selectedNode.name.split('.').pop() }} 矢量工程图</span>
                </div>
                <div class="info-row">
                  <span class="k">存储状态：</span>
                  <span class="v tag ok">物理文件完整在线</span>
                </div>
              </div>
            </div>

            <!-- 操作与演进记录卡片 -->
            <div class="card sub-card">
              <div class="sub-card-title">
                <DemoIcon name="shield-check" :size="15" />
                <span>流转与替换追溯</span>
              </div>
              <div class="info-table">
                <div class="info-row">
                  <span class="k">上传提交人：</span>
                  <span class="v font-bold">{{ selectedNode.uploadedBy }}</span>
                </div>
                <div class="info-row">
                  <span class="k">上传时间：</span>
                  <span class="v mono">{{ selectedNode.uploadedAt }}</span>
                </div>
                <div v-if="selectedNode.replacedBy" class="info-row">
                  <span class="k">替换执行人：</span>
                  <span class="v font-bold text-accent">{{ selectedNode.replacedBy }}</span>
                </div>
                <div v-if="selectedNode.replacedAt" class="info-row">
                  <span class="k">替换时间：</span>
                  <span class="v mono">{{ selectedNode.replacedAt }}</span>
                </div>
                <div class="info-row full-width">
                  <span class="k">变更/更新说明：</span>
                  <span class="v reason-text">{{ selectedNode.replaceReason || '无附加说明' }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 安全与审计留痕说明 -->
          <div class="note history-security-note">
            <DemoIcon name="lock" :size="15" />
            <div>
              <b>历史图纸版本防护机制：</b>
              本系统严格遵循工程图纸审计规范，所有历史阶段图纸物理文件与矢量 DXF 均永久保全。替换操作不删除任何原文件，支持随时回溯下载与比对。
            </div>
          </div>
        </template>
      </section>
    </main>
  </div>
</template>

<style scoped>
.file-history-page {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  background: var(--bg);
  color: var(--text-1);
  overflow: hidden;
}

/* 顶部导航 */
.history-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 52px;
  padding: 0 20px;
  background: var(--panel-top, var(--panel));
  border-bottom: 1px solid var(--line);
  z-index: 20;
  flex-shrink: 0;
  gap: 16px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.back-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: var(--radius-sm, 6px);
  background: var(--panel-2);
  border: 1px solid var(--line);
  color: var(--text-1);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.back-btn:hover {
  background: var(--hover);
  border-color: var(--accent);
  color: var(--accent);
}

.divider {
  width: 1px;
  height: 18px;
  background: var(--line);
}

.history-title-meta {
  display: flex;
  align-items: center;
  gap: 10px;
}

.title-icon {
  color: var(--accent);
}

.file-main-name {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-1);
  max-width: 380px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 核心两栏 */
.history-body {
  flex: 1;
  display: grid;
  grid-template-columns: 360px 1fr;
  gap: 16px;
  padding: 16px 20px;
  overflow: hidden;
  min-height: 0;
}

/* 左侧演进树侧栏 */
.tree-sidebar {
  display: flex;
  flex-direction: column;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--radius-md, 8px);
  overflow: hidden;
}

.sidebar-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--line);
  font-size: 14px;
  font-weight: 700;
}

.sidebar-head svg {
  color: var(--accent);
}

.tree-timeline-container {
  flex: 1;
  overflow-y: auto;
  padding: 16px 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.tree-node-item {
  display: flex;
  gap: 12px;
  cursor: pointer;
  padding: 6px 8px;
  border-radius: 6px;
  transition: all 0.15s ease;
}

.tree-node-item:hover {
  background: var(--hover);
}

.tree-node-item.active {
  background: var(--panel-2);
}

.tree-track {
  display: flex;
  flex-direction: column;
  align-items: center;
  width: 20px;
  flex-shrink: 0;
  padding-top: 4px;
}

.node-dot {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--panel-2);
  border: 1.5px solid var(--line-strong);
  color: var(--text-3);
  z-index: 2;
  transition: all 0.2s ease;
}

.tree-node-item.active .node-dot {
  border-color: var(--accent);
  color: var(--accent);
  background: var(--panel);
  transform: scale(1.15);
}

.tree-node-item.is-current .node-dot {
  border-color: #22c55e;
  color: #22c55e;
}

.node-line {
  flex: 1;
  width: 2px;
  background: var(--line);
  margin: 4px 0;
  min-height: 24px;
}

.tree-node-item.active .node-line {
  background: var(--accent-light, var(--line-strong));
}

.node-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.node-card-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.node-ver {
  font-family: 'JetBrains Mono', monospace;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.tree-node-item.active .node-ver {
  color: var(--accent);
}

.tag-xs {
  font-size: 10.5px;
  padding: 1px 5px;
}

.node-size {
  font-size: 11.5px;
  color: var(--text-3);
  margin-left: auto;
}

.node-card-name {
  font-size: 12.5px;
  color: var(--text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-card-meta {
  font-size: 11px;
  color: var(--text-3);
  display: flex;
  align-items: center;
  gap: 4px;
}

.dot-sep {
  opacity: 0.5;
}

/* 右侧详情主面板 */
.node-detail-panel {
  display: flex;
  flex-direction: column;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--radius-md, 8px);
  padding: 24px 28px;
  overflow-y: auto;
  gap: 24px;
}

.detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  border-bottom: 1px solid var(--line);
  padding-bottom: 20px;
}

.detail-header-left {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ver-badge-large {
  display: flex;
  align-items: center;
  gap: 10px;
}

.v-text {
  font-family: 'JetBrains Mono', monospace;
  font-size: 22px;
  font-weight: 800;
  color: var(--accent);
}

.detail-file-name {
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--text-1);
  line-height: 1.3;
}

.detail-header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 18px;
}

.sub-card {
  padding: 16px 18px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.sub-card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.sub-card-title svg {
  color: var(--accent);
}

.info-table {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.info-row {
  display: flex;
  align-items: flex-start;
  font-size: 13px;
  gap: 8px;
}

.info-row .k {
  color: var(--text-3);
  width: 100px;
  flex-shrink: 0;
}

.info-row .v {
  color: var(--text-1);
  overflow-wrap: anywhere;
}

.full-width {
  flex-direction: column;
  gap: 4px;
}

.full-width .k {
  width: 100%;
}

.reason-text {
  background: var(--panel);
  padding: 8px 12px;
  border-radius: 4px;
  border: 1px solid var(--line);
  width: 100%;
  box-sizing: border-box;
  font-size: 12.5px;
  line-height: 1.5;
}

.history-security-note {
  margin-top: auto;
  font-size: 12px;
  line-height: 1.6;
}

@media (max-width: 960px) {
  .history-body {
    grid-template-columns: 1fr;
    overflow-y: auto;
  }
}
</style>
