import { useRef, useCallback } from 'react'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { rpcCall } from '../lib/rpc'
import TeamCard from '../components/TeamCard'
import TaskFlowArrows from '../components/TaskFlowArrows'
import type { Task } from '../lib/types'
import { RefreshCw } from 'lucide-react'

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

  // Card element refs for SVG arrows
  const cardRefs = useRef<Record<string, HTMLElement | null>>({})
  const setCardRef = useCallback((teamId: string) => (el: HTMLElement | null) => {
    cardRefs.current[teamId] = el
  }, [])

  const isLoading = teamsQuery.isLoading
  const queryClient = useQueryClient()

  return (
    <div style={{ height: '100%', overflowY: 'auto', padding: '32px 32px' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 28 }}>
        <h1 style={{ margin: 0, fontSize: 22, fontWeight: 700, letterSpacing: '-0.03em' }}>
          Teams
        </h1>
        {teams.length > 0 && (
          <span style={{ fontSize: 13, opacity: 0.4 }}>{teams.length} running</span>
        )}
        <div style={{ flex: 1 }} />
        <button
          onClick={() => queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })}
          style={{ background: 'none', border: 'none', cursor: 'pointer', opacity: 0.4, color: 'inherit', padding: 4 }}
          title="Refresh"
        >
          <RefreshCw size={14} />
        </button>
      </div>

      {/* Loading */}
      {isLoading && (
        <div style={{ opacity: 0.35, fontSize: 13 }}>Loading teams…</div>
      )}

      {/* Empty state */}
      {!isLoading && teams.length === 0 && (
        <div style={{
          display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
          height: 320, gap: 16, opacity: 0.4, textAlign: 'center',
        }}>
          <div style={{ fontSize: 40 }}>◎</div>
          <div>
            <div style={{ fontWeight: 600, fontSize: 15, marginBottom: 6 }}>No teams running</div>
            <div style={{ fontSize: 13 }}>
              Start a team with <code style={{ fontFamily: 'monospace', background: 'light-dark(rgba(0,0,0,0.06), rgba(255,255,255,0.06))', padding: '1px 5px', borderRadius: 4 }}>agen8 team start &lt;profile&gt;</code>
            </div>
          </div>
        </div>
      )}

      {/* Team cards grid (relative so SVG can overlay) */}
      {teams.length > 0 && (
        <div style={{ position: 'relative' }}>
          <div style={{
            display: 'flex', flexWrap: 'wrap', gap: 20,
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

          {/* SVG arrows overlay for cross-team task flow */}
          <TaskFlowArrows tasks={tasks} cardRefs={cardRefs.current} />
        </div>
      )}
    </div>
  )
}
