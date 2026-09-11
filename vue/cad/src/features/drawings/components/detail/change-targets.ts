import { fileCategory, type FileCategory } from '../../../../utils/model-formats.ts'

interface TargetFileSource {
  id: string
  name: string
  fileCategory?: string
}

interface TargetOwnerSource {
  no: string
  name: string
  files: TargetFileSource[]
  otherFiles: TargetFileSource[]
}

interface TargetPartSource extends TargetOwnerSource {
  children: TargetPartSource[]
}

export interface ChangeTargetGroup {
  no: string
  name: string
  kind: '总图' | '零件图'
  files: Array<{
    id: string
    name: string
    category: FileCategory
  }>
}

function flattenParts(nodes: readonly TargetPartSource[]): TargetPartSource[] {
  return nodes.flatMap((node) => [node, ...flattenParts(node.children)])
}

function ownerFiles(owner: TargetOwnerSource): ChangeTargetGroup['files'] {
  return [...owner.files, ...owner.otherFiles]
    .filter((file, index, all) => all.findIndex((candidate) => candidate.id === file.id) === index)
    .map((file) => ({
      id: file.id,
      name: file.name,
      category: fileCategory(file),
    }))
}

export function buildChangeTargetGroups(
  drawing: TargetOwnerSource | null,
  structure: readonly TargetPartSource[],
): ChangeTargetGroup[] {
  if (!drawing) return []
  const owners = [
    { owner: drawing, kind: '总图' as const },
    ...flattenParts(structure).map((owner) => ({ owner, kind: '零件图' as const })),
  ]
  return owners
    .map(({ owner, kind }) => ({ no: owner.no, name: owner.name, kind, files: ownerFiles(owner) }))
    .filter((group) => group.files.length > 0)
}
