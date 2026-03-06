import { useStore } from '../lib/store'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { Settings, Search } from 'lucide-react'

function TotalCost({ teamIds }: { teamIds: string[] }) {
  const statuses = teamIds.map(id => useTeamStatus(id))
  const total = statuses.reduce((sum, s) => sum + (s.data?.totalCostUSD ?? 0), 0)
  if (total === 0) return null
  return <span style={{ fontSize: 13, color: 'light-dark(#71717a, #a1a1aa)' }}>${total.toFixed(2)}</span>
}

function ActiveCount({ teamIds }: { teamIds: string[] }) {
  const statuses = teamIds.map(id => useTeamStatus(id))
  const active = statuses.filter(s => (s.data?.active ?? 0) > 0).length
  if (active === 0) return null
  return (
    <span style={{
      fontSize: 12, fontWeight: 600,
      background: 'rgba(34,197,94,0.15)', color: '#22c55e',
      borderRadius: 999, padding: '2px 8px',
    }}>
      {active} active
    </span>
  )
}

export default function TopBar() {
  const { focusedTeamId, setFocusedTeamId, setPaletteOpen } = useStore()
  const teamsQuery = useProjectTeams()
  const teams = teamsQuery.data ?? []
  const teamIds = teams.map(t => t.teamId)

  return (
    <header style={{
      height: 48,
      display: 'flex', alignItems: 'center', gap: 12,
      padding: '0 16px',
      borderBottom: '1px solid light-dark(rgba(0,0,0,0.08), rgba(255,255,255,0.08))',
      background: 'light-dark(rgba(255,255,255,0.8), rgba(15,15,16,0.8))',
      backdropFilter: 'blur(12px)',
      WebkitBackdropFilter: 'blur(12px)',
      position: 'relative', zIndex: 10,
      flexShrink: 0,
    }}>
      {focusedTeamId ? (
        <button
          onClick={() => setFocusedTeamId(null)}
          style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '4px 8px', borderRadius: 6, fontSize: 13, color: 'inherit', opacity: 0.6 }}
        >
          ← Teams
        </button>
      ) : (
        <span style={{ fontWeight: 700, fontSize: 15, letterSpacing: '-0.02em' }}>agen8</span>
      )}

      <div style={{ flex: 1 }} />

      {teamIds.length > 0 && (
        <>
          <ActiveCount teamIds={teamIds} />
          <TotalCost teamIds={teamIds} />
        </>
      )}

      <button
        onClick={() => setPaletteOpen(true)}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '4px 10px', borderRadius: 8, border: '1px solid light-dark(rgba(0,0,0,0.1), rgba(255,255,255,0.1))',
          background: 'light-dark(rgba(0,0,0,0.04), rgba(255,255,255,0.04))',
          cursor: 'pointer', fontSize: 12, color: 'inherit', opacity: 0.7,
        }}
      >
        <Search size={12} />
        <span>⌘K</span>
      </button>

      <button style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 4, borderRadius: 6, opacity: 0.6, color: 'inherit' }}>
        <Settings size={16} />
      </button>
    </header>
  )
}
