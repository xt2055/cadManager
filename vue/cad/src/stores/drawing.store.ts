import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { drawingQueryService, drawingReadModelMapper } from '@/app/container'
import type { DrawingReadModelSnapshot, DrawingSummaryView, PartView, StructureNodeView } from '@/modules/drawing'

function emptyModel(): DrawingReadModelSnapshot {
  return { drawings: [], parts: [], structureByDrawing: {}, bom: [] }
}

/** 图纸 Read Model 缓存：只保存查询结果和加载状态，不承载写事务。 */
export const useDrawingStore = defineStore('drawing-read-model', () => {
  const model = ref<DrawingReadModelSnapshot>(emptyModel())
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)
  let loadingPromise: Promise<void> | null = null

  const drawings = computed(() => model.value.drawings)
  const parts = computed(() => model.value.parts)
  const bom = computed(() => model.value.bom)

  async function load(): Promise<void> {
    if (loaded.value) return
    if (loadingPromise) return loadingPromise
    loading.value = true
    error.value = null
    loadingPromise = drawingQueryService.loadSnapshot()
      .then((snapshot) => {
        model.value = drawingReadModelMapper.map(snapshot)
        loaded.value = true
      })
      .catch((loadError: unknown) => {
        error.value = loadError instanceof Error ? loadError.message : String(loadError)
        throw loadError
      })
      .finally(() => {
        loading.value = false
        loadingPromise = null
      })
    return loadingPromise
  }

  async function refresh(): Promise<void> {
    loaded.value = false
    await load()
  }

  function invalidate() {
    loaded.value = false
    model.value = emptyModel()
  }

  function getDrawing(idOrNo: string): DrawingSummaryView | null {
    return drawings.value.find((drawing) => drawing.id === idOrNo || drawing.no === idOrNo) ?? null
  }

  function getPart(idOrNo: string): PartView | null {
    return parts.value.find((part) => part.id === idOrNo || part.no === idOrNo) ?? null
  }

  function getStructure(drawingNo: string): StructureNodeView[] {
    return model.value.structureByDrawing[drawingNo] ?? []
  }

  function getBom(drawingNo: string) {
    return bom.value.filter((item) => item.drawingNo === drawingNo)
  }

  return {
    drawings,
    parts,
    bom,
    loaded,
    loading,
    error,
    load,
    refresh,
    invalidate,
    getDrawing,
    getPart,
    getStructure,
    getBom,
  }
})
