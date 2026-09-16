import { lifecycleApi } from './lifecycle.service'
import type { AnnotationContent, AnnotationDocument, AnnotationTemplate, AnnotationWorkspace } from '@/features/reviews/annotation-model'

export interface ReviewAnnotationFile { attachmentId: string; versionId: string; name: string; version: string; markCount: number }

export const reviewAnnotationService = {
  files(caseId: string) { return lifecycleApi<ReviewAnnotationFile[]>(`/review-annotation-files?caseId=${encodeURIComponent(caseId)}`) },
  load(caseId: string, attachmentId: string) {
    return lifecycleApi<AnnotationWorkspace>(`/review-annotations?caseId=${encodeURIComponent(caseId)}&attachmentId=${encodeURIComponent(attachmentId)}`)
  },
  save(scope: AnnotationWorkspace, revision: number, content: AnnotationContent) {
    return lifecycleApi<AnnotationDocument>('/review-annotations', { method: 'PUT', body: JSON.stringify({
      caseId: scope.caseId, attachmentId: scope.attachmentId, versionId: scope.versionId, nodeId: scope.nodeId, revision, content,
    }) })
  },
  templates() { return lifecycleApi<AnnotationTemplate[]>('/review-annotation-templates') },
  createTemplate(text: string, category: string, ownerId: string) {
    return lifecycleApi<AnnotationTemplate>('/review-annotation-templates', { method: 'POST', body: JSON.stringify({ text, category, ownerId }) })
  },
  deleteTemplate(id: string) { return lifecycleApi(`/review-annotation-templates?id=${encodeURIComponent(id)}`, { method: 'DELETE' }) },
  updateTemplate(template: AnnotationTemplate) { return lifecycleApi<AnnotationTemplate>('/review-annotation-templates', { method: 'PUT', body: JSON.stringify(template) }) },
}
