<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import DxfParser from 'dxf-parser'

interface Props {
  dxfUrl?: string | null
}

const props = defineProps<Props>()
const containerRef = ref<HTMLDivElement | null>(null)
const loading = ref(false)
const errorMessage = ref('')

let viewer: any = null
let resizeObserver: ResizeObserver | null = null
let viewDxfConstructor: any = null

function clearViewer() {
  if (!viewer) return
  try {
    viewer.sceneRemoveViewerCtrl?.()
    viewer.deleteCurrentNodeCtrl?.()
  } catch (error) {
    console.warn('销毁 view-dxf 实例失败', error)
  }
  viewer = null
  if (containerRef.value) {
    containerRef.value.replaceChildren()
  }
}

async function loadViewer(url: string) {
  const container = containerRef.value
  if (!container) return

  clearViewer()
  loading.value = true
  errorMessage.value = ''

  try {
    const token = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
    const response = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)

    const buffer = await response.arrayBuffer()
    const bytes = new Uint8Array(buffer)
    const header = new TextDecoder('ascii').decode(bytes.slice(0, Math.min(bytes.byteLength, 8192)))
    const decoder = /ANSI_936|GB2312|GBK|CP936|GB18030/i.test(header)
      ? new TextDecoder('gb18030')
      : new TextDecoder('utf-8')
    let text: string
    try {
      text = decoder.decode(buffer)
    } catch {
      text = new TextDecoder('gb18030').decode(buffer)
    }

    const dxfData = new DxfParser().parseSync(text)
    const width = Math.max(1, container.clientWidth)
    const height = Math.max(1, container.clientHeight)
    if (!viewDxfConstructor) {
      const module = await import('view-dxf')
      viewDxfConstructor = module.default
    }
    viewer = new viewDxfConstructor(dxfData, container, width, height, null, (callback: unknown) => {
      if (callback && typeof callback === 'object' && 'type' in callback && (callback as any).type === 'messageInfoDxf') {
        console.warn('view-dxf:', callback)
      }
    })
  } catch (error: any) {
    errorMessage.value = `view-dxf 加载失败: ${error?.message || error}`
  } finally {
    loading.value = false
  }
}

function resizeViewer() {
  if (!viewer || !containerRef.value) return
  const width = Math.max(1, containerRef.value.clientWidth)
  const height = Math.max(1, containerRef.value.clientHeight)
  viewer.resetCameraCtrl?.(width, height)
}

watch(() => props.dxfUrl, (url) => {
  if (url) void loadViewer(url)
  else clearViewer()
})

onMounted(() => {
  resizeObserver = new ResizeObserver(() => resizeViewer())
  if (containerRef.value) resizeObserver.observe(containerRef.value)
  if (props.dxfUrl) void loadViewer(props.dxfUrl)
})

onUnmounted(() => {
  resizeObserver?.disconnect()
  clearViewer()
})
</script>

<template>
  <div ref="containerRef" class="view-dxf-viewer">
    <div v-if="loading" class="view-dxf-status">正在加载 view-dxf 渲染引擎...</div>
    <div v-if="errorMessage" class="view-dxf-error">{{ errorMessage }}</div>
  </div>
</template>

<style scoped>
.view-dxf-viewer {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 400px;
  overflow: hidden;
  background: var(--cad-bg, #1a1d24);
}

.view-dxf-status,
.view-dxf-error {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: grid;
  place-items: center;
  padding: 20px;
  color: var(--text-2, #cbd5e1);
  background: color-mix(in srgb, var(--panel, #1e2228) 84%, transparent);
}

.view-dxf-error {
  color: var(--danger, #f87171);
}
</style>
