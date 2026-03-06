import { useTeamManifest } from '../hooks/useTeamStatus'
import { useStore } from '../lib/store'
import Conversation from '../components/Conversation'
import ContextPanel from '../components/ContextPanel'
import MailSlideOver from '../components/MailSlideOver'
import ArtifactsSlideOver from '../components/ArtifactsSlideOver'
import PulseDot from '../components/PulseDot'
import { useTeamStatus } from '../hooks/useTeamStatus'

interface TeamFocusProps {
  teamId: string
}

export default function TeamFocus({ teamId }: TeamFocusProps) {
  const { mailOpen, artifactsOpen } = useStore()
  const manifestQuery = useTeamManifest(teamId)
  const statusQuery = useTeamStatus(teamId)
  const manifest = manifestQuery.data
  const status = statusQuery.data

  const threadId = manifest?.coordinatorThreadId ?? null
  const coordinatorRole = manifest?.coordinatorRole ?? null
  const coordinatorRunId = manifest?.coordinatorRunId ?? null
  const isActive = (status?.active ?? 0) > 0
  const cardStatus = isActive ? 'active' : 'idle'

  return (
    <div style={{ display: 'flex', height: '100%', position: 'relative' }}>
      {/* Main conversation area */}
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
        {/* Team header bar */}
        <div style={{
          padding: '10px 24px',
          borderBottom: '1px solid var(--border)',
          display: 'flex', alignItems: 'center', gap: 10,
          flexShrink: 0,
          background: 'var(--bg-panel)',
        }}>
          <PulseDot status={cardStatus} size={7} />
          <span style={{
            fontWeight: 600, fontSize: 14,
            color: 'var(--text-1)',
            letterSpacing: '-0.02em',
          }}>
            {manifest?.profileId ?? teamId.slice(0, 12)}
          </span>
          {manifest?.teamModel && (
            <span className="mono" style={{
              fontSize: 11, color: 'var(--text-3)',
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border)',
              padding: '2px 8px', borderRadius: 'var(--r-sm)',
              letterSpacing: '0.01em',
            }}>
              {manifest.teamModel}
            </span>
          )}
          <div style={{ flex: 1 }} />
          {threadId && (
            <span className="mono" style={{
              fontSize: 10, color: 'var(--text-3)',
            }}>
              {threadId.slice(0, 8)}
            </span>
          )}
        </div>

        {/* Conversation */}
        <div style={{ flex: 1, minHeight: 0 }}>
          <Conversation threadId={threadId} teamId={teamId} coordinatorRole={coordinatorRole} coordinatorRunId={coordinatorRunId} />
        </div>
      </div>

      {/* Right sidebar */}
      <div style={{ width: 380, flexShrink: 0, display: 'flex', flexDirection: 'column' }}>
        <ContextPanel teamId={teamId} threadId={threadId} />
      </div>

      {/* Slide-over panels */}
      {mailOpen && <MailSlideOver teamId={teamId} />}
      {artifactsOpen && <ArtifactsSlideOver teamId={teamId} />}
    </div>
  )
}
