import { useRef, useCallback } from 'react'
import { useStore } from '../lib/store'
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
  const focusedProjectRoot = useStore(s => s.focusedProjectRoot)
  const teamsQuery = useProjectTeams(focusedProjectRoot)
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
    <div style={{ height: '100%', overflowY: 'auto', padding: '40px 44px' }}>
      {/* Header */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 12,
        marginBottom: 36,
      }}>
        <div>
          <h1 style={{
            margin: 0, fontSize: 26, fontWeight: 700,
            letterSpacing: '-0.04em', color: 'var(--text-1)',
            lineHeight: 1.1,
          }}>
            Teams
          </h1>
          {teams.length > 0 && (
            <div style={{ fontSize: 13, color: 'var(--text-3)', marginTop: 4 }}>
              {teams.length} team{teams.length !== 1 ? 's' : ''} running
            </div>
          )}
        </div>
        <div style={{ flex: 1 }} />
        <button
          className="btn-outline"
          onClick={() => queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })}
          title="Refresh teams list"
        >
          <RefreshCw size={13} />
          Refresh
        </button>
      </div>

      {/* Loading */}
      {isLoading && (
        <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
          {[1, 2, 3].map(i => (
            <div key={i} className="skeleton" style={{
              width: 300, height: 180, borderRadius: 'var(--r-xl)',
            }} />
          ))}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && teams.length === 0 && (
        <div style={{
          display: 'flex', flexDirection: 'column',
          alignItems: 'center', justifyContent: 'center',
          height: 400, gap: 24, textAlign: 'center',
        }}>
          <div style={{
            width: 72, height: 72, borderRadius: 20,
            background: 'var(--accent-dim)',
            border: '1px solid rgba(139,123,248,0.2)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}>
            <Zap size={32} color="var(--accent)" fill="var(--accent)" strokeWidth={0} />
          </div>
          <div>
            <div style={{ fontWeight: 600, fontSize: 18, color: 'var(--text-1)', marginBottom: 8, letterSpacing: '-0.02em' }}>
              No teams running
            </div>
            <div style={{ fontSize: 14, color: 'var(--text-3)', lineHeight: 1.6 }}>
              Start a team from your terminal
            </div>
            <code className="mono" style={{
              display: 'inline-block', marginTop: 14,
              fontSize: 13, color: 'var(--accent)',
              background: 'var(--bg-elevated)',
              border: '1px solid var(--border)',
              padding: '8px 18px', borderRadius: 'var(--r-md)',
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
            display: 'flex', flexWrap: 'wrap', gap: 18,
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
