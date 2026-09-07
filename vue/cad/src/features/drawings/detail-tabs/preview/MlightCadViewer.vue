<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { assertCadWorkerAssets, getCadWorkerUrls } from './cad-worker-assets'
import { findWipeoutMasks } from './cad-entity-filters'
import { installCadFontDiagnostics, normalizeCadToleranceEntities, preloadCadSymbolFonts, resolveCadFontsBaseUrl } from '@/services/cad-fonts.service'
import { registerCadConverters } from '@/services/cad-converters.service'

interface Props {
  dxfUrl?: string | null
  fileName?: string | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'layers-loaded', layers: Array<{ name: string; color: string; visible: boolean }>): void
  (event: 'zoom-change', zoom: number): void
}>()
const containerRef = ref<HTMLDivElement | null>(null)
const loading = ref(false)
const errorMessage = ref('')

let manager: any = null
let managerGeneration = 0
let openGeneration = 0
let disposed = false
let destroyPromise: Promise<void> | null = null

function useMainThreadCadDraw() {
  if (!import.meta.env.DEV || typeof window === 'undefined') return false
  const queryEnabled = new URLSearchParams(window.location.search).get('cad-main-thread') === '1'
  const storageEnabled = window.localStorage.getItem('cad_use_main_thread_draw') !== '0'
  return queryEnabled || storageEnabled
}

async function finishInitialRender(currentManager: any) {
  const view = currentManager?.curView
  if (!view) return

  // openDocument() 可能早于字体和延迟文字几何完成返回。等待场景空闲后
  // 再适配一次，否则形位公差等 MTEXT/DIMENSION 可能要等用户点击才出现。
  if (typeof view.waitUntilIdle === 'function') {
    await view.waitUntilIdle(60_000)
  }

  view.zoomToFitDrawing(60_000)
  if (typeof view.waitUntilIdle === 'function') {
    await view.waitUntilIdle(60_000)
  }
  view.isDirty = true
  view.isHtmlDirty = true
}

function getAccessToken() {
  return localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
}

async function destroyViewer() {
  if (destroyPromise) return destroyPromise
  const currentManager = manager
  manager = null
  if (!currentManager) return
  destroyPromise = (async () => {
    try {
      // MLightCAD 1.6.3 的 destroy 内部可能产生一个脱离返回 Promise 的
      // Worker 终止异常，因此同时挂接同步和异步两层保护。
      const result = currentManager.destroy()
      await Promise.resolve(result).catch(() => undefined)
    } catch (error: any) {
      if (!/renderer terminated/i.test(String(error?.message || error))) {
        console.warn('销毁 MLightCAD 查看器失败', error)
      }
    } finally {
      destroyPromise = null
    }
  })()
  return destroyPromise
}

async function loadViewer(url: string) {
  const container = containerRef.value
  if (!container || disposed) return

  const generation = ++openGeneration
  await destroyViewer()
  if (generation !== openGeneration || disposed || !containerRef.value) return

  loading.value = true
  errorMessage.value = ''

  try {
    const token = getAccessToken()
    const response = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const buffer = await response.arrayBuffer()
    if (generation !== openGeneration || disposed || !containerRef.value) return

    const { AcApDocManager, AcEdOpenMode } = await import('@mlightcad/cad-simple-viewer')
    const workerUrls = getCadWorkerUrls()
    await assertCadWorkerAssets(workerUrls)
    if (!(await AcApDocManager.checkWebworkerReadiness(workerUrls))) {
      throw new Error(`CAD Worker 不可访问：${JSON.stringify(workerUrls)}`)
    }
    await registerCadConverters(workerUrls.dwgParser)
    const mainThreadDraw = useMainThreadCadDraw()
    console.info('[CAD][字体诊断] 绘制线程', JSON.stringify({ mainThreadDraw }))
    manager = AcApDocManager.createInstance({
      container: containerRef.value,
      autoResize: true,
      baseUrl: await resolveCadFontsBaseUrl(),
      // 与官方示例保持一致，使用 Worker 绘制复杂标注块。
      useMainThreadDraw: mainThreadDraw,
      webworkerFileUrls: {
        ...workerUrls,
      },
      checkWorkersOnInit: true,
      openDocumentDefaults: {
        minimumChunkSize: 1000,
        mode: AcEdOpenMode.Read,
        progressiveRendering: false,
        sysVars: {
          lwdisplay: false,
        },
      },
    })
    managerGeneration = generation

    await installCadFontDiagnostics(manager)

    // 先加载 GDT/SHX，再打开图纸，避免 TOLERANCE 首帧被普通字母字体替代。
    await preloadCadSymbolFonts(manager)

    if (generation !== openGeneration || disposed) {
      await destroyViewer()
      return
    }

    // 按原始图纸名称交给 MLightCAD，让它依据 .dwg 后缀选择 DWG 解析器。
    const fileName = props.fileName || url.split('?')[0]?.split('/').pop() || 'drawing.dwg'
    const opened = await manager.openDocument(fileName, buffer, {
      minimumChunkSize: 1000,
      mode: AcEdOpenMode.Read,
      drawNoPlotLayers: false,
      progressiveRendering: false,
      sysVars: {
        lwdisplay: false,
      },
    })
    if (!opened) throw new Error(`MLightCAD 无法解析图纸（传入文件名: ${fileName}）`)
    normalizeCadToleranceEntities(manager.curDocument.database)
    const wipeoutMasks = findWipeoutMasks(manager.curDocument.database)
    const layers = manager.curDocument.layerStore.getLayers().map((layer: any) => ({
      name: layer.name,
      color: layer.cssColor || '#00f3ff',
      visible: layer.isOn && !layer.isFrozen,
    }))
    emit('layers-loaded', layers)
    await finishInitialRender(manager)
    if (wipeoutMasks.length > 0) {
      manager.curView.removeEntity(wipeoutMasks)
      manager.curView.isDirty = true
      manager.curView.isHtmlDirty = true
    }
    emit('zoom-change', 1)
    await nextTick()
  } catch (error: any) {
    if (managerGeneration === generation) await destroyViewer()
    if (generation === openGeneration) {
      errorMessage.value = `MLightCAD 加载失败: ${error?.message || error}`
    }
  } finally {
    if (generation === openGeneration) loading.value = false
  }
}

function setLayerVisibility(layerName: string, visible: boolean) {
  manager?.curDocument.layerStore.setLayerOn(layerName, visible)
}

function zoomIn() {
  manager?.sendStringToExecute('zoom\n2x\n')
}

function zoomOut() {
  manager?.sendStringToExecute('zoom\n0.5x\n')
}

function resetView() {
  manager?.curView.zoomToFitDrawing(60_000)
  emit('zoom-change', 1)
}

defineExpose({ setLayerVisibility, zoomIn, zoomOut, resetView })

watch(() => props.dxfUrl, (url) => {
  if (url) void loadViewer(url)
  else void destroyViewer()
})

onMounted(() => {
  disposed = false
  console.info('[MlightCadViewer] 组件已挂载', {
    url: props.dxfUrl,
    fileName: props.fileName,
  })
  if (props.dxfUrl) void loadViewer(props.dxfUrl)
})

onUnmounted(() => {
  disposed = true
  openGeneration++
  void destroyViewer()
})
</script>

<template>
  <div ref="containerRef" class="mlightcad-viewer">
    <div v-if="loading" class="mlightcad-status">正在加载 MLightCAD 渲染引擎...</div>
    <div v-if="errorMessage" class="mlightcad-error">{{ errorMessage }}</div>
  </div>
</template>

<style scoped>
.mlightcad-viewer {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 400px;
  overflow: hidden;
  background: #000;
}

.mlightcad-status,
.mlightcad-error {
  position: absolute;
  inset: 0;
  z-index: 5;
  display: grid;
  place-items: center;
  padding: 20px;
  color: #cbd5e1;
  background: rgba(10, 13, 20, 0.86);
}

.mlightcad-error {
  color: #f87171;
}
</style>
