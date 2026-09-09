import type { ReplaceDrawingBomInput } from '@/types/application.types'

export function drawingBomRequestBody(input: ReplaceDrawingBomInput) {
  return {
    expectedRevision: input.expectedRevision,
    items: input.items.map((item, index) => ({
      no: item.no || index + 1,
      id: item.id,
      name: item.name,
      spec: item.spec,
      qty: item.qty,
      weight: item.weight,
      remark: item.remark,
      ...(item.sourceFileId ? { sourceAttachmentVersionId: item.sourceFileId } : {}),
    })),
  }
}
