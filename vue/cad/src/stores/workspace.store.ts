import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { DrawingId, PartId } from '@/modules/drawing'

export type WorkspaceSelection =
  | { type: 'drawing'; id: DrawingId }
  | { type: 'part'; id: PartId }

/** 只保存当前工作区的身份和展示偏好，不保存完整领域对象。 */
export const useWorkspaceStore = defineStore('workspace', () => {
  const selection = ref<WorkspaceSelection | null>(null)
  const treeOpen = ref(true)
  const selectedStructureIndex = ref(0)

  function selectDrawing(id: DrawingId) {
    selection.value = { type: 'drawing', id }
    selectedStructureIndex.value = 0
  }

  function selectPart(id: PartId) {
    selection.value = { type: 'part', id }
  }

  function clearSelection() {
    selection.value = null
    selectedStructureIndex.value = 0
  }

  function setTreeOpen(value: boolean) { treeOpen.value = value }
  function toggleTree() { treeOpen.value = !treeOpen.value }
  function setSelectedStructureIndex(value: number) { selectedStructureIndex.value = Math.max(0, value) }

  return {
    selection,
    treeOpen,
    selectedStructureIndex,
    selectDrawing,
    selectPart,
    clearSelection,
    setTreeOpen,
    toggleTree,
    setSelectedStructureIndex,
  }
})
