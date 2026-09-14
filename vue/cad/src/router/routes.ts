import type { RouteRecordRaw } from 'vue-router'
import { RouteName } from './route-names'
import type { UserRole } from '@/types/domain.types'

export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: () => import('@/layouts/AuthLayout.vue'),
    children: [
      {
        path: '',
        name: RouteName.Login,
        component: () => import('@/features/auth/pages/LoginPage.vue'),
        meta: { requiresAuth: false },
      },
    ],
  },
  {
    path: '/cad-render-test',
    name: RouteName.CadRenderTest,
    component: () => import('@/features/drawings/pages/CadRenderTestPage.vue'),
    meta: { requiresAuth: false },
  },
  {
    path: '/',
    component: () => import('@/layouts/DesktopLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        redirect: '/dashboard',
      },
      { path: 'patents', name: 'patents', component: () => import('@/features/patents/PatentPage.vue') },
      {
        path: 'dashboard',
        name: RouteName.Dashboard,
        component: () => import('@/features/dashboard/pages/DashboardPage.vue'),
      },
      {
        path: 'drawings',
        name: RouteName.DrawingLibrary,
        component: () =>
          import('@/features/drawings/pages/DrawingLibraryPage.vue'),
      },
      {
        path: 'part-indexes',
        name: RouteName.PartIndexLibrary,
        component: () =>
          import('@/features/part-index/pages/PartIndexLibraryPage.vue'),
      },
      {
        path: 'part-indexes/:attachmentId',
        name: RouteName.PartIndexDetail,
        component: () =>
          import('@/features/part-index/pages/PartIndexDetailPage.vue'),
      },
      {
        path: 'drawings/create',
        name: RouteName.DrawingCreate,
        component: () =>
          import('@/features/drawings/pages/DrawingCreatePage.vue'),
      },
      {
        path: 'drawings/:drawingId/compare',
        name: RouteName.DrawingCompare,
        component: () => import('@/features/drawings/pages/DrawingComparePage.vue'),
      },
      {
        path: 'drawings/:drawingId/view',
        name: RouteName.DrawingViewer,
        component: () =>
          import('@/features/drawings/pages/DrawingViewerPage.vue'),
      },
      {
        path: 'drawings/:drawingId/file-history',
        name: RouteName.DrawingFileHistory,
        component: () =>
          import('@/features/drawings/pages/DrawingFileHistoryPage.vue'),
      },
      {
        path: 'drawings/:drawingId',
        component: () =>
          import('@/features/drawings/layouts/DrawingDetailLayout.vue'),
        children: [
          { path: 'documents', name: 'drawing-documents', component: () => import('@/features/drawings/pages/DrawingDocumentsPage.vue') },
          { path: 'lifecycle', name: 'drawing-lifecycle', component: () => import('@/features/drawings/pages/DrawingLifecyclePage.vue') },
          {
            path: 'changes',
            name: RouteName.DrawingChanges,
            component: () => import('@/features/drawings/pages/DrawingChangesPage.vue'),
          },
          {
            path: '',
            name: RouteName.DrawingDetail,
            redirect: { name: RouteName.DrawingPreview },
          },
          {
            path: 'preview',
            name: RouteName.DrawingPreview,
            component: () =>
              import('@/features/drawings/detail-tabs/preview/DrawingPreviewTab.vue'),
          },
          {
            path: 'structure',
            // Structure and 3D models share the existing drawing identity.
            name: RouteName.DrawingStructure,
            component: () =>
              import('@/features/drawings/detail-tabs/structure/DrawingStructureTab.vue'),
          },
          {
            path: 'models',
            name: RouteName.DrawingModels,
            component: () => import('@/features/drawings/detail-tabs/models/DrawingModelsTab.vue'),
          },
          {
            path: 'versions',
            name: RouteName.DrawingVersions,
            component: () =>
              import('@/features/drawings/detail-tabs/versions/DrawingVersionsTab.vue'),
          },
          {
            path: 'borrow',
            name: RouteName.DrawingBorrow,
            component: () =>
              import('@/features/drawings/detail-tabs/borrow/DrawingBorrowTab.vue'),
          },
          {
            path: 'material',
            name: RouteName.DrawingMaterial,
            component: () =>
              import('@/features/drawings/detail-tabs/material/DrawingMaterialTab.vue'),
          },
          {
            path: 'process',
            name: RouteName.DrawingProcess,
            component: () =>
              import('@/features/drawings/detail-tabs/process/DrawingProcessTab.vue'),
          },
          {
            path: 'review',
            name: RouteName.DrawingReview,
            component: () =>
              import('@/features/drawings/detail-tabs/review/DrawingReviewTab.vue'),
          },
          {
            path: 'properties',
            name: RouteName.DrawingProperties,
            component: () =>
              import('@/features/drawings/detail-tabs/properties/DrawingPropertiesTab.vue'),
          },
        ],
      },
      {
        path: 'reviews',
        component: () => import('@/features/reviews/pages/ReviewCenterPage.vue'),
        children: [
          {
            path: '',
            redirect: '/reviews/pending',
          },
          {
            path: 'pending',
            name: RouteName.ReviewPending,
            component: () => import('@/features/reviews/pages/ReviewPendingPage.vue'),
          },
          {
            path: 'task/:drawingNo',
            name: RouteName.ReviewWorkspace,
            component: () => import('@/features/reviews/pages/ReviewWorkspacePage.vue'),
          },
          {
            path: 'completed',
            name: RouteName.ReviewCompleted,
            component: () =>
              import('@/features/reviews/pages/ReviewCompletedPage.vue'),
          },
        ],
      },
      {
        path: 'operation-logs',
        name: RouteName.OperationLogs,
        component: () =>
          import('@/features/operation-logs/pages/OperationLogPage.vue'),
      },
      {
        path: 'settings',
        name: RouteName.Settings,
        component: () => import('@/features/settings/pages/SettingsPage.vue'),
      },
    ],
  },
  {
    path: '/drawings/:drawingId/edit',
    name: RouteName.DrawingEditor,
    component: () =>
      import('@/features/drawings/pages/DrawingEditorPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/admin',
    component: () => import('@/features/admin/layouts/AdminLayout.vue'),
    meta: { requiresAuth: true, roles: ['admin'] satisfies UserRole[], permission: 'admin.access' },
    children: [
      {
        path: '',
        redirect: '/admin/accounts',
      },
      {
        path: 'accounts',
        name: RouteName.AdminAccounts,
        component: () => import('@/features/admin/pages/AccountManagementPage.vue'),
      },
      {
        path: 'review-flows',
        name: RouteName.AdminReviewFlows,
        component: () => import('@/features/admin/pages/ReviewFlowManagementPage.vue'),
      },
      {
        path: 'drawings',
        name: RouteName.AdminDrawings,
        component: () => import('@/features/admin/pages/AdminDrawingsPage.vue'),
      },
      {
        path: 'change-approvals',
        name: RouteName.AdminChangeApprovals,
        component: () => import('@/features/admin/pages/AdminChangeApprovalsPage.vue'),
      },
      {
        path: 'change-records',
        name: RouteName.AdminChangeRecords,
        component: () => import('@/features/admin/pages/AdminChangeRecordsPage.vue'),
      },
      {
        path: 'logs',
        name: RouteName.AdminLogs,
        component: () => import('@/features/admin/pages/AdminOperationLogPage.vue'),
      },
      {
        path: 'system-logs',
        name: RouteName.AdminSystemLogs,
        component: () => import('@/features/admin/pages/SystemLogPage.vue'),
      },
      {
        path: 'updates',
        name: RouteName.AdminUpdates,
        component: () => import('@/features/admin/pages/UpdateManagementPage.vue'),
      },
      {
        path: 'attributes',
        name: RouteName.AdminAttributes,
        component: () => import('@/features/admin/pages/AttributeManagementPage.vue'),
      },
      {
        path: 'conversions',
        name: RouteName.AdminConversions,
        component: () => import('@/features/admin/pages/ConversionQueuePage.vue'),
      },
    ],
  },
]
