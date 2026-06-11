import { useEffect, useMemo, useState } from 'react'
import { useLocation } from 'wouter'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, KeyRound, Minus, Plus, RotateCcw, Terminal, Trash2, UserRound } from 'lucide-react'
import { toast } from 'sonner'
import { useAuth } from '../../hooks/useAuth'
import { createAPIKey, listAPIKeys, revokeAPIKey } from '../../lib/authClient'
import {
  buildMCPSetup,
  CLAUDE_SKILL_COMMAND,
  CODEX_SKILL_COMMAND,
  type MCPSetupSnippets,
} from '../../lib/mcpSetup'
import {
  useStore,
  FONT_SCALE_DEFAULT,
  FONT_SCALE_MAX,
  FONT_SCALE_MIN,
  FONT_SCALE_STEP,
  type DefaultProjectView,
  type FontFamily,
  type Theme,
} from '../../lib/store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatDate } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { AuthAPIKey } from '@/lib/types'
import ConfirmationDialog from '@/components/ConfirmationDialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn, copyText } from '@/lib/utils'

/* Each theme tile renders a real miniature of the palette so the choice is
   visible before it's applied. Colors are pinned literals (not live tokens)
   so the preview is accurate regardless of the currently-active theme. */
type ThemeSwatch = { bg: string; surface: string; accent: string; text: string; muted: string }

const themeOptions: Array<{ value: Theme; label: string; group: 'Dark' | 'Light'; swatch: ThemeSwatch }> = [
  { value: 'dark', label: 'Charcoal', group: 'Dark', swatch: { bg: '#1a1a1c', surface: '#26262a', accent: '#3b82f6', text: '#f0f0f4', muted: '#636378' } },
  { value: 'midnight', label: 'Midnight', group: 'Dark', swatch: { bg: '#000000', surface: '#131317', accent: '#3b82f6', text: '#f5f5f8', muted: '#6a6a7e' } },
  { value: 'dim', label: 'Navy', group: 'Dark', swatch: { bg: '#151d2d', surface: '#213046', accent: '#1d9bf0', text: '#e7e9ea', muted: '#536471' } },
  { value: 'nebula', label: 'Nebula', group: 'Dark', swatch: { bg: '#16151d', surface: '#242230', accent: '#8b5cf6', text: '#f1f0f7', muted: '#6f6a85' } },
  { value: 'nord', label: 'Nord', group: 'Dark', swatch: { bg: '#2e3440', surface: '#3b4252', accent: '#5e81ac', text: '#eceff4', muted: '#7b8394' } },
  { value: 'rose', label: 'Rosé', group: 'Dark', swatch: { bg: '#191724', surface: '#26233a', accent: '#c4567a', text: '#e0def4', muted: '#6e6a86' } },
  { value: 'forest', label: 'Forest', group: 'Dark', swatch: { bg: '#0e1613', surface: '#1a2c24', accent: '#059669', text: '#e7f3ec', muted: '#5f7a6e' } },
  { value: 'ember', label: 'Ember', group: 'Dark', swatch: { bg: '#1d2021', surface: '#32302f', accent: '#d65d0e', text: '#ebdbb2', muted: '#928374' } },
  { value: 'light', label: 'Light', group: 'Light', swatch: { bg: '#ffffff', surface: '#f5f5f7', accent: '#2563eb', text: '#1d1d1f', muted: '#98989d' } },
  { value: 'sepia', label: 'Paper', group: 'Light', swatch: { bg: '#f4ecd8', surface: '#efe6cf', accent: '#b45309', text: '#3a2f1d', muted: '#9a876a' } },
  { value: 'solarized', label: 'Solarized', group: 'Light', swatch: { bg: '#fdf6e3', surface: '#f3ecd4', accent: '#268bd2', text: '#586e75', muted: '#93a1a1' } },
]

/* Preserve declaration order within each group so the picker reads top-to-
   bottom the same way the arrays are authored. */
const themeGroupOrder: Array<'Dark' | 'Light'> = ['Dark', 'Light']
const fontGroupOrder: Array<'Sans' | 'Serif' | 'Mono'> = ['Sans', 'Serif', 'Mono']

const defaultViewOptions: Array<{ value: DefaultProjectView; label: string; description: string }> = [
  { value: 'dashboard', label: 'Dashboard', description: 'Start from project health, missions, and actions.' },
  { value: 'strategy', label: 'Context Map', description: 'Open directly into the mission and context graph.' },
]

const fontFamilyOptions: Array<{ value: FontFamily; label: string; note: string; category: 'Sans' | 'Serif' | 'Mono'; stack: string }> = [
  { value: 'inter', label: 'Inter', note: 'Modern UI sans', category: 'Sans', stack: "'Inter Variable', system-ui, sans-serif" },
  { value: 'geist', label: 'Geist', note: 'Geometric sans', category: 'Sans', stack: "'Geist Variable', system-ui, sans-serif" },
  { value: 'figtree', label: 'Figtree', note: 'Friendly rounded', category: 'Sans', stack: "'Figtree Variable', system-ui, sans-serif" },
  { value: 'space-grotesk', label: 'Space Grotesk', note: 'Techy display', category: 'Sans', stack: "'Space Grotesk Variable', system-ui, sans-serif" },
  { value: 'atkinson', label: 'Atkinson', note: 'High legibility', category: 'Sans', stack: "'Atkinson Hyperlegible', system-ui, sans-serif" },
  { value: 'system', label: 'System', note: 'Native OS font', category: 'Sans', stack: "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif" },
  { value: 'serif', label: 'Source Serif', note: 'Reading serif', category: 'Serif', stack: "'Source Serif 4 Variable', Georgia, serif" },
  { value: 'lora', label: 'Lora', note: 'Warm book serif', category: 'Serif', stack: "'Lora Variable', Georgia, serif" },
  { value: 'fraunces', label: 'Fraunces', note: 'Expressive display', category: 'Serif', stack: "'Fraunces Variable', Georgia, serif" },
  { value: 'mono', label: 'JetBrains Mono', note: 'Monospace', category: 'Mono', stack: "'JetBrains Mono Variable', ui-monospace, monospace" },
]

const skillInstallCommands = [
  {
    harness: 'Codex',
    command: CODEX_SKILL_COMMAND,
    path: '~/.codex/skills/agen8/SKILL.md',
  },
  {
    harness: 'Claude CLI',
    command: CLAUDE_SKILL_COMMAND,
    path: '~/.claude/skills/agen8/SKILL.md',
  },
]

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
        <h3 className="m-0 text-[0.875rem] font-semibold text-[var(--text-1)]">{title}</h3>
        {description && <p className="mt-1 mb-0 text-[0.75rem] leading-relaxed text-[var(--text-3)]">{description}</p>}
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

function CopySnippetButton({ value, label = 'Copy' }: { value: string; label?: string }) {
  async function handleCopy() {
    try {
      await copyText(value)
      toast.success('Copied')
    } catch {
      toast.error('Copy failed')
    }
  }

  return (
    <Button type="button" variant="ghost" size="sm" className="gap-1.5" onClick={() => void handleCopy()}>
      <Copy size={13} />
      {label}
    </Button>
  )
}

function SetupSnippet({
  title,
  description,
  value,
}: {
  title: string
  description?: string
  value: string
}) {
  return (
    <div className="grid gap-2 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <div className="text-[0.8125rem] font-semibold text-[var(--text-1)]">{title}</div>
          {description && <p className="m-0 mt-1 text-[0.6875rem] leading-relaxed text-[var(--text-3)]">{description}</p>}
        </div>
        <CopySnippetButton value={value} />
      </div>
      <code className="block max-h-[220px] overflow-auto whitespace-pre-wrap break-all rounded-[var(--r-sm)] bg-[var(--bg-app)] px-2.5 py-2 text-[0.75rem] leading-relaxed text-[var(--text-1)]">
        {value}
      </code>
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
        <h2 className="m-0 text-[1.0625rem] font-semibold text-[var(--text-1)]">Account</h2>
        <p className="mt-1 mb-0 text-[0.8125rem] text-[var(--text-3)]">Your profile and sign-in details for this Agen8 instance.</p>
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
            <span className="text-[0.75rem] text-[var(--text-3)]">Shown as {displayName(auth.user)}</span>
            <Button type="button" size="sm" disabled={!profileDirty || savingProfile} onClick={() => void handleSaveProfile()}>
              {savingProfile ? 'Saving...' : 'Save profile'}
            </Button>
          </div>
        </SettingsRow>

        <SettingsRow title="Access" description="Account metadata and session control.">
          <div className="grid gap-3 xl:grid-cols-[1fr_auto] xl:items-center">
            <div className="grid gap-2 text-[0.75rem] text-[var(--text-2)] sm:grid-cols-2">
              <div>
                <span className="block text-[var(--text-3)]">Type</span>
                <span>{accountType(auth.user)}</span>
              </div>
              <div>
                <span className="block text-[var(--text-3)]">Joined</span>
                <span>{formatDate(auth.user?.createdAt, { fallback: 'Unknown' })}</span>
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
        <h2 className="m-0 text-[1.0625rem] font-semibold text-[var(--text-1)]">Security</h2>
        <p className="mt-1 mb-0 text-[0.8125rem] text-[var(--text-3)]">Manage active sessions and account access.</p>
      </div>
      <SettingsPanel>
        <SettingsRow title="Sessions" description="Browsers currently signed in to this account.">
          <div className="flex flex-col gap-3">
            <div className="grid gap-3 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="h-1.5 w-1.5 rounded-full bg-[var(--green)]" aria-hidden="true" />
                  <span className="text-[0.8125rem] font-semibold text-[var(--text-1)]">Current browser</span>
                  <span className="rounded-full bg-[var(--green-dim)] px-1.5 py-[1px] text-[0.625rem] font-medium uppercase leading-4 text-[var(--green)]">
                    Active
                  </span>
                </div>
                <div className="mt-1 truncate text-[0.75rem] text-[var(--text-3)]">
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

export function AccountMCPAccessSection() {
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [snippets, setSnippets] = useState<MCPSetupSnippets | null>(null)
  const [secret, setSecret] = useState('')
  const [revokeTarget, setRevokeTarget] = useState<AuthAPIKey | null>(null)
  const [revoking, setRevoking] = useState(false)

  const keysQuery = useQuery({
    queryKey: qk.apiKeys,
    queryFn: listAPIKeys,
  })
  const keys = keysQuery.data ?? []

  async function handleCreateKey() {
    setCreating(true)
    try {
      const result = await createAPIKey('Agen8 MCP key')
      const generated = buildMCPSetup(result.secret)
      setSecret(result.secret)
      setSnippets(generated)
      await queryClient.invalidateQueries({ queryKey: qk.apiKeys })
      toast.success('MCP key generated')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to generate MCP key')
    } finally {
      setCreating(false)
    }
  }

  async function handleRevokeKey() {
    if (!revokeTarget) return
    setRevoking(true)
    try {
      await revokeAPIKey(revokeTarget.id)
      await queryClient.invalidateQueries({ queryKey: qk.apiKeys })
      setRevokeTarget(null)
      toast.success('MCP key revoked')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to revoke MCP key')
    } finally {
      setRevoking(false)
    }
  }

  return (
    <section id="settings-mcp-access" className="grid gap-4">
      <div>
        <h2 className="m-0 text-[1.0625rem] font-semibold text-[var(--text-1)]">MCP access</h2>
        <p className="mt-1 mb-0 text-[0.8125rem] text-[var(--text-3)]">
          Generate the token and setup text needed to connect Codex, Claude Code, or another MCP client.
        </p>
      </div>
      <SettingsPanel>
        <SettingsRow title="Harness token" description="Keys are shown once. Generate a new one any time you need another client connection.">
          <div className="grid gap-3">
            <div className="grid gap-3 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
              <div className="min-w-0">
                <div className="mb-1 flex items-center gap-2 text-[0.8125rem] font-semibold text-[var(--text-1)]">
                  <KeyRound size={14} aria-hidden />
                  Agen8 MCP key
                </div>
                <p className="m-0 text-[0.75rem] leading-relaxed text-[var(--text-3)]">
                  Use this for local MCP clients. Keep it private like a password.
                </p>
              </div>
              <Button type="button" size="sm" onClick={() => void handleCreateKey()} disabled={creating}>
                {creating ? 'Generating...' : snippets ? 'Generate another key' : 'Generate MCP key'}
              </Button>
            </div>

            {snippets && (
              <div className="grid gap-3">
                <SetupSnippet
                  title="API key"
                  description="Copy this now. Agen8 will not show the same key again."
                  value={secret}
                />
                <SetupSnippet
                  title="MCP URL"
                  description="Use this endpoint when a client asks for a server URL."
                  value={snippets.url}
                />
                <SetupSnippet
                  title=".mcp.json"
                  description="Use this project config when your MCP client reads JSON server entries."
                  value={snippets.jsonConfig}
                />
                <SetupSnippet
                  title="Codex command"
                  description="Adds Agen8 through the Codex CLI."
                  value={snippets.codexCommand}
                />
                <SetupSnippet
                  title="Claude Code command"
                  description="Adds Agen8 for Claude Code at user scope."
                  value={snippets.claudeCommand}
                />
                <SetupSnippet
                  title="Hooks — Claude Code"
                  description="Optional. Run inside a project to see when its agents are waiting on you."
                  value={snippets.hooksClaudeCommand}
                />
                <SetupSnippet
                  title="Hooks — Codex"
                  description="Optional. Run once; covers all Codex sessions."
                  value={snippets.hooksCodexCommand}
                />
              </div>
            )}

            <div className="grid gap-2">
              <div className="text-[0.8125rem] font-semibold text-[var(--text-1)]">Existing MCP keys</div>
              {keysQuery.isLoading ? (
                <div className="rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-3 text-[0.75rem] text-[var(--text-3)]">
                  Loading keys...
                </div>
              ) : keys.length === 0 ? (
                <div className="rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-3 text-[0.75rem] text-[var(--text-3)]">
                  No MCP keys yet.
                </div>
              ) : (
                <div className="overflow-hidden rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)]">
                  {keys.map((key) => (
                    <div
                      key={key.id}
                      className="grid gap-3 border-b border-[var(--border)] px-3 py-3 last:border-b-0 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="truncate text-[0.8125rem] font-semibold text-[var(--text-1)]">{key.name}</span>
                          <span className={cn(
                            'rounded-full px-1.5 py-[1px] text-[0.625rem] font-medium uppercase leading-4',
                            key.active
                              ? 'bg-[var(--green-dim)] text-[var(--green)]'
                              : 'bg-[var(--bg-elevated)] text-[var(--text-3)]',
                          )}>
                            {key.active ? 'Active' : 'Revoked'}
                          </span>
                        </div>
                        <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[0.6875rem] text-[var(--text-3)]">
                          <span className="font-mono">{key.prefix}</span>
                          <span>Created {formatDate(key.createdAt, { fallback: 'Unknown' })}</span>
                          {key.revokedAt && <span>Revoked {formatDate(key.revokedAt, { fallback: 'Unknown' })}</span>}
                        </div>
                      </div>
                      <Button
                        type="button"
                        variant="ghost-danger"
                        size="sm"
                        className="gap-1.5"
                        onClick={() => setRevokeTarget(key)}
                        disabled={!key.active}
                      >
                        <Trash2 size={13} />
                        Revoke
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </SettingsRow>
      </SettingsPanel>
      <ConfirmationDialog
        open={!!revokeTarget}
        title="Revoke MCP key"
        message={revokeTarget ? `Revoke "${revokeTarget.name}"? Clients using this key will stop connecting to Agen8.` : ''}
        confirmLabel={revoking ? 'Revoking...' : 'Revoke key'}
        tone="danger"
        busy={revoking}
        onClose={() => setRevokeTarget(null)}
        onConfirm={() => void handleRevokeKey()}
      />
    </section>
  )
}

export function AccountSkillSection() {
  async function copyCommand(command: string) {
    try {
      await copyText(command)
      toast.success('Command copied')
    } catch {
      toast.error('Copy failed')
    }
  }

  return (
    <section id="settings-skill" className="grid gap-4">
      <div>
        <h2 className="m-0 text-[1.0625rem] font-semibold text-[var(--text-1)]">Agen8 skill</h2>
        <p className="mt-1 mb-0 text-[0.8125rem] text-[var(--text-3)]">Install or refresh the harness instructions that teach agents how to use Agen8.</p>
      </div>
      <SettingsPanel>
        <SettingsRow title="Install" description="Run the command for each harness you use. Rerun it after updating Agen8.">
          <div className="grid gap-3">
            {skillInstallCommands.map((item) => (
              <div
                key={item.harness}
                className="grid gap-3 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
              >
                <div className="min-w-0">
                  <div className="mb-2 flex items-center gap-2 text-[0.8125rem] font-semibold text-[var(--text-1)]">
                    <Terminal size={14} aria-hidden />
                    {item.harness}
                  </div>
                  <code className="block overflow-x-auto rounded-[var(--r-sm)] bg-[var(--bg-app)] px-2.5 py-2 text-[0.75rem] text-[var(--text-1)]">
                    {item.command}
                  </code>
                  <p className="mt-2 mb-0 text-[0.6875rem] text-[var(--text-3)]">
                    Installs to {item.path}.
                  </p>
                </div>
                <Button type="button" variant="ghost" size="sm" className="gap-1.5" onClick={() => void copyCommand(item.command)}>
                  <Copy size={13} />
                  Copy
                </Button>
              </div>
            ))}
          </div>
        </SettingsRow>
      </SettingsPanel>
    </section>
  )
}

function ThemeTile({
  label,
  swatch,
  selected,
  onSelect,
}: {
  label: string
  swatch: ThemeSwatch
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'group flex flex-col gap-2 rounded-[var(--r-lg)] border p-2 text-left transition-all',
        selected
          ? 'border-[var(--accent)] ring-1 ring-[var(--accent)] ring-offset-1 ring-offset-[var(--bg-app)]'
          : 'border-[var(--border)] hover:border-[var(--border-strong)]',
      )}
    >
      <div
        className="relative flex h-[68px] flex-col justify-between overflow-hidden rounded-[var(--r-md)] p-2.5"
        style={{ background: swatch.bg, boxShadow: 'inset 0 0 0 1px rgba(128,128,128,0.12)' }}
      >
        <div className="flex flex-col gap-1.5">
          <span className="block h-1.5 w-3/4 rounded-full" style={{ background: swatch.text }} />
          <span className="block h-1.5 w-1/2 rounded-full" style={{ background: swatch.muted }} />
        </div>
        <div className="flex items-center gap-1.5">
          <span className="h-3.5 w-9 rounded-full" style={{ background: swatch.accent }} />
          <span className="h-3.5 w-3.5 rounded-full" style={{ background: swatch.surface, boxShadow: 'inset 0 0 0 1px rgba(128,128,128,0.18)' }} />
        </div>
      </div>
      <div className="flex items-center justify-between px-0.5">
        <span className={cn('text-[0.75rem] font-medium', selected ? 'text-[var(--text-1)]' : 'text-[var(--text-2)]')}>
          {label}
        </span>
        {selected && <Check size={13} className="text-[var(--accent)]" aria-hidden />}
      </div>
    </button>
  )
}

function FontTile({
  label,
  note,
  stack,
  selected,
  onSelect,
}: {
  label: string
  note: string
  stack: string
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        'flex items-center gap-3 rounded-[var(--r-md)] border px-3 py-2.5 text-left transition-all',
        selected
          ? 'border-[var(--accent)] ring-1 ring-[var(--accent)]'
          : 'border-[var(--border)] hover:border-[var(--border-strong)] hover:bg-[var(--bg-hover)]',
      )}
    >
      <span
        className="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--r-sm)] bg-[var(--bg-surface)] text-[1.125rem] leading-none text-[var(--text-1)]"
        style={{ fontFamily: stack }}
        aria-hidden
      >
        Ag
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[0.8125rem] font-medium text-[var(--text-1)]" style={{ fontFamily: stack }}>
          {label}
        </span>
        <span className="block truncate text-[0.6875rem] text-[var(--text-3)]">{note}</span>
      </span>
      {selected && <Check size={14} className="shrink-0 text-[var(--accent)]" aria-hidden />}
    </button>
  )
}

/* A tiny group heading that lets the grids below breathe as the option
   count grows — the picker stays scannable instead of one long wall. */
function PickerGroupLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-2 mt-1 flex items-center gap-2 first:mt-0">
      <span className="text-[0.625rem] font-semibold uppercase tracking-[0.08em] text-[var(--text-3)]">
        {children}
      </span>
      <span className="h-px flex-1 bg-[var(--border)]" aria-hidden />
    </div>
  )
}

export function AccountPreferencesSection() {
  const theme = useStore((s) => s.theme)
  const setTheme = useStore((s) => s.setTheme)
  const fontFamily = useStore((s) => s.fontFamily)
  const setFontFamily = useStore((s) => s.setFontFamily)
  const fontScale = useStore((s) => s.fontScale)
  const stepFontScale = useStore((s) => s.stepFontScale)
  const resetFontScale = useStore((s) => s.resetFontScale)
  const defaultProjectView = useStore((s) => s.defaultProjectView)
  const setDefaultProjectView = useStore((s) => s.setDefaultProjectView)

  const atMin = fontScale <= FONT_SCALE_MIN
  const atMax = fontScale >= FONT_SCALE_MAX
  const isDefaultScale = fontScale === FONT_SCALE_DEFAULT

  return (
    <section id="settings-preferences" className="grid gap-4">
      <div>
        <h2 className="m-0 text-[1.0625rem] font-semibold text-[var(--text-1)]">Preferences</h2>
        <p className="mt-1 mb-0 text-[0.8125rem] text-[var(--text-3)]">Personal defaults saved to your account.</p>
      </div>
      <SettingsPanel>
        <SettingsRow title="Theme" description="Pick a palette. Changes apply instantly across the app.">
          <div className="flex flex-col gap-4">
            {themeGroupOrder.map((group) => {
              const items = themeOptions.filter((option) => option.group === group)
              if (items.length === 0) return null
              return (
                <div key={group}>
                  <PickerGroupLabel>{group}</PickerGroupLabel>
                  <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(7rem,1fr))]">
                    {items.map(({ value, label, swatch }) => (
                      <ThemeTile
                        key={value}
                        label={label}
                        swatch={swatch}
                        selected={theme === value}
                        onSelect={() => setTheme(value)}
                      />
                    ))}
                  </div>
                </div>
              )
            })}
          </div>
        </SettingsRow>

        <SettingsRow title="Typeface" description="The font used across the interface.">
          <div className="flex flex-col gap-4">
            {fontGroupOrder.map((category) => {
              const items = fontFamilyOptions.filter((option) => option.category === category)
              if (items.length === 0) return null
              return (
                <div key={category}>
                  <PickerGroupLabel>{category}</PickerGroupLabel>
                  <div className="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(13rem,1fr))]">
                    {items.map(({ value, label, note, stack }) => (
                      <FontTile
                        key={value}
                        label={label}
                        note={note}
                        stack={stack}
                        selected={fontFamily === value}
                        onSelect={() => setFontFamily(value)}
                      />
                    ))}
                  </div>
                </div>
              )
            })}
          </div>
        </SettingsRow>

        <SettingsRow title="Text size" description="Scales every label, button, and panel together.">
          <div className="flex flex-col gap-4">
            <div className="flex flex-wrap items-center gap-3">
              <div className="inline-flex items-center gap-1 rounded-[var(--r-md)] border border-[var(--border)] p-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label="Decrease text size"
                  disabled={atMin}
                  onClick={() => stepFontScale(-FONT_SCALE_STEP)}
                >
                  <Minus size={15} />
                </Button>
                <div className="min-w-[58px] text-center text-[0.8125rem] font-semibold tabular-nums text-[var(--text-1)]">
                  {fontScale}<span className="ml-0.5 text-[0.6875rem] font-normal text-[var(--text-3)]">px</span>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label="Increase text size"
                  disabled={atMax}
                  onClick={() => stepFontScale(FONT_SCALE_STEP)}
                >
                  <Plus size={15} />
                </Button>
              </div>
              <span className="text-[0.6875rem] text-[var(--text-3)]">
                {FONT_SCALE_MIN}–{FONT_SCALE_MAX}px
              </span>
              {!isDefaultScale && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="gap-1.5 text-[var(--text-3)] hover:text-[var(--text-1)]"
                  onClick={resetFontScale}
                >
                  <RotateCcw size={12} />
                  Reset
                </Button>
              )}
            </div>
            <p
              className="m-0 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-2.5 leading-snug text-[var(--text-2)]"
              style={{ fontSize: `${fontScale}px` }}
            >
              The quick brown fox jumps over the lazy dog.
            </p>
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
            <p className="m-0 text-[0.75rem] leading-relaxed text-[var(--text-3)]">
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
      <AccountMCPAccessSection />
      <AccountSkillSection />
      <AccountPreferencesSection />
    </>
  )
}
