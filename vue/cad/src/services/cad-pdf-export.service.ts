import { AcApDocManager, AcEdOpenMode } from '@mlightcad/cad-simple-viewer'
import { AcApPdfConvertor } from '@mlightcad/cad-pdf-plugin'
import { registerCadConverters } from '@/services/cad-converters.service'
import { assertCadWorkerAssets, getCadWorkerUrls } from '@/features/drawings/detail-tabs/preview/cad-worker-assets'
import { normalizeCadToleranceEntities, preloadCadSymbolFonts, resolveCadFontsBaseUrl } from '@/services/cad-fonts.service'
import { findWipeoutMasks } from '@/features/drawings/detail-tabs/preview/cad-entity-filters'

let headlessContainer: HTMLElement | null = null

async function getOrCreateCadManager(): Promise<AcApDocManager> {
  const workerUrls = getCadWorkerUrls()
  await assertCadWorkerAssets(workerUrls)
  await registerCadConverters(workerUrls.dwgParser)
  await preloadCadSymbolFonts()

  try {
    return AcApDocManager.instance
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
      webworkerFileUrls: {
        ...workerUrls,
      },
      openDocumentDefaults: {
        minimumChunkSize: 1000,
        mode: AcEdOpenMode.Read,
        progressiveRendering: false,
        sysVars: {
          lwdisplay: false,
        },
      },
    })
    if (!instance) {
      throw new Error('创建 CAD 解析器实例失败')
    }
    return instance
  }
}

/**
 * 将 CAD 图纸二进制流（DWG / DXF）在后台转换为矢量 PDF Blob
 * @param buffer CAD 图纸二进制数据
 * @param fileName 图纸名称（带 .dwg 或 .dxf 扩展名，用于解析器判断）
 */
export async function convertCadToPdfBlob(buffer: ArrayBuffer, fileName: string): Promise<Blob> {
  const manager = await getOrCreateCadManager()
  const effectiveFileName = fileName.toLowerCase().endsWith('.dxf') ? fileName : `${fileName.replace(/\.[^/.]+$/, '')}.dwg`

  const opened = await manager.openDocument(effectiveFileName, buffer, {
    minimumChunkSize: 1000,
    mode: AcEdOpenMode.Read,
    drawNoPlotLayers: false,
    progressiveRendering: false,
    sysVars: {
      lwdisplay: false,
    },
  })

  if (!opened) {
    throw new Error(`无法解析图纸文件：${fileName}`)
  }

  try {
    const doc = manager.curDocument
    if (!doc || !doc.database) {
      throw new Error(`图纸数据库未正确初始化：${fileName}`)
    }

    normalizeCadToleranceEntities(doc.database)
    const wipeoutMasks = findWipeoutMasks(doc.database)
    if (wipeoutMasks.length > 0 && manager.curView) {
      manager.curView.removeEntity(wipeoutMasks)
    }

    const converter = new AcApPdfConvertor()

    let capturedBlob: Blob | null = null
    const win = typeof window !== 'undefined' ? (window as unknown as Record<string, unknown>) : null
    const prevSaveAs = win ? win.saveAs : undefined
    const prevCreateObjectURL = typeof URL !== 'undefined' ? URL.createObjectURL : undefined
    const origAnchorClick = typeof HTMLAnchorElement !== 'undefined' ? HTMLAnchorElement.prototype.click : undefined

    try {
      if (win) {
        win.saveAs = (blob: Blob) => {
          capturedBlob = blob
        }
      }

      if (typeof URL !== 'undefined' && prevCreateObjectURL) {
        URL.createObjectURL = function (obj: unknown) {
          if (obj instanceof Blob) {
            capturedBlob = obj
          }
          return prevCreateObjectURL.call(this, obj as Blob)
        }
      }

      if (typeof HTMLAnchorElement !== 'undefined' && origAnchorClick) {
        HTMLAnchorElement.prototype.click = function (this: HTMLAnchorElement) {
          if (this.download && /\.pdf$/i.test(this.download)) {
            return
          }
          return origAnchorClick.apply(this)
        }
      }

      await converter.convert(manager.context)

      if (!capturedBlob) {
        await new Promise((resolve) => setTimeout(resolve, 60))
      }
    } finally {
      if (win) {
        win.saveAs = prevSaveAs
      }
      if (typeof URL !== 'undefined' && prevCreateObjectURL) {
        URL.createObjectURL = prevCreateObjectURL
      }
      if (typeof HTMLAnchorElement !== 'undefined' && origAnchorClick) {
        HTMLAnchorElement.prototype.click = origAnchorClick
      }
    }

    if (!capturedBlob) {
      throw new Error(`PDF 导出未生成有效数据：${fileName}`)
    }

    return capturedBlob
  } finally {
    if (manager.curDocument) {
      await manager.closeDocument(manager.curDocument).catch(() => {})
    }
  }
}
