import { useRef, useCallback } from 'react'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import TeamCard from '../components/TeamCard'
import TaskFlowArrows from '../components/TaskFlowArrows'
import type { Task } from '../lib/types'
import { RefreshCw, Zap } from 'lucide-react'

function useAllTasks(teamIds: string[]) {
  return useQuery<Task[]>({
    queryKey: ['task.list.all', teamIds],
    queryFn: async () => {
      const results = await Promise.all(
        teamIds.map(id =>
          rpcCall<{ tasks: Task[] }>('task.list', { teamId: id, limit: 50 })
            .then(r => r.tasks ?? [])
            .catch(() => [] as Task[])
        )
      )
      return results.flat()
    },
    enabled: teamIds.length > 0,
    refetchInterval: 3000,
    staleTime: 2000,
  })
}

export default function Overview() {
  const teamsQuery = useProjectTeams()
  const teams = teamsQuery.data ?? []
  const teamIds = teams.map(t => t.teamId)
  const tasksQuery = useAllTasks(teamIds)
  const tasks = tasksQuery.data ?? []

  const cardRefs = useRef<Record<string, HTMLElement | null>>({})
  const setCardRef = useCallback((teamId: string) => (el: HTMLElement | null) => {
    cardRefs.current[teamId] = el
  }, [])

  const isLoading = teamsQuery.isLoading
  const queryClient = useQueryClient()

  return (
    <div style={{ height: '100%', overflowY: 'auto', padding: '36px 40px' }}>
      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 12,
        marginBottom: 32,
      }}>
        <div>
          <h1 style={{
            margin: 0, fontSize: 24, fontWeight: 700,
            letterSpacing: '-0.04em', color: 'var(--text-1)',
            lineHeight: 1.1,
          }}>
            Teams
          </h1>
          {teams.length > 0 && (
            <div style={{ fontSize: 13, color: 'var(--text-3)', marginTop: 3 }}>
              {teams.length} team{teams.length !== 1 ? 's' : ''} running
            </div>
          )}
        </div>
        <div style={{ flex: 1 }} />
        <button
          onClick={() => queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })}
          style={{
            background: 'none', border: '1px solid var(--border)',
            borderRadius: 'var(--r-md)',
            cursor: 'pointer', color: 'var(--text-3)',
            padding: '6px 10px',
            display: 'flex', alignItems: 'center', gap: 6,
            fontSize: 12,
            transition: 'border-color 0.1s, color 0.1s',
          }}
          onMouseEnter={e => {
            e.currentTarget.style.borderColor = 'var(--border-strong)'
            e.currentTarget.style.color = 'var(--text-2)'
          }}
          onMouseLeave={e => {
            e.currentTarget.style.borderColor = 'var(--border)'
            e.currentTarget.style.color = 'var(--text-3)'
          }}
          title="Refresh"
        >
          <RefreshCw size={13} />
          Refresh
        </button>
      </div>

      {/* Loading */}
      {isLoading && (
        <div style={{ color: 'var(--text-3)', fontSize: 13 }}>Loading…</div>
      )}

      {/* Empty state */}
      {!isLoading && teams.length === 0 && (
        <div style={{
          display: 'flex', flexDirection: 'column',
          alignItems: 'center', justifyContent: 'center',
          height: 360, gap: 20, textAlign: 'center',
        }}>
          <div style={{
            width: 64, height: 64, borderRadius: 18,
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}>
            <Zap size={28} color="var(--text-3)" />
          </div>
          <div>
            <div style={{ fontWeight: 600, fontSize: 16, color: 'var(--text-1)', marginBottom: 8, letterSpacing: '-0.02em' }}>
              No teams running
            </div>
            <div style={{ fontSize: 13, color: 'var(--text-3)', lineHeight: 1.6 }}>
              Start a team from your terminal:
            </div>
            <code style={{
              display: 'inline-block', marginTop: 10,
              fontFamily: '"SF Mono", "Fira Code", ui-monospace, monospace',
              fontSize: 13, color: 'var(--text-2)',
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border)',
              padding: '6px 14px', borderRadius: 'var(--r-md)',
              letterSpacing: '0.01em',
            }}>
              agen8 team start &lt;profile&gt;
            </code>
          </div>
        </div>
      )}

      {/* Cards grid */}
      {teams.length > 0 && (
        <div style={{ position: 'relative' }}>
          <div style={{
            display: 'flex', flexWrap: 'wrap', gap: 16,
          }}>
            {teams.map(team => (
              <div
                key={team.teamId}
                ref={setCardRef(team.teamId)}
                style={{ position: 'relative' }}
              >
                <TeamCard team={team} />
              </div>
            ))}
          </div>
          <TaskFlowArrows tasks={tasks} cardRefs={cardRefs.current} />
        </div>
      )}
    </div>
  )
}
