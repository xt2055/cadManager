/** 业务层统一错误基类，页面可据此决定提示文案或刷新策略。 */
export class DomainError extends Error {
  public constructor(message: string, public readonly code: string) {
    super(message)
    this.name = 'DomainError'
  }
}

export class RevisionConflictError extends DomainError {
  public constructor(message = '数据已被其他用户修改，请刷新后重试') { super(message, 'REVISION_CONFLICT') }
}

export class PartNoConflictError extends DomainError {
  public constructor(message = '零件编号已存在') { super(message, 'PART_NO_CONFLICT') }
}

export class BorrowedPartRequiresForkError extends DomainError {
  public constructor(message = '借用件不能直接修改，请先分叉') { super(message, 'BORROWED_PART_REQUIRES_FORK') }
}

export class PartObsoleteError extends DomainError {
  public constructor(message = '零件已废止，不能继续使用') { super(message, 'PART_OBSOLETE') }
}

export class StructureCycleError extends DomainError {
  public constructor(message = '结构关系不能形成环') { super(message, 'STRUCTURE_CYCLE') }
}

export class UploadNotReadyError extends DomainError {
  public constructor(message = '文件尚未完成处理，暂时不能提交') { super(message, 'UPLOAD_NOT_READY') }
}
