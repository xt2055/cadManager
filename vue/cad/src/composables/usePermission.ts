export function usePermission() {
  function hasPermission(_permission: string): boolean {
    return true
  }

  return { hasPermission }
}
