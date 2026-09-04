import { defineStore } from 'pinia'
import { ref } from 'vue'
import { drawingRelationQueryService } from '@/app/container'
import type { Branch, BorrowRecord } from '@/types/domain.types'

/** 图纸分支与借用关系 Read Model；页面不再从旧 Domain Store 读取关系集合。 */
export const useDrawingRelationsStore = defineStore('drawing-relations', () => {
  const branches = ref<Branch[]>([])
  const borrows = ref<BorrowRecord[]>([])
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)
  let loadingPromise: Promise<void> | null = null

  async function load(): Promise<void> {
    if (loaded.value) return
    if (loadingPromise) return loadingPromise
    loading.value = true
    error.value = null
    loadingPromise = Promise.all([
      drawingRelationQueryService.listBranches(),
      drawingRelationQueryService.listBorrows(),
    ])
      .then(([nextBranches, nextBorrows]) => {
        branches.value = nextBranches
        borrows.value = nextBorrows
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

  function invalidate() {
    loaded.value = false
    branches.value = []
    borrows.value = []
    error.value = null
  }

  return { branches, borrows, loaded, loading, error, load, invalidate }
})
