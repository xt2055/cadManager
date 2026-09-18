import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { DrawingStatus } from '@/types/domain.types'

function initialState() {
  return {
    query: '', status: '' as DrawingStatus | '', media: '',
    mode: 'drawing' as 'drawing' | 'part',
    attributeFilters: {} as Record<string, string>,
    expandedProjects: new Set<string>(),
    scrollTop: 0, tableScrollLeft: 0,
  }
}

/** 返回详情前的检索位置；换账号时丢弃上一账号的视图。 */
export const useDrawingLibraryUiStore = defineStore('drawing-library-ui', () => {
  const owner = ref('')
  const state = ref(initialState())
  function forUser(userId: string) {
    if (owner.value !== userId) {
      owner.value = userId
      state.value = initialState()
    }
    return state.value
  }
  return { state, forUser }
})
