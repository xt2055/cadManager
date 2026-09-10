export function toggleFileSelection(selected: Set<string>, id: string): Set<string> {
  const next = new Set(selected)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  return next
}

export function selectAllFiles(ids: string[], selected: Set<string>): Set<string> {
  return ids.length > 0 && ids.every((id) => selected.has(id)) ? new Set() : new Set(ids)
}

export function selectedFiles<T extends { id: string }>(files: T[], selected: Set<string>): T[] {
  return files.filter((file) => selected.has(file.id))
}
