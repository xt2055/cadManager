import type { UserManagementInput } from '@/types/application.types'
import type { UserAccount, UserRole } from '@/types/domain.types'

export const USER_ROLE_OPTIONS: ReadonlyArray<{ value: UserRole; label: string }> = [
  { value: 'designer', label: '设计人员' },
  { value: 'reviewer', label: '审核人员' },
  { value: 'admin', label: '管理员' },
]

export function buildUserRoleUpdate(
  user: Pick<UserAccount, 'account' | 'displayName' | 'status'>,
  roles: UserRole[],
): UserManagementInput {
  if (!roles.length) throw new Error('请至少选择一个身份')
  return {
    account: user.account,
    displayName: user.displayName,
    roles: [...roles],
    status: user.status,
  }
}
