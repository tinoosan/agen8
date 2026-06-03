import { lazy, Suspense } from 'react'
import { isSpreadsheetFile, isMarkdownFile, isDocxFile } from './filePreviewUtils'
import ArtifactPreviewPane from './ArtifactPreviewPane'
import DocumentViewer from './DocumentViewer'
import type { ArtifactNode, ArtifactGetResult } from '../../lib/types'

const SpreadsheetViewer = lazy(() => import('./SpreadsheetViewer'))
const DocxViewer = lazy(() => import('./DocxViewer'))

type Variant = 'page' | 'slideover'

interface ArtifactViewerProps {
  file: ArtifactNode
  preview: ArtifactGetResult | undefined
  isLoading: boolean
  error: boolean
  variant: Variant
}

/**
 * Dispatcher that routes files to specialized viewers by type.
 * Falls through to ArtifactPreviewPane for anything without a dedicated viewer.
 *
 * Extension point: add branches here for document (BlockNote), presentation
 * (Reveal.js), and other future viewers.
 */
export default function ArtifactViewer(props: ArtifactViewerProps) {
  const path = props.file.vpath ?? props.file.diskPath ?? props.file.label ?? ''

  // DocumentViewer is imported directly (not lazy) because it handles
  // BlockNote loading internally via dynamic import(). The heavy BlockNote
  // chunk only loads in-browser when the editor mounts.
  if (isMarkdownFile(path)) {
    return (
      <DocumentViewer
        file={props.file}
        preview={props.preview}
        isLoading={props.isLoading}
        error={props.error}
        variant={props.variant}
      />
    )
  }

  if (isDocxFile(path)) {
    return (
      <Suspense fallback={<div className="flex items-center justify-center h-full opacity-60">Loading viewer...</div>}>
        <DocxViewer file={props.file} preview={props.preview} isLoading={props.isLoading} error={props.error} />
      </Suspense>
    )
  }

  if (isSpreadsheetFile(path)) {
    return (
      <Suspense
        fallback={
          <div className="flex items-center justify-center h-full opacity-60">
            Loading viewer...
          </div>
        }
      >
        <SpreadsheetViewer
          file={props.file}
          preview={props.preview}
          isLoading={props.isLoading}
          error={props.error}
        />
      </Suspense>
    )
  }

  return <ArtifactPreviewPane {...props} />
}
