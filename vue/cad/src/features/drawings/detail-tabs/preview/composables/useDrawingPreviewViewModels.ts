import { computed, type Ref } from 'vue'

import type { DrawingSummaryView, PartView } from '@/modules/drawing'
import { versionDisplayLabel } from '@/modules/versioning/versioning-service'
import type { DownloadCandidate, DownloadFormat } from '@/services/drawing-batch-download.service'
import type { DrawingFile } from '@/types/domain.types'
import { formatFileSize } from '../drawing-preview-format'
import type { LocalActiveEditSession } from './useDrawingEditSessions'
import type { ReidentifyItem } from './useDrawingReidentify'

interface ClosedEditSessionRecord {
  sessionId: string
  fileName: string
  savedAt: string
}

interface UseDrawingPreviewViewModelsOptions {
  borrow: {
    candidateProjects: Ref<DrawingSummaryView[]>
    selectedProjectDetail: Ref<DrawingSummaryView | null>
    candidateParts: Ref<PartView[]>
    selectedPartDetail: Ref<PartView | null>
    getProjectPartCount: (projectNo: string) => number
    getPartProjectName: (part: PartView) => string
  }
  sessions: {
    projectActiveSessions: Ref<LocalActiveEditSession[]>
    closingSessionIds: Ref<Set<string>>
    closedSessions: Ref<ClosedEditSessionRecord[]>
    isHeartbeatFresh: (session: LocalActiveEditSession) => boolean
  }
  download: {
    candidates: Ref<DownloadCandidate[]>
    format: Ref<DownloadFormat>
    selectedIds: Ref<Set<string>>
    hasFormat: (candidate: DownloadCandidate, format: DownloadFormat) => boolean
  }
  reidentify: {
    list: Ref<ReidentifyItem[]>
  }
  replacement: {
    targetFile: Ref<DrawingFile | null>
    selectedBlob: Ref<File | null>
  }
}

/**
 * 子组件视图数据投影：把各 composable 的状态映射成子组件 props 需要的纯展示数据。
 *
 * 页面只负责装配（composable → 投影 → 组件），投影规则集中在这里：
 * 子组件因此不需要读 store、不需要 import 格式化函数，也拿不到可写的领域状态。
 * 本 composable 不持有状态、不调服务，只是一个纯映射层。
 */
export function useDrawingPreviewViewModels(options: UseDrawingPreviewViewModelsOptions) {
  function borrowPartCard(part: PartView) {
    return {
      no: part.no,
      name: part.name,
      parentNo: part.parentNo ?? '',
      partType: part.partType ?? '',
      material: part.material ?? '',
      spec: part.spec ?? '',
      weight: part.weight ?? 0,
      qty: part.qty ?? 0,
      surfaceTreatment: part.surfaceTreatment ?? '',
      fileNames: (part.files ?? []).map((file) => file.name),
      projectName: options.borrow.getPartProjectName(part),
    }
  }

  const borrowProjectCards = computed(() =>
    options.borrow.candidateProjects.value.map((project) => ({
      no: project.no,
      name: project.name,
      partCount: options.borrow.getProjectPartCount(project.no),
    })),
  )

  const borrowPartCards = computed(() => options.borrow.candidateParts.value.map(borrowPartCard))

  const borrowSelectedProject = computed(() => {
    const project = options.borrow.selectedProjectDetail.value
    return project ? { no: project.no, name: project.name } : null
  })

  const borrowSelectedPart = computed(() =>
    options.borrow.selectedPartDetail.value ? borrowPartCard(options.borrow.selectedPartDetail.value) : null,
  )

  const sessionRows = computed(() =>
    options.sessions.projectActiveSessions.value.map((session) => ({
      sessionId: session.sessionId,
      fileName: session.fileName,
      startedAt: session.startedAt,
      uncPath: session.uncPath,
      heartbeatFresh: options.sessions.isHeartbeatFresh(session),
      closing: options.sessions.closingSessionIds.value.has(session.sessionId),
    })),
  )

  const closedSessionRows = computed(() =>
    options.sessions.closedSessions.value.map((session) => ({
      sessionId: session.sessionId,
      fileName: session.fileName,
      savedAt: session.savedAt,
    })),
  )

  const downloadRows = computed(() =>
    options.download.candidates.value.map((candidate) => ({
      id: candidate.file.id,
      name: candidate.file.name,
      sizeLabel: candidate.file.size,
      available: options.download.hasFormat(candidate, options.download.format.value),
      checked: options.download.selectedIds.value.has(candidate.file.id),
    })),
  )

  const reidentifyRows = computed(() =>
    options.reidentify.list.value.map((item) => ({
      id: item.file.id,
      name: item.file.name,
      oldPartNo: item.oldPartNo,
      newPartNo: item.newPartNo,
      checked: item.checked,
    })),
  )

  const replaceOriginalVersion = computed(() =>
    options.replacement.targetFile.value ? versionDisplayLabel(options.replacement.targetFile.value.version) : 'v1.0',
  )

  const replaceNewSize = computed(() => formatFileSize(options.replacement.selectedBlob.value?.size || 0))

  return {
    borrowProjectCards,
    borrowPartCards,
    borrowSelectedProject,
    borrowSelectedPart,
    sessionRows,
    closedSessionRows,
    downloadRows,
    reidentifyRows,
    replaceOriginalVersion,
    replaceNewSize,
  }
}
