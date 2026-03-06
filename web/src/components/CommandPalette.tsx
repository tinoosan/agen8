import { Command } from 'cmdk'
import { useStore } from '../lib/store'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { rpcCall } from '../lib/rpc'
import { useQueryClient } from '@tanstack/react-query'

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
        background: 'rgba(0,0,0,0.4)',
        display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
        paddingTop: '15vh',
        backdropFilter: 'blur(4px)',
      }}
      onClick={() => setPaletteOpen(false)}
    >
      <Command
        onClick={e => e.stopPropagation()}
        style={{
          width: 560,
          background: 'light-dark(#ffffff, #1a1a1c)',
          borderRadius: 16,
          border: '1px solid light-dark(rgba(0,0,0,0.1), rgba(255,255,255,0.1))',
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          overflow: 'hidden',
        }}
      >
        <Command.Input
          placeholder="Switch team, search…"
          autoFocus
          style={{
            width: '100%', padding: '14px 16px',
            border: 'none', outline: 'none',
            background: 'transparent', color: 'inherit',
            fontSize: 14, borderBottom: '1px solid light-dark(rgba(0,0,0,0.08), rgba(255,255,255,0.08))',
          }}
        />
        <Command.List style={{ maxHeight: 360, overflowY: 'auto', padding: '8px 0' }}>
          <Command.Empty style={{ padding: '24px 16px', textAlign: 'center', opacity: 0.4, fontSize: 13 }}>
            No results
          </Command.Empty>

          {teams.length > 0 && (
            <Command.Group heading={<span style={{ fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', opacity: 0.35, padding: '0 16px' }}>Teams</span>}>
              {teams.map(team => (
                <Command.Item
                  key={team.teamId}
                  value={`team-${team.teamId}-${team.profileId}`}
                  onSelect={() => { setFocusedTeamId(team.teamId); setPaletteOpen(false) }}
                  style={{
                    padding: '8px 16px', cursor: 'pointer',
                    display: 'flex', alignItems: 'center', gap: 10,
                    fontSize: 13,
                  }}
                >
                  <span style={{ flex: 1 }}>{team.profileId ?? team.teamId.slice(0, 12)}</span>
                  <span style={{ fontSize: 11, opacity: 0.4 }}>{team.status}</span>
                </Command.Item>
              ))}
            </Command.Group>
          )}

          <Command.Group heading={<span style={{ fontSize: 10, fontWeight: 700, letterSpacing: '0.08em', textTransform: 'uppercase', opacity: 0.35, padding: '0 16px' }}>Actions</span>}>
            <Command.Item
              value="overview back home"
              onSelect={() => { setFocusedTeamId(null); setPaletteOpen(false) }}
              style={{ padding: '8px 16px', cursor: 'pointer', fontSize: 13 }}
            >
              ← Back to overview
            </Command.Item>
            {teams.length > 0 && teams.map(team => (
              <Command.Item
                key={`stop-${team.teamId}`}
                value={`stop delete team ${team.profileId}`}
                onSelect={() => stopTeam(team.teamId)}
                style={{ padding: '8px 16px', cursor: 'pointer', fontSize: 13, color: '#ef4444' }}
              >
                Stop {team.profileId ?? 'team'}
              </Command.Item>
            ))}
          </Command.Group>
        </Command.List>
      </Command>
    </div>
  )
}
