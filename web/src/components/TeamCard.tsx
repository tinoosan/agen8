import { useStore } from '../lib/store'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'
import PulseDot from './PulseDot'
import type { ProjectTeamSummary } from '../lib/types'
import { Trash2, ArrowRight } from 'lucide-react'

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
      className="animate-fade-up"
      style={{
        background: 'var(--bg-panel)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--r-xl)',
        padding: '16px 18px',
        cursor: 'pointer',
        display: 'flex', flexDirection: 'column', gap: 14,
        minWidth: 270, maxWidth: 340, flex: '1 1 270px',
        position: 'relative',
        transition: 'border-color 0.15s, box-shadow 0.15s, transform 0.15s',
        boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
      }}
      onMouseEnter={e => {
        const el = e.currentTarget as HTMLElement
        el.style.borderColor = 'var(--border-strong)'
        el.style.boxShadow = '0 4px 24px rgba(0,0,0,0.4)'
        el.style.transform = 'translateY(-1px)'
      }}
      onMouseLeave={e => {
        const el = e.currentTarget as HTMLElement
        el.style.borderColor = 'var(--border)'
        el.style.boxShadow = '0 1px 3px rgba(0,0,0,0.3)'
        el.style.transform = ''
      }}
    >
      {/* Active accent line */}
      {isActive && (
        <div style={{
          position: 'absolute', top: 0, left: 16, right: 16, height: 2,
          background: 'linear-gradient(90deg, var(--accent), var(--green))',
          borderRadius: '0 0 2px 2px',
          opacity: 0.7,
        }} />
      )}

      {/* Header row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
        <PulseDot status={cardStatus} size={8} />
        <span style={{
          fontWeight: 600, fontSize: 14,
          flex: 1,
          letterSpacing: '-0.02em',
          color: 'var(--text-1)',
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>
          {team.profileId ?? team.teamId.slice(0, 12)}
        </span>
        <span style={{
          fontSize: 11, color: 'var(--text-3)',
          fontWeight: 500,
        }}>
          {statusLabel[cardStatus]}
        </span>
        <button
          onClick={handleDelete}
          style={{
            background: 'none', border: 'none', cursor: 'pointer',
            padding: 4, borderRadius: 'var(--r-sm)',
            color: 'var(--text-3)',
            display: 'flex', alignItems: 'center',
            transition: 'color 0.1s, background 0.1s',
          }}
          onMouseEnter={e => {
            e.currentTarget.style.color = 'var(--red)'
            e.currentTarget.style.background = 'var(--red-dim)'
          }}
          onMouseLeave={e => {
            e.currentTarget.style.color = 'var(--text-3)'
            e.currentTarget.style.background = 'transparent'
          }}
        >
          <Trash2 size={12} />
        </button>
      </div>

      {/* Primary role activity */}
      {data?.roles?.[0] && (
        <div style={{
          fontSize: 12, color: 'var(--text-2)',
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          lineHeight: 1.4,
        }}>
          <span style={{ color: 'var(--text-3)', marginRight: 6 }}>{data.roles[0].role}</span>
          {data.roles[0].info || (isActive ? 'working…' : 'idle')}
        </div>
      )}

      {/* Role pills */}
      {roleSummary.length > 0 && (
        <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap' }}>
          {roleSummary.map(role => {
            const isRoleActive = !!role.info && !role.info.toLowerCase().includes('idle')
            return (
              <div
                key={role.role}
                title={`${role.role}: ${role.info}`}
                style={{
                  display: 'flex', alignItems: 'center', gap: 5,
                  padding: '3px 8px',
                  borderRadius: 999,
                  border: '1px solid var(--border)',
                  background: isRoleActive ? 'var(--bg-active)' : 'transparent',
                  fontSize: 11, color: 'var(--text-2)',
                  transition: 'background 0.1s',
                }}
              >
                <PulseDot status={isRoleActive ? 'active' : 'idle'} size={5} />
                <span>{role.role}</span>
              </div>
            )
          })}
        </div>
      )}

      {/* Footer */}
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        paddingTop: 10,
        borderTop: '1px solid var(--border)',
        marginTop: 'auto',
      }}>
        <div style={{ display: 'flex', gap: 12, fontSize: 11, color: 'var(--text-3)' }}>
          {taskCount !== null && (
            <span>{taskCount} task{taskCount !== 1 ? 's' : ''}</span>
          )}
          {costStr && (
            <span style={{ color: 'var(--text-3)' }}>{costStr}</span>
          )}
        </div>
        <button
          onClick={(e) => { e.stopPropagation(); setFocusedTeamId(team.teamId) }}
          style={{
            display: 'flex', alignItems: 'center', gap: 5,
            fontSize: 12, fontWeight: 500,
            padding: '5px 10px', borderRadius: 'var(--r-md)',
            border: '1px solid var(--border)',
            background: 'var(--bg-surface)',
            cursor: 'pointer', color: 'var(--text-2)',
            transition: 'border-color 0.1s, color 0.1s, background 0.1s',
          }}
          onMouseEnter={e => {
            e.currentTarget.style.borderColor = 'var(--border-strong)'
            e.currentTarget.style.color = 'var(--text-1)'
            e.currentTarget.style.background = 'var(--bg-elevated)'
          }}
          onMouseLeave={e => {
            e.currentTarget.style.borderColor = 'var(--border)'
            e.currentTarget.style.color = 'var(--text-2)'
            e.currentTarget.style.background = 'var(--bg-surface)'
          }}
        >
          Open
          <ArrowRight size={11} />
        </button>
      </div>
    </div>
  )
}
