export function paginationPages(
  page: number,
  totalPages: number,
  maxVisible = 5,
): number[] {
  if (totalPages < 1 || maxVisible < 1) return []

  const current = Math.min(Math.max(1, page), totalPages)
  const visible = Math.min(maxVisible, totalPages)
  const start = Math.min(
    Math.max(1, current - Math.floor(visible / 2)),
    totalPages - visible + 1,
  )

  return Array.from({ length: visible }, (_, index) => start + index)
}

export interface SelectOption {
  value: string
  label: string
}

export function mergeSelectOptions(
  options: SelectOption[],
  selectedValue: string,
): SelectOption[] {
  const selected = options.find((option) => option.value === selectedValue)
    ?? (selectedValue ? { value: selectedValue, label: selectedValue } : undefined)

  return [...new Map([
    ...options,
    ...(selected ? [selected] : []),
  ].map((option) => [option.value, option])).values()]
}

export function paginationRange(
  page: number,
  pageSize: number,
  total: number,
): string {
  if (total < 1) return '0'

  const start = (page - 1) * pageSize + 1
  return `${start}–${Math.min(page * pageSize, total)}`
}
