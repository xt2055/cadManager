/**
 * 预览页展示用的纯格式化函数。
 *
 * 说明：仓库里 drawing-operations.store / drawing-read-model / DrawingCreatePage 等
 * 各自带有一份同名实现，本模块只服务 preview 特性，不做全库收敛，避免顺手改动无关模块。
 */

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function formatCurrentTime(): string {
  const current = new Date()
  const year = current.getFullYear()
  const month = String(current.getMonth() + 1).padStart(2, '0')
  const day = String(current.getDate()).padStart(2, '0')
  const hours = String(current.getHours()).padStart(2, '0')
  const minutes = String(current.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}`
}
