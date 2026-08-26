export interface CadWorkerUrls {
  dwgParser: string
  mtextRender: string
}

export function getCadWorkerUrls(): CadWorkerUrls {
  const assetBaseUrl = import.meta.env.DEV
    ? new URL('/assets/', window.location.origin)
    : new URL('./', import.meta.url)

  return {
    dwgParser: new URL('libredwg-parser-worker.js', assetBaseUrl).href,
    mtextRender: new URL('mtext-renderer-worker.js', assetBaseUrl).href,
  }
}

export async function assertCadWorkerAssets(urls: CadWorkerUrls): Promise<void> {
  const checks = await Promise.all(
    Object.entries(urls).map(async ([name, url]) => {
      const response = await fetch(url, {
        method: 'HEAD',
        cache: 'no-store',
      })
      const contentType = response.headers.get('content-type') || ''

      if (!response.ok || /text\/html/i.test(contentType)) {
        throw new Error(`${name} Worker 资源不可用：${url}（HTTP ${response.status}，${contentType || '未知类型'}）`)
      }

      return response
    }),
  )

  if (checks.length !== Object.keys(urls).length) {
    throw new Error('CAD Worker 资源检查失败')
  }
}
