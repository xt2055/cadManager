export interface PrintableMaterialFile {
  name: string
  storageKey?: string
}

export function originalMaterialWorkbook<T extends PrintableMaterialFile>(files: T[]): T | undefined {
  return files.find((file) => Boolean(file.storageKey) && /\.xlsx$/i.test(file.name))
}

export function hasGeneratedBomIds(items: Array<{ id: string }>): boolean {
  return items.some((item) => /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(item.id))
}
