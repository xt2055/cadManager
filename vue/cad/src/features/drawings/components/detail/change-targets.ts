import { fileCategory, type FileCategory } from '../../../../utils/model-formats.ts'

interface TargetFileSource {
  id: string
  name: string
  fileCategory?: string
  role?: string
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

/** 文件类别在上传创建时写入 attachments.file_role，变更清单直接沿用，不再按所属对象推断。 */
export type ChangeTargetRole = 'assembly' | 'part' | 'other'

export interface ChangeTargetGroup {
  no: string
  name: string
  kind: '总图' | '零件图'
  files: Array<{
    id: string
    name: string
    category: FileCategory
    role: ChangeTargetRole
  }>
}

export function changeTargetRoleLabel(role: ChangeTargetRole): string {
  if (role === 'assembly') return '总图'
  if (role === 'part') return '零件图'
  return '其他文件'
}

function changeTargetRole(role: string | undefined): ChangeTargetRole {
  return role === 'assembly' || role === 'part' ? role : 'other'
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
      role: changeTargetRole(file.role),
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
