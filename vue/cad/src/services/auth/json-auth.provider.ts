import { dataManager } from '@/services/data-manager'
import type { UserAccount } from '@/types/domain.types'
import type { AuthProvider, AuthResult, AuthUser, LoginRequest, StoredAuthSession } from '@/features/auth/types/auth.types'

function toAuthUser(user: UserAccount): AuthUser {
  return {
    id: user.id,
    account: user.account,
    displayName: user.displayName,
    roles: user.roles,
    status: user.status,
  }
}

function createToken(): string {
  return `cad-json-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

export class JsonAuthProvider implements AuthProvider {
  async login(request: LoginRequest): Promise<AuthResult> {
    const account = request.account.trim().toLowerCase()
    const document = await dataManager.load()
    const user = document.users.find((item) => item.account.toLowerCase() === account)

    if (!user || user.password !== request.password) {
      throw new Error('账号或密码错误')
    }
    if (user.status !== 'active') {
      throw new Error('账号已被禁用，请联系管理员')
    }

    user.lastLoginAt = new Date().toISOString()
    await dataManager.save(document)

    return {
      token: createToken(),
      user: toAuthUser(user),
    }
  }

  async restore(session: StoredAuthSession): Promise<AuthUser | null> {
    const document = await dataManager.load()
    const user = document.users.find((item) => item.id === session.userId)
    if (!user || user.status !== 'active') return null
    return toAuthUser(user)
  }
}
