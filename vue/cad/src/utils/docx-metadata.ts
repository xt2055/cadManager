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

export async function readDocxAuthor(file: Blob): Promise<string | undefined> {
  const zip = await JSZip.loadAsync(file)
  const headerNames = Object.keys(zip.files)
    .filter((name) => /^word\/header\d+\.xml$/i.test(name))
    .sort()

  for (const headerName of headerNames) {
    const entry = zip.file(headerName)
    if (!entry) continue
    const rows = headerTableRows(await entry.async('string'))
    for (let rowIndex = 0; rowIndex < rows.length - 1; rowIndex += 1) {
      const labels = rows[rowIndex] ?? []
      const values = rows[rowIndex + 1] ?? []
      const authorIndex = labels.findIndex((label) => label.replace(/\s+/g, '').includes('编制'))
      const author = authorIndex >= 0 ? values[authorIndex]?.trim() : ''
      if (author && !/^(编制|标准化|审核|批准|日期)$/.test(author)) return author
    }
  }

  return undefined
}
