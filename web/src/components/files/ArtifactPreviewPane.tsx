import { useEffect, useMemo, useRef, useState } from 'react'
import { Copy, Download, MoreHorizontal, Search } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { ArtifactGetResult, ArtifactNode } from '../../lib/types'
import CodeView from './CodeView'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn, copyText } from '@/lib/utils'
import { basename, decodeBase64, downloadBlob, formatBytes, getFileExt, isMarkdownFile, isSvgFile } from './filePreviewUtils'

type Variant = 'page' | 'slideover'

function buildPreviewUrl(bytesB64: string, content: string, contentKind: string, contentType: string, isSvg: boolean): string | null {
  if (isSvg && content) {
    const blob = new Blob([content], { type: contentType || 'image/svg+xml' })
    return URL.createObjectURL(blob)
  }
  if (!bytesB64 || (contentKind !== 'image' && contentKind !== 'pdf')) return null
  const bytes = decodeBase64(bytesB64)
  const blob = new Blob([bytes], { type: contentType || 'application/octet-stream' })
  return URL.createObjectURL(blob)
}

export default function ArtifactPreviewPane({
  file,
  preview,
  isLoading,
  error,
  variant,
}: {
  file: ArtifactNode
  preview: ArtifactGetResult | undefined
  isLoading: boolean
  error: boolean
  variant: Variant
}) {
  const [contentSearch, setContentSearch] = useState('')
  const [svgMode, setSvgMode] = useState<'preview' | 'source'>('preview')
  const fileKey = file?.nodeKey ?? ''
  const prevKeyRef = useRef(fileKey)
  if (fileKey !== prevKeyRef.current) {
    prevKeyRef.current = fileKey
    if (contentSearch) setContentSearch('')
    if (svgMode !== 'preview') setSvgMode('preview')
  }

  const selectedPath = file.displayName ?? file.vpath ?? file.label ?? ''
  const content = preview?.content ?? ''
  const contentKind = preview?.contentKind ?? 'text'
  const contentType = preview?.contentType ?? ''
  const bytesB64 = preview?.bytesB64 ?? ''
  const truncated = preview?.truncated ?? false
  const bytesRead = preview?.bytesRead ?? 0
  const fileSize = preview?.fileSize ?? bytesRead
  const isText = contentKind === 'text'
  const isSvg = isSvgFile(selectedPath, contentType)
  const isMd = isText && isMarkdownFile(selectedPath) && !isSvg
  const lineCount = content ? content.split('\n').length : 0
  const byteCount = fileSize || (content ? new Blob([content]).size : 0)
  const showInlinePDF = contentKind === 'pdf'
  const showInlineImage = contentKind === 'image' || isSvg
  const showSvgSource = isSvg && isText && svgMode === 'source'
  const previewUrl = useMemo(
    () => buildPreviewUrl(bytesB64, content, contentKind, contentType, isSvg),
    [bytesB64, content, contentKind, contentType, isSvg],
  )

  useEffect(() => {
    return () => {
      if (previewUrl) URL.revokeObjectURL(previewUrl)
    }
  }, [previewUrl])

  function handleDownload() {
    if (isSvg && content) {
      downloadBlob(basename(selectedPath), new Blob([content], { type: contentType || 'image/svg+xml' }))
      return
    }
    if (!bytesB64) return
    const bytes = decodeBase64(bytesB64)
    downloadBlob(basename(selectedPath), new Blob([bytes], { type: contentType || 'application/octet-stream' }))
  }

  function handleCopy() {
    if (!isText || !content) return
    void copyText(content)
  }

  const isCompact = variant === 'slideover'
  const showHeaderTitle = !isCompact
  const canDownload = !!bytesB64 || (isSvg && !!content)
  const canCopy = isText && !!content
  const hasActions = canDownload || canCopy

  return (
    <div className="flex flex-col h-full">
      <div className={isCompact ? 'px-4 py-3 border-b border-[var(--border)] shrink-0 flex flex-col gap-3' : ''} style={isCompact ? undefined : {
        padding: '12px 20px', borderBottom: '1px solid var(--border)', flexShrink: 0,
        display: 'flex', alignItems: 'center', gap: 10,
      }}>
        <div className={isCompact ? 'flex min-w-0 items-start justify-between gap-3' : 'flex-1 min-w-0'}>
          <div className="min-w-0">
            {showHeaderTitle && (
              <div className="mono truncate text-[0.8125rem] text-[var(--text-1)] font-semibold">
                {selectedPath}
              </div>
            )}
            {(content || contentKind !== 'text') && (
              <div className={cn('flex flex-wrap gap-x-2 gap-y-1 text-[0.625rem] text-[var(--text-3)] tabular-nums', showHeaderTitle && 'mt-1')}>
                {isText && <span>{lineCount} lines</span>}
                <span>{formatBytes(byteCount)}</span>
                {contentType && <span className="max-w-[220px] truncate">{contentType}</span>}
                <span className="text-[var(--accent)] font-medium">{getFileExt(selectedPath)}</span>
              </div>
            )}
          </div>
        </div>

        <div className={isCompact ? 'flex min-w-0 flex-wrap items-center gap-2' : 'flex items-center gap-2.5'}>
          {isText && !isMd && content && (
            <div className="flex min-w-[150px] flex-1 items-center gap-1.5 rounded-[var(--r-sm)] border border-[var(--border)] bg-[var(--bg-surface)] px-2 py-1.5 sm:flex-none">
              <Search size={11} className="shrink-0 text-[var(--text-3)]" />
              <input
                type="text"
                placeholder="Find..."
                aria-label="Search file content"
                value={contentSearch}
                onChange={e => setContentSearch(e.target.value)}
                className="min-w-0 flex-1 border-none bg-transparent text-[0.6875rem] text-[var(--text-1)] outline-none font-[inherit] sm:w-[120px]"
              />
            </div>
          )}

          {isSvg && isText && (
            <div className="flex items-center rounded-[var(--r-sm)] border border-[var(--border)] overflow-hidden">
              <button
                onClick={() => setSvgMode('preview')}
                className="px-2.5 py-1 text-[0.6875rem]"
                style={{ background: svgMode === 'preview' ? 'var(--bg-active)' : 'transparent', color: 'var(--text-1)' }}
              >
                Preview
              </button>
              <button
                onClick={() => setSvgMode('source')}
                className="px-2.5 py-1 text-[0.6875rem] border-l border-[var(--border)]"
                style={{ background: svgMode === 'source' ? 'var(--bg-active)' : 'transparent', color: 'var(--text-1)' }}
              >
                Source
              </button>
            </div>
          )}

          {hasActions && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 shrink-0"
                  aria-label="File actions"
                  title="File actions"
                >
                  <MoreHorizontal size={14} />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-[140px] text-xs">
                {canDownload && (
                  <DropdownMenuItem onSelect={handleDownload}>
                    <Download size={13} />
                    Download
                  </DropdownMenuItem>
                )}
                {canCopy && (
                  <DropdownMenuItem onSelect={handleCopy}>
                    <Copy size={13} />
                    Copy contents
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </div>

      <div
        className={cn(
          isCompact && 'flex-1 min-h-0 overflow-auto',
          isCompact && (isText && !isMd ? 'px-4 py-3' : 'p-4'),
        )}
        style={isCompact ? undefined : { flex: 1, minHeight: 0, overflow: 'auto', padding: isText && !isMd ? '12px 16px' : 16 }}
      >
        {isLoading ? (
          <div className="flex items-center justify-center h-full">
            <span className="spinner spinner-md" />
          </div>
        ) : error ? (
          <div
            className="text-xs text-[var(--red)] px-4 py-3 rounded-[var(--r-md)]"
            style={{
              background: 'color-mix(in srgb, var(--red) 8%, transparent)',
              border: '1px solid color-mix(in srgb, var(--red) 20%, transparent)',
            }}
          >
            Failed to load file contents.
          </div>
        ) : showInlineImage && previewUrl && !showSvgSource ? (
          <div className="flex items-center justify-center min-h-full">
            <img
              src={previewUrl}
              alt={basename(selectedPath)}
              className="max-w-full max-h-full object-contain rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)]"
            />
          </div>
        ) : showInlinePDF && previewUrl ? (
          <iframe
            src={previewUrl}
            title={basename(selectedPath)}
            className="w-full min-h-[70vh] border border-[var(--border)] rounded-[var(--r-md)] bg-[var(--bg-surface)]"
          />
        ) : !isText ? (
          <div className="flex flex-col gap-3 max-w-[520px] p-[18px] rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)]">
            <div className="text-sm font-semibold text-[var(--text-1)]">Binary preview unavailable</div>
            <div className="text-xs text-[var(--text-2)] leading-[1.6]">
              {truncated ? 'Preview/download unavailable in viewer due to size.' : 'This file type is detected correctly, but inline preview is not supported in v1.'}
            </div>
            <div className="mono text-[0.6875rem] text-[var(--text-2)] leading-[1.8]">
              <div>Name: {basename(selectedPath)}</div>
              <div>Type: {contentType || 'application/octet-stream'}</div>
              <div>Size: {formatBytes(byteCount)}</div>
              <div>Status: {truncated ? 'Truncated' : 'Download available'}</div>
            </div>
          </div>
        ) : isMd ? (
          <div className="md-prose text-[0.84375rem] text-[var(--text-1)] leading-[1.7] max-w-[680px]">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
          </div>
        ) : isText ? (
          <CodeView content={content} filePath={selectedPath} search={contentSearch} />
        ) : null}
        {truncated && (
          <div className="mt-3 text-[0.6875rem] text-[var(--text-3)] px-2.5 py-1.5 bg-[var(--bg-surface)] rounded-[var(--r-sm)] border border-[var(--border)] inline-flex items-center gap-1">
            Preview truncated at {bytesRead.toLocaleString()} bytes
          </div>
        )}
      </div>
    </div>
  )
}
