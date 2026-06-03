/**
 * AccountChip — footer user row with status dot, avatar, identity label,
 * and theme toggle. Expands into a Popover showing per-service status
 * and account actions (settings, credentials, logout).
 *
 * Uses shadcn Popover for proper focus management, keyboard dismiss,
 * and enter/exit transitions.
 */
import { useState } from 'react'
import { useLocation } from 'wouter'
import {
  UserRound, Plug, MapPin, LogOut, Moon, Sun,
} from 'lucide-react'
import { useAuth } from '../../hooks/useAuth'
import { useStore } from '../../lib/store'
import { accountDisplayName, accountSubtitle } from '../../lib/accountHelpers'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

export function AccountChip() {
  const auth = useAuth()
  const [, navigate] = useLocation()
  const [open, setOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)

  const bridgeConnected = !!auth.bridge?.connected

  // Composite dot: green if nothing needs attention, amber otherwise.
  const needsAttention = auth.bridge !== null && !bridgeConnected
  const dotColor = needsAttention ? 'var(--amber)' : 'var(--green)'

  const identity = accountDisplayName(auth.user)
  const subtitle = accountSubtitle(auth.user)

  const setTheme = useStore((s) => s.setTheme)
  const theme = useStore((s) => s.theme)

  async function handleHostedLogout() {
    setLoggingOut(true)
    try {
      await auth.logout()
      navigate('/login')
    } finally {
      setLoggingOut(false)
      setOpen(false)
    }
  }

  const busy = loggingOut

  const rowCls = 'flex items-center gap-2 w-full px-3 py-2 rounded-[6px] text-[13px] text-[var(--text-2)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)] cursor-pointer border-none bg-transparent transition-colors'
  const rowStyle = { letterSpacing: '-0.08px' } as const
  const dotCls = 'w-1.5 h-1.5 min-w-[6px] rounded-full shrink-0'
  const statusLabelCls = 'ml-auto text-[11px] text-[var(--text-3)]'
  const statusLabelStyle = { letterSpacing: '-0.06px' } as const

  return (
    <Popover open={open} onOpenChange={setOpen}>
      {/* Footer user row — dot + avatar + name + theme icons */}
      <div className="flex items-center gap-[7px] w-full cursor-pointer" style={{ fontSize: '12px' }}>
        <PopoverTrigger asChild>
          <button
            type="button"
            className="flex items-center gap-[7px] flex-1 min-w-0 bg-transparent border-none cursor-pointer p-0 text-left"
            title={identity}
          >
            <span className={dotCls} style={{ backgroundColor: dotColor }} />
            <span className="w-4 h-4 rounded-full bg-[var(--bg-elevated)] flex items-center justify-center shrink-0">
              <UserRound size={10} className="text-[var(--text-2)]" />
            </span>
            <span className="truncate text-[var(--text-2)] font-medium">{identity}</span>
          </button>
        </PopoverTrigger>
        <span className="ml-auto flex items-center gap-1">
          <button
            type="button"
            className="bg-transparent border-none cursor-pointer p-0.5 text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors"
            onClick={() => setTheme(theme === 'light' ? 'dark' : 'light')}
            title={theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'}
          >
            {theme === 'light' ? <Sun size={13} /> : <Moon size={13} />}
          </button>
        </span>
      </div>

      <PopoverContent
        side="top"
        align="start"
        sideOffset={6}
        className="w-[var(--radix-popover-trigger-width)] min-w-[220px] p-2 bg-[var(--bg-panel)] rounded-[12px] shadow-[var(--shadow-xl)] max-h-[min(70vh,480px)] overflow-y-auto"
      >
        {/* Account header */}
        {auth.user && (
          <div className="px-3 py-2 mb-1">
            <div className="text-[13px] font-semibold text-[var(--text-1)] truncate" style={{ letterSpacing: '-0.08px' }}>
              {identity}
            </div>
            {subtitle && (
              <div className="text-[11px] text-[var(--text-3)] truncate mt-0.5" style={{ letterSpacing: '-0.06px' }}>
                {subtitle}
              </div>
            )}
          </div>
        )}

        {/* Per-service status rows */}
        {auth.bridge !== null && (
          <div className={rowCls} style={rowStyle} title={bridgeConnected ? 'Local daemon connected' : 'Local daemon offline'}>
            <span className={dotCls} style={{ backgroundColor: bridgeConnected ? 'var(--green)' : 'var(--amber)' }} />
            <Plug size={13} />
            <span>Local daemon</span>
            <span className={statusLabelCls} style={statusLabelStyle}>
              {bridgeConnected ? 'Connected' : 'Offline'}
            </span>
          </div>
        )}

        {/* Separator + actions */}
        <div className="h-px my-1.5 bg-[var(--border)] opacity-40" />

        <button type="button" className={rowCls} style={rowStyle} onClick={() => { setOpen(false); navigate('/account') }}>
          <UserRound size={13} />
          <span>Settings</span>
        </button>
        <button type="button" className={rowCls} style={rowStyle} onClick={() => { setOpen(false); navigate('/locations') }}>
          <MapPin size={13} />
          <span>Locations</span>
        </button>
        <button type="button" className={rowCls} style={rowStyle} onClick={() => void handleHostedLogout()} disabled={busy}>
          <LogOut size={13} />
          <span>{busy ? 'Signing out…' : 'Sign out'}</span>
        </button>
      </PopoverContent>
    </Popover>
  )
}
