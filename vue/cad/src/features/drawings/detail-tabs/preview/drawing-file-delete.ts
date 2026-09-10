export function canDeleteDrawingFiles(input: {
  status: string
  creator?: string
  userName?: string
  admin: boolean
}): boolean {
  if (input.status === 'archived') return false
  return input.admin || Boolean(input.creator && input.creator === input.userName)
}
