import { ApiDataProvider } from './api-data-provider'
import { createEmptyDataDocument, normalizeDataDocument, type DataDocument } from './data.types'
import type { DataProvider } from './data-provider'
import { JsonDataProvider } from './json-data-provider'

export interface DataManager {
  load(): Promise<DataDocument>
  save(document: DataDocument): Promise<void>
}

export function createDataProvider(): DataProvider {
  const configuredProvider = import.meta.env.VITE_DATA_PROVIDER
  const provider = configuredProvider || (import.meta.env.PROD ? 'api' : 'json')

  return provider === 'api' ? new ApiDataProvider() : new JsonDataProvider()
}

export class DefaultDataManager implements DataManager {
  private readonly provider: DataProvider

  constructor(provider: DataProvider = createDataProvider()) {
    this.provider = provider
  }

  async load(): Promise<DataDocument> {
    return normalizeDataDocument(await this.provider.load())
  }

  async save(document: DataDocument): Promise<void> {
    await this.provider.save(normalizeDataDocument(document))
  }
}

export const dataManager: DataManager = new DefaultDataManager()

export function emptyDataDocument(): DataDocument {
  return createEmptyDataDocument()
}
