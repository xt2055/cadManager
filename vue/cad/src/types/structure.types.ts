import type { PartView } from '@/modules/drawing'

export interface StructureTreeNode {
  part: PartView
  children: StructureTreeNode[]
}
