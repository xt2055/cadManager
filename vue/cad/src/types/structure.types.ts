import type { StructurePart } from './domain.types'

export interface StructureTreeNode {
  part: StructurePart
  children: StructureTreeNode[]
}
