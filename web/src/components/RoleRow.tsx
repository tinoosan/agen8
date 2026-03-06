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
    <div style={{
      display: 'flex', alignItems: 'flex-start', gap: 9,
      padding: '7px 0',
      borderBottom: '1px solid var(--border)',
    }}>
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
          <div style={{
            fontSize: 11, color: 'var(--text-3)',
            overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          }}>
            {role.info}
          </div>
        )}
      </div>
    </div>
  )
}
