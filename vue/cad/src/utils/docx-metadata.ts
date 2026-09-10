import JSZip from 'jszip'

function xmlCellTexts(cell: string): string {
  return [...cell.matchAll(/<w:t(?:\s[^>]*)?>([\s\S]*?)<\/w:t>/g)]
    .map((match) => match[1] ?? '')
    .join('')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&#39;/g, "'")
    .replace(/&quot;/g, '"')
    .trim()
}

function headerTableRows(xml: string): string[][] {
  return [...xml.matchAll(/<w:tr(?:\s[^>]*)?>([\s\S]*?)<\/w:tr>/g)].map((rowMatch) => {
    const row = rowMatch[1] ?? ''
    return [...row.matchAll(/<w:tc(?:\s[^>]*)?>([\s\S]*?)<\/w:tc>/g)].map((cellMatch) =>
      xmlCellTexts(cellMatch[1] ?? ''),
    )
  })
}

function validAuthor(value: string | undefined): string | undefined {
  const author = value
    ?.replace(/^\s*编制(?:人员)?\s*[:：]?\s*/, '')
    .split(/\s*(?:校对|审核|标准化|批准|日期)\s*[:：]?/)[0]
    ?.trim()
  if (!author || /^(编制|编制人员|校对|标准化|审核|批准|日期)$/.test(author)) return undefined
  return author
}

export function authorFromDocxRows(rows: string[][]): string | undefined {
  for (let rowIndex = 0; rowIndex < rows.length; rowIndex += 1) {
    const row = rows[rowIndex] ?? []
    for (let columnIndex = 0; columnIndex < row.length; columnIndex += 1) {
      const cell = row[columnIndex] ?? ''
      const compact = cell.replace(/\s+/g, '')
      const inlineLabel = /^编制(?:人员)?[:：]/.test(compact)
      const standaloneLabel = /^编制(?:人员)?[:：]?$/.test(compact)
      if (!inlineLabel && !standaloneLabel) continue

      if (inlineLabel) {
        const inline = validAuthor(cell)
        if (inline) return inline
      }

      const beside = validAuthor(row[columnIndex + 1])
      if (beside) return beside

      const below = validAuthor(rows[rowIndex + 1]?.[columnIndex])
      if (below) return below
    }
  }
  return undefined
}

export async function readDocxAuthor(file: Blob): Promise<string | undefined> {
  const zip = await JSZip.loadAsync(file)
  const documentNames = Object.keys(zip.files)
    .filter((name) => /^word\/header\d+\.xml$/i.test(name) || name === 'word/document.xml')
    .sort((left, right) => Number(left === 'word/document.xml') - Number(right === 'word/document.xml'))

  for (const documentName of documentNames) {
    const entry = zip.file(documentName)
    if (!entry) continue
    const author = authorFromDocxRows(headerTableRows(await entry.async('string')))
    if (author) return author
  }

  return undefined
}
