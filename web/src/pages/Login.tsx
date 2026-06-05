import { useState } from 'react'
import { useLocation } from 'wouter'
import { Moon, Sun } from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { useStore } from '../lib/store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/** The agen8 mark geometry (from agen8-mark.svg), scaled up as a hero motif. */
const HEX = 'M128 36 L207 82 L207 174 L128 220 L49 174 L49 82 Z'
const HEX_ACCENT_EDGE = 'M128 36 L207 82'

/** Brand panel: a fixed navy surface with the hexagon mark blown up and an
 *  accent glow. Intentionally constant across all three themes — only the form
 *  side follows dark/dim/light. */
function BrandHero() {
  return (
    <aside
      className="relative hidden lg:flex flex-col justify-between overflow-hidden p-12 text-white"
      style={{
        background:
          'radial-gradient(115% 115% at 12% 8%, rgba(59,130,246,0.38) 0%, transparent 52%),' +
          'radial-gradient(100% 90% at 92% 100%, rgba(59,130,246,0.20) 0%, transparent 58%),' +
          'linear-gradient(155deg, #0B1220 0%, #14213A 100%)',
      }}
    >
      {/* Hero hexagon motif — the full mark, centered, with network nodes */}
      <svg
        viewBox="0 0 256 256"
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 m-auto h-auto w-[440px] max-w-[68%]"
        fill="none"
      >
        {/* faint connectors out to network nodes */}
        <path d="M207 82 L240 58" stroke="#3B82F6" strokeOpacity="0.5" strokeWidth="1.5" />
        <path d="M49 174 L20 202" stroke="#ffffff" strokeOpacity="0.14" strokeWidth="1.5" />
        <path d="M128 220 L128 250" stroke="#ffffff" strokeOpacity="0.12" strokeWidth="1.5" />
        {/* main hexagon */}
        <path d={HEX} stroke="#ffffff" strokeOpacity="0.22" strokeWidth="2.5" strokeLinejoin="round" />
        {/* accent signature edge + glowing node */}
        <path d={HEX_ACCENT_EDGE} stroke="#3B82F6" strokeWidth="3.5" strokeLinecap="round" />
        <circle cx="207" cy="82" r="6.5" fill="#3B82F6" className="animate-pulse" />
        {/* network node dots */}
        <circle cx="240" cy="58" r="3" fill="#3B82F6" fillOpacity="0.75" />
        <circle cx="49" cy="82" r="3" fill="#ffffff" fillOpacity="0.4" />
        <circle cx="20" cy="202" r="2.5" fill="#ffffff" fillOpacity="0.28" />
        <circle cx="128" cy="250" r="2.5" fill="#ffffff" fillOpacity="0.3" />
      </svg>

      {/* Wordmark */}
      <div className="relative z-10 flex items-center gap-2.5">
        <svg viewBox="0 0 256 256" className="h-7 w-7" fill="none" aria-hidden="true">
          <path d={HEX} stroke="#ffffff" strokeWidth="14" strokeLinejoin="round" strokeOpacity="0.92" />
          <path d={HEX_ACCENT_EDGE} stroke="#3B82F6" strokeWidth="14" strokeLinecap="round" />
          <circle cx="207" cy="82" r="14" fill="#3B82F6" />
        </svg>
        <span className="text-[1.0625rem] font-semibold tracking-[-0.03em]">agen8</span>
      </div>

      {/* Headline + tagline */}
      <div className="relative z-10 max-w-[26ch]">
        <h2 className="m-0 text-[1.875rem] font-semibold leading-[1.15] tracking-[-0.03em]">
          Mission control for your agents.
        </h2>
        <p className="mt-3 mb-0 text-[0.875rem] leading-relaxed text-white/65">
          Register work context, coordinate autonomous roles, and stay in the loop on every decision.
        </p>
      </div>
    </aside>
  )
}

export default function Login() {
  const auth = useAuth()
  const [, navigate] = useLocation()
  const theme = useStore((s) => s.theme)
  const setTheme = useStore((s) => s.setTheme)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submittingPassword, setSubmittingPassword] = useState(false)

  function toggleTheme() {
    setTheme(theme === 'light' ? 'dark' : 'light')
  }

  const ThemeIcon = theme === 'light' ? Sun : Moon
  const toggleLabel = theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmittingPassword(true)
    setError('')
    try {
      await auth.login({ email, password })
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setSubmittingPassword(false)
    }
  }

  return (
    <div className="grid h-full w-full grid-cols-1 overflow-hidden lg:grid-cols-[1.1fr_1fr]">
      <BrandHero />

      <main className="relative flex items-center justify-center bg-[var(--bg-app)] px-6 py-10">
        <button
          type="button"
          onClick={toggleTheme}
          className="absolute right-5 top-5 rounded-[var(--r-md)] p-2 text-[var(--text-3)] transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-2)]"
          title={toggleLabel}
          aria-label={toggleLabel}
        >
          <ThemeIcon size={18} />
        </button>

        <form onSubmit={handleSubmit} className="flex w-full max-w-[360px] animate-fade-in flex-col gap-5">
          {/* Compact wordmark — only when the hero panel is hidden (mobile) */}
          <div className="flex items-center gap-2.5 lg:hidden">
            <svg viewBox="0 0 256 256" className="h-7 w-7" fill="none" aria-hidden="true">
              <path d={HEX} stroke="var(--text-1)" strokeWidth="14" strokeLinejoin="round" />
              <path d={HEX_ACCENT_EDGE} stroke="var(--accent)" strokeWidth="14" strokeLinecap="round" />
              <circle cx="207" cy="82" r="14" fill="var(--accent)" />
            </svg>
            <span className="text-[1.0625rem] font-semibold tracking-[-0.03em] text-[var(--text-1)]">agen8</span>
          </div>

          <div className="flex flex-col gap-1.5">
            <h1 className="m-0 text-[1.625rem] font-semibold tracking-[-0.03em] text-[var(--text-1)]">Welcome back</h1>
            <p className="m-0 text-[0.8125rem] text-[var(--text-3)]">Sign in to your agen8 workspace.</p>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="email" className="text-[0.75rem] text-[var(--text-2)]">
              Email
            </Label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              autoComplete="email"
              required
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="password" className="text-[0.75rem] text-[var(--text-2)]">
              Password
            </Label>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              required
            />
          </div>

          {error && (
            <div
              role="alert"
              className="rounded-[var(--r-md)] border border-[var(--red)]/30 bg-[var(--red)]/10 px-3 py-2 text-[0.75rem] text-[var(--red)]"
            >
              {error}
            </div>
          )}

          <Button type="submit" size="lg" className="w-full" disabled={submittingPassword || auth.isLoading}>
            {submittingPassword ? 'Signing in...' : 'Sign in'}
          </Button>
        </form>
      </main>
    </div>
  )
}
