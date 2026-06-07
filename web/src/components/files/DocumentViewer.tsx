import { useState, useCallback, useRef, useEffect, useMemo, type ComponentProps } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Copy, Download, FileText, AlertTriangle, Pencil, Eye, Save, X, MoreHorizontal } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { toast } from 'sonner'
import { copyText } from '@/lib/utils'
import type { ArtifactNode, ArtifactGetResult } from '../../lib/types'
import { basename, displayVirtualPathPart, downloadBlob, formatBytes } from './filePreviewUtils'
import { rpcCall } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
import { useNavigation } from '../../lib/routing'

interface DocumentViewerProps {
  file: ArtifactNode
  preview: ArtifactGetResult | undefined
  isLoading: boolean
  error: boolean
  variant?: 'page' | 'slideover'
}

function Placeholder({ icon: Icon, title, detail }: { icon: typeof FileText; title: string; detail: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-1.5 px-6">
      <Icon className="h-5 w-5 mb-1" style={{ color: 'var(--text-3)' }} />
      <p className="text-[0.8125rem] font-medium" style={{ color: 'var(--text-1)' }}>{title}</p>
      <p className="text-[0.75rem] text-center max-w-xs" style={{ color: 'var(--text-3)' }}>{detail}</p>
    </div>
  )
}

function textToBase64(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  const chunkSize = 0x8000
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize))
  }
  return btoa(binary)
}

export default function DocumentViewer({ file, preview, isLoading, error, variant = 'page' }: DocumentViewerProps) {
  const { projectId, focusedProjectRoot } = useNavigation()
  const queryClient = useQueryClient()
  const isSlideover = variant === 'slideover'
  const filePath = file.vpath ?? file.diskPath ?? file.label ?? ''
  const fileName = basename(filePath)
  const content = preview?.content ?? ''
  const breadcrumbParts = useMemo(() => filePath.split('/').filter(Boolean), [filePath])

  // Custom code renderer with syntax highlighting
  const mdComponents: ComponentProps<typeof ReactMarkdown>['components'] = useMemo(() => ({
    code({ className, children, ...props }) {
      const match = /language-(\w+)/.exec(className || '')
      const codeStr = String(children).replace(/\n$/, '')
      if (match) {
        return (
          <SyntaxHighlighter
            style={vscDarkPlus}
            language={match[1]}
            PreTag="div"
            customStyle={{
              margin: 0,
              padding: 0,
              background: 'transparent',
              border: 'none',
              borderRadius: 0,
              fontSize: 'inherit',
              lineHeight: 'inherit',
            }}
          >
            {codeStr}
          </SyntaxHighlighter>
        )
      }
      return <code className={className} {...props}>{children}</code>
    },
  }), [])

  const [isEditing, setIsEditing] = useState(false)
  const [editContent, setEditContent] = useState('')
  const [saving, setSaving] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // When entering edit mode, snapshot current content
  const handleStartEdit = useCallback(() => {
    setEditContent(content)
    setIsEditing(true)
  }, [content])

  const handleCancelEdit = useCallback(() => {
    setIsEditing(false)
    setEditContent('')
  }, [])

  const handleSave = useCallback(async () => {
    if (!focusedProjectRoot || !filePath) return
    setSaving(true)
    try {
      const bytesB64 = textToBase64(editContent)
      await rpcCall('files.upload', {
        projectId: projectId ?? undefined,
        projectRoot: focusedProjectRoot,
        path: filePath,
        bytesB64,
      })
      await queryClient.invalidateQueries({ queryKey: qk.filePreview(projectId, focusedProjectRoot, filePath) })
      setIsEditing(false)
      toast.success('Document saved')
    } catch (err) {
      toast.error(`Save failed: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      setSaving(false)
    }
  }, [editContent, filePath, focusedProjectRoot, projectId, queryClient])

  // Auto-resize textarea
  useEffect(() => {
    if (isEditing && textareaRef.current) {
      const el = textareaRef.current
      el.style.height = 'auto'
      el.style.height = `${el.scrollHeight}px`
    }
  }, [isEditing, editContent])

  const handleExportHTML = useCallback(() => {
    const src = isEditing ? editContent : content
    const fullHtml = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>${fileName}</title>
<style>body{font-family:system-ui,sans-serif;max-width:680px;margin:2rem auto;padding:0 1rem;line-height:1.6;color:#333}
h1,h2,h3{margin-top:1.5em}code{background:#f5f5f5;padding:2px 6px;border-radius:3px;font-size:0.9em}
pre{background:#f5f5f5;padding:1rem;border-radius:6px;overflow-x:auto}table{border-collapse:collapse;width:100%}
td,th{border:1px solid #ddd;padding:8px}th{background:#f5f5f5}blockquote{border-left:3px solid #3B82F6;margin:1em 0;padding:.5em 1em;color:#666}</style>
</head><body><article>${src}</article></body></html>`
    const blob = new Blob([fullHtml], { type: 'text/html;charset=utf-8' })
    downloadBlob(fileName.replace(/\.\w+$/, '.html'), blob)
  }, [content, editContent, isEditing, fileName])

  const handleExportMD = useCallback(() => {
    const src = isEditing ? editContent : content
    const blob = new Blob([src], { type: 'text/markdown;charset=utf-8' })
    downloadBlob(fileName, blob)
  }, [content, editContent, isEditing, fileName])

  const handleCopyContent = useCallback(async () => {
    const src = isEditing ? editContent : content
    if (!src) return
    try {
      await copyText(src)
      toast.success('File content copied')
    } catch (err) {
      toast.error(`Copy failed: ${err instanceof Error ? err.message : String(err)}`)
    }
  }, [content, editContent, isEditing])

  if (isLoading) {
    return (
      <div
        className="flex flex-col h-full overflow-hidden"
        style={
          isSlideover
            ? { background: 'transparent' }
            : { borderRadius: 'var(--r-lg)', border: '1px solid var(--border)', background: 'var(--bg-panel)' }
        }
      >
        <div
          className={isSlideover ? 'h-10 flex items-center gap-3 px-4 border-b' : 'h-9 flex items-center gap-3 px-3 border-b'}
          style={{ borderColor: 'color-mix(in srgb, var(--border) 55%, transparent)' }}
        >
          <Skeleton className="h-3 w-32" />
          <div className="flex-1" />
          <Skeleton className="h-6 w-16" />
        </div>
        <div className={isSlideover ? 'flex-1 px-6 py-5 max-w-[760px] mx-auto w-full' : 'flex-1 px-6 py-6 max-w-[680px]'}>
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

  if (error) return <Placeholder icon={AlertTriangle} title="Failed to load file" detail="The file could not be fetched from the workspace." />
  if (!content.trim() && !isEditing) return <Placeholder icon={FileText} title="No document content" detail="This file appears to be empty." />

  return (
    <div
      className="flex flex-col h-full overflow-hidden"
      style={
        isSlideover
          ? { background: 'transparent' }
          : { borderRadius: 'var(--r-lg)', border: '1px solid var(--border)', background: 'var(--bg-panel)' }
      }
    >
      {/* Toolbar */}
      <div
        className={isSlideover ? 'flex items-center gap-2 h-10 px-4 shrink-0 border-b' : 'flex items-center gap-2 h-9 px-3 shrink-0 border-b'}
        style={{ borderColor: 'color-mix(in srgb, var(--border) 55%, transparent)' }}
      >
        {isSlideover ? (
          <div className="flex-1 min-w-0 overflow-hidden">
            <Breadcrumb>
              <BreadcrumbList className="flex-nowrap overflow-hidden">
                {breadcrumbParts.map((part, index) => {
                  const isLast = index === breadcrumbParts.length - 1
                  return (
                    <div key={`${part}-${index}`} className="flex items-center min-w-0">
                      <BreadcrumbItem className="min-w-0">
                        {isLast ? (
                          <BreadcrumbPage className="max-w-[190px] truncate text-xs">
                            {displayVirtualPathPart(part, index, focusedProjectRoot)}
                          </BreadcrumbPage>
                        ) : (
                          <span className="max-w-[120px] truncate text-xs text-[var(--text-3)]">
                            {displayVirtualPathPart(part, index, focusedProjectRoot)}
                          </span>
                        )}
                      </BreadcrumbItem>
                      {!isLast && <BreadcrumbSeparator />}
                    </div>
                  )
                })}
              </BreadcrumbList>
            </Breadcrumb>
          </div>
        ) : (
          <>
            <FileText className="h-3.5 w-3.5 shrink-0" style={{ color: 'var(--text-3)' }} />
            <span className="text-[0.75rem] font-medium truncate" style={{ color: 'var(--text-2)' }}>{fileName}</span>
            {preview?.fileSize != null && (
              <span className="text-[0.6875rem] shrink-0" style={{ color: 'var(--text-4)' }}>{formatBytes(preview.fileSize)}</span>
            )}
          </>
        )}
        <div className="flex-1 min-w-4" />

        {isEditing ? (
          <>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[0.6875rem] gap-1 shrink-0"
              onClick={handleCancelEdit}
              disabled={saving}
            >
              <X className="h-2.5 w-2.5" /> Cancel
            </Button>
            <Button
              size="sm"
              className="h-6 px-2.5 text-[0.6875rem] gap-1 shrink-0"
              onClick={handleSave}
              disabled={saving}
            >
              <Save className="h-2.5 w-2.5" /> {saving ? 'Saving...' : 'Save'}
            </Button>
          </>
        ) : (
          <>
            {isSlideover ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 shrink-0 text-[var(--text-2)] hover:text-[var(--text-1)]"
                    aria-label="Document actions"
                  >
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="min-w-[170px]">
                  <DropdownMenuItem onClick={handleStartEdit}>
                    <Pencil className="mr-2 h-3.5 w-3.5" />
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => { void handleCopyContent() }}>
                    <Copy className="mr-2 h-3.5 w-3.5" />
                    Copy content
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={handleExportMD}>
                    <Download className="mr-2 h-3.5 w-3.5" />
                    Download MD
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={handleExportHTML}>
                    <Download className="mr-2 h-3.5 w-3.5" />
                    Download HTML
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-[0.6875rem] gap-1 shrink-0"
                  onClick={handleStartEdit}
                >
                  <Pencil className="h-2.5 w-2.5" /> Edit
                </Button>
                <Button variant="ghost" size="sm" className="h-6 px-2 text-[0.6875rem] gap-1 shrink-0" onClick={() => { void handleCopyContent() }}>
                  <Copy className="h-2.5 w-2.5" /> Copy content
                </Button>
                <div className="w-px h-3.5 shrink-0" style={{ background: 'var(--border)' }} />
                <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[0.6875rem] gap-1 shrink-0" onClick={handleExportMD}>
                  <Download className="h-2.5 w-2.5" /> MD
                </Button>
                <Button variant="ghost" size="sm" className="h-6 px-1.5 text-[0.6875rem] gap-1 shrink-0" onClick={handleExportHTML}>
                  <Download className="h-2.5 w-2.5" /> HTML
                </Button>
              </>
            )}
          </>
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-h-0 overflow-auto">
        {isEditing ? (
          <div className="h-full">
            <textarea
              ref={textareaRef}
              value={editContent}
              onChange={(e) => setEditContent(e.target.value)}
              className="w-full h-full resize-none border-0 outline-none p-6 text-[0.8125rem] leading-[1.7]"
              style={{
                background: 'transparent',
                color: 'var(--text-1)',
                fontFamily: 'var(--font-mono)',
                caretColor: 'var(--accent)',
              }}
              spellCheck={false}
            />
          </div>
        ) : (
          <div
            className={isSlideover
              ? 'md-prose text-[0.84375rem] leading-[1.7] max-w-[760px] mx-auto py-5 px-6'
              : 'md-prose text-[0.84375rem] leading-[1.7] max-w-[680px] mx-auto py-6 px-6'}
            style={{ color: 'var(--text-1)' }}
          >
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={mdComponents}>{content}</ReactMarkdown>
          </div>
        )}
      </div>

      {/* Edit mode footer */}
      {isEditing && (
        <div
          className="flex items-center justify-between h-7 px-3 text-[0.6875rem] shrink-0 border-t"
          style={{ color: 'var(--text-3)', borderColor: 'var(--border)' }}
        >
          <span>Editing markdown source</span>
          <Button
            variant="ghost"
            size="sm"
            className="h-5 px-1.5 text-[0.625rem] gap-1"
            onClick={() => {
              // Preview: toggle back to read with current edit content
              // (not saved yet — just a preview)
            }}
          >
            <Eye className="h-2.5 w-2.5" /> Preview
          </Button>
        </div>
      )}
    </div>
  )
}
