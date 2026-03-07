import { useStore, type Theme, type ActiveView } from '../lib/store'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { Search, ChevronLeft, Zap, Sun, Moon, Monitor, LayoutGrid, BarChart3, ScrollText, Coins } from 'lucide-react'

function ThemePicker() {
  const { theme, setTheme } = useStore()
  const options: { value: Theme; icon: typeof Sun; label: string }[] = [
    { value: 'light', icon: Sun, label: 'Light' },
    { value: 'dim', icon: Monitor, label: 'Dim' },
    { value: 'dark', icon: Moon, label: 'Dark' },
  ]
  return (
    <div className="theme-picker">
      {options.map(({ value, icon: Icon, label }) => (
        <button
          key={value}
          className={theme === value ? 'active' : ''}
          onClick={() => setTheme(value)}
          title={label}
        >
          <Icon size={13} />
        </button>
      ))}
    </div>
  )
}

function TotalCost({ teamIds }: { teamIds: string[] }) {
  const statuses = teamIds.map(id => useTeamStatus(id))
  const total = statuses.reduce((sum, s) => sum + (s.data?.totalCostUSD ?? 0), 0)
  if (total === 0) return null
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 5,
      fontSize: 12, fontWeight: 600,
      color: 'var(--amber)',
      background: 'var(--amber-dim)',
      padding: '3px 10px', borderRadius: 999,
      letterSpacing: '-0.01em',
      fontVariantNumeric: 'tabular-nums',
    }}>
      <Coins size={12} />
      ${total.toFixed(2)}
    </span>
  )
}

function ActiveCount({ teamIds }: { teamIds: string[] }) {
  const statuses = teamIds.map(id => useTeamStatus(id))
  const active = statuses.filter(s => (s.data?.active ?? 0) > 0).length
  if (active === 0) return null
  return (
    <span className="badge badge-green" style={{ fontSize: 11, fontWeight: 500, padding: '2px 8px' }}>
      {active} active
    </span>
  )
}

function NavTabs() {
  const { activeView, setActiveView } = useStore()
  const tabs: { value: ActiveView; label: string; icon: typeof LayoutGrid }[] = [
    { value: 'overview', label: 'Teams', icon: LayoutGrid },
    { value: 'dashboard', label: 'Dashboard', icon: BarChart3 },
    { value: 'logs', label: 'Logs', icon: ScrollText },
  ]
  return (
    <div style={{ display: 'flex', gap: 2, marginLeft: 8 }}>
      {tabs.map(({ value, label, icon: Icon }) => {
        const active = activeView === value
        return (
          <button
            key={value}
            onClick={() => setActiveView(value)}
            style={{
              display: 'flex', alignItems: 'center', gap: 5,
              padding: '5px 10px',
              borderRadius: 'var(--r-md)',
              border: 'none',
              background: active ? 'var(--bg-active)' : 'transparent',
              color: active ? 'var(--text-1)' : 'var(--text-3)',
              fontSize: 12,
              fontWeight: 500,
              fontFamily: 'inherit',
              cursor: 'pointer',
              transition: 'color 0.15s, background 0.15s',
            }}
          >
            <Icon size={12} />
            {label}
          </button>
        )
      })}
    </div>
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
      display: 'flex', alignItems: 'center', gap: 10,
      padding: '0 16px',
      borderBottom: '1px solid var(--border)',
      background: 'color-mix(in srgb, var(--bg-app) 92%, transparent)',
      backdropFilter: 'blur(20px)',
      WebkitBackdropFilter: 'blur(20px)',
      position: 'relative', zIndex: 10,
      flexShrink: 0,
    }}>
      {focusedTeamId ? (
        <button
          className="btn-ghost"
          onClick={() => setFocusedTeamId(null)}
          style={{ gap: 5, padding: '5px 10px', fontSize: 13, color: 'var(--text-2)' }}
        >
          <ChevronLeft size={14} />
          <span>Teams</span>
        </button>
      ) : (
        <>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <div style={{
              width: 24, height: 24, borderRadius: 7,
              background: 'linear-gradient(135deg, #8b7bf8, #6366f1)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              flexShrink: 0,
            }}>
              <Zap size={13} color="#fff" fill="#fff" strokeWidth={0} />
            </div>
            <span style={{
              fontWeight: 600, fontSize: 15,
              letterSpacing: '-0.03em',
              color: 'var(--text-1)',
            }}>agen8</span>
          </div>
          <NavTabs />
        </>
      )}

      <div style={{ flex: 1 }} />

      {teamIds.length > 0 && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <ActiveCount teamIds={teamIds} />
          <TotalCost teamIds={teamIds} />
        </div>
      )}

      {/* Divider */}
      <div style={{ width: 1, height: 18, background: 'var(--border-strong)', margin: '0 2px' }} />

      <button
        className="btn-surface"
        onClick={() => setPaletteOpen(true)}
        style={{ padding: '5px 10px', gap: 7 }}
      >
        <Search size={12} />
        <span style={{ fontSize: 12 }}>Search</span>
        <kbd style={{
          fontSize: 10, fontFamily: 'inherit',
          background: 'var(--bg-app)', color: 'var(--text-3)',
          padding: '1px 5px', borderRadius: 4,
          border: '1px solid var(--border)',
          lineHeight: 1.5,
        }}>⌘K</kbd>
      </button>

      <ThemePicker />
    </header>
  )
}
