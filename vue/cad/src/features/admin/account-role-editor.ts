import type { UserManagementInput } from '@/types/application.types'
import type { UserAccount, UserRole } from '@/types/domain.types'

// 顺序即职责顺序：计划员建档并派活，设计人员接活编制，审核人员签署，管理员兜底。
export const USER_ROLE_OPTIONS: ReadonlyArray<{ value: UserRole; label: string; description: string }> = [
  { value: 'planner', label: '计划员', description: '创建图纸并把图纸指派给负责人' },
  { value: 'designer', label: '设计人员', description: '在被指派后编制图纸并送审' },
  { value: 'reviewer', label: '审核人员', description: '处理待审任务并签署结论' },
  { value: 'admin', label: '管理员', description: '账号、流程、资产与系统管理' },
]

export function buildUserRoleUpdate(
  user: Pick<UserAccount, 'account' | 'displayName' | 'status'>,
  roles: UserRole[],
  changes: { account?: string; displayName?: string } = {},
): UserManagementInput {
  if (!roles.length) throw new Error('请至少选择一个身份')
  const account = (changes.account ?? user.account).trim()
  if (!account) throw new Error('登录账号不能为空')
  const displayName = (changes.displayName ?? user.displayName).trim()
  if (!displayName) throw new Error('姓名不能为空')
  return {
    account,
    displayName,
    roles: [...roles],
    status: user.status,
  }
}
