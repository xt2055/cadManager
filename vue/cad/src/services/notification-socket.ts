import { getApiBaseUrl } from '@/services/api-base.service'

export type NotificationConnectionState = 'connecting' | 'connected' | 'disconnected' | 'unauthorized'

export function connectNotifications(token: string, callbacks: {
  changed: () => void
  ready: () => void
  state: (state: NotificationConnectionState) => void
}): () => void {
  let socket: WebSocket | undefined
  let retry: ReturnType<typeof setTimeout> | undefined
  let watchdog: ReturnType<typeof setTimeout> | undefined
  let stopped = false
  let attempts = 0

  function retryLater() {
    if (stopped || retry) return
    callbacks.state('disconnected')
    retry = setTimeout(() => {
      retry = undefined
      connect()
    }, Math.min(30000, 1000 * 2 ** Math.min(attempts++, 5)) + Math.random() * 500)
  }
  function connect() {
    if (stopped) return
    callbacks.state('connecting')
    try {
      const url = new URL(`${getApiBaseUrl()}/notifications/ws`, window.location.href)
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
      const current = new WebSocket(url)
      socket = current
      function armWatchdog() {
        if (watchdog) clearTimeout(watchdog)
        watchdog = setTimeout(() => current.close(), 45000)
      }
      armWatchdog()
      current.onopen = () => {
        if (stopped || current !== socket) return
        current.send(JSON.stringify({ type: 'auth', token }))
      }
      current.onmessage = event => {
        if (stopped || current !== socket) return
        armWatchdog()
        try {
          const message = JSON.parse(String(event.data)) as { type: string }
          if (message.type === 'ready') {
            attempts = 0
            callbacks.state('connected')
            callbacks.ready()
          } else if (message.type === 'changed') callbacks.changed()
          else if (message.type === 'ping') current.send(JSON.stringify({ type: 'pong' }))
          else if (message.type === 'unauthorized') {
            stopped = true
            callbacks.state('unauthorized')
            current.close()
          } else if (message.type === 'unavailable') current.close()
        } catch { current.close() }
      }
      current.onerror = () => current.close()
      current.onclose = () => {
        if (current !== socket) return
        if (watchdog) clearTimeout(watchdog)
        socket = undefined
        retryLater()
      }
    } catch { retryLater() }
  }
  function resume() {
    if (stopped || socket || document.visibilityState === 'hidden') return
    if (retry) clearTimeout(retry)
    retry = undefined
    connect()
  }
  window.addEventListener('online', resume)
  document.addEventListener('visibilitychange', resume)
  connect()
  return () => {
    stopped = true
    if (retry) clearTimeout(retry)
    if (watchdog) clearTimeout(watchdog)
    socket?.close()
    window.removeEventListener('online', resume)
    document.removeEventListener('visibilitychange', resume)
  }
}
