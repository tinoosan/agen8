import { useMemo, useState } from 'react'
import { ExternalLink, HardDrive, KeyRound, Plus, RefreshCw, Server, ShieldCheck, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { useCredentialCreate } from '../hooks/useCredentials'
import {
  useClaudeAuthStatus,
  useClaudeLogin,
  useClaudeLoginComplete,
  useCodexAuthStatus,
  useCodexLogin,
  useCreateLocation,
  useDeleteLocation,
  useInstallClaude,
  useInstallCodex,
  useLocations,
  useProbeLocation,
  type ClaudeLoginResult,
  type CodexLoginResult,
} from '../hooks/useLocations'
import type { ExecutionLocation } from '../lib/types'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'

type SSHAuthChoice = 'password' | 'key'

function locationName(location: ExecutionLocation): string {
  if (location.label?.trim()) return location.label.trim()
  if (location.kind === 'local') return 'This machine'
  if (location.address?.host) return location.address.host
  return location.id
}

function locationEndpoint(location: ExecutionLocation): string {
  if (location.kind === 'local') return location.address?.host?.trim() || 'Local daemon host'
  const address = location.address
  if (!address?.host) return location.kind
  const user = address.username ? `${address.username}@` : ''
  const port = address.port ? `:${address.port}` : ''
  return `${user}${address.host}${port}`
}

function formatDate(value?: string): string {
  if (!value) return 'Never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function capabilityLabel(name: string): string {
  switch (name) {
    case 'fileBrowsing':
      return 'Files'
    case 'exec':
      return 'Shell'
    case 'codex':
      return 'Codex'
    case 'claude':
      return 'Claude'
    case 'reachable':
      return 'Reachable'
    default:
      return name
  }
}

function locationAuthLabel(location: ExecutionLocation): string | null {
  if (location.kind !== 'ssh') return null
  if (location.auth?.credentialId) return 'Credential attached'
  return 'No credential'
}

function claudeLoginRequired(location: ExecutionLocation): boolean {
  return /not logged in|claude.*login/i.test(location.lastProbe?.message ?? '')
}

function LocationRow({
  location,
  probing,
  installingCodex,
  installingClaude,
  checkingCodexAuth,
  loggingIntoCodex,
  checkingClaudeAuth,
  loggingIntoClaude,
  deleting,
  onProbe,
  onInstallCodex,
  onInstallClaude,
  onCheckCodexAuth,
  onCodexLogin,
  onCheckClaudeAuth,
  onClaudeLogin,
  onDelete,
}: {
  location: ExecutionLocation
  probing: boolean
  installingCodex: boolean
  installingClaude: boolean
  checkingCodexAuth: boolean
  loggingIntoCodex: boolean
  checkingClaudeAuth: boolean
  loggingIntoClaude: boolean
  deleting: boolean
  onProbe: () => void
  onInstallCodex: () => void
  onInstallClaude: () => void
  onCheckCodexAuth: () => void
  onCodexLogin: () => void
  onCheckClaudeAuth: () => void
  onClaudeLogin: () => void
  onDelete: () => void
}) {
  const Icon = location.kind === 'local' ? HardDrive : Server
  const capabilities = location.capabilities ?? []

  return (
    <div className="grid gap-4 border-b border-[var(--border)] py-5 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--r-md)] bg-[var(--bg-surface)] text-[var(--text-2)]">
            <Icon size={16} />
          </span>
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <h3 className="m-0 truncate text-[14px] font-semibold text-[var(--text-1)]">{locationName(location)}</h3>
              <span className={cn(
                'shrink-0 rounded-full px-1.5 py-[1px] text-[10px] font-medium uppercase leading-4',
                location.ready ? 'bg-[var(--green-dim)] text-[var(--green)]' : 'bg-[var(--bg-surface)] text-[var(--text-3)]',
              )}>
                {location.ready ? 'Ready' : location.status.replaceAll('_', ' ')}
              </span>
            </div>
            <p className="m-0 mt-1 truncate font-[var(--font-mono,monospace)] text-[11px] text-[var(--text-3)]">
              {locationEndpoint(location)}
            </p>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          {capabilities.map((capability) => (
            <span
              key={capability.name}
              className={cn(
                'inline-flex items-center gap-1.5 rounded-full border px-2 py-1 text-[11px]',
                capability.status === 'passed'
                  ? 'border-[color-mix(in_srgb,var(--green)_35%,transparent)] text-[var(--green)]'
                  : 'border-[var(--border)] text-[var(--text-3)]',
              )}
            >
              <span className={cn('h-1.5 w-1.5 rounded-full', capability.status === 'passed' ? 'bg-[var(--green)]' : 'bg-[var(--text-4)]')} />
              {capabilityLabel(capability.name)}
            </span>
          ))}
        </div>

        {locationAuthLabel(location) && (
          <p className="m-0 mt-3 text-[12px] text-[var(--text-3)]">{locationAuthLabel(location)}</p>
        )}

        {location.lastProbe?.message && (
          <p className="m-0 mt-3 text-[12px] leading-relaxed text-[var(--red)]">{location.lastProbe.message}</p>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-3 lg:justify-end">
        <div className="text-right text-[11px] text-[var(--text-3)]">
          <div>Last checked</div>
          <div className="text-[var(--text-2)]">{formatDate(location.lastProbe?.probedAt ?? location.updatedAt)}</div>
        </div>
        <Button variant="secondary" size="sm" onClick={onInstallCodex} disabled={installingCodex || probing}>
          {installingCodex ? 'Installing...' : 'Install Codex'}
        </Button>
        <Button variant="secondary" size="sm" onClick={onCodexLogin} disabled={loggingIntoCodex || probing}>
          <KeyRound size={13} />
          {loggingIntoCodex ? 'Starting...' : 'Codex login'}
        </Button>
        <Button variant="ghost" size="sm" onClick={onCheckCodexAuth} disabled={checkingCodexAuth || probing}>
          {checkingCodexAuth ? 'Checking...' : 'Check Codex'}
        </Button>
        <Button variant="secondary" size="sm" onClick={onInstallClaude} disabled={installingClaude || probing}>
          {installingClaude ? 'Installing...' : 'Install Claude'}
        </Button>
        <Button variant={claudeLoginRequired(location) ? 'default' : 'secondary'} size="sm" onClick={onClaudeLogin} disabled={loggingIntoClaude || probing}>
          <KeyRound size={13} />
          {loggingIntoClaude ? 'Starting...' : 'Claude login'}
        </Button>
        <Button variant="ghost" size="sm" onClick={onCheckClaudeAuth} disabled={checkingClaudeAuth || probing}>
          {checkingClaudeAuth ? 'Checking...' : 'Check Claude'}
        </Button>
        <Button variant="secondary" size="sm" onClick={onProbe} disabled={probing}>
          <RefreshCw size={13} />
          {probing ? 'Checking...' : 'Check'}
        </Button>
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="ghost-danger" size="icon" aria-label={`Delete ${locationName(location)}`} disabled={deleting || probing || installingCodex}>
              <Trash2 size={14} />
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete location</AlertDialogTitle>
              <AlertDialogDescription>
                Delete {locationName(location)}. Locations with active projects cannot be deleted.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                className="bg-[var(--red)] text-white hover:bg-[var(--red)]/90"
                onClick={onDelete}
              >
                {deleting ? 'Deleting...' : 'Delete'}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </div>
  )
}

export default function Locations() {
  const locationsQuery = useLocations()
  const createCredential = useCredentialCreate()
  const createLocation = useCreateLocation()
  const probeLocation = useProbeLocation()
  const installCodex = useInstallCodex()
  const installClaude = useInstallClaude()
  const codexAuthStatus = useCodexAuthStatus()
  const codexLogin = useCodexLogin()
  const claudeAuthStatus = useClaudeAuthStatus()
  const claudeLogin = useClaudeLogin()
  const claudeLoginComplete = useClaudeLoginComplete()
  const deleteLocation = useDeleteLocation()
  const locations = locationsQuery.data ?? []
  const readyCount = locations.filter((location) => location.ready).length
  const [showSSHForm, setShowSSHForm] = useState(false)
  const [label, setLabel] = useState('')
  const [host, setHost] = useState('')
  const [username, setUsername] = useState('')
  const [port, setPort] = useState('22')
  const [authChoice, setAuthChoice] = useState<SSHAuthChoice>('password')
  const [credentialLabelInput, setCredentialLabelInput] = useState('')
  const [password, setPassword] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [codexLoginResult, setCodexLoginResult] = useState<(CodexLoginResult & { locationId: string }) | null>(null)
  const [claudeLoginResult, setClaudeLoginResult] = useState<(ClaudeLoginResult & { locationId: string }) | null>(null)
  const [claudeAuthCode, setClaudeAuthCode] = useState('')

  const sortedLocations = useMemo(() => {
    return [...locations].sort((a, b) => {
      if (a.id === 'local') return -1
      if (b.id === 'local') return 1
      return locationName(a).localeCompare(locationName(b))
    })
  }, [locations])

  async function handleCreateLocal() {
    try {
      await createLocation.mutateAsync({ kind: 'local', label: 'This machine' })
      toast.success('Local location is ready')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create local location')
    }
  }

  async function handleCreateSSH(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const cleanHost = host.trim()
    const cleanUsername = username.trim()
    const numericPort = Number.parseInt(port, 10)
    if (!cleanHost || !cleanUsername || !Number.isFinite(numericPort) || numericPort <= 0) {
      toast.error('Host, username, and port are required')
      return
    }
    if (authChoice === 'password' && !password.trim()) {
      toast.error('SSH password is required')
      return
    }
    if (authChoice === 'key' && !privateKey.trim()) {
      toast.error('Private key is required')
      return
    }
    try {
      const created = await createCredential.mutateAsync({
        kind: authChoice === 'password' ? 'ssh_password' : 'ssh_key',
        label: credentialLabelInput.trim() || `${cleanUsername}@${cleanHost}`,
        storageKind: 'local_encrypted',
        secrets: authChoice === 'password'
          ? { password: password.trim() }
          : {
              privateKey: privateKey.trim(),
              ...(passphrase.trim() ? { passphrase: passphrase.trim() } : {}),
            },
      })
      await createLocation.mutateAsync({
        kind: 'ssh',
        label: label.trim() || cleanHost,
        address: { host: cleanHost, username: cleanUsername, port: numericPort },
        auth: { mode: 'keyRef', credentialId: created.credential.id },
      })
      setLabel('')
      setHost('')
      setUsername('')
      setPort('22')
      setCredentialLabelInput('')
      setPassword('')
      setPrivateKey('')
      setPassphrase('')
      setAuthChoice('password')
      setShowSSHForm(false)
      toast.success('Location added')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add location')
    }
  }

  async function handleProbe(locationId: string) {
    try {
      await probeLocation.mutateAsync(locationId)
      toast.success('Location checked')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to check location')
    }
  }

  async function handleInstallCodex(locationId: string) {
    try {
      await installCodex.mutateAsync(locationId)
      toast.success('Codex installed')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to install Codex')
    }
  }

  async function handleCheckCodexAuth(locationId: string) {
    try {
      const status = await codexAuthStatus.mutateAsync(locationId)
      if (status.loggedIn) {
        toast.success(`Codex is logged in${status.method ? ` (${status.method})` : ''}`)
        return
      }
      toast.error('Codex login is required on this location')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to check Codex auth')
    }
  }

  async function handleCodexLogin(locationId: string) {
    try {
      const result = await codexLogin.mutateAsync(locationId)
      setCodexLoginResult({ ...result, locationId })
      if (result.loginUrl) {
        window.open(result.loginUrl, '_blank', 'noopener,noreferrer')
        toast.success('Codex login opened')
        return
      }
      toast.success('Codex login started')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start Codex login')
    }
  }

  async function handleInstallClaude(locationId: string) {
    try {
      await installClaude.mutateAsync(locationId)
      toast.success('Claude Code installed')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to install Claude Code')
    }
  }

  async function handleCheckClaudeAuth(locationId: string) {
    try {
      const status = await claudeAuthStatus.mutateAsync(locationId)
      if (status.loggedIn) {
        toast.success(`Claude is logged in${status.authMethod ? ` (${status.authMethod})` : ''}`)
        return
      }
      toast.error('Claude login is required on this location')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to check Claude auth')
    }
  }

  async function handleClaudeLogin(locationId: string) {
    try {
      const result = await claudeLogin.mutateAsync(locationId)
      setClaudeLoginResult({ ...result, locationId })
      setClaudeAuthCode('')
      if (result.loginUrl) {
        window.open(result.loginUrl, '_blank', 'noopener,noreferrer')
        toast.success('Claude login opened')
        return
      }
      toast.success('Claude login started')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start Claude login')
    }
  }

  async function handleCompleteClaudeLogin(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!claudeLoginResult) return
    const code = claudeAuthCode.trim()
    if (!code) {
      toast.error('Authorization code is required')
      return
    }
    try {
      const result = await claudeLoginComplete.mutateAsync({ locationId: claudeLoginResult.locationId, code })
      setClaudeLoginResult({ ...result, locationId: claudeLoginResult.locationId })
      setClaudeAuthCode('')
      if (/login successful/i.test(result.output)) {
        toast.success('Claude login completed')
        await handleCheckClaudeAuth(claudeLoginResult.locationId)
        return
      }
      toast.success('Claude login code submitted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to complete Claude login')
    }
  }

  async function handleDeleteLocation(locationId: string) {
    try {
      await deleteLocation.mutateAsync(locationId)
      toast.success('Location deleted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete location')
    }
  }

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto grid w-full max-w-[1080px] gap-8">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h1 className="m-0 text-2xl font-bold tracking-[-0.04em] text-[var(--text-1)]">Locations</h1>
            <p className="mt-1 mb-0 max-w-[620px] text-[13px] leading-relaxed text-[var(--text-3)]">
              Locations are machines Agen8 can use to browse and register project roots. Harnesses still own their own sessions.
            </p>
          </div>
          <div className="flex gap-2">
            {!locations.some((location) => location.id === 'local') && (
              <Button variant="secondary" onClick={() => void handleCreateLocal()} disabled={createLocation.isPending}>
                <HardDrive size={14} />
                Add local
              </Button>
            )}
            <Button onClick={() => setShowSSHForm((open) => !open)}>
              <Plus size={14} />
              SSH location
            </Button>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          <div className="border-y border-[var(--border)] py-4">
            <div className="text-[24px] font-semibold text-[var(--text-1)]">{locations.length}</div>
            <div className="mt-1 text-[12px] text-[var(--text-3)]">Configured locations</div>
          </div>
          <div className="border-y border-[var(--border)] py-4">
            <div className="text-[24px] font-semibold text-[var(--green)]">{readyCount}</div>
            <div className="mt-1 text-[12px] text-[var(--text-3)]">Ready for projects</div>
          </div>
          <div className="border-y border-[var(--border)] py-4">
            <div className="flex items-center gap-2 text-[24px] font-semibold text-[var(--text-1)]">
              <ShieldCheck size={20} />
              Local
            </div>
            <div className="mt-1 text-[12px] text-[var(--text-3)]">Default project location</div>
          </div>
        </div>

        {showSSHForm && (
          <form onSubmit={(event) => void handleCreateSSH(event)} className="grid gap-5 border-y border-[var(--border)] py-5">
            <div>
              <h2 className="m-0 text-[17px] font-semibold text-[var(--text-1)]">Add SSH Location</h2>
              <p className="mt-1 mb-0 text-[13px] text-[var(--text-3)]">Sign in to the machine once. Agen8 saves the credential securely and uses it when checking or browsing this location.</p>
            </div>
            <div className="grid gap-4 lg:grid-cols-4">
              <div className="grid gap-2">
                <Label htmlFor="location-label">Label</Label>
                <Input id="location-label" value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Build server" />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="location-host">Host</Label>
                <Input id="location-host" value={host} onChange={(event) => setHost(event.target.value)} placeholder="192.168.1.50" />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="location-username">User</Label>
                <Input id="location-username" value={username} onChange={(event) => setUsername(event.target.value)} placeholder="santino" />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="location-port">Port</Label>
                <Input id="location-port" inputMode="numeric" value={port} onChange={(event) => setPort(event.target.value)} />
              </div>
            </div>

            <div className="grid gap-4 border-t border-[var(--border)] pt-5">
              <div className="grid gap-3">
                <div>
                  <h3 className="m-0 text-[14px] font-semibold text-[var(--text-1)]">Sign in</h3>
                  <p className="m-0 mt-1 text-[12px] leading-relaxed text-[var(--text-3)]">Use the same SSH login you would use in a terminal. Password login is the default; use a key when the host requires one.</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button type="button" variant={authChoice === 'password' ? 'default' : 'secondary'} size="sm" onClick={() => setAuthChoice('password')}>
                    Password
                  </Button>
                  <Button type="button" variant={authChoice === 'key' ? 'default' : 'secondary'} size="sm" onClick={() => setAuthChoice('key')}>
                    Private key
                  </Button>
                </div>
              </div>

              <div className="grid gap-2 sm:max-w-[420px]">
                <Label htmlFor="location-credential-label">Saved as</Label>
                <Input
                  id="location-credential-label"
                  value={credentialLabelInput}
                  onChange={(event) => setCredentialLabelInput(event.target.value)}
                  placeholder="Leave blank to use user@host"
                />
              </div>

              {authChoice === 'password' && (
                <div className="grid gap-2 sm:max-w-[420px]">
                  <Label htmlFor="location-ssh-password">Password</Label>
                  <Input
                    id="location-ssh-password"
                    type="password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    autoComplete="current-password"
                  />
                </div>
              )}

              {authChoice === 'key' && (
                <div className="grid gap-4">
                  <div className="grid gap-2">
                    <Label htmlFor="location-private-key">Private key</Label>
                    <Textarea
                      id="location-private-key"
                      value={privateKey}
                      onChange={(event) => setPrivateKey(event.target.value)}
                      className="min-h-[150px] font-mono text-[12px]"
                      placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                    />
                  </div>
                  <div className="grid gap-2 sm:max-w-[420px]">
                    <Label htmlFor="location-passphrase">Passphrase</Label>
                    <Input
                      id="location-passphrase"
                      type="password"
                      value={passphrase}
                      onChange={(event) => setPassphrase(event.target.value)}
                    />
                  </div>
                </div>
              )}
            </div>
            <div className="flex gap-2">
              <Button type="submit" disabled={createLocation.isPending || createCredential.isPending}>
                {createLocation.isPending || createCredential.isPending ? 'Adding...' : 'Add location'}
              </Button>
              <Button type="button" variant="ghost" onClick={() => setShowSSHForm(false)}>Cancel</Button>
            </div>
          </form>
        )}

        {codexLoginResult && (
          <section className="grid gap-3 border-y border-[var(--border)] py-5">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h2 className="m-0 text-[17px] font-semibold text-[var(--text-1)]">Codex Login</h2>
                <p className="mt-1 mb-0 max-w-[620px] text-[13px] leading-relaxed text-[var(--text-3)]">
                  Complete the Codex browser sign-in, then use Check Codex on the location row.
                </p>
              </div>
              <Button type="button" variant="ghost" onClick={() => setCodexLoginResult(null)}>Dismiss</Button>
            </div>
            {codexLoginResult.loginUrl && (
              <a
                href={codexLoginResult.loginUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex w-fit items-center gap-2 text-[13px] font-medium text-[var(--accent)]"
              >
                <ExternalLink size={14} />
                Open Codex sign-in
              </a>
            )}
            <pre className="max-h-[180px] overflow-auto rounded-[var(--r-md)] bg-[var(--bg-surface)] p-3 text-[11px] leading-relaxed text-[var(--text-2)]">
              {codexLoginResult.output || 'Codex login started.'}
            </pre>
          </section>
        )}

        {claudeLoginResult && (
          <section className="grid gap-3 border-y border-[var(--border)] py-5">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h2 className="m-0 text-[17px] font-semibold text-[var(--text-1)]">Claude Login</h2>
                <p className="mt-1 mb-0 max-w-[620px] text-[13px] leading-relaxed text-[var(--text-3)]">
                  Complete the Claude browser sign-in. If Claude shows an authorization code, paste it here to finish the remote CLI login.
                </p>
              </div>
              <Button type="button" variant="ghost" onClick={() => { setClaudeLoginResult(null); setClaudeAuthCode('') }}>Dismiss</Button>
            </div>
            {claudeLoginResult.loginUrl && (
              <a
                href={claudeLoginResult.loginUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex w-fit items-center gap-2 text-[13px] font-medium text-[var(--accent)]"
              >
                <ExternalLink size={14} />
                Open Claude sign-in
              </a>
            )}
            <form className="flex max-w-[620px] flex-col gap-2 sm:flex-row sm:items-end" onSubmit={handleCompleteClaudeLogin}>
              <div className="grid flex-1 gap-2">
                <Label htmlFor="claude-auth-code">Authorization code</Label>
                <Input
                  id="claude-auth-code"
                  value={claudeAuthCode}
                  onChange={(event) => setClaudeAuthCode(event.target.value)}
                  autoComplete="one-time-code"
                />
              </div>
              <Button type="submit" disabled={claudeLoginComplete.isPending}>
                {claudeLoginComplete.isPending ? 'Submitting...' : 'Submit code'}
              </Button>
            </form>
            <pre className="max-h-[180px] overflow-auto rounded-[var(--r-md)] bg-[var(--bg-surface)] p-3 text-[11px] leading-relaxed text-[var(--text-2)]">
              {claudeLoginResult.output || 'Claude login started.'}
            </pre>
          </section>
        )}

        <section className="border-y border-[var(--border)]">
          {locationsQuery.isLoading ? (
            <div className="py-10 text-center"><span className="spinner spinner-sm" /></div>
          ) : sortedLocations.length === 0 ? (
            <div className="py-10 text-[13px] text-[var(--text-3)]">No locations are configured.</div>
          ) : (
            sortedLocations.map((location) => (
              <LocationRow
                key={location.id}
                location={location}
                probing={probeLocation.isPending && probeLocation.variables === location.id}
                installingCodex={installCodex.isPending && installCodex.variables === location.id}
                installingClaude={installClaude.isPending && installClaude.variables === location.id}
                checkingCodexAuth={codexAuthStatus.isPending && codexAuthStatus.variables === location.id}
                loggingIntoCodex={codexLogin.isPending && codexLogin.variables === location.id}
                checkingClaudeAuth={claudeAuthStatus.isPending && claudeAuthStatus.variables === location.id}
                loggingIntoClaude={claudeLogin.isPending && claudeLogin.variables === location.id}
                deleting={deleteLocation.isPending && deleteLocation.variables === location.id}
                onProbe={() => void handleProbe(location.id)}
                onInstallCodex={() => void handleInstallCodex(location.id)}
                onInstallClaude={() => void handleInstallClaude(location.id)}
                onCheckCodexAuth={() => void handleCheckCodexAuth(location.id)}
                onCodexLogin={() => void handleCodexLogin(location.id)}
                onCheckClaudeAuth={() => void handleCheckClaudeAuth(location.id)}
                onClaudeLogin={() => void handleClaudeLogin(location.id)}
                onDelete={() => void handleDeleteLocation(location.id)}
              />
            ))
          )}
        </section>
      </div>
    </div>
  )
}
