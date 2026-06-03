import { useEffect, useMemo, useState } from 'react'
import { useLocation } from 'wouter'
import { Moon, Sun, UserRound } from 'lucide-react'
import { toast } from 'sonner'
import { useAuth } from '../../hooks/useAuth'
import { useStore, type DefaultProjectView, type Theme } from '../../lib/store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

const themeOptions: Array<{ value: Theme; label: string; icon: typeof Moon }> = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
]

const defaultViewOptions: Array<{ value: DefaultProjectView; label: string; description: string }> = [
  { value: 'dashboard', label: 'Dashboard', description: 'Start from project health, missions, and actions.' },
  { value: 'strategy', label: 'Strategy map', description: 'Open directly into the mission and context graph.' },
]

function formatDate(value?: string): string {
  if (!value) return 'Unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

function displayName(user: { name?: string; email?: string } | null): string {
  const name = user?.name?.trim()
  if (name) return name
  const email = user?.email?.trim()
  if (email) return email
  return 'Account'
}

function accountType(user: { email?: string } | null): string {
  const email = user?.email?.trim().toLowerCase() ?? ''
  if (!email) return 'Account'
  return 'Email account'
}

function SettingsPanel({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div className={cn('border-y border-[var(--border)]', className)}>
      {children}
    </div>
  )
}

function SettingsRow({
  title,
  description,
  children,
  className,
}: {
  title: string
  description?: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('grid gap-4 border-b border-[var(--border)] py-5 last:border-b-0 xl:grid-cols-[220px_minmax(0,1fr)]', className)}>
      <div>
        <h3 className="m-0 text-[14px] font-semibold text-[var(--text-1)]">{title}</h3>
        {description && <p className="mt-1 mb-0 text-[12px] leading-relaxed text-[var(--text-3)]">{description}</p>}
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

export function AccountProfileSection() {
  const auth = useAuth()
  const [, navigate] = useLocation()
  const [name, setName] = useState(auth.user?.name ?? '')
  const [email, setEmail] = useState(auth.user?.email ?? '')
  const [savingProfile, setSavingProfile] = useState(false)

  useEffect(() => {
    setName(auth.user?.name ?? '')
    setEmail(auth.user?.email ?? '')
  }, [auth.user?.email, auth.user?.name])

  const normalizedName = name.trim()
  const normalizedEmail = email.trim().toLowerCase()
  const profileDirty = useMemo(() => (
    normalizedName !== (auth.user?.name ?? '').trim()
    || normalizedEmail !== (auth.user?.email ?? '').trim().toLowerCase()
  ), [auth.user?.email, auth.user?.name, normalizedEmail, normalizedName])

  async function handleSaveProfile() {
    if (!normalizedName) {
      toast.error('Name is required')
      return
    }
    if (!normalizedEmail || !normalizedEmail.includes('@')) {
      toast.error('Enter a valid email address')
      return
    }
    setSavingProfile(true)
    try {
      await auth.updateProfile({ name: normalizedName, email: normalizedEmail })
      toast.success('Profile updated')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update profile')
    } finally {
      setSavingProfile(false)
    }
  }

  async function handleLogout() {
    try {
      await auth.logout()
      navigate('/login')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to sign out')
    }
  }

  return (
    <section id="settings-account" className="grid gap-4">
      <div>
        <h2 className="m-0 text-[17px] font-semibold text-[var(--text-1)]">Account</h2>
        <p className="mt-1 mb-0 text-[13px] text-[var(--text-3)]">Your profile and sign-in details for this Agen8 instance.</p>
      </div>

      <SettingsPanel>
        <SettingsRow title="Profile" description="How Agen8 identifies you in the app.">
          <div className="grid gap-4 xl:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="account-name" className="inline-flex items-center gap-2">
                <UserRound size={14} />
                Name
              </Label>
              <Input id="account-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Your name" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="account-email">Email</Label>
              <Input id="account-email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@example.com" />
            </div>
          </div>
          <div className="mt-4 flex items-center justify-between gap-3">
            <span className="text-[12px] text-[var(--text-3)]">Shown as {displayName(auth.user)}</span>
            <Button type="button" size="sm" disabled={!profileDirty || savingProfile} onClick={() => void handleSaveProfile()}>
              {savingProfile ? 'Saving...' : 'Save profile'}
            </Button>
          </div>
        </SettingsRow>

        <SettingsRow title="Access" description="Account metadata and session control.">
          <div className="grid gap-3 xl:grid-cols-[1fr_auto] xl:items-center">
            <div className="grid gap-2 text-[12px] text-[var(--text-2)] sm:grid-cols-2">
              <div>
                <span className="block text-[var(--text-3)]">Type</span>
                <span>{accountType(auth.user)}</span>
              </div>
              <div>
                <span className="block text-[var(--text-3)]">Joined</span>
                <span>{formatDate(auth.user?.createdAt)}</span>
              </div>
            </div>
            <Button variant="ghost-danger" size="sm" onClick={() => void handleLogout()}>
              Sign out
            </Button>
          </div>
        </SettingsRow>
      </SettingsPanel>
    </section>
  )
}

export function AccountSecuritySection() {
  const auth = useAuth()
  const [signingOut, setSigningOut] = useState(false)

  async function handleLogout() {
    setSigningOut(true)
    try {
      await auth.logout()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to sign out')
      setSigningOut(false)
    }
  }

  return (
    <section id="settings-security" className="grid gap-4">
      <div>
        <h2 className="m-0 text-[17px] font-semibold text-[var(--text-1)]">Security</h2>
        <p className="mt-1 mb-0 text-[13px] text-[var(--text-3)]">Manage active sessions and account access.</p>
      </div>
      <SettingsPanel>
        <SettingsRow title="Sessions" description="Browsers currently signed in to this account.">
          <div className="flex flex-col gap-3">
            <div className="grid gap-3 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="h-1.5 w-1.5 rounded-full bg-[var(--green)]" aria-hidden="true" />
                  <span className="text-[13px] font-semibold text-[var(--text-1)]">Current browser</span>
                  <span className="rounded-full bg-[var(--green-dim)] px-1.5 py-[1px] text-[10px] font-medium uppercase leading-4 text-[var(--green)]">
                    Active
                  </span>
                </div>
                <div className="mt-1 truncate text-[12px] text-[var(--text-3)]">
                  Signed in as {displayName(auth.user)}
                </div>
              </div>
              <Button variant="ghost-danger" size="sm" onClick={() => void handleLogout()} disabled={signingOut}>
                {signingOut ? 'Signing out...' : 'Sign out'}
              </Button>
            </div>
          </div>
        </SettingsRow>
      </SettingsPanel>
    </section>
  )
}

export function AccountPreferencesSection() {
  const theme = useStore((s) => s.theme)
  const setTheme = useStore((s) => s.setTheme)
  const defaultProjectView = useStore((s) => s.defaultProjectView)
  const setDefaultProjectView = useStore((s) => s.setDefaultProjectView)

  return (
    <section id="settings-preferences" className="grid gap-4">
      <div>
        <h2 className="m-0 text-[17px] font-semibold text-[var(--text-1)]">Preferences</h2>
        <p className="mt-1 mb-0 text-[13px] text-[var(--text-3)]">Personal defaults saved in this browser.</p>
      </div>
      <SettingsPanel>
        <SettingsRow title="Appearance" description="Choose how Agen8 looks in this browser.">
          <div className="grid gap-2">
            <Label htmlFor="theme-select">Theme</Label>
            <Select value={theme} onValueChange={(value) => setTheme(value as Theme)}>
              <SelectTrigger id="theme-select" className="w-full max-w-[280px] border-[var(--border)] bg-transparent">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {themeOptions.map(({ value, label, icon: Icon }) => (
                  <SelectItem key={value} value={value}>
                    <span className="inline-flex items-center gap-2">
                      <Icon size={13} />
                      {label}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </SettingsRow>
        <SettingsRow title="Project start view" description="Pick where projects open by default.">
          <div className="grid gap-2">
            <Label htmlFor="default-view-select">Default project view</Label>
            <Select value={defaultProjectView} onValueChange={(value) => setDefaultProjectView(value as DefaultProjectView)}>
              <SelectTrigger id="default-view-select" className="w-full max-w-[280px] border-[var(--border)] bg-transparent">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {defaultViewOptions.map(({ value, label }) => (
                  <SelectItem key={value} value={value}>{label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="m-0 text-[12px] leading-relaxed text-[var(--text-3)]">
              {defaultViewOptions.find((option) => option.value === defaultProjectView)?.description}
            </p>
          </div>
        </SettingsRow>
      </SettingsPanel>
    </section>
  )
}

export function AccountSettingsSections() {
  return (
    <>
      <AccountProfileSection />
      <AccountSecuritySection />
      <AccountPreferencesSection />
    </>
  )
}
