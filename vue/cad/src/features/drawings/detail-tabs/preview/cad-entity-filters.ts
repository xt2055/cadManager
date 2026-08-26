export function findWipeoutMasks(database: any): any[] {
  const masks: any[] = []
  let wipeoutCount = 0

  for (const block of database.tables.blockTable.newIterator(true)) {
    for (const entity of block.newIterator()) {
      if (String(entity.dxfTypeName || entity.type || '').toUpperCase() !== 'WIPEOUT') {
        continue
      }
      wipeoutCount++
      entity.visibility = false
      masks.push(entity)
    }
  }

  console.info('[CAD] WIPEOUT 检测结果', { wipeoutCount, objectIds: masks.map(entity => entity.objectId) })
  return masks
}
