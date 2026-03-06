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

  // Determine status color
  const statusColor =
    status === 'active' ? 'var(--accent)' :
      status === 'done' ? 'var(--green)' :
        status === 'failed' ? 'var(--red)' :
          'var(--text-4)'

  return (
    <div
      style={{
        display: 'flex', flexDirection: 'column', gap: 4,
        padding: '10px 14px',
        marginBottom: 6,
        background: 'rgba(255, 255, 255, 0.02)',
        border: '1px solid rgba(255, 255, 255, 0.04)',
        borderRadius: 'var(--r-md)',
        transition: 'all 0.2s cubic-bezier(0.16, 1, 0.3, 1)',
        cursor: 'default',
        position: 'relative',
        backdropFilter: 'blur(8px)',
        WebkitBackdropFilter: 'blur(8px)',
      }}
      className="role-glass-hover"
      title={role.info || undefined}
    >
      {/* Top Header line: Status Indicator + Uppercase Role Name */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        {/* Sleek status indicator */}
        <div style={{
          width: 6, height: 6, borderRadius: '50%',
          background: statusColor,
          boxShadow: status === 'active' ? `0 0 8px ${statusColor}, 0 0 4px ${statusColor}` : 'none',
          opacity: status === 'idle' ? 0.3 : 1
        }} />

        {/* Role Name */}
        <div className="truncate" style={{
          fontSize: 10, fontWeight: 700,
          color: status === 'active' ? 'var(--text-1)' : 'var(--text-3)',
          textTransform: 'uppercase',
          letterSpacing: '0.08em',
        }}>
          {role.role.replace(/-/g, ' ')}
        </div>
      </div>

      {/* Info Line */}
      <div className="truncate" style={{
        fontSize: 12, color: 'var(--text-2)',
        fontWeight: 400,
        paddingLeft: 12, // Indent to align with text above
      }}>
        {role.info || 'Waiting for tasks...'}
      </div>
    </div>
  )
}
