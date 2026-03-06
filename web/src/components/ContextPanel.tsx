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
  const isLoading = statusQuery.isLoading

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      padding: '16px 14px 12px',
      borderLeft: '1px solid var(--border)',
      background: 'var(--bg-panel)',
      minWidth: 0,
    }}>
      {/* Roles section */}
      <div className="section-label">Team</div>
      <div style={{ marginBottom: 6 }}>
        {isLoading ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: '6px 0' }}>
            <div className="skeleton" style={{ width: '100%', height: 14 }} />
            <div className="skeleton" style={{ width: '80%', height: 14 }} />
            <div className="skeleton" style={{ width: '60%', height: 14 }} />
          </div>
        ) : roles.length === 0 ? (
          <div style={{ fontSize: 11, color: 'var(--text-3)', padding: '6px 0' }}>No roles</div>
        ) : (
          roles.map(role => <RoleRow key={role.role} role={role} />)
        )}
      </div>

      <div style={{ height: 1, background: 'var(--border)', margin: '12px 0 10px' }} />

      {/* Activity */}
      <div className="section-label">Activity</div>
      <ActivityFeed teamId={teamId} />

      {/* Bottom action buttons */}
      <div style={{
        display: 'flex', gap: 6, marginTop: 12, paddingTop: 12,
        borderTop: '1px solid var(--border)',
      }}>
        <button
          className="btn-surface"
          onClick={() => setMailOpen(true)}
          style={{ flex: 1, position: 'relative' }}
        >
          <Mail size={12} />
          Mail
          {badgeCount > 0 && (
            <span
              className="badge badge-red"
              style={{ position: 'absolute', top: -6, right: -2 }}
            >
              {badgeCount}
            </span>
          )}
        </button>
        <button
          className="btn-surface"
          onClick={() => setArtifactsOpen(true)}
          style={{ flex: 1 }}
        >
          <FolderOpen size={12} />
          Files
        </button>
      </div>
    </div>
  )
}
