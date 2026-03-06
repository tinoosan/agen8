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
      display: 'flex', alignItems: 'flex-start', gap: 8,
      padding: '6px 0',
    }}>
      <div style={{ paddingTop: 3, flexShrink: 0 }}>
        <PulseDot status={status} size={8} />
      </div>
      <div style={{ minWidth: 0, flex: 1 }}>
        <div style={{ fontSize: 12, fontWeight: 600, opacity: 0.9, marginBottom: 1 }}>
          {role.role}
        </div>
        {role.info && (
          <div style={{
            fontSize: 11, opacity: 0.55,
            overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          }}>
            {role.info}
          </div>
        )}
      </div>
    </div>
  )
}
