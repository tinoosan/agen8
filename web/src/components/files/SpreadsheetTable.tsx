import { useMemo, useState } from 'react'
import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  type ColumnDef,
  type FilterFn,
  type SortingState,
  useReactTable,
} from '@tanstack/react-table'
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight, ChevronUp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { CellData, ParsedSheet } from './spreadsheetTypes'

function SortIndicator({ sorted }: { sorted: false | 'asc' | 'desc' }) {
  if (sorted === 'asc') return <ArrowUp className="ml-1 h-3 w-3 shrink-0" style={{ color: 'var(--accent)' }} />
  if (sorted === 'desc') return <ArrowDown className="ml-1 h-3 w-3 shrink-0" style={{ color: 'var(--accent)' }} />
  return <ChevronUp className="ml-1 h-3 w-3 shrink-0 opacity-0 group-hover:opacity-25 transition-opacity duration-150" />
}

const cellGridFilter: FilterFn<CellData[]> = (row, _columnId, filterValue) => {
  const search = String(filterValue).toLowerCase()
  return row.original.some((cell) => (
    !!cell && (
      String(cell.raw ?? '').toLowerCase().includes(search) ||
      cell.display.toLowerCase().includes(search)
    )
  ))
}

export default function SpreadsheetTable({
  sheet,
  globalFilter,
  setGlobalFilter,
}: {
  sheet: ParsedSheet
  globalFilter: string
  setGlobalFilter: (value: string) => void
}) {
  const [sorting, setSorting] = useState<SortingState>([])

  const numericCols = useMemo(() => {
    const result = new Set<number>()
    if (sheet.rows.length === 0) return result
    const sampleSize = Math.min(sheet.rows.length, 20)
    for (let column = 0; column < sheet.headers.length; column += 1) {
      let numCount = 0
      for (let row = 0; row < sampleSize; row += 1) {
        if (sheet.rows[row]?.[column]?.type === 'n') numCount += 1
      }
      if (numCount > sampleSize * 0.6) result.add(column)
    }
    return result
  }, [sheet.headers.length, sheet.rows])

  const columns = useMemo<ColumnDef<CellData[]>[]>(() => [
    {
      id: '_n',
      header: () => null,
      cell: ({ row }) => (
        <span className="text-[11px] select-none tabular-nums" style={{ color: 'var(--text-4)' }}>
          {row.index + 1}
        </span>
      ),
      size: 36,
      enableSorting: false,
      enableGlobalFilter: false,
    },
    ...sheet.headers.map((header, colIdx) => ({
      id: `col_${colIdx}`,
      accessorFn: (row: CellData[]) => row[colIdx]?.raw,
      header: ({ column }: { column: { toggleSorting: () => void; getIsSorted: () => false | 'asc' | 'desc' } }) => (
        <button
          type="button"
          className="group flex items-center gap-0.5 transition-colors duration-100 w-full"
          style={{ color: 'var(--text-2)', justifyContent: numericCols.has(colIdx) ? 'flex-end' : 'flex-start' }}
          onClick={() => column.toggleSorting()}
        >
          <span className="text-[12px] font-medium">{header}</span>
          <SortIndicator sorted={column.getIsSorted()} />
        </button>
      ),
      cell: ({ row }: { row: { original: CellData[] } }) => {
        const cell = row.original[colIdx]
        if (!cell || cell.raw == null) return <span style={{ color: 'var(--text-4)' }}>&mdash;</span>
        return <span className={cell.type === 'n' ? 'tabular-nums' : ''}>{cell.display}</span>
      },
      size: sheet.colWidths[colIdx] * 8,
    })),
  ], [numericCols, sheet.colWidths, sheet.headers])

  const table = useReactTable({
    data: sheet.rows,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    globalFilterFn: cellGridFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: { pagination: { pageSize: 100 } },
  })

  const filteredCount = table.getFilteredRowModel().rows.length
  const totalCount = sheet.rows.length

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 min-h-0 overflow-auto">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id} style={{ borderColor: 'var(--border-strong)' }}>
                {headerGroup.headers.map((header) => {
                  const colIdx = header.id.startsWith('col_') ? parseInt(header.id.slice(4), 10) : -1
                  const isNum = colIdx >= 0 && numericCols.has(colIdx)
                  const colWidth = colIdx >= 0 ? sheet.colWidths[colIdx] * 8 : undefined
                  return (
                    <TableHead
                      key={header.id}
                      className="whitespace-nowrap h-9 px-3"
                      style={{
                        position: 'sticky',
                        top: 0,
                        zIndex: 10,
                        background: 'var(--bg-surface)',
                        textAlign: isNum ? 'right' : 'left',
                        ...(header.id === '_n' ? { width: 36, paddingLeft: 12, paddingRight: 4 } : {}),
                        ...(colWidth ? { width: colWidth, minWidth: colWidth } : {}),
                      }}
                    >
                      {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.length > 0 ? (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id} className="transition-colors duration-75 hover:bg-[var(--bg-hover)]" style={{ borderColor: 'var(--border)' }}>
                  {row.getVisibleCells().map((cell) => {
                    const colIdx = cell.column.id.startsWith('col_') ? parseInt(cell.column.id.slice(4), 10) : -1
                    const cellData = colIdx >= 0 ? row.original[colIdx] : null
                    const isNum = cellData?.type === 'n'
                    const colWidth = colIdx >= 0 ? sheet.colWidths[colIdx] * 8 : undefined
                    return (
                      <TableCell
                        key={cell.id}
                        className="whitespace-nowrap px-3 py-[6px] text-[13px]"
                        style={{
                          color: cell.column.id === '_n' ? undefined : 'var(--text-1)',
                          textAlign: isNum ? 'right' : 'left',
                          ...(cell.column.id === '_n' ? { width: 36, paddingLeft: 12, paddingRight: 4 } : {}),
                          ...(colWidth ? { width: colWidth, minWidth: colWidth } : {}),
                        }}
                      >
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </TableCell>
                    )
                  })}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center">
                  <div className="flex flex-col items-center gap-1" style={{ color: 'var(--text-3)' }}>
                    <span className="text-[13px]">No matching rows</span>
                    {globalFilter && (
                      <button
                        type="button"
                        className="text-[12px] hover:underline"
                        style={{ color: 'var(--accent)' }}
                        onClick={() => setGlobalFilter('')}
                      >
                        Clear filter
                      </button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex items-center justify-between h-8 px-3 text-[11px] shrink-0 border-t" style={{ color: 'var(--text-3)', borderColor: 'var(--border)' }}>
        <span className="tabular-nums">
          {globalFilter ? `${filteredCount} of ${totalCount}` : totalCount} row{totalCount !== 1 && !globalFilter ? 's' : ''} &middot; {sheet.headers.length} col{sheet.headers.length !== 1 ? 's' : ''}
        </span>
        {table.getPageCount() > 1 && (
          <div className="flex items-center gap-0.5">
            <Button variant="ghost" size="sm" className="h-5 w-5 p-0" onClick={() => table.previousPage()} disabled={!table.getCanPreviousPage()}>
              <ChevronLeft className="h-3 w-3" />
            </Button>
            <span className="tabular-nums px-1.5" style={{ minWidth: 48, textAlign: 'center' }}>
              {table.getState().pagination.pageIndex + 1} / {table.getPageCount()}
            </span>
            <Button variant="ghost" size="sm" className="h-5 w-5 p-0" onClick={() => table.nextPage()} disabled={!table.getCanNextPage()}>
              <ChevronRight className="h-3 w-3" />
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
