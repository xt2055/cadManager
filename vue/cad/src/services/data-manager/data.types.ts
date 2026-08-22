import type {
  ActivityLog,
  AdminLog,
  AdminUser,
  BomItem,
  Branch,
  BorrowRecord,
  CraftFile,
  Drawing,
  DrawingVersion,
  HiddenObject,
  MyReview,
  ReviewFlow,
  ReviewNode,
  StructurePart,
} from '@/types/demo.types'

export interface DataDocument {
  version: 1
  drawings: Drawing[]
  structure: StructurePart[]
  versions: DrawingVersion[]
  branches: Branch[]
  borrows: BorrowRecord[]
  bom: BomItem[]
  crafts: CraftFile[]
  logs: ActivityLog[]
  reviewNodes: ReviewNode[]
  myReviews: MyReview[]
  users: AdminUser[]
  flows: ReviewFlow[]
  hiddenList: HiddenObject[]
  adminLogs: AdminLog[]
}

export function createEmptyDataDocument(): DataDocument {
  return {
    version: 1,
    drawings: [],
    structure: [],
    versions: [],
    branches: [],
    borrows: [],
    bom: [],
    crafts: [],
    logs: [],
    reviewNodes: [],
    myReviews: [],
    users: [],
    flows: [],
    hiddenList: [],
    adminLogs: [],
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function readArray<T>(value: unknown): T[] {
  return Array.isArray(value) ? value as T[] : []
}

export function normalizeDataDocument(value: unknown): DataDocument {
  const source = isRecord(value) ? value : {}

  return {
    version: 1,
    drawings: readArray<Drawing>(source.drawings),
    structure: readArray<StructurePart>(source.structure),
    versions: readArray<DrawingVersion>(source.versions),
    branches: readArray<Branch>(source.branches),
    borrows: readArray<BorrowRecord>(source.borrows),
    bom: readArray<BomItem>(source.bom),
    crafts: readArray<CraftFile>(source.crafts),
    logs: readArray<ActivityLog>(source.logs),
    reviewNodes: readArray<ReviewNode>(source.reviewNodes),
    myReviews: readArray<MyReview>(source.myReviews),
    users: readArray<AdminUser>(source.users),
    flows: readArray<ReviewFlow>(source.flows),
    hiddenList: readArray<HiddenObject>(source.hiddenList),
    adminLogs: readArray<AdminLog>(source.adminLogs),
  }
}
