import { isTauri } from '@tauri-apps/api/core'

import { drawingFileService } from '@/app/container'
import { convertCadToPdfBlob } from '@/services/cad-pdf-export.service'
import { saveDownloadFile } from '@/services/tauri/cad-edit.service'
import type { DrawingFile } from '@/types/domain.types'

/**
 * 图纸批量下载的 IO 层：读取 EXB / DWG / PDF、CAD 转 PDF、文件名去重、打包 ZIP，
 * 以及三种保存方式（Tauri 原生另存为 → 浏览器 showSaveFilePicker → a 标签降级）。
 *
 * 这里不碰 Vue 状态：进度通过与 toast 都靠回调交回调用方。
 */

export type DownloadFormat = 'exb' | 'dwg' | 'pdf'

/** 可下载项：把附件里 EXB / DWG / PDF 三种来源的存储键一次算清，供选择与打包复用。 */
export interface DownloadCandidate {
  file: DrawingFile
  exbKey: string
  dwgKey: string
  pdfKey: string
  canConvertToPdf: boolean
}

export function toDownloadCandidates(files: readonly DrawingFile[]): DownloadCandidate[] {
  return files.map((file) => {
    const isDirectPdf = /\.pdf$/i.test(file.name || '') || /\.pdf$/i.test(file.storageKey || '')
    const exbKey = file.rawStorageKey || (/\.exb$/i.test(file.storageKey || '') ? file.storageKey! : '')
    const dwgKey = file.currentStorageKey || (/\.dwg$/i.test(file.storageKey || '') ? file.storageKey! : '')
    const pdfKey = isDirectPdf ? (file.storageKey || file.currentStorageKey || '') : ''
    const canConvertToPdf = Boolean(dwgKey || (/\.dxf$/i.test(file.storageKey || '') ? file.storageKey : ''))

    return {
      file,
      exbKey,
      dwgKey,
      pdfKey,
      canConvertToPdf,
    }
  })
}

export function candidateHasFormat(item: DownloadCandidate, format: DownloadFormat): boolean {
  if (format === 'exb') return Boolean(item.exbKey)
  if (format === 'dwg') return Boolean(item.dwgKey)
  if (format === 'pdf') return Boolean(item.pdfKey || item.canConvertToPdf)
  return false
}

export function downloadFormatLabel(format: DownloadFormat): string {
  return format === 'exb' ? 'EXB原始格式' : format === 'dwg' ? 'DWG格式' : 'PDF格式'
}

export interface BuildDownloadZipInput {
  format: DownloadFormat
  selected: readonly DownloadCandidate[]
  onProgress: (message: string) => void
}

/** 逐个读取（PDF 模式下先转换）后打包；单个文件失败只计数，不中断整包。 */
export async function buildDownloadZip(input: BuildDownloadZipInput): Promise<{ bytes: Uint8Array; failed: number }> {
  const JSZip = (await import('jszip')).default
  const zip = new JSZip()
  const usedNames = new Set<string>()
  let failed = 0

  for (const [index, item] of input.selected.entries()) {
    const baseName = item.file.name.replace(/\.[^/.]+$/, '')
    let fileName = `${baseName}.${input.format}`
    let suffix = 1
    while (usedNames.has(fileName.toLowerCase())) {
      fileName = `${baseName}(${suffix++}).${input.format}`
    }
    usedNames.add(fileName.toLowerCase())

    try {
      if (input.format === 'pdf') {
        input.onProgress(`正在生成 PDF ${index + 1}/${input.selected.length} · ${item.file.name}`)
        if (item.pdfKey) {
          // 原本就是 PDF 格式的附件
          const content = await drawingFileService.read(item.pdfKey)
          zip.file(fileName, content)
        } else {
          // CAD 图纸（DWG / DXF）：读取二进制数据并使用 CAD 查看器渲染为高清 PDF
          const cadSourceKey = item.dwgKey || item.file.storageKey || ''
          if (!cadSourceKey) throw new Error('缺少 CAD 图纸源文件')
          const cadBlob = await drawingFileService.read(cadSourceKey)
          const cadBuffer = await cadBlob.arrayBuffer()
          const pdfBlob = await convertCadToPdfBlob(cadBuffer, item.file.name)
          zip.file(fileName, pdfBlob)
        }
      } else {
        input.onProgress(`正在获取 ${index + 1}/${input.selected.length} · ${item.file.name}`)
        const key = input.format === 'exb' ? item.exbKey : item.dwgKey
        const content = await drawingFileService.read(key)
        // 直接存放在 zip 根目录下，不套外层文件夹
        zip.file(fileName, content)
      }
    } catch (itemError) {
      console.error(`获取/转换文件失败：${item.file.name}`, itemError)
      failed += 1
    }
  }

  input.onProgress('正在打包 zip...')
  const bytes = await zip.generateAsync({ type: 'uint8array' })
  return { bytes, failed }
}

export type DownloadSaveVia = 'tauri' | 'picker' | 'anchor'

export interface DownloadSaveResult {
  /** 用户在保存框中取消时为 'cancelled'。 */
  via: DownloadSaveVia | 'cancelled'
  /** 仅 Tauri 模式返回落盘路径。 */
  savedPath?: string
}

/** 保存 ZIP：桌面端走原生另存为，浏览器优先 File System Access，最后降级 a 标签。 */
export async function saveDownloadZip(name: string, bytes: Uint8Array): Promise<DownloadSaveResult> {
  // 1. 桌面客户端模式：调起系统原生“另存为”文件选择框
  if (isTauri()) {
    const savedPath = await saveDownloadFile(name, bytes)
    if (!savedPath) {
      // 用户在文件选择框中点击了取消
      return { via: 'cancelled' }
    }
    return { via: 'tauri', savedPath }
  }

  // 2. 浏览器端模式：优先调起浏览器原生另存为文件选择器
  const blob = new Blob([bytes.buffer as ArrayBuffer], { type: 'application/zip' })
  const showSaveFilePicker = (window as unknown as { showSaveFilePicker?: (options: unknown) => Promise<FileSystemFileHandle> }).showSaveFilePicker
  if (typeof showSaveFilePicker === 'function') {
    try {
      const handle = await showSaveFilePicker({
        suggestedName: name,
        types: [{
          description: 'ZIP 压缩包 (*.zip)',
          accept: { 'application/zip': ['.zip'] },
        }],
      })
      const writable = await handle.createWritable()
      await writable.write(blob)
      await writable.close()
      return { via: 'picker' }
    } catch (err: unknown) {
      if (err && typeof err === 'object' && 'name' in err && err.name === 'AbortError') {
        // 用户取消保存
        return { via: 'cancelled' }
      }
      // 选择器不可用等异常：继续走 a 标签降级
    }
  }

  // 3. 浏览器降级：触发 a 标签下载
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name
  anchor.click()
  URL.revokeObjectURL(url)
  return { via: 'anchor' }
}
