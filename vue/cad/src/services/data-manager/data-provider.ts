import type { DataDocument } from './data.types'

export interface DataProvider {
  load(): Promise<DataDocument>
  save(document: DataDocument): Promise<void>
}
