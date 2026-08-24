import { useAuthStore } from '@/stores/auth.store'

export function usePermission() {
  const authStore = useAuthStore()

  function hasPermission(permission: string): boolean {
    return authStore.hasPermission(permission)
  }

  return { hasPermission }
}
