import { useStore } from '../lib/store'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'
import PulseDot from './PulseDot'
import type { ProjectTeamSummary } from '../lib/types'

interface TeamCardProps {
  team: ProjectTeamSummary
}

function inferCardStatus(status?: string, active?: number): 'active' | 'idle' | 'pending' | 'failed' | 'done' {
  if (status === 'running' || (active ?? 0) > 0) return 'active'
  if (status === 'failed') return 'failed'
  if (status === 'done' || status === 'stopped') return 'done'
  return 'idle'
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

  async function handleDelete(e: React.MouseEvent) {
    e.stopPropagation()
    if (!confirm(`Delete team "${team.profileId ?? team.teamId}"?`)) return
    await rpcCall('team.delete', { teamId: team.teamId })
    queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })
  }

  return (
    <div
      onClick={() => setFocusedTeamId(team.teamId)}
      style={{
        background: 'light-dark(rgba(255,255,255,0.7), rgba(25,25,27,0.7))',
        border: '1px solid light-dark(rgba(0,0,0,0.08), rgba(255,255,255,0.08))',
        borderRadius: 16,
        padding: '18px 20px',
        cursor: 'pointer',
        transition: 'box-shadow 0.15s, transform 0.15s',
        backdropFilter: 'blur(8px)',
        WebkitBackdropFilter: 'blur(8px)',
        display: 'flex', flexDirection: 'column', gap: 12,
        minWidth: 260, maxWidth: 360,
        boxShadow: '0 1px 4px light-dark(rgba(0,0,0,0.06), rgba(0,0,0,0.3))',
      }}
      onMouseEnter={e => {
        (e.currentTarget as HTMLElement).style.boxShadow = '0 4px 20px light-dark(rgba(0,0,0,0.12), rgba(0,0,0,0.5))'
        ;(e.currentTarget as HTMLElement).style.transform = 'translateY(-1px)'
      }}
      onMouseLeave={e => {
        (e.currentTarget as HTMLElement).style.boxShadow = '0 1px 4px light-dark(rgba(0,0,0,0.06), rgba(0,0,0,0.3))'
        ;(e.currentTarget as HTMLElement).style.transform = ''
      }}
    >
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <PulseDot status={cardStatus} size={8} />
        <span style={{ fontWeight: 700, fontSize: 15, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {team.profileId ?? team.teamId.slice(0, 12)}
        </span>
        <button
          onClick={handleDelete}
          style={{
            background: 'none', border: 'none', cursor: 'pointer',
            padding: '2px 4px', borderRadius: 4,
            fontSize: 11, opacity: 0.3, color: 'inherit',
          }}
          onMouseEnter={e => ((e.currentTarget as HTMLElement).style.opacity = '0.8')}
          onMouseLeave={e => ((e.currentTarget as HTMLElement).style.opacity = '0.3')}
        >
          ×
        </button>
      </div>

      {/* Status/activity line */}
      {data?.roles?.[0] && (
        <div style={{ fontSize: 12, opacity: 0.55, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {data.roles[0].role}: {data.roles[0].info || (isActive ? 'working…' : 'idle')}
        </div>
      )}

      {/* Role dots */}
      {roleSummary.length > 0 && (
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {roleSummary.map(role => (
            <div
              key={role.role}
              title={`${role.role}: ${role.info}`}
              style={{
                display: 'flex', alignItems: 'center', gap: 4,
                fontSize: 11, opacity: 0.7,
              }}
            >
              <PulseDot
                status={role.info?.toLowerCase().includes('idle') || !role.info ? 'idle' : 'active'}
                size={6}
              />
              <span>{role.role}</span>
            </div>
          ))}
        </div>
      )}

      {/* Footer */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', gap: 10, fontSize: 11, opacity: 0.5 }}>
          {data !== undefined && (
            <span>{(data.active + data.pending)} tasks</span>
          )}
          {costStr && <span>{costStr}</span>}
        </div>
        <button
          onClick={(e) => { e.stopPropagation(); setFocusedTeamId(team.teamId) }}
          style={{
            fontSize: 12, padding: '4px 12px', borderRadius: 8,
            border: '1px solid light-dark(rgba(0,0,0,0.12), rgba(255,255,255,0.12))',
            background: 'none', cursor: 'pointer', color: 'inherit',
          }}
        >
          Open
        </button>
      </div>
    </div>
  )
}
