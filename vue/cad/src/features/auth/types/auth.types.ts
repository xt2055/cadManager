import type { UserRole, UserStatus } from '@/types/domain.types'

export interface AuthUser {
  id: string
  account: string
  displayName: string
  roles: UserRole[]
  status: UserStatus
}

export interface LoginRequest {
  account: string
  password: string
  rememberMe: boolean
}

export interface AuthResult {
  token: string
  user: AuthUser
}

export interface StoredAuthSession {
  token: string
  userId: string
}

export interface AuthProvider {
  login(request: LoginRequest): Promise<AuthResult>
  restore(session: StoredAuthSession): Promise<AuthUser | null>
}
