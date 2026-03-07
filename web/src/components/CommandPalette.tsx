import { Command } from 'cmdk'
import { useStore } from '../lib/store'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'
import PulseDot from './PulseDot'
import { Home, Trash2, Pause, Play, Square, Eraser } from 'lucide-react'

const DETACHED = 'detached-control'

export default function CommandPalette() {
  const { setPaletteOpen, setFocusedTeamId } = useStore()
  const teamsQuery = useProjectTeams()
  const teams = teamsQuery.data ?? []
  const queryClient = useQueryClient()

  async function deleteTeam(teamId: string) {
    if (!confirm('Delete this team? This action cannot be undone.')) return
    await rpcCall('team.delete', { teamId })
    queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })
    setPaletteOpen(false)
  }

  async function pauseTeam(sessionId: string) {
    await rpcCall('session.pause', { threadId: DETACHED, sessionId })
    queryClient.invalidateQueries({ queryKey: ['team.getStatus'] })
    queryClient.invalidateQueries({ queryKey: ['activity'] })
    setPaletteOpen(false)
  }

  async function resumeTeam(sessionId: string) {
    await rpcCall('session.resume', { threadId: DETACHED, sessionId })
    queryClient.invalidateQueries({ queryKey: ['team.getStatus'] })
    queryClient.invalidateQueries({ queryKey: ['activity'] })
    setPaletteOpen(false)
  }

  async function stopTeam(sessionId: string) {
    if (!confirm('Stop all runs for this team? This cannot be undone.')) return
    await rpcCall('session.stop', { threadId: DETACHED, sessionId })
    queryClient.invalidateQueries({ queryKey: ['team.getStatus'] })
    queryClient.invalidateQueries({ queryKey: ['activity'] })
    setPaletteOpen(false)
  }

  async function clearHistory(teamId: string) {
    if (!confirm('Clear all history for this team? This cannot be undone.')) return
    await rpcCall('session.clearHistory', { threadId: DETACHED, teamId })
    queryClient.invalidateQueries({ queryKey: ['team.getStatus'] })
    queryClient.invalidateQueries({ queryKey: ['activity'] })
    queryClient.invalidateQueries({ queryKey: ['logs.query'] })
    queryClient.invalidateQueries({ queryKey: ['item.list'] })
    setPaletteOpen(false)
  }

  return (
    <div
      style={{
        position: 'fixed', inset: 0, zIndex: 100,
        background: 'rgba(0,0,0,0.6)',
        display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
        paddingTop: '14vh',
        backdropFilter: 'blur(8px)',
      }}
      onClick={() => setPaletteOpen(false)}
    >
      <Command
        onClick={e => e.stopPropagation()}
        className="animate-scale-in"
        style={{
          width: 540,
          background: 'var(--bg-surface)',
          borderRadius: 'var(--r-xl)',
          border: '1px solid var(--border-strong)',
          boxShadow: '0 24px 80px rgba(0,0,0,0.6), 0 0 0 1px rgba(255,255,255,0.04)',
          overflow: 'hidden',
        }}
      >
        <div style={{
          display: 'flex', alignItems: 'center',
          padding: '0 16px',
          borderBottom: '1px solid var(--border)',
        }}>
          <Command.Input
            placeholder="Search teams or run actions…"
            autoFocus
            style={{
              flex: 1,
              padding: '15px 0',
              border: 'none', outline: 'none',
              background: 'transparent', color: 'var(--text-1)',
              fontSize: 14,
              fontFamily: 'inherit',
            }}
          />
          <kbd
            onClick={() => setPaletteOpen(false)}
            style={{
              fontSize: 11, cursor: 'pointer',
              background: 'var(--bg-elevated)', color: 'var(--text-3)',
              padding: '2px 7px', borderRadius: 5,
              border: '1px solid var(--border)',
              fontFamily: 'inherit',
              transition: 'color 0.15s',
            }}
          >ESC</kbd>
        </div>

        <Command.List style={{ maxHeight: 360, overflowY: 'auto', padding: '6px 0' }}>
          <Command.Empty style={{
            padding: '32px 16px', textAlign: 'center',
            color: 'var(--text-3)', fontSize: 13,
          }}>
            No results found
          </Command.Empty>

          {teams.length > 0 && (
            <Command.Group heading="Teams">
              {teams.map(team => {
                const status = team.status === 'running' ? 'active' : team.status === 'failed' ? 'failed' : 'idle'
                return (
                  <Command.Item
                    key={team.teamId}
                    value={`team-${team.teamId}-${team.profileId}`}
                    onSelect={() => { setFocusedTeamId(team.teamId); setPaletteOpen(false) }}
                    style={{
                      padding: '10px 14px', cursor: 'pointer',
                      display: 'flex', alignItems: 'center', gap: 10,
                      fontSize: 13, color: 'var(--text-1)',
                      borderRadius: 'var(--r-md)',
                      margin: '1px 6px',
                    }}
                  >
                    <PulseDot status={status} size={7} />
                    <span style={{ flex: 1, fontWeight: 500 }}>
                      {team.profileId ?? team.teamId.slice(0, 12)}
                    </span>
                    <span style={{
                      fontSize: 11, color: 'var(--text-3)',
                      textTransform: 'capitalize',
                    }}>
                      {team.status}
                    </span>
                  </Command.Item>
                )
              })}
            </Command.Group>
          )}

          <Command.Group heading="Actions">
            <Command.Item
              value="overview back home all teams"
              onSelect={() => { setFocusedTeamId(null); setPaletteOpen(false) }}
              style={{
                padding: '10px 14px', cursor: 'pointer', fontSize: 13,
                display: 'flex', alignItems: 'center', gap: 10,
                color: 'var(--text-1)',
                borderRadius: 'var(--r-md)',
                margin: '1px 6px',
              }}
            >
              <Home size={13} style={{ color: 'var(--text-3)' }} />
              Back to overview
            </Command.Item>

            {teams.map(team => {
              const sid = team.primarySessionId
              const name = team.profileId ?? 'team'
              return [
                sid && (
                  <Command.Item
                    key={`pause-${team.teamId}`}
                    value={`pause team ${team.profileId}`}
                    onSelect={() => pauseTeam(sid)}
                    style={{
                      padding: '10px 14px', cursor: 'pointer', fontSize: 13,
                      display: 'flex', alignItems: 'center', gap: 10,
                      color: 'var(--text-1)',
                      borderRadius: 'var(--r-md)',
                      margin: '1px 6px',
                    }}
                  >
                    <Pause size={13} style={{ color: 'var(--text-3)' }} />
                    Pause {name}
                  </Command.Item>
                ),
                sid && (
                  <Command.Item
                    key={`resume-${team.teamId}`}
                    value={`resume team ${team.profileId}`}
                    onSelect={() => resumeTeam(sid)}
                    style={{
                      padding: '10px 14px', cursor: 'pointer', fontSize: 13,
                      display: 'flex', alignItems: 'center', gap: 10,
                      color: 'var(--text-1)',
                      borderRadius: 'var(--r-md)',
                      margin: '1px 6px',
                    }}
                  >
                    <Play size={13} style={{ color: 'var(--text-3)' }} />
                    Resume {name}
                  </Command.Item>
                ),
                sid && (
                  <Command.Item
                    key={`stop-${team.teamId}`}
                    value={`stop team ${team.profileId}`}
                    onSelect={() => stopTeam(sid)}
                    style={{
                      padding: '10px 14px', cursor: 'pointer', fontSize: 13,
                      display: 'flex', alignItems: 'center', gap: 10,
                      color: 'var(--red)',
                      borderRadius: 'var(--r-md)',
                      margin: '1px 6px',
                    }}
                  >
                    <Square size={13} />
                    Stop {name}
                  </Command.Item>
                ),
                <Command.Item
                  key={`clear-${team.teamId}`}
                  value={`clear history team ${team.profileId}`}
                  onSelect={() => clearHistory(team.teamId)}
                  style={{
                    padding: '10px 14px', cursor: 'pointer', fontSize: 13,
                    display: 'flex', alignItems: 'center', gap: 10,
                    color: 'var(--amber)',
                    borderRadius: 'var(--r-md)',
                    margin: '1px 6px',
                  }}
                >
                  <Eraser size={13} />
                  Clear history for {name}
                </Command.Item>,
                <Command.Item
                  key={`delete-${team.teamId}`}
                  value={`delete remove team ${team.profileId}`}
                  onSelect={() => deleteTeam(team.teamId)}
                  style={{
                    padding: '10px 14px', cursor: 'pointer', fontSize: 13,
                    display: 'flex', alignItems: 'center', gap: 10,
                    color: 'var(--red)',
                    borderRadius: 'var(--r-md)',
                    margin: '1px 6px',
                  }}
                >
                  <Trash2 size={13} />
                  Delete {name}
                </Command.Item>,
              ]
            })}
          </Command.Group>
        </Command.List>
      </Command>
    </div>
  )
}
