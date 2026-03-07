import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useStore } from '../lib/store'
import { rpcCall } from '../lib/rpc'
import { X, FileText, File, FileCode } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useArtifactFiles } from '../hooks/useArtifactFiles'
import type { ArtifactGetResult, ArtifactNode } from '../lib/types'

interface ArtifactsSlideOverProps {
  threadId: string | null
  teamId: string
}

function getFileIcon(path: string) {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  if (['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs'].includes(ext)) return <FileCode size={14} />
  if (['md', 'txt', 'json', 'yaml', 'yml'].includes(ext)) return <FileText size={14} />
  return <File size={14} />
}

function getFileExt(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ext ? `.${ext}` : ''
}

function isMarkdownFile(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ext === 'md' || ext === 'markdown'
}

export default function ArtifactsSlideOver({ threadId, teamId }: ArtifactsSlideOverProps) {
  const { setArtifactsOpen } = useStore()

  const query = useArtifactFiles(threadId, teamId)
  const artifacts = query.data ?? []
  const [selectedFile, setSelectedFile] = useState<ArtifactNode | null>(null)

  useEffect(() => {
    if (artifacts.length === 0) {
      setSelectedFile(null)
      return
    }
    setSelectedFile((prev) => {
      if (prev && artifacts.some((artifact) => artifact.nodeKey === prev.nodeKey)) {
        return prev
      }
      return artifacts[0]
    })
  }, [artifacts])

  const previewQuery = useQuery<ArtifactGetResult>({
    queryKey: ['artifact.get', threadId, teamId, selectedFile?.nodeKey ?? null],
    queryFn: async () => rpcCall<ArtifactGetResult>('artifact.get', {
      threadId: threadId ?? undefined,
      teamId,
      artifactId: selectedFile?.artifactId,
      vpath: selectedFile?.vpath,
      maxBytes: 256 * 1024,
    }),
    enabled: !!threadId && !!selectedFile,
    retry: false,
  })

  return (
    <div
      className="slide-over-backdrop animate-slide-in-right"
      onClick={() => setArtifactsOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 520, height: '100%',
          background: 'var(--bg-panel)',
          borderLeft: '1px solid var(--border)',
          display: 'flex', flexDirection: 'column',
          boxShadow: '-12px 0 48px rgba(0,0,0,0.4)',
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
            <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
              Files
            </span>
            {artifacts.length > 0 && (
              <span style={{
                fontSize: 11, color: 'var(--text-3)',
                background: 'var(--bg-elevated)',
                border: '1px solid var(--border)',
                borderRadius: 999, padding: '0 6px',
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

        {/* Artifact list */}
        <div style={{ flex: '0 0 40%', minHeight: 0, overflowY: 'auto', padding: '8px' }}>
          {query.isLoading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, padding: 12 }}>
              {[1, 2, 3].map(i => <div key={i} className="skeleton" style={{ width: '100%', height: 36, borderRadius: 'var(--r-md)' }} />)}
            </div>
          ) : artifacts.length === 0 ? (
            <div style={{
              display: 'flex', flexDirection: 'column', alignItems: 'center',
              padding: '40px 20px', textAlign: 'center', gap: 8,
            }}>
              <File size={24} style={{ color: 'var(--text-3)' }} />
              <div style={{ color: 'var(--text-3)', fontSize: 13 }}>No files yet</div>
              <div style={{ color: 'var(--text-3)', fontSize: 11, lineHeight: 1.5 }}>
                Files will appear here as agents generate them
              </div>
            </div>
          ) : (
            artifacts.map((a, i) => {
              const path = a.displayName ?? a.vpath ?? a.artifactId ?? `artifact-${i}`
              const ext = getFileExt(path)
              return (
                <div
                  key={a.nodeKey ?? a.artifactId ?? i}
                  className="row-hover"
                  onClick={() => setSelectedFile(a)}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 10,
                    padding: '9px 10px',
                    cursor: 'pointer',
                    borderRadius: 'var(--r-md)',
                    background: selectedFile?.nodeKey === a.nodeKey ? 'var(--bg-elevated)' : 'transparent',
                  }}
                >
                  <div style={{ color: 'var(--text-3)', flexShrink: 0, display: 'flex', alignItems: 'center' }}>
                    {getFileIcon(path)}
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="truncate" style={{
                      fontSize: 12, color: 'var(--text-1)', fontWeight: 500,
                      letterSpacing: '-0.01em',
                    }}>
                      {path}
                    </div>
                    <div style={{ fontSize: 10, color: 'var(--text-3)', marginTop: 1, display: 'flex', gap: 6 }}>
                      {a.role && <span>{a.role}</span>}
                      {ext && <span style={{ fontWeight: 500, color: 'var(--accent)', opacity: 0.7 }}>{ext}</span>}
                    </div>
                  </div>
                </div>
              )
            })
          )}
        </div>

        <div style={{ borderTop: '1px solid var(--border)', flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
          <div style={{ padding: '10px 12px', borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
            <div style={{ fontSize: 11, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>Preview</div>
            <div className="truncate" style={{ fontSize: 12, color: 'var(--text-1)', marginTop: 4, fontWeight: 500 }}>
              {selectedFile ? (selectedFile.displayName ?? selectedFile.vpath ?? selectedFile.label) : 'No file selected'}
            </div>
          </div>
          <div style={{ flex: 1, minHeight: 0, overflow: 'auto', padding: 12 }}>
            {!selectedFile ? (
              <div style={{ fontSize: 12, color: 'var(--text-3)' }}>Select a file to view its contents.</div>
            ) : previewQuery.isLoading ? (
              <div style={{ fontSize: 12, color: 'var(--text-3)' }}>Loading preview…</div>
            ) : previewQuery.error ? (
              <div style={{ fontSize: 12, color: 'var(--red)' }}>Failed to load file contents.</div>
            ) : (
              <>
                {selectedFile && isMarkdownFile(selectedFile.displayName ?? selectedFile.vpath ?? selectedFile.label) ? (
                  <div className="md-prose" style={{ fontSize: 12.5, color: 'var(--text-2)' }}>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>
                      {previewQuery.data?.content ?? ''}
                    </ReactMarkdown>
                  </div>
                ) : (
                  <pre
                    className="mono"
                    style={{
                      margin: 0,
                      whiteSpace: 'pre-wrap',
                      wordBreak: 'break-word',
                      fontSize: 11.5,
                      lineHeight: 1.6,
                      color: 'var(--text-1)',
                      background: 'var(--bg-surface)',
                      border: '1px solid var(--border)',
                      borderRadius: 'var(--r-md)',
                      padding: 12,
                    }}
                  >
                    {previewQuery.data?.content ?? ''}
                  </pre>
                )}
                {previewQuery.data?.truncated && (
                  <div style={{ marginTop: 10, fontSize: 11, color: 'var(--text-3)' }}>
                    Preview truncated at {previewQuery.data.bytesRead.toLocaleString()} bytes.
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
