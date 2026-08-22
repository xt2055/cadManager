export const RouteName = {
  Login: 'login',

  Dashboard: 'dashboard',

  DrawingLibrary: 'drawing-library',
  DrawingCreate: 'drawing-create',
  DrawingDetail: 'drawing-detail',

  DrawingPreview: 'drawing-preview',
  DrawingStructure: 'drawing-structure',
  DrawingVersions: 'drawing-versions',
  DrawingBorrow: 'drawing-borrow',
  DrawingMaterial: 'drawing-material',
  DrawingProcess: 'drawing-process',
  DrawingReview: 'drawing-review',
  DrawingProperties: 'drawing-properties',

  ReviewCenter: 'review-center',
  ReviewPending: 'review-pending',
  ReviewCompleted: 'review-completed',

  OperationLogs: 'operation-logs',

  Settings: 'settings',

  Admin: 'admin',
  AdminAccounts: 'admin-accounts',
  AdminReviewFlows: 'admin-review-flows',
  AdminDrawingControl: 'admin-drawing-control',
  AdminLogs: 'admin-logs',
} as const

export type RouteNameKey = (typeof RouteName)[keyof typeof RouteName]
