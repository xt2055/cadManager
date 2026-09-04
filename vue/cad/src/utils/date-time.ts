/** 将接口返回的 ISO 时间转换为适合界面展示的本地时间。 */
export function formatReadableDateTime(value: string | undefined, fallback = '—'): string {
  const text = value?.trim()
  if (!text) return fallback
  const date = new Date(text)
  if (Number.isNaN(date.getTime())) return text

  const parts = new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).formatToParts(date)
  const get = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value || ''
  return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}:${get('second')}`
}
