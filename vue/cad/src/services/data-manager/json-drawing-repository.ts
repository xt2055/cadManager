import type { BomItem, Drawing, StructurePart } from '@/types/domain.types'
import { normalizeBom, normalizeDrawings, normalizeStructure, readSeedModule } from './data.types'
import seedDocument from './data.seed.json'

const MODULE_STORAGE_PREFIX = 'cad:data-module:v1:'

/** JSON 调试模式的图纸/结构/BOM Repository；DataProvider 只负责能力编排。 */
export class JsonDrawingRepository {
  public constructor(
    private readonly storage: Storage = window.localStorage,
  ) {}

  listDrawings(): Drawing[] {
    return normalizeDrawings(this.readModule('drawings', readSeedModule(seedDocument, 'drawings', [])))
  }

  listStructure(): StructurePart[] {
    return normalizeStructure(this.readModule('structure', readSeedModule(seedDocument, 'structure', [])))
  }

  saveStructure(items: StructurePart[]): void {
    this.writeModule('structure', items)
  }

  listBom(): BomItem[] {
    return normalizeBom(this.readModule('bom', readSeedModule(seedDocument, 'bom', [])))
  }

  saveBom(items: BomItem[]): void {
    this.writeModule('bom', items)
  }

  private readModule<T>(name: string, fallback: T): T {
    const raw = this.storage.getItem(`${MODULE_STORAGE_PREFIX}${name}`)
    if (!raw) return fallback
    try {
      return JSON.parse(raw) as T
    } catch (error) {
      throw new Error(`本地图纸模块解析失败：${error instanceof Error ? error.message : String(error)}`)
    }
  }

  private writeModule<T>(name: string, value: T): void {
    this.storage.setItem(`${MODULE_STORAGE_PREFIX}${name}`, JSON.stringify(value, null, 2))
  }
}
