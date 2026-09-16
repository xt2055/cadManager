import type { NotificationItem } from '@/services/notification.service'

export interface NotificationAlertBatch {
  /** true = 本次是首次同步，仅建立基线，不产生提醒 */
  seeded: boolean
  /** 需要弹窗的未读通知，按 createdAt 由旧到新排序 */
  alerts: NotificationItem[]
}

export interface NotificationAlertTracker {
  collect(items: NotificationItem[]): NotificationAlertBatch
  reset(): void
}

function createdAtValue(item: NotificationItem) {
  const value = Date.parse(item.createdAt)
  return Number.isNaN(value) ? 0 : value
}

/**
 * 服务端推送只告知“有新事件”，新通知必须由前端按 id 差集判定。
 * 首次同步只建立基线：登录时已存在的历史未读不逐条弹窗。
 */
export function createNotificationAlertTracker(): NotificationAlertTracker {
  const known = new Set<string>()
  let seeded = false

  function collect(items: NotificationItem[]): NotificationAlertBatch {
    if (!seeded) {
      seeded = true
      for (const item of items) known.add(item.id)
      return { seeded: true, alerts: [] }
    }
    const alerts: NotificationItem[] = []
    for (const item of items) {
      if (known.has(item.id)) continue
      known.add(item.id)
      if (item.readAt) continue
      alerts.push(item)
    }
    alerts.sort((left, right) => createdAtValue(left) - createdAtValue(right))
    return { seeded: false, alerts }
  }

  function reset() {
    known.clear()
    seeded = false
  }

  return { collect, reset }
}
