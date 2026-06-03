import { describe, expect, it } from 'vitest'
import { isSpreadsheetFile, isTextSpreadsheet, getFileExt, basename } from './filePreviewUtils'

describe('isSpreadsheetFile', () => {
  it('returns true for CSV', () => {
    expect(isSpreadsheetFile('/workspace/data.csv')).toBe(true)
  })

  it('returns true for TSV', () => {
    expect(isSpreadsheetFile('/workspace/data.tsv')).toBe(true)
  })

  it('returns true for XLSX', () => {
    expect(isSpreadsheetFile('/workspace/report.xlsx')).toBe(true)
  })

  it('returns true for XLS', () => {
    expect(isSpreadsheetFile('/workspace/legacy.xls')).toBe(true)
  })

  it('is case-insensitive for extension', () => {
    expect(isSpreadsheetFile('/workspace/DATA.CSV')).toBe(true)
    expect(isSpreadsheetFile('/workspace/FILE.XLSX')).toBe(true)
  })

  it('returns false for non-spreadsheet files', () => {
    expect(isSpreadsheetFile('/workspace/main.py')).toBe(false)
    expect(isSpreadsheetFile('/workspace/README.md')).toBe(false)
    expect(isSpreadsheetFile('/workspace/data.json')).toBe(false)
    expect(isSpreadsheetFile('/workspace/image.png')).toBe(false)
  })

  it('returns false for empty path', () => {
    expect(isSpreadsheetFile('')).toBe(false)
  })
})

describe('isTextSpreadsheet', () => {
  it('returns true for CSV and TSV', () => {
    expect(isTextSpreadsheet('data.csv')).toBe(true)
    expect(isTextSpreadsheet('data.tsv')).toBe(true)
  })

  it('returns false for XLSX (binary)', () => {
    expect(isTextSpreadsheet('data.xlsx')).toBe(false)
    expect(isTextSpreadsheet('data.xls')).toBe(false)
  })

  it('returns false for non-spreadsheet files', () => {
    expect(isTextSpreadsheet('file.py')).toBe(false)
  })
})

describe('getFileExt', () => {
  it('returns extension with dot', () => {
    expect(getFileExt('file.csv')).toBe('.csv')
    expect(getFileExt('/path/to/file.xlsx')).toBe('.xlsx')
  })

  it('returns last segment for extensionless file', () => {
    // getFileExt splits on "." — "Makefile" has no dot, so pop() returns the whole name
    expect(getFileExt('Makefile')).toBe('.makefile')
  })
})

describe('basename', () => {
  it('extracts filename from path', () => {
    expect(basename('/workspace/data.csv')).toBe('data.csv')
    expect(basename('file.txt')).toBe('file.txt')
  })
})
