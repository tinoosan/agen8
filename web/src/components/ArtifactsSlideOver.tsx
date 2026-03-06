import { useQuery } from '@tanstack/react-query'
import { useStore } from '../lib/store'
import { rpcCall } from '../lib/rpc'
import { X, FileText, File, FileCode } from 'lucide-react'
import type { Artifact } from '../lib/types'

interface ArtifactsSlideOverProps {
  teamId: string
}

function formatBytes(bytes?: number): string {
  if (!bytes) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
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

export default function ArtifactsSlideOver({ teamId }: ArtifactsSlideOverProps) {
  const { setArtifactsOpen } = useStore()

  const query = useQuery<Artifact[]>({
    queryKey: ['artifact.list', teamId],
    queryFn: async () => {
      const res = await rpcCall<{ artifacts: Artifact[] }>('artifact.list', { teamId })
      return res.artifacts ?? []
    },
    enabled: !!teamId,
    refetchInterval: 5000,
  })

  const artifacts = query.data ?? []

  return (
    <div
      className="slide-over-backdrop animate-slide-in-right"
      onClick={() => setArtifactsOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 380, height: '100%',
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
        <div style={{ flex: 1, overflowY: 'auto', padding: '8px' }}>
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
              const path = a.vpath ?? a.artifactId ?? `artifact-${i}`
              const ext = getFileExt(path)
              return (
                <div
                  key={a.artifactId ?? i}
                  className="row-hover"
                  style={{
                    display: 'flex', alignItems: 'center', gap: 10,
                    padding: '9px 10px',
                    cursor: 'default',
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
                      <span>{formatBytes(a.sizeBytes)}</span>
                      {ext && <span style={{ fontWeight: 500, color: 'var(--accent)', opacity: 0.7 }}>{ext}</span>}
                    </div>
                  </div>
                </div>
              )
            })
          )}
        </div>
      </div>
    </div>
  )
}
