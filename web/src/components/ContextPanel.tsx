import { useStore } from '../lib/store'
import { useTeamStatus, useTeamManifest } from '../hooks/useTeamStatus'
import { useMail } from '../hooks/useMail'
import RoleRow from './RoleRow'
import ActivityFeed from './ActivityFeed'
import { Mail, FolderOpen, ListChecks } from 'lucide-react'
import { useRuntimeState } from '../hooks/useRuntimeState'
import { useModelList } from '../hooks/useModelList'

interface ContextPanelProps {
  teamId: string
  threadId: string | null
}

export default function ContextPanel({ teamId, threadId }: ContextPanelProps) {
  const { setMailOpen, setArtifactsOpen, setPlanOpen, setModelPickerTarget, setReasoningPickerTarget, focusedRole, setFocusedRole } = useStore()
  const statusQuery = useTeamStatus(teamId)
  const manifestQuery = useTeamManifest(teamId)
  const runtimeQuery = useRuntimeState(threadId || '')
  const modelQuery = useModelList(threadId || '')
  const models = modelQuery.data?.models ?? []
  const { badgeCount } = useMail(teamId)
  const roles = statusQuery.data?.roles ?? []
  const isLoading = statusQuery.isLoading
  const manifest = manifestQuery.data

  const roleByRunId = statusQuery.data?.roleByRunId || {}
  const statsByRole: Record<string, { tokens: number; cost: number; model?: string }> = {}

  // Compute actual replica counts from manifest roles
  const replicaCountByRole: Record<string, number> = {}
  for (const r of manifest?.roles ?? []) {
    replicaCountByRole[r.roleName] = (replicaCountByRole[r.roleName] || 0) + 1
  }
  const desiredReplicasByRole = manifest?.desiredReplicasByRole ?? {}

  for (const run of runtimeQuery.data?.runs || []) {
    const role = roleByRunId[run.runId] || '(coordinator)'
    if (!statsByRole[role]) statsByRole[role] = { tokens: 0, cost: 0 }
    statsByRole[role].tokens += run.runTotalTokens
    statsByRole[role].cost += run.runTotalCostUSD
    if (run.model) statsByRole[role].model = run.model
  }

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100%',
      padding: '16px 14px 12px',
      borderLeft: '1px solid var(--border)',
      background: 'var(--bg-panel)',
      minWidth: 0,
    }}>
      {/* ── Team roles section ─────────────────────────── */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8 }}>
        <span className="section-label" style={{ marginBottom: 0 }}>Team</span>
        {roles.length > 0 && (
          <span className="section-count">· {roles.length}</span>
        )}
      </div>

      <div style={{ marginBottom: 6 }}>
        {isLoading ? (
          <div className="roles-container">
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <div className="skeleton" style={{ width: '100%', height: 50 }} />
              <div className="skeleton" style={{ width: '100%', height: 50 }} />
              <div className="skeleton" style={{ width: '100%', height: 50 }} />
            </div>
          </div>
        ) : roles.length === 0 ? (
          <div style={{ fontSize: 11, color: 'var(--text-3)', padding: '6px 0' }}>No roles</div>
        ) : (
          <div className="roles-container">
            {roles.map(role => (
              <RoleRow
                key={role.role}
                role={role}
                stats={statsByRole[role.role]}
                onViewTranscript={setFocusedRole}
                onChangeModel={threadId ? (r) => setModelPickerTarget({ role: r, threadId: threadId! }) : undefined}
                onSetReasoning={threadId && models.some(m => m.id === statsByRole[role.role]?.model && m.isReasoning) ? (r) => setReasoningPickerTarget({ role: r, threadId: threadId! }) : undefined}
                isActive={focusedRole === role.role}
                replicaCount={replicaCountByRole[role.role]}
                desiredReplicas={desiredReplicasByRole[role.role]}
              />
            ))}
          </div>
        )}
      </div>

      <div style={{ height: 1, background: 'var(--border)', margin: '8px 0 10px' }} />

      {/* ── Activity section ───────────────────────────── */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8 }}>
        <span className="section-label" style={{ marginBottom: 0 }}>Activity</span>
      </div>
      <ActivityFeed teamId={teamId} threadId={threadId} />

      {/* ── Bottom action tray ─────────────────────────── */}
      <div style={{
        display: 'flex', gap: 8, marginTop: 12, paddingTop: 12,
        borderTop: '1px solid var(--border)',
      }}>
        <button
          className="action-tray-btn"
          onClick={() => setMailOpen(true)}
        >
          <Mail size={14} />
          Mail
          {badgeCount > 0 && (
            <span
              className="badge badge-red"
              style={{ position: 'absolute', top: -6, right: -4 }}
            >
              {badgeCount}
            </span>
          )}
        </button>
        <button
          className="action-tray-btn"
          onClick={() => setArtifactsOpen(true)}
        >
          <FolderOpen size={14} />
          Files
        </button>
        <button
          className="action-tray-btn"
          onClick={() => setPlanOpen(true)}
        >
          <ListChecks size={14} />
          Plan
        </button>
      </div>
    </div>
  )
}
