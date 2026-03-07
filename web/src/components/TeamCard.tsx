import { useStore } from '../lib/store'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'
import PulseDot from './PulseDot'
import type { ProjectTeamSummary } from '../lib/types'
import { Trash2, Coins } from 'lucide-react'

interface TeamCardProps {
  team: ProjectTeamSummary
}

function inferCardStatus(status?: string, active?: number): 'active' | 'idle' | 'pending' | 'failed' | 'done' {
  if (status === 'running' || (active ?? 0) > 0) return 'active'
  if (status === 'failed') return 'failed'
  if (status === 'done' || status === 'stopped') return 'done'
  return 'idle'
}

const statusLabel: Record<string, string> = {
  active: 'Running',
  idle: 'Idle',
  pending: 'Starting',
  failed: 'Failed',
  done: 'Done',
}

const statusLabelColor: Record<string, string> = {
  active: 'var(--green)',
  idle: 'var(--text-3)',
  pending: 'var(--amber)',
  failed: 'var(--red)',
  done: 'var(--blue)',
}

export default function TeamCard({ team }: TeamCardProps) {
  const { setFocusedTeamId } = useStore()
  const statusQuery = useTeamStatus(team.teamId)
  const queryClient = useQueryClient()
  const data = statusQuery.data

  const cardStatus = inferCardStatus(team.status, data?.active)
  const isActive = cardStatus === 'active'

  const costStr = data?.totalCostUSD
    ? `$${data.totalCostUSD.toFixed(2)}`
    : null

  const roleSummary = data?.roles?.slice(0, 5) ?? []
  const taskCount = data !== undefined ? data.active + data.pending : null

  async function handleDelete(e: React.MouseEvent) {
    e.stopPropagation()
    if (!confirm(`Delete team "${team.profileId ?? team.teamId}"?`)) return
    await rpcCall('team.delete', { teamId: team.teamId })
    queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })
  }

  return (
    <div
      onClick={() => setFocusedTeamId(team.teamId)}
      className="card-interactive animate-fade-up"
      style={{
        padding: '18px 20px',
        display: 'flex', flexDirection: 'column', gap: 14,
        minWidth: 280, maxWidth: 360, flex: '1 1 280px',
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      {/* Active accent line */}
      {isActive && (
        <div style={{
          position: 'absolute', top: 0, left: 20, right: 20, height: 2,
          background: 'linear-gradient(90deg, var(--accent), var(--green))',
          borderRadius: '0 0 2px 2px',
          opacity: 0.8,
        }} />
      )}

      {/* Header row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <PulseDot status={cardStatus} size={8} />
        <span style={{
          fontWeight: 600, fontSize: 14,
          flex: 1,
          letterSpacing: '-0.02em',
          color: 'var(--text-1)',
        }} className="truncate">
          {team.profileId ?? team.teamId.slice(0, 12)}
        </span>
        <span style={{
          fontSize: 11,
          color: statusLabelColor[cardStatus] ?? 'var(--text-3)',
          fontWeight: 500,
        }}>
          {statusLabel[cardStatus]}
        </span>
        <button
          onClick={handleDelete}
          className="btn-ghost-danger"
          aria-label="Delete team"
        >
          <Trash2 size={12} />
        </button>
      </div>

      {/* Primary role activity */}
      {data?.roles?.[0] && (
        <div className="truncate" style={{
          fontSize: 12, color: 'var(--text-2)',
          lineHeight: 1.5,
        }}>
          <span style={{ color: 'var(--text-3)', marginRight: 6, fontWeight: 500 }}>{data.roles[0].role}</span>
          {data.roles[0].info || (isActive ? 'working…' : 'idle')}
        </div>
      )}

      {/* Role pills */}
      {roleSummary.length > 1 && (
        <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap' }}>
          {roleSummary.slice(1).map(role => {
            const isRoleActive = !!role.info && !role.info.toLowerCase().includes('idle')
            return (
              <div
                key={role.role}
                title={`${role.role}: ${role.info || 'idle'}`}
                style={{
                  display: 'flex', alignItems: 'center', gap: 5,
                  padding: '2px 8px',
                  borderRadius: 999,
                  border: '1px solid var(--border)',
                  background: isRoleActive ? 'var(--bg-active)' : 'transparent',
                  fontSize: 11, color: isRoleActive ? 'var(--text-2)' : 'var(--text-3)',
                  transition: 'background 0.15s',
                }}
              >
                <PulseDot status={isRoleActive ? 'active' : 'idle'} size={5} />
                <span className="truncate" style={{ maxWidth: 80 }}>{role.role}</span>
              </div>
            )
          })}
        </div>
      )}

      {/* Footer */}
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        paddingTop: 12,
        borderTop: '1px solid var(--border)',
        marginTop: 'auto',
      }}>
        <div style={{ display: 'flex', gap: 14, fontSize: 11, color: 'var(--text-3)', fontVariantNumeric: 'tabular-nums' }}>
          {taskCount !== null && (
            <span>{taskCount} task{taskCount !== 1 ? 's' : ''}</span>
          )}
          {costStr && (
            <span style={{
              display: 'inline-flex', alignItems: 'center', gap: 4,
              color: 'var(--amber)', fontWeight: 600,
            }}>
              <Coins size={11} />
              {costStr}
            </span>
          )}
          {!data && <div className="skeleton" style={{ width: 60, height: 12 }} />}
        </div>
      </div>
    </div>
  )
}
