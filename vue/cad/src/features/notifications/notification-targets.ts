import type { NotificationItem } from '@/services/notification.service'

/**
 * 待办审核由后端在通知上标记 target='review-workspace'，
 * 这类提醒要直达审核工作台，而不是跳到图纸详情的审核页。
 */
export function opensReviewWorkspace(item: NotificationItem | null | undefined): boolean {
  return item?.target === 'review-workspace' && Boolean(item.drawingNo)
}

/** 通知条目的关联操作文案，弹窗与通知中心抽屉共用。 */
export function notificationActionLabel(item: NotificationItem | null | undefined): string {
  if (opensReviewWorkspace(item)) return '转到审核工作台'
  return item?.kind === 'change' ? '查看变更工单' : '查看图纸审批'
}
