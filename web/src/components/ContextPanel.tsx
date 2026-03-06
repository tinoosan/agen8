import { useStore } from '../lib/store'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { useMail } from '../hooks/useMail'
import RoleRow from './RoleRow'
import ActivityFeed from './ActivityFeed'
import { Mail, FolderOpen } from 'lucide-react'

interface ContextPanelProps {
  teamId: string
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div style={{
      fontSize: 10, fontWeight: 600,
      letterSpacing: '0.08em', textTransform: 'uppercase',
      color: 'var(--text-3)',
      marginBottom: 4,
    }}>
      {children}
    </div>
  )
}

export default function ContextPanel({ teamId }: ContextPanelProps) {
  const { setMailOpen, setArtifactsOpen } = useStore()
  const statusQuery = useTeamStatus(teamId)
  const { badgeCount } = useMail(teamId)
  const roles = statusQuery.data?.roles ?? []

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      padding: '16px 14px 12px',
      borderLeft: '1px solid var(--border)',
      background: 'var(--bg-panel)',
      minWidth: 0,
    }}>
      {/* Roles section */}
      <SectionLabel>Team</SectionLabel>
      <div style={{ marginBottom: 4 }}>
        {roles.length === 0 ? (
          <div style={{ fontSize: 11, color: 'var(--text-3)', padding: '6px 0' }}>Loading…</div>
        ) : (
          roles.map(role => <RoleRow key={role.role} role={role} />)
        )}
      </div>

      <div style={{ height: 1, background: 'var(--border)', margin: '14px 0 12px' }} />

      {/* Activity */}
      <SectionLabel>Activity</SectionLabel>
      <ActivityFeed teamId={teamId} />

      {/* Bottom action buttons */}
      <div style={{
        display: 'flex', gap: 6, marginTop: 12, paddingTop: 12,
        borderTop: '1px solid var(--border)',
      }}>
        <button
          onClick={() => setMailOpen(true)}
          style={{
            flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 5,
            padding: '7px 0', borderRadius: 'var(--r-md)', cursor: 'pointer',
            border: '1px solid var(--border)',
            background: 'var(--bg-elevated)',
            color: 'var(--text-2)', fontSize: 12, fontWeight: 500,
            position: 'relative',
            transition: 'border-color 0.1s, color 0.1s, background 0.1s',
          }}
          onMouseEnter={e => {
            e.currentTarget.style.borderColor = 'var(--border-strong)'
            e.currentTarget.style.color = 'var(--text-1)'
          }}
          onMouseLeave={e => {
            e.currentTarget.style.borderColor = 'var(--border)'
            e.currentTarget.style.color = 'var(--text-2)'
          }}
        >
          <Mail size={12} />
          Mail
          {badgeCount > 0 && (
            <span style={{
              position: 'absolute', top: -5, right: 3,
              background: 'var(--red)', color: '#fff',
              fontSize: 9, fontWeight: 700, borderRadius: 999,
              padding: '1px 5px',
              lineHeight: 1.6,
            }}>
              {badgeCount}
            </span>
          )}
        </button>
        <button
          onClick={() => setArtifactsOpen(true)}
          style={{
            flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 5,
            padding: '7px 0', borderRadius: 'var(--r-md)', cursor: 'pointer',
            border: '1px solid var(--border)',
            background: 'var(--bg-elevated)',
            color: 'var(--text-2)', fontSize: 12, fontWeight: 500,
            transition: 'border-color 0.1s, color 0.1s, background 0.1s',
          }}
          onMouseEnter={e => {
            e.currentTarget.style.borderColor = 'var(--border-strong)'
            e.currentTarget.style.color = 'var(--text-1)'
          }}
          onMouseLeave={e => {
            e.currentTarget.style.borderColor = 'var(--border)'
            e.currentTarget.style.color = 'var(--text-2)'
          }}
        >
          <FolderOpen size={12} />
          Files
        </button>
      </div>
    </div>
  )
}
