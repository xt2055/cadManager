import { AcDbNativeDxfConverter } from '@mlightcad/data-model'
import { normalizeCadDxfBlockReferences } from './cad-custom-blocks'
import { logRawCadDwgModel } from './cad-fonts.service'

class CadDxfConverter extends AcDbNativeDxfConverter {
  override read(...args: Parameters<AcDbNativeDxfConverter['read']>) {
    return super.read(normalizeCadDxfBlockReferences(args[0]), args[1], args[2])
  }
}

export async function registerCadConverters(parserWorkerUrl: string) {
  const [{ AcDbDatabaseConverterManager, AcDbFileType }, { AcDbLibreDwgConverter }] = await Promise.all([
    import('@mlightcad/data-model'),
    import('@mlightcad/libredwg-converter'),
  ])
  const manager = AcDbDatabaseConverterManager.instance
  if (!(manager.get(AcDbFileType.DXF) instanceof CadDxfConverter)) {
    // 原生 DXF 转换器可能已由 MLightCAD 注册，需要替换为兼容入口。
    manager.register(AcDbFileType.DXF, new CadDxfConverter())
  }
  if (!manager.get(AcDbFileType.DWG)) {
    manager.register(AcDbFileType.DWG, new (class extends AcDbLibreDwgConverter {
      protected override async parse(data: ArrayBuffer, timeout?: number) {
        const model = await super.parse(data, timeout)
        if (import.meta.env.DEV) logRawCadDwgModel(model)
        return model
      }
    })({ convertByEntityType: false, useWorker: true, parserWorkerUrl }))
  }
}
