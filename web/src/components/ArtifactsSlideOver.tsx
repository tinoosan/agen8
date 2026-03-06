import { useQuery } from '@tanstack/react-query'
import { useStore } from '../lib/store'
import { rpcCall } from '../lib/rpc'
import { X, FileText } from 'lucide-react'
import type { Artifact } from '../lib/types'

interface ArtifactsSlideOverProps {
  teamId: string
}

function formatBytes(bytes?: number): string {
  if (!bytes) return ''
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`
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
        background: 'rgba(0,0,0,0.3)',
      }}
      onClick={() => setArtifactsOpen(false)}
    >
      <div
        onClick={e => e.stopPropagation()}
        style={{
          width: 360, height: '100%',
          background: 'light-dark(#ffffff, #111113)',
          display: 'flex', flexDirection: 'column',
          boxShadow: '-8px 0 40px rgba(0,0,0,0.2)',
        }}
      >
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '16px 16px', borderBottom: '1px solid light-dark(rgba(0,0,0,0.08), rgba(255,255,255,0.08))',
        }}>
          <span style={{ fontWeight: 700, fontSize: 15 }}>Artifacts</span>
          <button onClick={() => setArtifactsOpen(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'inherit', opacity: 0.5 }}>
            <X size={18} />
          </button>
        </div>

        <div style={{ flex: 1, overflowY: 'auto', padding: '12px 8px' }}>
          {artifacts.length === 0 ? (
            <div style={{ padding: 24, textAlign: 'center', opacity: 0.3, fontSize: 13 }}>No artifacts yet</div>
          ) : (
            artifacts.map((a, i) => (
              <div
                key={a.artifactId ?? i}
                style={{
                  display: 'flex', alignItems: 'center', gap: 10,
                  padding: '8px 12px', borderRadius: 8,
                }}
              >
                <FileText size={14} style={{ opacity: 0.4, flexShrink: 0 }} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {a.vpath ?? a.artifactId}
                  </div>
                  <div style={{ fontSize: 10, opacity: 0.4 }}>
                    {a.role && <span>{a.role} · </span>}
                    {formatBytes(a.sizeBytes)}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
