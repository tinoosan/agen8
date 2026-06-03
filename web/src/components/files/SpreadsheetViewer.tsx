import { useEffect, useState } from 'react'
import ExcelJS from 'exceljs'
import Papa from 'papaparse'
import { AlertTriangle, Download, Search, TableIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { basename, downloadBlob, formatBytes, isTextSpreadsheet } from './filePreviewUtils'
import SpreadsheetTable from './SpreadsheetTable'
import { parseCSV, parseXLSX } from './spreadsheetParser'
import type { ParseResult, SpreadsheetViewerProps } from './spreadsheetTypes'

function Placeholder({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-1.5 px-6">
      <AlertTriangle className="h-5 w-5 mb-1" style={{ color: 'var(--text-3)' }} />
      <p className="text-[13px] font-medium" style={{ color: 'var(--text-1)' }}>{title}</p>
      <p className="text-[12px] text-center max-w-xs" style={{ color: 'var(--text-3)' }}>{detail}</p>
    </div>
  )
}

function EmptyPlaceholder() {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-1.5 px-6">
      <TableIcon className="h-5 w-5 mb-1" style={{ color: 'var(--text-3)' }} />
      <p className="text-[13px] font-medium" style={{ color: 'var(--text-1)' }}>No data found</p>
      <p className="text-[12px] text-center max-w-xs" style={{ color: 'var(--text-3)' }}>This spreadsheet appears to be empty.</p>
    </div>
  )
}

function LoadingState() {
  return (
    <div className="flex flex-col h-full overflow-hidden" style={{ borderRadius: 'var(--r-lg)', border: '1px solid var(--border)', background: 'var(--bg-panel)' }}>
      <div className="h-9 flex items-center gap-4 px-3 border-b" style={{ borderColor: 'var(--border)', background: 'var(--bg-surface)' }}>
        <Skeleton className="h-3 w-14" />
        <Skeleton className="h-3 w-20" />
        <Skeleton className="h-3 w-16" />
        <Skeleton className="h-3 w-12" />
      </div>
      {Array.from({ length: 12 }).map((_, index) => (
        <div key={index} className="flex items-center gap-5 px-3 h-[31px] border-b" style={{ borderColor: 'var(--border)' }}>
          <Skeleton className="h-2.5 w-5" />
          <Skeleton className="h-2.5 w-14" />
          <Skeleton className="h-2.5 w-20" />
          <Skeleton className="h-2.5 w-10" />
        </div>
      ))}
    </div>
  )
}

export default function SpreadsheetViewer({ file, preview, isLoading, error }: SpreadsheetViewerProps) {
  const [activeSheet, setActiveSheet] = useState(0)
  const [globalFilter, setGlobalFilter] = useState('')
  const [parsed, setParsed] = useState<ParseResult>({ sheets: [], error: null })

  const filePath = file.vpath ?? file.diskPath ?? file.label ?? ''
  const fileName = basename(filePath)

  useEffect(() => {
    if (!preview) return
    const currentPreview = preview
    let cancelled = false

    async function parse() {
      try {
        let result: ParseResult
        if (isTextSpreadsheet(filePath)) {
          result = !currentPreview.content && !currentPreview.bytesB64 ? { sheets: [], error: null } : parseCSV(currentPreview.content ?? '')
        } else if (!currentPreview.bytesB64) {
          result = { sheets: [], error: 'No binary data available for this file.' }
        } else {
          result = await parseXLSX(currentPreview.bytesB64)
        }
        if (!cancelled) setParsed(result)
      } catch (err) {
        if (!cancelled) setParsed({ sheets: [], error: `Failed to parse spreadsheet: ${err instanceof Error ? err.message : String(err)}` })
      }
    }

    void parse()
    return () => { cancelled = true }
  }, [filePath, preview])

  if (isLoading) return <LoadingState />
  if (error) return <Placeholder title="Failed to load file" detail="The file could not be fetched from the workspace." />
  if (parsed.error) return <Placeholder title="Unable to parse spreadsheet" detail={parsed.error} />
  if (parsed.sheets.length === 0 || parsed.sheets.every((sheet) => sheet.rows.length === 0 && sheet.headers.length === 0)) {
    return <EmptyPlaceholder />
  }

  const currentSheet = parsed.sheets[activeSheet] ?? parsed.sheets[0]

  function handleExportCSV() {
    const exportData = currentSheet.rows.map((row) => Object.fromEntries(currentSheet.headers.map((header, index) => [header, row[index]?.raw])))
    const csv = Papa.unparse(exportData)
    downloadBlob(fileName.replace(/\.\w+$/, '.csv'), new Blob([csv], { type: 'text/csv;charset=utf-8' }))
  }

  async function handleExportXLSX() {
    const workbook = new ExcelJS.Workbook()
    const worksheet = workbook.addWorksheet(currentSheet.name)
    worksheet.addRow(currentSheet.headers)
    for (const row of currentSheet.rows) {
      worksheet.addRow(row.map((cell) => cell.raw))
    }
    const buffer = await workbook.xlsx.writeBuffer()
    downloadBlob(fileName.replace(/\.\w+$/, '.xlsx'), new Blob([buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }))
  }

  return (
    <div className="flex flex-col h-full overflow-hidden" style={{ borderRadius: 'var(--r-lg)', border: '1px solid var(--border)', background: 'var(--bg-panel)' }}>
      <div className="flex items-center gap-2 h-9 px-3 shrink-0 border-b" style={{ borderColor: 'var(--border)' }}>
        <span className="text-[12px] font-medium truncate" style={{ color: 'var(--text-2)' }}>{fileName}</span>
        <span className="text-[11px] shrink-0" style={{ color: 'var(--text-4)' }}>
          {currentSheet.rows.length} &times; {currentSheet.headers.length}
          {preview?.fileSize != null ? ` \u00b7 ${formatBytes(preview.fileSize)}` : ''}
        </span>
        {preview?.truncated && (
          <Badge variant="warning" className="text-[10px] h-[18px] px-1.5 shrink-0">Truncated</Badge>
        )}
        <div className="flex-1 min-w-4" />
        <div className="relative shrink-0">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 pointer-events-none" style={{ color: 'var(--text-4)' }} />
          <Input
            placeholder="Filter..."
            value={globalFilter}
            onChange={(event) => setGlobalFilter(event.target.value)}
            className="h-6 w-36 pl-7 text-[11px] rounded-md"
          />
        </div>
        <div className="w-px h-3.5 shrink-0" style={{ background: 'var(--border)' }} />
        <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[11px] gap-1 shrink-0" onClick={handleExportCSV}>
          <Download className="h-2.5 w-2.5" /> CSV
        </Button>
        <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[11px] gap-1 shrink-0" onClick={() => { void handleExportXLSX() }}>
          <Download className="h-2.5 w-2.5" /> XLSX
        </Button>
      </div>

      {parsed.sheets.length > 1 && (
        <div className="px-3 py-1 shrink-0 border-b" style={{ borderColor: 'var(--border)' }}>
          <Tabs value={String(activeSheet)} onValueChange={(value) => { setActiveSheet(Number(value)); setGlobalFilter('') }}>
            <TabsList className="h-6">
              {parsed.sheets.map((sheet, index) => (
                <TabsTrigger key={sheet.name} value={String(index)} className="text-[11px] h-5 px-2">
                  {sheet.name}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
      )}

      <div className="flex-1 min-h-0">
        <SpreadsheetTable key={activeSheet} sheet={currentSheet} globalFilter={globalFilter} setGlobalFilter={setGlobalFilter} />
      </div>
    </div>
  )
}
