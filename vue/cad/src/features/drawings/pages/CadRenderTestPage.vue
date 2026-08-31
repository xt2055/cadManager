<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { assertCadWorkerAssets, getCadWorkerUrls } from '../detail-tabs/preview/cad-worker-assets'
import { findWipeoutMasks } from '../detail-tabs/preview/cad-entity-filters'
import { resolveCadFontsBaseUrl } from '@/services/cad-fonts.service'

const router = useRouter()
const containerRef = ref<HTMLDivElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const renderMode = ref<'worker' | 'main'>('worker')
const loading = ref(false)
const statusMessage = ref('请选择一个 DWG 或 DXF 文件')
const errorMessage = ref('')

let manager: any = null
let loadId = 0

async function destroyManager() {
  const currentManager = manager
  manager = null
  if (!currentManager) return

  try {
    await Promise.resolve(currentManager.destroy()).catch(() => undefined)
  } catch (error) {
    console.warn('销毁 CAD 测试查看器失败', error)
  }
}

async function openSelectedFile() {
  const file = selectedFile.value
  const container = containerRef.value
  if (!file || !container) return

  const currentLoadId = ++loadId
  await destroyManager()
  if (currentLoadId !== loadId) return

  loading.value = true
  errorMessage.value = ''
  statusMessage.value = `正在使用 ${renderMode.value === 'worker' ? 'Worker' : '主线程'} 渲染：${file.name}`

  try {
    const buffer = await file.arrayBuffer()
    if (currentLoadId !== loadId) return

    const [{ AcApDocManager, AcEdOpenMode }, { AcDbDatabaseConverterManager, AcDbFileType }, { AcDbLibreDwgConverter }] = await Promise.all([
      import('@mlightcad/cad-simple-viewer'),
      import('@mlightcad/data-model'),
      import('@mlightcad/libredwg-converter'),
    ])
    const workerUrls = getCadWorkerUrls()
    await assertCadWorkerAssets(workerUrls)
    const workersReady = await AcApDocManager.checkWebworkerReadiness(workerUrls)
    if (!workersReady) {
      throw new Error(`CAD Worker 不可访问：${JSON.stringify(workerUrls)}`)
    }
    const converterManager = AcDbDatabaseConverterManager.instance
    if (!converterManager.get(AcDbFileType.DWG)) {
      converterManager.register(AcDbFileType.DWG, new AcDbLibreDwgConverter({
        convertByEntityType: false,
        useWorker: true,
        parserWorkerUrl: workerUrls.dwgParser,
      }))
    }

    manager = AcApDocManager.createInstance({
      container,
      autoResize: true,
      baseUrl: await resolveCadFontsBaseUrl(),
      useMainThreadDraw: renderMode.value === 'main',
      webworkerFileUrls: workerUrls,
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
    if (!manager) throw new Error('无法创建 CAD 查看器实例')

    const opened = await manager.openDocument(file.name, buffer, {
      minimumChunkSize: 1000,
      mode: AcEdOpenMode.Read,
      drawNoPlotLayers: false,
      progressiveRendering: false,
      sysVars: {
        lwdisplay: false,
      },
    })
    if (!opened) throw new Error(`无法解析文件：${file.name}`)
    const wipeoutMasks = findWipeoutMasks(manager.curDocument.database)
    await manager.curView?.waitUntilIdle?.(60_000)
    if (wipeoutMasks.length > 0) {
      manager.curView.removeEntity(wipeoutMasks)
      manager.curView.isDirty = true
      manager.curView.isHtmlDirty = true
    }
    if (currentLoadId !== loadId) return

    statusMessage.value = `已加载：${file.name}，渲染模式：${renderMode.value === 'worker' ? 'Worker' : '主线程'}`
  } catch (error: any) {
    if (currentLoadId === loadId) {
      errorMessage.value = error?.message || String(error)
      statusMessage.value = '加载失败，请查看浏览器控制台'
    }
    await destroyManager()
  } finally {
    if (currentLoadId === loadId) loading.value = false
  }
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  selectedFile.value = input.files?.[0] || null
  if (selectedFile.value) void openSelectedFile()
}

function changeRenderMode(mode: 'worker' | 'main') {
  if (renderMode.value === mode) return
  renderMode.value = mode
  if (selectedFile.value) void openSelectedFile()
}

function fitDrawing() {
  manager?.curView?.zoomToFitDrawing(60_000)
}

function clearFile() {
  ++loadId
  selectedFile.value = null
  errorMessage.value = ''
  statusMessage.value = '请选择一个 DWG 或 DXF 文件'
  if (fileInputRef.value) fileInputRef.value.value = ''
  void destroyManager()
}

onMounted(() => {
  statusMessage.value = '请选择一个 DWG 或 DXF 文件'
})

onBeforeUnmount(() => {
  ++loadId
  void destroyManager()
})
</script>

<template>
  <div class="cad-test-page">
    <header class="cad-test-toolbar">
      <div>
        <h1>MLightCAD 独立渲染测试</h1>
        <p>此页面不使用业务图纸转换和额外场景处理，直接将上传文件交给 CAD 渲染器。</p>
      </div>

      <div class="cad-test-actions">
        <label class="file-button">
          选择 CAD 文件
          <input
            ref="fileInputRef"
            type="file"
            accept=".dwg,.dxf"
            @change="handleFileChange"
          >
        </label>
        <button type="button" @click="fitDrawing">适配图纸</button>
        <button type="button" @click="clearFile">清空</button>
        <button type="button" class="back-button" @click="router.back()">返回</button>
      </div>
    </header>

    <section class="cad-test-info">
      <span>{{ statusMessage }}</span>
      <span v-if="selectedFile">文件大小：{{ (selectedFile.size / 1024 / 1024).toFixed(2) }} MB</span>
      <span v-if="loading" class="loading-text">加载中...</span>
      <span v-if="errorMessage" class="error-text">{{ errorMessage }}</span>
      <div class="render-mode">
        <span>渲染线程：</span>
        <button
          type="button"
          :class="{ active: renderMode === 'worker' }"
          @click="changeRenderMode('worker')"
        >
          Worker
        </button>
        <button
          type="button"
          :class="{ active: renderMode === 'main' }"
          @click="changeRenderMode('main')"
        >
          主线程
        </button>
      </div>
    </section>

    <main ref="containerRef" class="cad-test-viewer">
      <div v-if="!selectedFile && !loading" class="cad-test-empty">
        请选择要测试的 DWG 或 DXF 文件
      </div>
    </main>
  </div>
</template>

<style scoped>
.cad-test-page {
  display: flex;
  flex-direction: column;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  background: #10151d;
  color: #d8e0ea;
}

.cad-test-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 16px 20px;
  border-bottom: 1px solid #273242;
  background: #151c26;
}

h1 {
  margin: 0 0 6px;
  color: #f2f5f8;
  font-size: 18px;
}

p {
  margin: 0;
  color: #8e9aaa;
  font-size: 12px;
}

.cad-test-actions,
.cad-test-info,
.render-mode {
  display: flex;
  align-items: center;
  gap: 8px;
}

button,
.file-button {
  padding: 7px 11px;
  border: 1px solid #384557;
  border-radius: 5px;
  background: #202a38;
  color: #d8e0ea;
  font-size: 12px;
  cursor: pointer;
}

button:hover,
.file-button:hover {
  border-color: #5d8fbd;
  background: #29384a;
}

.file-button input {
  display: none;
}

.back-button {
  margin-left: 8px;
}

.cad-test-info {
  flex-wrap: wrap;
  min-height: 42px;
  padding: 8px 20px;
  border-bottom: 1px solid #273242;
  color: #9eabba;
  font-size: 12px;
}

.loading-text {
  color: #f1c76d;
}

.error-text {
  color: #f18787;
}

.render-mode {
  margin-left: auto;
}

.render-mode button {
  padding: 4px 8px;
  color: #9eabba;
}

.render-mode button.active {
  border-color: #62a4d6;
  background: #1d4662;
  color: #dff2ff;
}

.cad-test-viewer {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: #000;
}

.cad-test-empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  color: #657386;
  font-size: 14px;
  pointer-events: none;
}

@media (max-width: 800px) {
  .cad-test-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }

  .cad-test-actions {
    flex-wrap: wrap;
  }

  .render-mode {
    margin-left: 0;
  }
}
</style>
