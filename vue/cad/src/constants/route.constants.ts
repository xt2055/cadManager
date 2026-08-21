export interface MainNavigationItem {
  key: string
  title: string
  icon: string
  routeName: string
}

export const mainNavigation: MainNavigationItem[] = [
  {
    key: 'dashboard',
    title: '工作台',
    icon: 'layout-dashboard',
    routeName: 'dashboard',
  },
  {
    key: 'drawing-library',
    title: '图纸库',
    icon: 'search',
    routeName: 'drawing-library',
  },
  {
    key: 'reviews',
    title: '图纸审核',
    icon: 'clipboard-check',
    routeName: 'review-pending',
  },
  {
    key: 'operation-logs',
    title: '操作记录',
    icon: 'history',
    routeName: 'operation-logs',
  },
  {
    key: 'admin',
    title: '后台管理',
    icon: 'shield',
    routeName: 'admin-accounts',
  },
]
