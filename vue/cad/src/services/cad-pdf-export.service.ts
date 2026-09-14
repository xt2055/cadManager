import { AcApDocManager, AcEdOpenMode } from '@mlightcad/cad-simple-viewer'
import { AcGeBox2d } from '@mlightcad/data-model'
import { jsPDF } from 'jspdf'
import { registerCadConverters } from '@/services/cad-converters.service'
import { assertCadWorkerAssets, getCadWorkerUrls } from '@/features/drawings/detail-tabs/preview/cad-worker-assets'
import { normalizeCadToleranceEntities, preloadCadSymbolFonts, resolveCadFontsBaseUrl } from '@/services/cad-fonts.service'
import { findWipeoutMasks } from '@/features/drawings/detail-tabs/preview/cad-entity-filters'
import { resolveCadPdfFrame } from './cad-pdf-frame'
import { convertCadPixelsToMonochrome } from './cad-pdf-monochrome'
import { installCadLineWeights } from './cad-render-compat'
import { installCadPdfStrokes } from './cad-pdf-strokes'

let headlessContainer: HTMLElement | null = null
let exportQueue: Promise<unknown> = Promise.resolve()

async function getOrCreateCadManager(): Promise<AcApDocManager> {
  const workerUrls = getCadWorkerUrls()
  await assertCadWorkerAssets(workerUrls)
  await registerCadConverters(workerUrls.dwgParser)

  let manager: AcApDocManager
  try {
    manager = AcApDocManager.instance
  } catch {
    if (!headlessContainer) {
      headlessContainer = document.createElement('div')
      headlessContainer.id = 'cad-pdf-headless-container'
      headlessContainer.style.cssText = 'width:800px;height:600px;position:fixed;left:-9999px;top:-9999px;visibility:hidden;'
      document.body.appendChild(headlessContainer)
    }
    const instance = AcApDocManager.createInstance({
      container: headlessContainer,
      autoResize: false,
      baseUrl: await resolveCadFontsBaseUrl(),
      useMainThreadDraw: true,
      webworkerFileUrls: workerUrls,
      openDocumentDefaults: {
        minimumChunkSize: 1000,
        mode: AcEdOpenMode.Read,
        progressiveRendering: false,
        // 导出必须保留实体/图层线宽；关闭 LWDISPLAY 会退化成统一细线。
        sysVars: { lwdisplay: true },
      },
    })
    if (!instance) throw new Error('创建 CAD 解析器实例失败')
    manager = instance
  }
  // 同时覆盖主线程与已有查看器的绘图 Worker。
  await preloadCadSymbolFonts(manager)
  return manager
}

/** 使用查看器的完整几何生成白底黑线的高清打印 PDF。 */
export function convertCadToPdfBlob(buffer: ArrayBuffer, fileName: string): Promise<Blob> {
  // 文档管理器是单例；并发导出不能互相切换或关闭文档。
  const result = exportQueue.then(() => exportCadPdf(buffer, fileName))
  exportQueue = result.catch(() => undefined)
  return result
}

async function exportCadPdf(buffer: ArrayBuffer, fileName: string): Promise<Blob> {
  const manager = await getOrCreateCadManager()
  const previousDocument = manager.curDocument
  const existingDocuments = new Set(manager.documents)
  let exportDocument: typeof previousDocument | undefined
  let restoreLineWeights: (() => void) | undefined
  let restoreStrokes: (() => void) | undefined
  const effectiveFileName = fileName.toLowerCase().endsWith('.dxf') ? fileName : `${fileName.replace(/\.[^/.]+$/, '')}.dwg`

  try {
    const opened = await manager.openDocument(effectiveFileName, buffer, {
      minimumChunkSize: 1000,
      mode: AcEdOpenMode.Read,
      drawNoPlotLayers: false,
      progressiveRendering: false,
      // 让 CAD 转换器创建带 linewidth 的 LineMaterial。
      sysVars: { lwdisplay: true },
    })
    if (!opened) throw new Error(`无法解析图纸文件：${fileName}`)
    exportDocument = manager.curDocument
    const view = manager.curView
    if (!exportDocument?.database || !view) throw new Error(`图纸未正确初始化：${fileName}`)

    if (!await view.waitUntilIdle()) throw new Error('图纸字体或图形尚未绘制完成，请稍后重试导出')
    const normalized = normalizeCadToleranceEntities(exportDocument.database)
    const wipeoutMasks = findWipeoutMasks(exportDocument.database)
    if (normalized > 0) {
      view.clear()
      await exportDocument.database.regen()
      if (!await view.waitUntilIdle()) throw new Error('图纸标注尚未绘制完成，请稍后重试导出')
    }
    if (wipeoutMasks.length > 0) view.removeEntity(wipeoutMasks)

    const layout = view.cadScene.activeLayout
    if (!layout) throw new Error('图纸没有可导出的布局')
    const box = layout.computeBatchBoundingBox()
    const webgl = view.renderer.internalRenderer
    const gl = webgl.getContext()
    const maxSize = Math.min(webgl.capabilities.maxTextureSize, gl.getParameter(gl.MAX_RENDERBUFFER_SIZE))
    const frame = resolveCadPdfFrame(box.min, box.max, maxSize)
    const bounds = new AcGeBox2d(frame.min, frame.max)

    // 按纸面毫米换算像素，并重新创建宽线几何；仅修改截图尺寸不会改变线宽。
    restoreLineWeights = installCadLineWeights(view.renderer, exportDocument.database, frame.width / frame.pageWidth)
    view.clear()
    await exportDocument.database.regen()
    if (!await view.waitUntilIdle()) throw new Error('打印线宽尚未绘制完成，请重试')
    if (wipeoutMasks.length > 0) view.removeEntity(wipeoutMasks)
    const printLayout = view.cadScene.activeLayout
    if (!printLayout) throw new Error('图纸没有可导出的布局')
    restoreStrokes = installCadPdfStrokes(printLayout.internalObject, frame.width / frame.pageWidth)

    // 直接渲染 CAD 布局，避免 SVG/PDF 重新解释字体、图案填充和 INSERT 坐标。
    // 独立相机按图纸边界构图，不受当前查看器的平移、缩放或窗口比例影响。
    const capture = view.renderer.renderEntityPreview(printLayout.internalObject, {
      width: frame.width,
      height: frame.height,
      bounds,
      margin: 1,
      backgroundColor: view.backgroundColor,
      backgroundAlpha: 1,
      viewportSize: { width: view.width, height: view.height },
      onRestored: () => { view.isDirty = true },
    })
    if (!capture) throw new Error('图纸没有可导出的图形')
    try {
      const context = capture.canvas.getContext('2d')
      if (!context) throw new Error('无法创建 PDF 黑白打印画布')
      const pixels = context.getImageData(0, 0, capture.canvas.width, capture.canvas.height)
      convertCadPixelsToMonochrome(pixels.data, view.backgroundColor)
      context.putImageData(pixels, 0, 0)
      const pdf = new jsPDF({
        orientation: frame.pageWidth >= frame.pageHeight ? 'landscape' : 'portrait',
        unit: 'mm',
        format: [frame.pageWidth, frame.pageHeight],
        compress: true,
      })
      // PNG 为无损编码；清晰度由渲染分辨率和纸面笔画宽度决定。
      pdf.addImage(capture.canvas, 'PNG', 0, 0, frame.pageWidth, frame.pageHeight, undefined, 'SLOW')
      return pdf.output('blob')
    } finally {
      capture.canvas.width = 0
      capture.canvas.height = 0
    }
  } finally {
    restoreStrokes?.()
    restoreLineWeights?.()
    // 只关闭本次导出的文档；失败或用户切换标签时不能误关当前图纸。
    if (exportDocument && !existingDocuments.has(exportDocument) && manager.documents.includes(exportDocument)) {
      await manager.closeDocument(exportDocument)
    }
    if (previousDocument && manager.documents.includes(previousDocument)) {
      await manager.activateDocument(previousDocument)
    }
  }
}
