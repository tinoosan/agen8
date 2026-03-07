import { useTeamManifest } from '../hooks/useTeamStatus'
import { useStore } from '../lib/store'
import { useRuntimeState } from '../hooks/useRuntimeState'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'
import Conversation from '../components/Conversation'
import RoleTranscript from '../components/RoleTranscript'
import ContextPanel from '../components/ContextPanel'
import MailSlideOver from '../components/MailSlideOver'
import ArtifactsSlideOver from '../components/ArtifactsSlideOver'
import PlanSlideOver from '../components/PlanSlideOver'
import PulseDot from '../components/PulseDot'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { Pause, Play, Square, Eraser, Brain } from 'lucide-react'
import { useState } from 'react'

const DETACHED = 'detached-control'

interface TeamFocusProps {
  teamId: string
}

export default function TeamFocus({ teamId }: TeamFocusProps) {
  const { mailOpen, artifactsOpen, planOpen, focusedRole, setReasoningPickerTarget } = useStore()
  const manifestQuery = useTeamManifest(teamId)
  const statusQuery = useTeamStatus(teamId)
  const teamsQuery = useProjectTeams()
  const queryClient = useQueryClient()
  const manifest = manifestQuery.data
  const status = statusQuery.data

  const threadId = manifest?.coordinatorThreadId ?? null
  const coordinatorRole = manifest?.coordinatorRole ?? null
  const coordinatorRunId = manifest?.coordinatorRunId ?? null
  const sessionId = teamsQuery.data?.find(t => t.teamId === teamId)?.primarySessionId ?? null
  const isActive = (status?.active ?? 0) > 0
  const isPaused = !isActive && (status?.pending ?? 0) > 0
  const cardStatus = isActive ? 'active' : 'idle'

  const [teamActionLoading, setTeamActionLoading] = useState(false)

  function invalidateTeamQueries() {
    queryClient.invalidateQueries({ queryKey: ['team.getStatus'] })
    queryClient.invalidateQueries({ queryKey: ['activity'] })
  }

  async function handlePause() {
    if (!sessionId || teamActionLoading) return
    setTeamActionLoading(true)
    try {
      await rpcCall('session.pause', { threadId: DETACHED, sessionId })
      invalidateTeamQueries()
    } finally { setTeamActionLoading(false) }
  }

  async function handleResume() {
    if (!sessionId || teamActionLoading) return
    setTeamActionLoading(true)
    try {
      await rpcCall('session.resume', { threadId: DETACHED, sessionId })
      invalidateTeamQueries()
    } finally { setTeamActionLoading(false) }
  }

  async function handleStop() {
    if (!sessionId || teamActionLoading) return
    if (!confirm('Stop all runs for this team? This cannot be undone.')) return
    setTeamActionLoading(true)
    try {
      await rpcCall('session.stop', { threadId: DETACHED, sessionId })
      invalidateTeamQueries()
    } finally { setTeamActionLoading(false) }
  }

  async function handleClearHistory() {
    if (!confirm('Clear all history for this team? This cannot be undone.')) return
    try {
      await rpcCall('session.clearHistory', { threadId: DETACHED, teamId })
      queryClient.invalidateQueries({ queryKey: ['team.getStatus'] })
      queryClient.invalidateQueries({ queryKey: ['activity'] })
      queryClient.invalidateQueries({ queryKey: ['logs.query'] })
      queryClient.invalidateQueries({ queryKey: ['item.list'] })
    } catch { /* ignore */ }
  }

  // Resolve focused role to runId for RoleTranscript
  const focusedRoleRecord = focusedRole
    ? manifest?.roles?.find(r => r.roleName === focusedRole)
    : null
  const focusedRunId = focusedRoleRecord?.runId ?? null

  // Get runtime stats for focused role
  const runtimeQuery = useRuntimeState(threadId ?? '')
  const focusedRunState = focusedRunId
    ? runtimeQuery.data?.runs?.find(r => r.runId === focusedRunId)
    : null

  return (
    <div style={{ display: 'flex', height: '100%', position: 'relative' }}>
      {/* Main conversation area */}
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
        {/* Team header bar — only show when not in role view */}
        {!focusedRole && (
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
            {/* Team control buttons */}
            {sessionId && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                {isActive && (
                  <button
                    onClick={handlePause}
                    disabled={teamActionLoading}
                    title="Pause team"
                    style={{
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      width: 28, height: 28, borderRadius: 'var(--r-sm)',
                      border: '1px solid var(--border)', background: 'var(--bg-elevated)',
                      color: 'var(--text-3)', cursor: 'pointer',
                      transition: 'color 0.15s, border-color 0.15s',
                    }}
                    className="row-hover"
                  >
                    <Pause size={13} />
                  </button>
                )}
                {isPaused && (
                  <button
                    onClick={handleResume}
                    disabled={teamActionLoading}
                    title="Resume team"
                    style={{
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      width: 28, height: 28, borderRadius: 'var(--r-sm)',
                      border: '1px solid var(--accent)', background: 'var(--accent-dim)',
                      color: 'var(--accent)', cursor: 'pointer',
                      transition: 'color 0.15s, border-color 0.15s',
                    }}
                    className="row-hover"
                  >
                    <Play size={13} />
                  </button>
                )}
                <button
                  onClick={handleStop}
                  disabled={teamActionLoading}
                  title="Stop team"
                  style={{
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    width: 28, height: 28, borderRadius: 'var(--r-sm)',
                    border: '1px solid var(--border)', background: 'var(--bg-elevated)',
                    color: 'var(--red)', cursor: 'pointer',
                    transition: 'color 0.15s, border-color 0.15s',
                  }}
                  className="row-hover"
                >
                  <Square size={13} />
                </button>
              </div>
            )}
            {threadId && (
              <button
                onClick={() => setReasoningPickerTarget({ role: null, threadId: threadId! })}
                title="Set reasoning (all roles)"
                style={{
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  width: 28, height: 28, borderRadius: 'var(--r-sm)',
                  border: '1px solid var(--border)', background: 'var(--bg-elevated)',
                  color: 'var(--text-3)', cursor: 'pointer',
                  transition: 'color 0.15s, border-color 0.15s',
                }}
                className="row-hover"
              >
                <Brain size={13} />
              </button>
            )}
            <button
              onClick={handleClearHistory}
              title="Clear history"
              style={{
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                width: 28, height: 28, borderRadius: 'var(--r-sm)',
                border: '1px solid var(--border)', background: 'var(--bg-elevated)',
                color: 'var(--text-3)', cursor: 'pointer',
                transition: 'color 0.15s, border-color 0.15s',
              }}
              className="row-hover"
            >
              <Eraser size={13} />
            </button>
            {threadId && (
              <span className="mono" style={{
                fontSize: 10, color: 'var(--text-3)',
              }}>
                {threadId.slice(0, 8)}
              </span>
            )}
          </div>
        )}

        {/* Conversation or RoleTranscript */}
        <div style={{ flex: 1, minHeight: 0 }}>
          {focusedRole ? (
            <RoleTranscript
              teamId={teamId}
              threadId={threadId}
              roleName={focusedRole}
              runId={focusedRunId}
              model={focusedRunState?.model}
              tokens={focusedRunState?.runTotalTokens}
              cost={focusedRunState?.runTotalCostUSD}
              status={focusedRunState?.effectiveStatus}
            />
          ) : (
            <Conversation
              threadId={threadId}
              teamId={teamId}
              coordinatorRole={coordinatorRole}
              coordinatorRunId={coordinatorRunId}
            />
          )}
        </div>
      </div>

      {/* Right sidebar */}
      <div style={{ width: 380, flexShrink: 0, display: 'flex', flexDirection: 'column' }}>
        <ContextPanel teamId={teamId} threadId={threadId} />
      </div>

      {/* Slide-over panels */}
      {mailOpen && <MailSlideOver teamId={teamId} />}
      {artifactsOpen && <ArtifactsSlideOver teamId={teamId} threadId={threadId} />}
      {planOpen && <PlanSlideOver teamId={teamId} threadId={threadId} />}
    </div>
  )
}
