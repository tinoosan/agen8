import { useEffect, useRef, useState } from 'react'
import { Link } from 'wouter'
import { WifiOff, KeyRound, Wifi } from 'lucide-react'
import { subscribeConnectionState, type ConnectionState } from '../lib/rpc'

/**
 * ConnectionBanner — the honest "you're looking at stale data" strip.
 *
 * Daemon restarts (air rebuilds during dev, upgrades in prod) drop the SSE
 * stream and fail queries; the app used to keep showing cached data with no
 * indication. This surfaces the rpc client's connection state:
 *
 *  - reconnecting: shown only after a grace period, so a fast daemon bounce
 *    never flashes a scary banner; clears itself when the stream reopens.
 *  - auth_blocked: the stream was refused (401/403) — reconnecting would be a
 *    lie, so it asks for sign-in instead.
 *  - a brief "Reconnected" confirmation when recovery follows a visible outage.
 */
const DISCONNECT_GRACE_MS = 3_000
const RECONNECTED_FLASH_MS = 2_500

type BannerMode = 'hidden' | 'reconnecting' | 'auth_blocked' | 'reconnected'

export default function ConnectionBanner() {
  const [mode, setMode] = useState<BannerMode>('hidden')
  const graceTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const flashTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wasVisiblyDown = useRef(false)

  useEffect(() => {
    const clearTimers = () => {
      if (graceTimer.current) clearTimeout(graceTimer.current)
      if (flashTimer.current) clearTimeout(flashTimer.current)
      graceTimer.current = null
      flashTimer.current = null
    }
    const unsubscribe = subscribeConnectionState((state: ConnectionState) => {
      clearTimers()
      if (state === 'reconnecting') {
        graceTimer.current = setTimeout(() => {
          wasVisiblyDown.current = true
          setMode('reconnecting')
        }, DISCONNECT_GRACE_MS)
        return
      }
      if (state === 'auth_blocked') {
        wasVisiblyDown.current = true
        setMode('auth_blocked')
        return
      }
      // connected
      if (wasVisiblyDown.current) {
        wasVisiblyDown.current = false
        setMode('reconnected')
        flashTimer.current = setTimeout(() => setMode('hidden'), RECONNECTED_FLASH_MS)
      } else {
        setMode('hidden')
      }
    })
    return () => {
      clearTimers()
      unsubscribe()
    }
  }, [])

  if (mode === 'hidden') return null

  const tone =
    mode === 'reconnected'
      ? { bg: 'color-mix(in srgb, var(--green) 14%, var(--bg-elevated))', fg: 'var(--green)' }
      : mode === 'auth_blocked'
        ? { bg: 'color-mix(in srgb, var(--red) 14%, var(--bg-elevated))', fg: 'var(--red)' }
        : { bg: 'color-mix(in srgb, var(--amber) 14%, var(--bg-elevated))', fg: 'var(--amber)' }

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex items-center justify-center gap-2 px-4 py-1.5 text-[0.75rem] font-medium shrink-0"
      style={{ background: tone.bg, color: tone.fg }}
    >
      {mode === 'reconnecting' && (
        <>
          <WifiOff size={13} aria-hidden />
          <span>Connection to agen8 lost — reconnecting. Data may be out of date.</span>
        </>
      )}
      {mode === 'auth_blocked' && (
        <>
          <KeyRound size={13} aria-hidden />
          <span>
            Session expired —{' '}
            <Link to="/login" className="underline underline-offset-2" style={{ color: 'inherit' }}>
              sign in again
            </Link>
          </span>
        </>
      )}
      {mode === 'reconnected' && (
        <>
          <Wifi size={13} aria-hidden />
          <span>Reconnected.</span>
        </>
      )}
    </div>
  )
}
