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
      className="animate-slide-in-right"
      style={{
        position: 'absolute', inset: 0, zIndex: 50,
        display: 'flex', justifyContent: 'flex-end',
        background: 'rgba(0,0,0,0.45)',
        backdropFilter: 'blur(3px)',
      }}
      onClick={() => setArtifactsOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 360, height: '100%',
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
        }}>
          <div>
            <span style={{ fontWeight: 600, fontSize: 14, color: 'var(--text-1)', letterSpacing: '-0.02em' }}>
              Files
            </span>
            {artifacts.length > 0 && (
              <span style={{ marginLeft: 8, fontSize: 11, color: 'var(--text-3)' }}>
                {artifacts.length} artifact{artifacts.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>
          <button
            onClick={() => setArtifactsOpen(false)}
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              color: 'var(--text-3)', padding: 5, borderRadius: 'var(--r-md)',
              display: 'flex', alignItems: 'center',
              transition: 'color 0.1s, background 0.1s',
            }}
            onMouseEnter={e => {
              e.currentTarget.style.color = 'var(--text-1)'
              e.currentTarget.style.background = 'var(--bg-hover)'
            }}
            onMouseLeave={e => {
              e.currentTarget.style.color = 'var(--text-3)'
              e.currentTarget.style.background = 'transparent'
            }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Artifact list */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '8px' }}>
          {artifacts.length === 0 ? (
            <div style={{ padding: 32, textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
              No files yet
            </div>
          ) : (
            artifacts.map((a, i) => {
              const path = a.vpath ?? a.artifactId ?? `artifact-${i}`
              const ext = getFileExt(path)
              return (
                <div
                  key={a.artifactId ?? i}
                  style={{
                    display: 'flex', alignItems: 'center', gap: 10,
                    padding: '9px 10px', borderRadius: 'var(--r-md)',
                    cursor: 'default',
                    transition: 'background 0.1s',
                  }}
                  onMouseEnter={e => (e.currentTarget.style.background = 'var(--bg-hover)')}
                  onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
                >
                  <div style={{ color: 'var(--text-3)', flexShrink: 0, display: 'flex', alignItems: 'center' }}>
                    {getFileIcon(path)}
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{
                      fontSize: 12, color: 'var(--text-1)', fontWeight: 500,
                      overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                      letterSpacing: '-0.01em',
                    }}>
                      {path}
                    </div>
                    <div style={{ fontSize: 10, color: 'var(--text-3)', marginTop: 1 }}>
                      {a.role && <span style={{ marginRight: 6 }}>{a.role}</span>}
                      {formatBytes(a.sizeBytes)}
                      {ext && <span style={{ marginLeft: 6, fontWeight: 500, color: 'var(--accent)', opacity: 0.7 }}>{ext}</span>}
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
