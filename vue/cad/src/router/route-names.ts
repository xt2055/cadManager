export const RouteName = {
  Login: 'login',

  Dashboard: 'dashboard',

  DrawingLibrary: 'drawing-library',
  DrawingCreate: 'drawing-create',
  DrawingDetail: 'drawing-detail',
  DrawingViewer: 'drawing-viewer',
  DrawingCompare: 'drawing-compare',
  DrawingEditor: 'drawing-editor',
  DrawingFileHistory: 'drawing-file-history',
  CadRenderTest: 'cad-render-test',
  PartIndexLibrary: 'part-index-list',
  PartIndexDetail: 'part-index-detail',

  DrawingPreview: 'drawing-preview',
  DrawingChanges: 'drawing-changes',
  DrawingModels: 'drawing-models',
  DrawingStructure: 'drawing-structure',
  DrawingVersions: 'drawing-versions',
  DrawingBorrow: 'drawing-borrow',
  DrawingMaterial: 'drawing-material',
  DrawingProcess: 'drawing-process',
  DrawingReview: 'drawing-review',
  DrawingProperties: 'drawing-properties',

  ReviewCenter: 'review-center',
  ReviewPending: 'review-pending',
  ReviewWorkspace: 'review-workspace',
  ReviewCompleted: 'review-completed',

  TaskBoard: 'task-board',

  OperationLogs: 'operation-logs',

  Settings: 'settings',

  Admin: 'admin',
  AdminAccounts: 'admin-accounts',
  AdminReviewFlows: 'admin-review-flows',
  AdminDrawings: 'admin-drawings',
  AdminChangeApprovals: 'admin-change-approvals',
  AdminChangeRecords: 'admin-change-records',
  AdminLogs: 'admin-logs',
  AdminSystemLogs: 'admin-system-logs',
  AdminUpdates: 'admin-updates',
  AdminAttributes: 'admin-attributes',
  AdminConversions: 'admin-conversions',
  AdminNotifications: 'admin-notifications',
} as const

export type RouteNameKey = (typeof RouteName)[keyof typeof RouteName]
