import { useState, useEffect, useCallback } from 'react'
import mammoth from 'mammoth'
import DOMPurify from 'dompurify'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Download, FileText, AlertTriangle } from 'lucide-react'
import type { ArtifactNode, ArtifactGetResult } from '../../lib/types'
import { basename, decodeBase64, downloadBlob, formatBytes } from './filePreviewUtils'

interface DocxViewerProps {
  file: ArtifactNode
  preview: ArtifactGetResult | undefined
  isLoading: boolean
  error: boolean
}

function Placeholder({ icon: Icon, title, detail }: { icon: typeof FileText; title: string; detail: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-1.5 px-6">
      <Icon className="h-5 w-5 mb-1" style={{ color: 'var(--text-3)' }} />
      <p className="text-[13px] font-medium" style={{ color: 'var(--text-1)' }}>{title}</p>
      <p className="text-[12px] text-center max-w-xs" style={{ color: 'var(--text-3)' }}>{detail}</p>
    </div>
  )
}

export default function DocxViewer({ file, preview, isLoading, error }: DocxViewerProps) {
  const filePath = file.vpath ?? file.diskPath ?? file.label ?? ''
  const fileName = basename(filePath)

  const [conversionResult, setConversionResult] = useState<
    { state: 'idle' } | { state: 'loading' } | { state: 'done'; html: string } | { state: 'error'; message: string }
  >(preview?.bytesB64 ? { state: 'loading' } : { state: 'idle' })

  useEffect(() => {
    if (!preview?.bytesB64) return
    let cancelled = false

    mammoth.convertToHtml({ arrayBuffer: decodeBase64(preview.bytesB64) })
      .then(result => { if (!cancelled) setConversionResult({ state: 'done', html: DOMPurify.sanitize(result.value) }) })
      .catch(err => { if (!cancelled) setConversionResult({ state: 'error', message: err instanceof Error ? err.message : String(err) }) })

    return () => { cancelled = true }
  }, [preview])

  const html = conversionResult.state === 'done' ? conversionResult.html : null
  const parseError = conversionResult.state === 'error' ? conversionResult.message : null
  const parsing = conversionResult.state === 'loading'

  const handleExportHTML = useCallback(() => {
    if (!html) return
    const fullHtml = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>${fileName}</title>
<style>body{font-family:system-ui,sans-serif;max-width:680px;margin:2rem auto;padding:0 1rem;line-height:1.6;color:#333}
h1,h2,h3{margin-top:1.5em}table{border-collapse:collapse;width:100%}
td,th{border:1px solid #ddd;padding:8px}th{background:#f5f5f5}
img{max-width:100%}</style>
</head><body>${DOMPurify.sanitize(html)}</body></html>`
    const blob = new Blob([fullHtml], { type: 'text/html;charset=utf-8' })
    downloadBlob(fileName.replace(/\.\w+$/, '.html'), blob)
  }, [html, fileName])

  // Loading
  if (isLoading || parsing) {
    return (
      <div
        className="flex flex-col h-full overflow-hidden"
        style={{ borderRadius: 'var(--r-lg)', border: '1px solid var(--border)', background: 'var(--bg-panel)' }}
      >
        <div className="h-9 flex items-center gap-3 px-3 border-b" style={{ borderColor: 'var(--border)' }}>
          <Skeleton className="h-3 w-32" />
          <div className="flex-1" />
          <Skeleton className="h-6 w-16" />
        </div>
        <div className="flex-1 px-6 py-6 max-w-[680px]">
          <Skeleton className="h-6 w-3/4 mb-4" />
          <Skeleton className="h-3 w-full mb-2" />
          <Skeleton className="h-3 w-full mb-2" />
          <Skeleton className="h-3 w-5/6 mb-6" />
          <Skeleton className="h-5 w-1/2 mb-3" />
          <Skeleton className="h-3 w-full mb-2" />
          <Skeleton className="h-3 w-4/5" />
        </div>
      </div>
    )
  }

  // Errors
  if (error) return <Placeholder icon={AlertTriangle} title="Failed to load file" detail="The file could not be fetched from the workspace." />
  if (parseError) return <Placeholder icon={AlertTriangle} title="Unable to parse document" detail={`Failed to parse DOCX: ${parseError}`} />
  if (!preview?.bytesB64) return <Placeholder icon={FileText} title="No document content" detail="This file appears to be empty or is not a valid DOCX." />
  if (html !== null && !html.trim()) return <Placeholder icon={FileText} title="Empty document" detail="This document contains no text content." />

  return (
    <div
      className="flex flex-col h-full overflow-hidden"
      style={{ borderRadius: 'var(--r-lg)', border: '1px solid var(--border)', background: 'var(--bg-panel)' }}
    >
      {/* Toolbar */}
      <div className="flex items-center gap-2 h-9 px-3 shrink-0 border-b" style={{ borderColor: 'var(--border)' }}>
        <FileText className="h-3.5 w-3.5 shrink-0" style={{ color: 'var(--text-3)' }} />
        <span className="text-[12px] font-medium truncate" style={{ color: 'var(--text-2)' }}>{fileName}</span>
        {preview?.fileSize != null && (
          <span className="text-[11px] shrink-0" style={{ color: 'var(--text-4)' }}>{formatBytes(preview.fileSize)}</span>
        )}
        <div className="flex-1 min-w-4" />
        <div className="w-px h-3.5 shrink-0" style={{ background: 'var(--border)' }} />
        <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[11px] gap-1 shrink-0" onClick={handleExportHTML}>
          <Download className="h-2.5 w-2.5" /> HTML
        </Button>
      </div>

      {/* Document content */}
      <div className="flex-1 min-h-0 overflow-auto">
        {html !== null && (
          <div
            className="md-prose text-[13.5px] leading-[1.7] max-w-[680px] mx-auto py-6 px-6"
            style={{ color: 'var(--text-1)' }}
            dangerouslySetInnerHTML={{ __html: html }}
          />
        )}
      </div>
    </div>
  )
}
