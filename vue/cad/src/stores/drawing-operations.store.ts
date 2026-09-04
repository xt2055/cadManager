import { useDomainStore } from './domain.store'

/**
 * 尚未完成原子命令拆分的图纸写操作兼容边界。
 * 页面只能依赖此边界，后续可逐项替换为 DrawingCommandService/专用 Store，
 * 避免新页面继续直接耦合 legacy domain store。
 */
export function useDrawingOperationsStore() {
  return useDomainStore()
}
