import { ApiAuthProvider } from './api-auth.provider'
import type { AuthProvider } from '@/features/auth/types/auth.types'

export function createAuthProvider(): AuthProvider {
  return new ApiAuthProvider()
}
