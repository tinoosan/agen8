import { useStore } from '../lib/store'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { Settings, Search, ChevronLeft, Zap } from 'lucide-react'

function TotalCost({ teamIds }: { teamIds: string[] }) {
  const statuses = teamIds.map(id => useTeamStatus(id))
  const total = statuses.reduce((sum, s) => sum + (s.data?.totalCostUSD ?? 0), 0)
  if (total === 0) return null
  return (
    <span style={{ fontSize: 12, color: 'var(--text-3)', letterSpacing: '-0.01em' }}>
      ${total.toFixed(2)}
    </span>
  )
}

function ActiveCount({ teamIds }: { teamIds: string[] }) {
  const statuses = teamIds.map(id => useTeamStatus(id))
  const active = statuses.filter(s => (s.data?.active ?? 0) > 0).length
  if (active === 0) return null
  return (
    <span style={{
      fontSize: 11, fontWeight: 500,
      background: 'var(--green-dim)',
      color: 'var(--green)',
      borderRadius: 999,
      padding: '2px 8px',
      border: '1px solid rgba(34,197,94,0.2)',
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
      height: 44,
      display: 'flex', alignItems: 'center', gap: 8,
      padding: '0 14px',
      borderBottom: '1px solid var(--border)',
      background: 'rgba(12,12,15,0.9)',
      backdropFilter: 'blur(20px)',
      WebkitBackdropFilter: 'blur(20px)',
      position: 'relative', zIndex: 10,
      flexShrink: 0,
    }}>
      {focusedTeamId ? (
        <button
          onClick={() => setFocusedTeamId(null)}
          style={{
            display: 'flex', alignItems: 'center', gap: 5,
            background: 'none', border: 'none', cursor: 'pointer',
            padding: '4px 8px', borderRadius: 'var(--r-md)',
            fontSize: 13, color: 'var(--text-2)',
            transition: 'color 0.1s, background 0.1s',
          }}
          onMouseEnter={e => {
            e.currentTarget.style.color = 'var(--text-1)'
            e.currentTarget.style.background = 'var(--bg-hover)'
          }}
          onMouseLeave={e => {
            e.currentTarget.style.color = 'var(--text-2)'
            e.currentTarget.style.background = 'transparent'
          }}
        >
          <ChevronLeft size={13} />
          <span>Teams</span>
        </button>
      ) : (
        <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
          <div style={{
            width: 22, height: 22, borderRadius: 6,
            background: 'linear-gradient(135deg, #8b7bf8, #6366f1)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            flexShrink: 0,
          }}>
            <Zap size={12} color="#fff" fill="#fff" strokeWidth={0} />
          </div>
          <span style={{
            fontWeight: 600, fontSize: 14,
            letterSpacing: '-0.025em',
            color: 'var(--text-1)',
          }}>agen8</span>
        </div>
      )}

      <div style={{ flex: 1 }} />

      {teamIds.length > 0 && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <ActiveCount teamIds={teamIds} />
          <TotalCost teamIds={teamIds} />
        </div>
      )}

      {/* Divider */}
      <div style={{ width: 1, height: 16, background: 'var(--border)', margin: '0 2px' }} />

      <button
        onClick={() => setPaletteOpen(true)}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '5px 10px', borderRadius: 'var(--r-md)',
          border: '1px solid var(--border)',
          background: 'var(--bg-surface)',
          cursor: 'pointer', fontSize: 12,
          color: 'var(--text-2)',
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
        <Search size={11} />
        <span>Search</span>
        <kbd style={{
          fontSize: 10, fontFamily: 'inherit',
          background: 'var(--bg-elevated)', color: 'var(--text-3)',
          padding: '1px 4px', borderRadius: 4,
          border: '1px solid var(--border)',
          lineHeight: 1.6,
        }}>⌘K</kbd>
      </button>

      <button
        style={{
          background: 'none', border: 'none', cursor: 'pointer',
          padding: 6, borderRadius: 'var(--r-md)',
          color: 'var(--text-3)',
          display: 'flex', alignItems: 'center',
          transition: 'color 0.1s, background 0.1s',
        }}
        onMouseEnter={e => {
          e.currentTarget.style.color = 'var(--text-2)'
          e.currentTarget.style.background = 'var(--bg-hover)'
        }}
        onMouseLeave={e => {
          e.currentTarget.style.color = 'var(--text-3)'
          e.currentTarget.style.background = 'transparent'
        }}
      >
        <Settings size={14} />
      </button>
    </header>
  )
}
