export type DrawingStatus = 'published' | 'reviewing' | 'draft' | 'hidden' | 'disabled'

export type ActivityType = 'view' | 'edit' | 'branch' | 'borrow' | 'check' | 'back'

export interface Drawing {
  no: string
  name: string
  kind: '总图' | '零件图'
  project: string
  material: string
  vendor: string
  status: DrawingStatus
  ver: string
  updated: string
  by: string
  borrow: number
  hasFile: boolean
  borrowFrom?: string
}

export interface ActivityLog {
  user: string
  act: ActivityType
  txt: string
  time: string
}

export interface StructurePart {
  no: string
  name: string
  material: string
  qty: number
  status: DrawingStatus
  ver: string
  hasFile: boolean
  borrowFrom?: string
}

export interface DrawingVersion {
  v: string
  by: string
  date: string
  note: string
  cur?: boolean
}

export interface Branch {
  name: string
  from: string
  by: string
  date: string
  status: '使用中' | '已禁用'
  desc: string
}

export interface BorrowRecord {
  dir: 'in' | 'out'
  project: string
  part: string
  user: string
  date: string
  sync: string
  status: '使用中' | '已归档'
}

export interface BomItem {
  no: number
  id: string
  name: string
  spec: string
  qty: number
  weight: number
  remark: string
}

export interface CraftFile {
  name: string
  op: string
  ver: string
  by: string
  date: string
  size: string
}

export interface ReviewNode {
  name: string
  user: string
  status: 'pass' | 'pending'
  time: string
  opinion: string
}

export interface ReviewData {
  flow: string
  nodes: ReviewNode[]
}

export interface MyReview {
  no: string
  name: string
  node: string
  by: string
  time: string
}

export interface AdminUser {
  acc: string
  name: string
  role: string
  status: '正常' | '已禁用'
  last: string
}

export interface ReviewFlow {
  name: string
  nodes: number
  on: boolean
  desc: string
}

export interface HiddenObject {
  no: string
  name: string
  op: string
  by: string
  date: string
}

export interface AdminLog {
  time: string
  user: string
  act: string
  obj: string
}
