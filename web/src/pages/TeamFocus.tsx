import { useTeamManifest } from '../hooks/useTeamStatus'
import { useStore } from '../lib/store'
import Conversation from '../components/Conversation'
import ContextPanel from '../components/ContextPanel'
import MailSlideOver from '../components/MailSlideOver'
import ArtifactsSlideOver from '../components/ArtifactsSlideOver'

interface TeamFocusProps {
  teamId: string
}

export default function TeamFocus({ teamId }: TeamFocusProps) {
  const { mailOpen, artifactsOpen } = useStore()
  const manifestQuery = useTeamManifest(teamId)
  const manifest = manifestQuery.data

  const threadId = manifest?.coordinatorThreadId ?? null

  return (
    <div style={{ display: 'flex', height: '100%', position: 'relative' }}>
      {/* Center: conversation */}
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
        {/* Team header */}
        <div style={{
          padding: '10px 20px',
          borderBottom: '1px solid light-dark(rgba(0,0,0,0.07), rgba(255,255,255,0.07))',
          display: 'flex', alignItems: 'center', gap: 10,
          flexShrink: 0,
        }}>
          <span style={{ fontWeight: 700, fontSize: 15 }}>
            {manifest?.profileId ?? teamId.slice(0, 12)}
          </span>
          {manifest?.teamModel && (
            <span style={{
              fontSize: 11, opacity: 0.4,
              fontFamily: 'monospace',
              background: 'light-dark(rgba(0,0,0,0.05), rgba(255,255,255,0.05))',
              padding: '1px 6px', borderRadius: 4,
            }}>
              {manifest.teamModel}
            </span>
          )}
        </div>

        <div style={{ flex: 1, minHeight: 0 }}>
          <Conversation threadId={threadId} />
        </div>
      </div>

      {/* Right: context panel */}
      <div style={{ width: 240, flexShrink: 0, display: 'flex', flexDirection: 'column' }}>
        <ContextPanel teamId={teamId} />
      </div>

      {/* Slide-overs */}
      {mailOpen && <MailSlideOver teamId={teamId} />}
      {artifactsOpen && <ArtifactsSlideOver teamId={teamId} />}
    </div>
  )
}
