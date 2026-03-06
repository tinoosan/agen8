import { useStore } from '../lib/store'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { useMail } from '../hooks/useMail'
import RoleRow from './RoleRow'
import ActivityFeed from './ActivityFeed'
import { Mail, FolderOpen } from 'lucide-react'

interface ContextPanelProps {
  teamId: string
}

export default function ContextPanel({ teamId }: ContextPanelProps) {
  const { setMailOpen, setArtifactsOpen } = useStore()
  const statusQuery = useTeamStatus(teamId)
  const { badgeCount } = useMail(teamId)
  const roles = statusQuery.data?.roles ?? []

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      padding: '16px 16px 12px',
      borderLeft: '1px solid light-dark(rgba(0,0,0,0.07), rgba(255,255,255,0.07))',
      background: 'light-dark(rgba(0,0,0,0.015), rgba(255,255,255,0.015))',
      minWidth: 0,
    }}>
      {/* Roles section */}
      <div style={{ marginBottom: 4 }}>
        <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', opacity: 0.35, marginBottom: 6 }}>
          Team
        </div>
        {roles.length === 0 ? (
          <div style={{ fontSize: 11, opacity: 0.3 }}>Loading…</div>
        ) : (
          roles.map(role => <RoleRow key={role.role} role={role} />)
        )}
      </div>

      <div style={{ height: 1, background: 'light-dark(rgba(0,0,0,0.07), rgba(255,255,255,0.07))', margin: '12px 0' }} />

      {/* Activity */}
      <div style={{ fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', opacity: 0.35, marginBottom: 6 }}>
        Activity
      </div>
      <ActivityFeed teamId={teamId} />

      {/* Bottom buttons */}
      <div style={{ display: 'flex', gap: 8, marginTop: 12, paddingTop: 12, borderTop: '1px solid light-dark(rgba(0,0,0,0.07), rgba(255,255,255,0.07))' }}>
        <button
          onClick={() => setMailOpen(true)}
          style={{
            flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            padding: '6px 0', borderRadius: 8, cursor: 'pointer',
            border: '1px solid light-dark(rgba(0,0,0,0.1), rgba(255,255,255,0.1))',
            background: 'none', color: 'inherit', fontSize: 12,
            position: 'relative',
          }}
        >
          <Mail size={12} />
          Mail
          {badgeCount > 0 && (
            <span style={{
              position: 'absolute', top: -6, right: 4,
              background: '#ef4444', color: '#fff',
              fontSize: 9, fontWeight: 700, borderRadius: 999,
              padding: '1px 5px',
            }}>
              {badgeCount}
            </span>
          )}
        </button>
        <button
          onClick={() => setArtifactsOpen(true)}
          style={{
            flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            padding: '6px 0', borderRadius: 8, cursor: 'pointer',
            border: '1px solid light-dark(rgba(0,0,0,0.1), rgba(255,255,255,0.1))',
            background: 'none', color: 'inherit', fontSize: 12,
          }}
        >
          <FolderOpen size={12} />
          Artifacts
        </button>
      </div>
    </div>
  )
}
