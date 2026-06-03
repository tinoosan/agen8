import ExcelJS from 'exceljs'
import Papa from 'papaparse'
import { decodeBase64 } from './filePreviewUtils'
import type { CellData, ParseResult, ParsedSheet } from './spreadsheetTypes'

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

export async function parseXLSX(bytesB64: string): Promise<ParseResult> {
  const workbook = new ExcelJS.Workbook()
  await workbook.xlsx.load(decodeBase64(bytesB64))

  if (workbook.worksheets.length === 0) {
    return { sheets: [], error: null }
  }

  const sheets: ParsedSheet[] = workbook.worksheets.map((worksheet) => {
    const name = worksheet.name
    if (worksheet.rowCount === 0) return { name, headers: [], rows: [], colWidths: [] }

    const headers: string[] = []
    const rows: CellData[][] = []
    const headerRow = worksheet.getRow(1)

    for (let column = 1; column <= worksheet.columnCount; column += 1) {
      const value = headerRow.getCell(column).value
      headers.push(value != null ? String(value) : `Col ${column}`)
    }

    for (let rowIndex = 2; rowIndex <= worksheet.rowCount; rowIndex += 1) {
      const worksheetRow = worksheet.getRow(rowIndex)
      const row: CellData[] = []
      for (let column = 1; column <= worksheet.columnCount; column += 1) {
        const value = worksheetRow.getCell(column).value
        if (value == null) {
          row.push({ raw: null, display: '', type: null })
          continue
        }
        const isNumber = typeof value === 'number'
        const isBoolean = typeof value === 'boolean'
        const isDate = value instanceof Date
        row.push({
          raw: value,
          display: isDate ? value.toISOString().split('T')[0] : String(value),
          type: isNumber ? 'n' : isBoolean ? 'b' : isDate ? 'd' : 's',
        })
      }
      rows.push(row)
    }

    const authorWidths = headers.map((_, index) => {
      const width = worksheet.getColumn(index + 1).width
      return width || undefined
    })

    return {
      name,
      headers,
      rows,
      colWidths: computeColWidths(headers, rows, authorWidths),
    }
  })

  return { sheets, error: null }
}
