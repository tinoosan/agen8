import { Command } from 'cmdk'
import { useStore } from '../lib/store'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'
import PulseDot from './PulseDot'
import { Home, StopCircle } from 'lucide-react'

export default function CommandPalette() {
  const { setPaletteOpen, setFocusedTeamId } = useStore()
  const teamsQuery = useProjectTeams()
  const teams = teamsQuery.data ?? []
  const queryClient = useQueryClient()

  async function stopTeam(teamId: string) {
    await rpcCall('team.delete', { teamId })
    queryClient.invalidateQueries({ queryKey: ['project.listTeams'] })
    setPaletteOpen(false)
  }

  return (
    <div
      style={{
        position: 'fixed', inset: 0, zIndex: 100,
        background: 'rgba(0,0,0,0.6)',
        display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
        paddingTop: '14vh',
        backdropFilter: 'blur(6px)',
      }}
      onClick={() => setPaletteOpen(false)}
    >
      <Command
        onClick={e => e.stopPropagation()}
        className="animate-scale-in"
        style={{
          width: 560,
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
            placeholder="Search teams, run actions…"
            autoFocus
            style={{
              flex: 1,
              padding: '14px 0',
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
              padding: '2px 6px', borderRadius: 5,
              border: '1px solid var(--border)',
              fontFamily: 'inherit',
            }}
          >ESC</kbd>
        </div>

        <Command.List style={{ maxHeight: 380, overflowY: 'auto', padding: '6px 0' }}>
          <Command.Empty style={{
            padding: '28px 16px', textAlign: 'center',
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
                      padding: '9px 14px', cursor: 'pointer',
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
                    <span style={{ fontSize: 11, color: 'var(--text-3)' }}>{team.status}</span>
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
                padding: '9px 14px', cursor: 'pointer', fontSize: 13,
                display: 'flex', alignItems: 'center', gap: 10,
                color: 'var(--text-1)',
                borderRadius: 'var(--r-md)',
                margin: '1px 6px',
              }}
            >
              <Home size={13} style={{ color: 'var(--text-3)' }} />
              All teams
            </Command.Item>

            {teams.map(team => (
              <Command.Item
                key={`stop-${team.teamId}`}
                value={`stop delete team ${team.profileId}`}
                onSelect={() => stopTeam(team.teamId)}
                style={{
                  padding: '9px 14px', cursor: 'pointer', fontSize: 13,
                  display: 'flex', alignItems: 'center', gap: 10,
                  color: 'var(--red)',
                  borderRadius: 'var(--r-md)',
                  margin: '1px 6px',
                }}
              >
                <StopCircle size={13} />
                Stop {team.profileId ?? 'team'}
              </Command.Item>
            ))}
          </Command.Group>
        </Command.List>
      </Command>
    </div>
  )
}
