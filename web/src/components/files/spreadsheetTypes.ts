import type { ArtifactGetResult, ArtifactNode } from '../../lib/types'

export interface SpreadsheetViewerProps {
  file: ArtifactNode
  preview: ArtifactGetResult | undefined
  isLoading: boolean
  error: boolean
}

export interface CellData {
  raw: unknown
  display: string
  type: string | null
}

export interface ParsedSheet {
  name: string
  headers: string[]
  rows: CellData[][]
  colWidths: number[]
}

export interface ParseResult {
  sheets: ParsedSheet[]
  error: string | null
}
