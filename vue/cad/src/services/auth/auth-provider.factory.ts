import { ApiAuthProvider } from './api-auth.provider'
import { JsonAuthProvider } from './json-auth.provider'
import { readDebugMode } from '@/services/runtime-config.service'
import type { AuthProvider } from '@/features/auth/types/auth.types'

export async function createAuthProvider(): Promise<AuthProvider> {
  const configuredProvider = import.meta.env.VITE_DATA_PROVIDER
  if (configuredProvider === 'json') return new JsonAuthProvider()
  if (configuredProvider === 'api') return new ApiAuthProvider()
  return (await readDebugMode()) ? new JsonAuthProvider() : new ApiAuthProvider()
}
