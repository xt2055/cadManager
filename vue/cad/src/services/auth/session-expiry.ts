/**
 * 会话失效（HTTP 401）广播。
 *
 * 后端 401 的含义只有一个：这个令牌已经无效。业务层过去会把 401 当成普通请求失败
 * 吞掉，于是用户看到的是「预览打不开」「列表空白」这类与真实原因无关的界面，
 * 唯一线索只剩控制台里的一行 401。
 *
 * 服务层在收到 401 时调用 notifySessionExpired()，由 main.ts 注册的处理器统一
 * 退出登录并回到登录页（带 redirect），把静默失败变成一次明确、可继续的重新登录。
 */
export type SessionExpiredHandler = () => void

const handlers = new Set<SessionExpiredHandler>()

/** 注册会话失效处理器，返回取消注册的函数。 */
export function onSessionExpired(handler: SessionExpiredHandler): () => void {
  handlers.add(handler)
  return () => {
    handlers.delete(handler)
  }
}

export function notifySessionExpired(): void {
  for (const handler of [...handlers]) {
    try {
      handler()
    } catch (error) {
      // 处理器自身出错不能影响请求路径，否则 401 会变成新的崩溃点。
      console.error('处理登录失效失败', error)
    }
  }
}
