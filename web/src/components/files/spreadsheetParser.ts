import Papa from 'papaparse'
import type { CellData, ParseResult } from './spreadsheetTypes'

function computeColWidths(headers: string[], rows: CellData[][], authorWidths: Array<number | undefined>): number[] {
  return headers.map((header, index) => {
    if (authorWidths[index]) return authorWidths[index]!
    const samples = [header.length, ...rows.slice(0, 50).map((row) => (row[index]?.display ?? '').length)]
    return Math.min(Math.max(...samples, 6), 40)
  })
}

export function parseCSV(content: string): ParseResult {
  const result = Papa.parse<Record<string, unknown>>(content, {
    header: true,
    dynamicTyping: true,
    skipEmptyLines: true,
  })

  if (result.errors.length > 0 && result.data.length === 0) {
    return { sheets: [], error: `CSV parse error: ${result.errors[0].message} (row ${result.errors[0].row})` }
  }

  const headers = result.meta.fields ?? []
  if (headers.length === 0 && result.data.length === 0) {
    return { sheets: [], error: null }
  }

  const rows: CellData[][] = result.data.map((record) => (
    headers.map((header) => {
      const value = record[header]
      return {
        raw: value,
        display: value == null ? '' : String(value),
        type: typeof value === 'number' ? 'n' : typeof value === 'boolean' ? 'b' : 's',
      }
    })
  ))

  return { sheets: [{ name: 'Sheet1', headers, rows, colWidths: computeColWidths(headers, rows, []) }], error: null }
}
