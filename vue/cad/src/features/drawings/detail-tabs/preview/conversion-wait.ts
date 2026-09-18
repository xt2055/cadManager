import { getApiBaseUrl } from '@/services/api-base.service'

/**
 * 等待后端把 EXB 转成 DWG。
 *
 * 上传任何 CAD 文件时后端都会自动入队转换（`upload/service.go` 的 `enqueueCADConversion`），
 * 浏览器解析不了 EXB，所以 EXB 的图号必须**等转换完成后读 DWG 图幅**再拿。
 * 这里只做轮询与状态归并，触发转换仍是上传本身的副作用，不需要前端额外发起。
 */

export interface ConversionWaitResult {
  /** 已经成为 DWG/DXF、可以读取图幅的附件。 */
  ready: string[]
  /** 重试用尽、需要人工介入的附件。 */
  failed: Array<{ id: string; error: string }>
  /** 轮询超时仍未完成的附件；不算失败，只是还没好。 */
  pending: string[]
}

export interface ConversionWaitOptions {
  intervalMs?: number
  timeoutMs?: number
  signal?: AbortSignal
  onProgress?: (done: number, total: number) => void
}

/** 状态取值见 `conversion_handler.go`：ready 为终态成功，failed 为终态失败。 */
const TERMINAL_FAILURE = 'failed'

function accessToken(): string {
  if (typeof window === 'undefined') return ''
  return window.localStorage.getItem('cad_access_token') || window.sessionStorage.getItem('cad_access_token') || ''
}

async function readStatus(id: string, signal?: AbortSignal): Promise<{ status: string; error: string }> {
  const token = accessToken()
  const response = await fetch(`${getApiBaseUrl()}/cad/conversions/${encodeURIComponent(id)}`, {
    headers: { Accept: 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    credentials: 'include',
    signal: signal ?? AbortSignal.timeout(10000),
  })
  const body = await response.json().catch(() => ({})) as { data?: { status?: string; error?: string }; message?: string }
  if (!response.ok) throw new Error(body.message || `读取转换状态失败：HTTP ${response.status}`)
  return { status: body.data?.status || 'pending', error: body.data?.error || '' }
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

/**
 * 轮询直到全部就绪、出现终态失败或超时。
 * 超时不抛错：把还没好的附件放进 `pending`，由调用方决定是提示用户稍后重试还是先行放行。
 */
export async function waitForConversion(ids: readonly string[], options: ConversionWaitOptions = {}): Promise<ConversionWaitResult> {
  const intervalMs = options.intervalMs ?? 2000
  const timeoutMs = options.timeoutMs ?? 180_000
  const deadline = Date.now() + timeoutMs
  const outstanding = new Set(ids)
  const failed = new Map<string, string>()

  const snapshot = (): ConversionWaitResult => ({
    ready: ids.filter((id) => !outstanding.has(id) && !failed.has(id)),
    failed: [...failed].map(([id, error]) => ({ id, error })),
    pending: [...outstanding],
  })

  if (!ids.length) return snapshot()
  options.onProgress?.(0, ids.length)

  while (outstanding.size && Date.now() < deadline) {
    options.signal?.throwIfAborted()
    for (const id of [...outstanding]) {
      // 单个附件读取失败按「还没好」处理，不因此中断整批等待。
      let state: { status: string; error: string }
      try {
        state = await readStatus(id, options.signal)
      } catch (error: unknown) {
        if (error instanceof DOMException && error.name === 'AbortError') throw error
        continue
      }
      if (state.status === 'ready') outstanding.delete(id)
      else if (state.status === TERMINAL_FAILURE) {
        outstanding.delete(id)
        failed.set(id, state.error || '图纸转换失败')
      }
    }
    options.onProgress?.(ids.length - outstanding.size, ids.length)
    if (outstanding.size) await wait(intervalMs)
  }

  return snapshot()
}
