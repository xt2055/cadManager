import { RouteName } from '@/router/route-names'
import type { DrawingDetailTabItem } from '@/constants/drawing.constants'

export const drawingDetailTabs: DrawingDetailTabItem[] = [
  { key: 'models', title: '3D 图纸', icon: 'box', routeName: RouteName.DrawingModels },
  {
    key: 'preview',
    title: '图纸文件',
    icon: 'folder',
    routeName: RouteName.DrawingPreview,
  },
  {
    key: 'structure',
    title: '图纸结构',
    icon: 'folder-tree',
    routeName: RouteName.DrawingStructure,
  },
  {
    key: 'changes',
    title: '变更工单',
    icon: 'folder-lock',
    routeName: RouteName.DrawingChanges,
  },
  {
    key: 'versions',
    title: '版本与分支',
    icon: 'git-branch',
    routeName: RouteName.DrawingVersions,
  },
  {
    key: 'borrow',
    title: '借用记录',
    icon: 'share-2',
    routeName: RouteName.DrawingBorrow,
  },
  {
    key: 'material',
    title: '备料表',
    icon: 'file-spreadsheet',
    routeName: RouteName.DrawingMaterial,
  },
  {
    key: 'process',
    title: '工艺文件',
    icon: 'file-text',
    routeName: RouteName.DrawingProcess,
  },
  {
    key: 'review',
    title: '审核流程',
    icon: 'stamp',
    routeName: RouteName.DrawingReview,
  },
  {
    key: 'properties',
    title: '属性与记录',
    icon: 'info',
    routeName: RouteName.DrawingProperties,
  },
]
