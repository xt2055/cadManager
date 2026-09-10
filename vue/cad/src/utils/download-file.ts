import { isTauri } from '@tauri-apps/api/core'
import { saveDownloadFile } from '@/services/tauri/cad-edit.service'

interface SavePickerWindow extends Window {
  showSaveFilePicker?: (options: {
    suggestedName: string
    types?: Array<{ description: string; accept: Record<string, string[]> }>
  }) => Promise<FileSystemFileHandle>
}

function extensionOf(name: string): string {
  const match = /\.[^.]+$/.exec(name)
  return match?.[0].toLowerCase() || ''
}

function mimeTypeFor(name: string, fallback: string): string {
  const extension = extensionOf(name)
  if (extension === '.zip') return 'application/zip'
  if (extension === '.xlsx') return 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
  if (extension === '.docx') return 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'
  if (extension === '.pdf') return 'application/pdf'
  return fallback
}

export async function saveDownload(name: string, content: Blob): Promise<boolean> {
  const bytes = new Uint8Array(await content.arrayBuffer())
  if (isTauri()) return (await saveDownloadFile(name, bytes)) !== null

  const picker = (window as SavePickerWindow).showSaveFilePicker
  if (picker) {
    try {
      const extension = extensionOf(name)
      const handle = await picker({
        suggestedName: name,
        ...(extension ? {
          types: [{
            description: `${extension.slice(1).toUpperCase()} 文件`,
            accept: { [mimeTypeFor(name, content.type || 'application/octet-stream')]: [extension] },
          }],
        } : {}),
      })
      const writable = await handle.createWritable()
      await writable.write(content)
      await writable.close()
      return true
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') return false
      throw error
    }
  }

  const url = URL.createObjectURL(content)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name
  anchor.click()
  URL.revokeObjectURL(url)
  return true
}

export async function saveFilesAsZip(name: string, files: Array<{ name: string; content: Blob }>): Promise<boolean> {
  const JSZip = (await import('jszip')).default
  const zip = new JSZip()
  const usedNames = new Set<string>()
  for (const file of files) {
    const original = file.name || '未命名文件'
    const extension = extensionOf(original)
    const stem = extension ? original.slice(0, -extension.length) : original
    let candidate = original
    let index = 1
    while (usedNames.has(candidate.toLowerCase())) candidate = `${stem}(${index++})${extension}`
    usedNames.add(candidate.toLowerCase())
    zip.file(candidate, file.content)
  }
  const bytes = await zip.generateAsync({ type: 'uint8array' })
  return saveDownload(name, new Blob([bytes.buffer as ArrayBuffer], { type: 'application/zip' }))
}
