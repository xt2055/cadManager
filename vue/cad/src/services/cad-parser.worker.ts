import { Dwg_File_Type, LibreDwg } from '@mlightcad/libredwg-web'
import { restoreCadDwgCustomBlocks } from './cad-custom-blocks'
import { normalizeCadDwgTextStyles } from './cad-text-styles'

self.onmessage = async (event: MessageEvent<{ id: string; input: ArrayBuffer }>) => {
  const { id, input } = event.data
  try {
    // WASM 与现有字体/Worker 资源一起离线部署，不请求外部 CDN。
    const wasmBase = import.meta.env.DEV
      ? new URL(import.meta.env.BASE_URL + 'assets/', self.location.origin)
      : new URL('./', import.meta.url)
    const lib = await LibreDwg.create(wasmBase.href)
    const ptr = lib.dwg_read_data(input, Dwg_File_Type.DWG)
    if (!ptr) throw new Error('无法读取 DWG 数据')
    try {
      const { database, stats } = lib.convertEx(ptr)
      normalizeCadDwgTextStyles(database)
      const restored = restoreCadDwgCustomBlocks(lib, ptr, database)
      self.postMessage({ id, success: true, data: { model: database, data: { unknownEntityCount: Math.max(0, stats.unknownEntityCount - restored) } } })
    } finally {
      lib.dwg_free(ptr)
    }
  } catch (error) {
    self.postMessage({ id, success: false, error: error instanceof Error ? error.message : String(error), errorCode: 'worker_error' })
  }
}
