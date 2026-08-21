import type { RouteRecordRaw } from 'vue-router'
import { RouteName } from './route-names'

export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: () => import('@/layouts/AuthLayout.vue'),
    children: [
      {
        path: '',
        name: RouteName.Login,
        component: () => import('@/features/auth/pages/LoginPage.vue'),
      },
    ],
  },
  {
    path: '/',
    component: () => import('@/layouts/DesktopLayout.vue'),
    children: [
      {
        path: '',
        redirect: '/dashboard',
      },
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
        path: 'drawings/create',
        name: RouteName.DrawingCreate,
        component: () =>
          import('@/features/drawings/pages/DrawingCreatePage.vue'),
      },
      {
        path: 'drawings/:drawingId',
        component: () =>
          import('@/features/drawings/layouts/DrawingDetailLayout.vue'),
        children: [
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
            name: RouteName.DrawingStructure,
            component: () =>
              import('@/features/drawings/detail-tabs/structure/DrawingStructureTab.vue'),
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
        path: 'admin',
        component: () => import('@/features/admin/layouts/AdminLayout.vue'),
        children: [
          {
            path: '',
            redirect: '/admin/accounts',
          },
          {
            path: 'accounts',
            name: RouteName.AdminAccounts,
            component: () =>
              import('@/features/admin/pages/AccountManagementPage.vue'),
          },
          {
            path: 'review-flows',
            name: RouteName.AdminReviewFlows,
            component: () =>
              import('@/features/admin/pages/ReviewFlowManagementPage.vue'),
          },
          {
            path: 'drawing-control',
            name: RouteName.AdminDrawingControl,
            component: () =>
              import('@/features/admin/pages/DrawingControlPage.vue'),
          },
          {
            path: 'logs',
            name: RouteName.AdminLogs,
            component: () =>
              import('@/features/admin/pages/AdminOperationLogPage.vue'),
          },
        ],
      },
    ],
  },
]
