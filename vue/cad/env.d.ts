/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

interface Window {
  /** 开发环境 CAD 字体诊断入口，正式构建不会注入。 */
  cadDocManager?: {
    loadFonts?: (fonts: string[]) => Promise<void>
  }
  cadFontDiagnostics?: {
    loadGdtFonts: () => Promise<unknown>
    inspectDrawingEntities: () => unknown
    inspect: () => unknown
  }
}
