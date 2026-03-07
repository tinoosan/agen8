import { useEffect, useState, useMemo, useRef } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { useStore } from '../lib/store'
import { rpcCall } from '../lib/rpc'
import { X, FileText, File, FileCode, Copy, Check, Search, ChevronLeft, ExternalLink } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useArtifactFiles } from '../hooks/useArtifactFiles'
import type { ArtifactGetResult, ArtifactNode } from '../lib/types'

interface ArtifactsSlideOverProps {
  threadId: string | null
  teamId: string
}

function getFileIcon(path: string, size = 14) {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  if (['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'java', 'c', 'cpp', 'h'].includes(ext)) return <FileCode size={size} />
  if (['md', 'txt', 'json', 'yaml', 'yml', 'toml', 'xml', 'html', 'css'].includes(ext)) return <FileText size={size} />
  return <File size={size} />
}

function getFileExt(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ext ? `.${ext}` : ''
}

function isMarkdownFile(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ext === 'md' || ext === 'markdown'
}

function isCodeFile(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'java', 'c', 'cpp', 'h', 'json', 'yaml', 'yml', 'toml', 'xml', 'html', 'css', 'sql', 'sh', 'bash', 'zsh'].includes(ext)
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function basename(path: string): string {
  const parts = path.split('/')
  return parts[parts.length - 1] || path
}

/* ── Copy button ───────────────────────────────── */

function CopyButton({ text, style }: { text: string; style?: React.CSSProperties }) {
  const [copied, setCopied] = useState(false)
  function handleCopy() {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <button
      onClick={handleCopy}
      title="Copy to clipboard"
      className="btn-surface"
      style={{
        display: 'flex', alignItems: 'center', gap: 5,
        padding: '4px 10px', fontSize: 11, fontWeight: 500,
        color: copied ? 'var(--green)' : undefined,
        transition: 'color 0.15s',
        ...style,
      }}
    >
      {copied ? <><Check size={11} /> Copied</> : <><Copy size={11} /> Copy</>}
    </button>
  )
}

/* ── Line-numbered code view ───────────────────── */

function CodeView({ content, search }: { content: string; search: string }) {
  const lines = content.split('\n')
  const searchLower = search.toLowerCase()

  return (
    <div className="mono" style={{
      fontSize: 12, lineHeight: 1.7,
      background: 'var(--bg-surface)',
      border: '1px solid var(--border)',
      borderRadius: 'var(--r-md)',
      overflow: 'auto',
    }}>
      <table style={{ borderCollapse: 'collapse', width: '100%' }}>
        <tbody>
          {lines.map((line, i) => {
            const isMatch = searchLower && line.toLowerCase().includes(searchLower)
            return (
              <tr key={i} style={{
                background: isMatch ? 'var(--amber-dim)' : undefined,
              }}>
                <td style={{
                  padding: '0 12px 0 10px',
                  textAlign: 'right',
                  userSelect: 'none',
                  color: 'var(--text-3)',
                  opacity: 0.5,
                  fontSize: 10,
                  width: 1,
                  whiteSpace: 'nowrap',
                  borderRight: '1px solid var(--border)',
                  verticalAlign: 'top',
                }}>
                  {i + 1}
                </td>
                <td style={{
                  padding: '0 14px',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  color: 'var(--text-1)',
                }}>
                  {line || '\u200b'}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

/* ── Content panel (always mounted, visibility toggled) ── */

function ContentPanel({ file, content, isLoading, error, truncated, bytesRead }: {
  file: ArtifactNode | null
  content: string
  isLoading: boolean
  error: boolean
  truncated: boolean
  bytesRead: number
}) {
  const [contentSearch, setContentSearch] = useState('')

  // Reset search when file changes
  const fileKey = file?.nodeKey ?? ''
  const prevKeyRef = useRef(fileKey)
  if (fileKey !== prevKeyRef.current) {
    prevKeyRef.current = fileKey
    if (contentSearch) setContentSearch('')
  }

  if (!file) return null

  const selectedPath = file.displayName ?? file.vpath ?? file.label
  const isCode = isCodeFile(selectedPath)
  const isMd = isMarkdownFile(selectedPath)
  const lineCount = content ? content.split('\n').length : 0
  const byteCount = bytesRead || (content ? new Blob([content]).size : 0)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Header */}
      <div style={{
        padding: '10px 16px', borderBottom: '1px solid var(--border)', flexShrink: 0,
        display: 'flex', alignItems: 'center', gap: 10,
      }}>
        <div style={{ color: 'var(--accent)', display: 'flex', alignItems: 'center', flexShrink: 0 }}>
          {getFileIcon(selectedPath, 16)}
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="mono truncate" style={{
            fontSize: 13, color: 'var(--text-1)', fontWeight: 600,
          }}>
            {basename(selectedPath)}
          </div>
          {content && (
            <div style={{
              fontSize: 10, color: 'var(--text-3)', marginTop: 1,
              display: 'flex', gap: 10, fontVariantNumeric: 'tabular-nums',
            }}>
              <span>{lineCount} lines</span>
              <span>{formatBytes(byteCount)}</span>
              <span style={{ color: 'var(--accent)', fontWeight: 500 }}>
                {getFileExt(selectedPath)}
              </span>
            </div>
          )}
        </div>

        {isCode && content && (
          <div style={{
            display: 'flex', alignItems: 'center', gap: 4,
            background: 'var(--bg-surface)', border: '1px solid var(--border)',
            borderRadius: 'var(--r-sm)', padding: '3px 8px',
          }}>
            <Search size={10} style={{ color: 'var(--text-3)' }} />
            <input
              type="text"
              placeholder="Find…"
              value={contentSearch}
              onChange={e => setContentSearch(e.target.value)}
              style={{
                border: 'none', outline: 'none', background: 'transparent',
                width: 100, fontSize: 11, color: 'var(--text-1)', fontFamily: 'inherit',
              }}
            />
          </div>
        )}

        {content && <CopyButton text={content} />}
      </div>

      {/* Content */}
      <div style={{ flex: 1, minHeight: 0, overflow: 'auto', padding: 16 }}>
        {isLoading ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
            <span className="spinner spinner-md" />
          </div>
        ) : error ? (
          <div style={{
            fontSize: 12, color: 'var(--red)',
            background: 'color-mix(in srgb, var(--red) 8%, transparent)',
            padding: '12px 16px', borderRadius: 'var(--r-md)',
            border: '1px solid color-mix(in srgb, var(--red) 20%, transparent)',
          }}>
            Failed to load file contents.
          </div>
        ) : isMd ? (
          <div className="md-prose" style={{
            fontSize: 13.5, color: 'var(--text-1)',
            lineHeight: 1.7, maxWidth: 680,
          }}>
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {content}
            </ReactMarkdown>
          </div>
        ) : isCode ? (
          <CodeView content={content} search={contentSearch} />
        ) : (
          <pre
            className="mono"
            style={{
              margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word',
              fontSize: 12, lineHeight: 1.7, color: 'var(--text-1)',
              background: 'var(--bg-surface)', border: '1px solid var(--border)',
              borderRadius: 'var(--r-md)', padding: 14,
            }}
          >
            {content}
          </pre>
        )}
        {truncated && (
          <div style={{
            marginTop: 12, fontSize: 11, color: 'var(--text-3)',
            padding: '6px 10px', background: 'var(--bg-surface)',
            borderRadius: 'var(--r-sm)', border: '1px solid var(--border)',
            display: 'inline-flex', alignItems: 'center', gap: 4,
          }}>
            ⚠ Preview truncated at {bytesRead.toLocaleString()} bytes
          </div>
        )}
      </div>
    </div>
  )
}

/* ── Main Component ────────────────────────────── */

export default function ArtifactsSlideOver({ threadId, teamId }: ArtifactsSlideOverProps) {
  const { setArtifactsOpen } = useStore()

  const query = useArtifactFiles(threadId, teamId)
  const artifacts = query.data ?? []
  const [fileSearch, setFileSearch] = useState('')
  const [selectedFile, setSelectedFile] = useState<ArtifactNode | null>(null)

  // Stable query key to prevent refetch flickering
  const selectedKey = selectedFile?.nodeKey ?? null

  useEffect(() => {
    if (artifacts.length === 0) setSelectedFile(null)
  }, [artifacts])

  // Single stable query for the selected file — keepPreviousData prevents flash
  const previewQuery = useQuery<ArtifactGetResult>({
    queryKey: ['artifact.get', threadId, teamId, selectedKey],
    queryFn: async () => rpcCall<ArtifactGetResult>('artifact.get', {
      threadId: threadId ?? undefined,
      teamId,
      artifactId: selectedFile?.artifactId,
      vpath: selectedFile?.vpath,
      maxBytes: 512 * 1024,
    }),
    enabled: !!threadId && !!selectedFile,
    retry: false,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  })

  const filteredArtifacts = useMemo(() => {
    if (!fileSearch) return artifacts
    const lower = fileSearch.toLowerCase()
    return artifacts.filter(a => {
      const path = (a.displayName ?? a.vpath ?? a.artifactId ?? '').toLowerCase()
      return path.includes(lower)
    })
  }, [artifacts, fileSearch])

  const isOpen = !!selectedFile
  // Narrow width when just file list, wide when reading
  const panelWidth = isOpen ? 900 : 340

  function handleSelectFile(file: ArtifactNode) {
    setSelectedFile(file)
  }

  function handleClose() {
    setSelectedFile(null)
  }

  return (
    <div
      className="slide-over-backdrop animate-slide-in-right"
      onClick={() => setArtifactsOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: panelWidth, maxWidth: '92vw', height: '100%',
          background: 'var(--bg-panel)',
          borderLeft: '1px solid var(--border)',
          display: 'flex', flexDirection: 'column',
          boxShadow: '-12px 0 48px rgba(0,0,0,0.4)',
          transition: 'width 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
          willChange: 'width',
        }}
      >
        {/* Header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '14px 16px',
          borderBottom: '1px solid var(--border)',
          flexShrink: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {isOpen && (
              <button
                className="btn-ghost"
                onClick={handleClose}
                style={{ padding: 4, marginRight: 2 }}
                title="Back to file list"
              >
                <ChevronLeft size={16} />
              </button>
            )}
            <div style={{
              width: 28, height: 28, borderRadius: 8,
              background: 'linear-gradient(135deg, var(--accent), color-mix(in srgb, var(--accent) 70%, var(--green)))',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}>
              <FileText size={14} color="#fff" />
            </div>
            <span style={{ fontWeight: 700, fontSize: 15, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
              Files
            </span>
            {artifacts.length > 0 && (
              <span style={{
                fontSize: 11, color: 'var(--text-3)',
                background: 'var(--bg-elevated)',
                border: '1px solid var(--border)',
                borderRadius: 999, padding: '0 7px',
                fontVariantNumeric: 'tabular-nums',
              }}>
                {artifacts.length}
              </span>
            )}
          </div>
          <button className="btn-ghost" onClick={() => setArtifactsOpen(false)} aria-label="Close">
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, minHeight: 0, display: 'flex' }}>
          {/* File list — always visible */}
          <div style={{
            width: isOpen ? 240 : '100%',
            flexShrink: 0,
            minHeight: 0,
            borderRight: isOpen ? '1px solid var(--border)' : 'none',
            display: 'flex', flexDirection: 'column',
            transition: 'width 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
            overflow: 'hidden',
          }}>
            {/* File search */}
            {artifacts.length > 3 && (
              <div style={{ padding: '8px', flexShrink: 0 }}>
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  background: 'var(--bg-surface)', border: '1px solid var(--border)',
                  borderRadius: 'var(--r-sm)', padding: '5px 8px',
                }}>
                  <Search size={11} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
                  <input
                    type="text"
                    placeholder="Filter…"
                    value={fileSearch}
                    onChange={e => setFileSearch(e.target.value)}
                    style={{
                      border: 'none', outline: 'none', background: 'transparent',
                      flex: 1, fontSize: 11, color: 'var(--text-1)', fontFamily: 'inherit',
                      minWidth: 0,
                    }}
                  />
                </div>
              </div>
            )}

            {/* File entries */}
            <div style={{ flex: 1, overflowY: 'auto', padding: '4px 8px' }}>
              {query.isLoading ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6, padding: 12 }}>
                  {[1, 2, 3].map(i => <div key={i} className="skeleton" style={{ width: '100%', height: 44, borderRadius: 'var(--r-md)' }} />)}
                </div>
              ) : filteredArtifacts.length === 0 ? (
                <div style={{
                  display: 'flex', flexDirection: 'column', alignItems: 'center',
                  padding: '48px 20px', textAlign: 'center', gap: 10,
                }}>
                  <div style={{
                    width: 48, height: 48, borderRadius: 12,
                    background: 'var(--bg-surface)', border: '1px solid var(--border)',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                  }}>
                    <File size={20} style={{ color: 'var(--text-3)' }} />
                  </div>
                  <div style={{ color: 'var(--text-3)', fontSize: 13 }}>
                    {fileSearch ? 'No files match' : 'No files yet'}
                  </div>
                  {!fileSearch && (
                    <div style={{ color: 'var(--text-3)', fontSize: 11, lineHeight: 1.5 }}>
                      Files will appear here as agents generate them
                    </div>
                  )}
                </div>
              ) : (
                filteredArtifacts.map((a, i) => {
                  const path = a.displayName ?? a.vpath ?? a.artifactId ?? `artifact-${i}`
                  const ext = getFileExt(path)
                  const isSelected = selectedFile?.nodeKey === a.nodeKey
                  return (
                    <div
                      key={a.nodeKey ?? a.artifactId ?? i}
                      className={isSelected ? '' : 'row-hover'}
                      onClick={() => handleSelectFile(a)}
                      style={{
                        display: 'flex', alignItems: 'center', gap: 10,
                        padding: '10px 12px',
                        cursor: 'pointer',
                        borderRadius: 'var(--r-md)',
                        background: isSelected ? 'var(--bg-active)' : 'transparent',
                        borderLeft: isSelected ? '2px solid var(--accent)' : '2px solid transparent',
                        marginBottom: 2,
                        transition: 'background 0.12s, border-color 0.12s',
                      }}
                    >
                      <div style={{
                        color: isSelected ? 'var(--accent)' : 'var(--text-3)',
                        flexShrink: 0, display: 'flex', alignItems: 'center',
                      }}>
                        {getFileIcon(path)}
                      </div>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div className="truncate" style={{
                          fontSize: 12, color: 'var(--text-1)', fontWeight: isSelected ? 600 : 500,
                          letterSpacing: '-0.01em',
                        }}>
                          {basename(path)}
                        </div>
                        <div style={{ fontSize: 10, color: 'var(--text-3)', marginTop: 2, display: 'flex', gap: 6 }}>
                          {a.role && <span>{a.role}</span>}
                          {ext && <span style={{ fontWeight: 500, color: 'var(--accent)', opacity: 0.7 }}>{ext}</span>}
                        </div>
                      </div>
                      {!isOpen && (
                        <ExternalLink size={12} style={{ color: 'var(--text-3)', flexShrink: 0, opacity: 0.4 }} />
                      )}
                    </div>
                  )
                })
              )}
            </div>
          </div>

          {/* Content panel — slides in from right */}
          {isOpen && (
            <div style={{ flex: 1, minHeight: 0, minWidth: 0 }}>
              <ContentPanel
                file={selectedFile}
                content={previewQuery.data?.content ?? ''}
                isLoading={previewQuery.isLoading && !previewQuery.isPlaceholderData}
                error={!!previewQuery.error}
                truncated={previewQuery.data?.truncated ?? false}
                bytesRead={previewQuery.data?.bytesRead ?? 0}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
