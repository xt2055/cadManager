import { getApiBaseUrl } from '@/services/api-base.service'

export interface NotificationItem {
  id: string
  kind: 'change' | 'review' | 'announcement'
  title: string
  content: string
  senderName: string
  drawingId: string
  /** 图纸编号：待办审核需要它才能打开审核工作台。 */
  drawingNo: string
  /** 跳转目标：review-workspace 表示直接进入审核工作台。 */
  target: string
  createdAt: string
  readAt: string | null
}
export interface NotificationInbox {
  items: NotificationItem[]
  total: number
  unread: number
  page: number
  pageSize: number
}
export interface NotificationRecipient {
  id: string
  displayName: string
  account: string
  status: string
}
async function request<T>(token: string, path: string, body?: unknown): Promise<T> {
  const response = await fetch(`${getApiBaseUrl()}${path}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(15000),
  })
  const result = await response.json()
  if (!response.ok) throw new Error(result.message || '通知请求失败，请稍后重试')
  return result.data as T
}
export const notificationService = {
  list: (token: string, page = 1, unread = false) => request<NotificationInbox>(token, `/notifications?page=${page}&unread=${unread ? '1' : '0'}`),
  read: (token: string, id: string) => request(token, `/notifications/${encodeURIComponent(id)}/read`, {}),
  readAll: (token: string) => request(token, '/notifications/read-all', {}),
  recipients: (token: string) => request<NotificationRecipient[]>(token, '/users'),
  send: (token: string, input: { title: string; content: string; broadcast: boolean; recipientIds: string[] }) => request<{ sent: number }>(token, '/admin/notifications', input),
}
