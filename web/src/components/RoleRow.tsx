import PulseDot from './PulseDot'
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
      className="row-hover"
      style={{
        display: 'flex', alignItems: 'flex-start', gap: 9,
        padding: '6px 4px',
        marginBottom: 1,
      }}
      title={role.info || undefined}
    >
      <div style={{ paddingTop: 4, flexShrink: 0 }}>
        <PulseDot status={status} size={7} />
      </div>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{
          fontSize: 12, fontWeight: 500,
          color: 'var(--text-1)',
          marginBottom: 1,
          letterSpacing: '-0.01em',
        }}>
          {role.role}
        </div>
        {role.info && (
          <div className="truncate" style={{
            fontSize: 11, color: 'var(--text-3)',
          }}>
            {role.info}
          </div>
        )}
      </div>
    </div>
  )
}
