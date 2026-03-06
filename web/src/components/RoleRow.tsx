import type { TeamRoleStatus } from '../lib/types'

interface RoleRowProps {
  role: TeamRoleStatus
}

function inferStatus(info: string): 'active' | 'idle' | 'pending' | 'failed' | 'done' {
  const lower = info.toLowerCase()
  if (lower.includes('idle') || lower.includes('waiting') || lower === '') return 'idle'
  if (lower.includes('fail') || lower.includes('error')) return 'failed'
  if (lower.includes('done') || lower.includes('complet')) return 'done'
  if (lower.includes('pending') || lower.includes('queue')) return 'pending'
  return 'active'
}

export default function RoleRow({ role }: RoleRowProps) {
  const status = inferStatus(role.info)

  return (
    <div
      style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '10px 12px',
        marginBottom: 8,
        background: 'var(--bg-elevated)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--r-md)',
        boxShadow: '0 2px 8px rgba(0,0,0,0.2)',
        transition: 'border-color 0.15s, transform 0.15s',
        cursor: 'default',
        position: 'relative',
        overflow: 'hidden',
      }}
      className="role-card-hover"
      title={role.info || undefined}
    >
      {/* Subtle active glow on the left edge if active */}
      {status === 'active' && (
        <div style={{
          position: 'absolute', top: 0, bottom: 0, left: 0, width: 3,
          background: 'var(--accent)',
          boxShadow: '0 0 8px var(--accent-glow)'
        }} />
      )}

      <div style={{ minWidth: 0, flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 2 }}>
          <div className="truncate" style={{
            fontSize: 13, fontWeight: 600,
            color: 'var(--text-1)',
            letterSpacing: '-0.01em',
          }}>
            {role.role}
          </div>
          {/* Status Badge */}
          {status === 'active' ? (
            <span style={{
              display: 'flex', alignItems: 'center', gap: 4,
              fontSize: 9, fontWeight: 700, color: 'var(--accent)',
              background: 'var(--accent-dim)', padding: '2px 6px',
              borderRadius: 4, textTransform: 'uppercase', letterSpacing: '0.04em'
            }}>
              <div className="spinner spinner-sm" style={{ width: 8, height: 8, borderWidth: 1.5, borderColor: 'rgba(139,123,248,0.3)', borderTopColor: 'var(--accent)' }} />
              Running
            </span>
          ) : status === 'done' ? (
            <span style={{
              fontSize: 9, fontWeight: 700, color: 'var(--green)',
              background: 'var(--green-dim)', padding: '2px 6px',
              borderRadius: 4, textTransform: 'uppercase', letterSpacing: '0.04em'
            }}>
              Done
            </span>
          ) : status === 'failed' ? (
            <span style={{
              fontSize: 9, fontWeight: 700, color: 'var(--red)',
              background: 'var(--red-dim)', padding: '2px 6px',
              borderRadius: 4, textTransform: 'uppercase', letterSpacing: '0.04em'
            }}>
              Error
            </span>
          ) : (
            <span style={{
              fontSize: 9, fontWeight: 600, color: 'var(--text-3)',
              background: 'var(--bg-active)', padding: '2px 6px',
              borderRadius: 4, textTransform: 'uppercase', letterSpacing: '0.04em'
            }}>
              Idle
            </span>
          )}
        </div>

        <div className="truncate" style={{
          fontSize: 11, color: 'var(--text-3)',
        }}>
          {role.info || 'Waiting for tasks…'}
        </div>
      </div>
    </div>
  )
}
