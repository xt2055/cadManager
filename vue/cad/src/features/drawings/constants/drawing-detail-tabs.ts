import { RouteName } from '@/router/route-names'
import type { DrawingDetailTabItem } from '@/constants/drawing.constants'

const DrawingDocuments = 'drawing-documents'
const DrawingLifecycle = 'drawing-lifecycle'

export const drawingDetailTabs: DrawingDetailTabItem[] = [
  { key: 'preview', title: '图纸文件', icon: 'folder', routeName: RouteName.DrawingPreview, group: '图纸内容' },
  { key: 'models', title: '3D 图纸', icon: 'box', routeName: RouteName.DrawingModels, group: '图纸内容' },
  { key: 'structure', title: '图纸结构', icon: 'folder-tree', routeName: RouteName.DrawingStructure, group: '图纸内容' },
  { key: 'changes', title: '变更工单', icon: 'folder-lock', routeName: RouteName.DrawingChanges, group: '变更与审核' },
  { key: 'review', title: '审核流程', icon: 'stamp', routeName: RouteName.DrawingReview, group: '变更与审核' },
  { key: 'versions', title: '版本与分支', icon: 'git-branch', routeName: RouteName.DrawingVersions, group: '变更与审核' },
  { key: 'material', title: '备料表', icon: 'file-spreadsheet', routeName: RouteName.DrawingMaterial, group: '生产资料' },
  { key: 'process', title: '工艺文件', icon: 'file-text', routeName: RouteName.DrawingProcess, group: '生产资料' },
  { key: 'documents', title: '资料档案', icon: 'archive', routeName: DrawingDocuments, group: '档案与记录' },
  { key: 'lifecycle', title: '生命周期', icon: 'activity', routeName: DrawingLifecycle, group: '档案与记录' },
  { key: 'borrow', title: '借用记录', icon: 'share-2', routeName: RouteName.DrawingBorrow, group: '档案与记录' },
  { key: 'properties', title: '属性与记录', icon: 'info', routeName: RouteName.DrawingProperties, group: '档案与记录' },
]
