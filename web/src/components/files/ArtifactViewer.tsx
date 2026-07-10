import { Suspense } from 'react'
import { isSpreadsheetFile, isMarkdownFile } from './filePreviewUtils'
import type { ArtifactNode, ArtifactGetResult } from '../../lib/types'
import { lazyWithRetry } from '../../lib/lazyWithRetry'

const ArtifactPreviewPane = lazyWithRetry(() => import('./ArtifactPreviewPane'), 'components/files/ArtifactPreviewPane')
const DocumentViewer = lazyWithRetry(() => import('./DocumentViewer'), 'components/files/DocumentViewer')
const SpreadsheetViewer = lazyWithRetry(() => import('./SpreadsheetViewer'), 'components/files/SpreadsheetViewer')

type Variant = 'page' | 'slideover'

interface ArtifactViewerProps {
  file: ArtifactNode
  preview: ArtifactGetResult | undefined
  isLoading: boolean
  error: boolean
  variant: Variant
  /** Soft-wrap long lines in the code view. Only ArtifactPreviewPane honors it. */
  wrap?: boolean
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
  const fallback = (
    <div className="flex h-full items-center justify-center opacity-60" role="status" aria-label="Loading file preview">
      <span className="spinner spinner-md" />
    </div>
  )

  if (isMarkdownFile(path)) {
    return (
      <Suspense fallback={fallback}>
        <DocumentViewer
          file={props.file}
          preview={props.preview}
          isLoading={props.isLoading}
          error={props.error}
          variant={props.variant}
        />
      </Suspense>
    )
  }

  if (isSpreadsheetFile(path)) {
    return (
      <Suspense
        fallback={fallback}
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

  return (
    <Suspense fallback={fallback}>
      <ArtifactPreviewPane {...props} />
    </Suspense>
  )
}
