/**
 * AccountChip — sidebar footer block. Account actions (Settings,
 * Credentials, Sign out) render as always-visible rows
 * for one-click access. A status/identity row (daemon health dot,
 * avatar, name, theme toggle) is pinned at the bottom.
 */
import { useState } from 'react'
import { useLocation } from 'wouter'
import {
  UserRound, Plug, KeyRound, LogOut, Moon, Sun,
} from 'lucide-react'
import { useAuth } from '../../hooks/useAuth'
import { useStore, isLightTheme } from '../../lib/store'
import { accountDisplayName } from '../../lib/accountHelpers'

const ROW_CLS = 'flex items-center gap-2 w-full px-2.5 py-[6px] rounded-[6px] text-[0.8125rem] text-[var(--text-3)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-2)] cursor-pointer border-none bg-transparent transition-colors text-left'
const ROW_STYLE = { letterSpacing: '-0.08px' } as const
const DOT_CLS = 'w-1.5 h-1.5 min-w-[6px] rounded-full shrink-0'

export function AccountChip() {
  const auth = useAuth()
  const [, navigate] = useLocation()
  const [loggingOut, setLoggingOut] = useState(false)

  const bridgeConnected = !!auth.bridge?.connected

  // Composite dot: green if nothing needs attention, amber otherwise.
  const needsAttention = auth.bridge !== null && !bridgeConnected
  const dotColor = needsAttention ? 'var(--amber)' : 'var(--green)'

  const identity = accountDisplayName(auth.user)

  const toggleThemeMode = useStore((s) => s.toggleThemeMode)
  const theme = useStore((s) => s.theme)
  const isLight = isLightTheme(theme)

  async function handleHostedLogout() {
    setLoggingOut(true)
    try {
      await auth.logout()
      navigate('/login')
    } finally {
      setLoggingOut(false)
    }
  }

  return (
    <div className="flex flex-col gap-0.5">
      {/* Account actions — one click each */}
      <button type="button" className={ROW_CLS} style={ROW_STYLE} onClick={() => navigate('/account')}>
        <UserRound size={14} className="shrink-0" />
        <span>Settings</span>
      </button>
      <button type="button" className={ROW_CLS} style={ROW_STYLE} onClick={() => navigate('/credentials')}>
        <KeyRound size={14} className="shrink-0" />
        <span>Credentials</span>
      </button>
      <button
        type="button"
        className={ROW_CLS}
        style={ROW_STYLE}
        onClick={() => void handleHostedLogout()}
        disabled={loggingOut}
      >
        <LogOut size={14} className="shrink-0" />
        <span>{loggingOut ? 'Signing out…' : 'Sign out'}</span>
      </button>

      <div className="h-px mx-2.5 my-1 bg-[var(--border)] opacity-40" />

      {/* Daemon status — only shown when a local daemon is present */}
      {auth.bridge !== null && (
        <div
          className="flex items-center gap-2 w-full px-2.5 py-[6px] rounded-[6px] text-[0.8125rem] text-[var(--text-3)]"
          style={ROW_STYLE}
          title={bridgeConnected ? 'Local daemon connected' : 'Local daemon offline'}
        >
          <span className={DOT_CLS} style={{ backgroundColor: bridgeConnected ? 'var(--green)' : 'var(--amber)' }} />
          <Plug size={13} className="shrink-0" />
          <span>Local daemon</span>
          <span className="ml-auto text-[0.6875rem] text-[var(--text-3)]" style={{ letterSpacing: '-0.06px' }}>
            {bridgeConnected ? 'Connected' : 'Offline'}
          </span>
        </div>
      )}

      {/* Identity + theme toggle, pinned at the bottom */}
      <div className="flex items-center gap-2 px-2.5 py-[6px]" style={{ fontSize: '0.75rem' }}>
        <span className={DOT_CLS} style={{ backgroundColor: dotColor }} />
        <span className="w-4 h-4 rounded-full bg-[var(--bg-elevated)] flex items-center justify-center shrink-0">
          <UserRound size={10} className="text-[var(--text-2)]" />
        </span>
        <span className="flex-1 min-w-0 truncate text-[var(--text-2)] font-medium" title={identity}>
          {identity}
        </span>
        <button
          type="button"
          className="shrink-0 bg-transparent border-none cursor-pointer p-0.5 text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors"
          onClick={toggleThemeMode}
          title={isLight ? 'Switch to dark mode' : 'Switch to light mode'}
          aria-label={isLight ? 'Switch to dark mode' : 'Switch to light mode'}
        >
          {isLight ? <Sun size={13} /> : <Moon size={13} />}
        </button>
      </div>
    </div>
  )
}
