/**
 * Locations — daemon-local and remote project-root addressability.
 *
 * Locations let Agen8 browse roots from the daemon machine or through SSH.
 * They deliberately do not manage Codex/Claude installation, login, channels,
 * or harness sessions.
 *
 * Layout follows the house pattern (see Members.tsx): ONE bordered surface of
 * rows that reflow from stacked (narrow) to inline (wide) via a CONTAINER query
 * — the inline sidebar eats ~272px, so a viewport breakpoint would overstate
 * the room the list actually has. Creation happens in a dialog, not a standing
 * form panel. Colour appears only on exception (offline / failed probe); a
 * healthy fleet reads as calm monochrome.
 */
import { useMemo, useState } from 'react'
import { AlertTriangle, FileDiff, HardDrive, Plus, RefreshCw, Search, Server, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCredentialCreate } from '../hooks/useCredentials'
import {
  useCreateLocation,
  useDeleteLocation,
  useLocations,
  useProbeLocation,
  useSetLocationGitDiff,
  type LocationAuthInput,
} from '../hooks/useLocations'
import { Switch } from '@/components/ui/switch'
import type { ExecutionLocation } from '../lib/types'
import { locationDescription, locationLabel, locationStatusLabel } from '../lib/projectFormat'
import { cn } from '@/lib/utils'

// At/above this many locations the list is a "fleet": reveal search + an issues
// filter and tighten row density. Below it, those controls would be noise.
const FLEET_MIN = 6

type AuthMode = 'existing' | 'password' | 'key'

interface SSHFormState {
  label: string
  host: string
  port: string
  username: string
  authMode: AuthMode
  credentialId: string
  password: string
  privateKey: string
  passphrase: string
}

const EMPTY_FORM: SSHFormState = {
  label: '',
  host: '',
  port: '22',
  username: '',
  authMode: 'existing',
  credentialId: '',
  password: '',
  privateKey: '',
  passphrase: '',
}

function statusDot(location: ExecutionLocation): string {
  if (location.ready) return 'var(--green)'
  if (location.status === 'offline') return 'var(--red)'
  return 'var(--amber)'
}

// Status text colour — muted when healthy, so colour reads as an exception.
function statusTextClass(location: ExecutionLocation): string {
  if (location.ready) return 'text-[var(--text-3)]'
  if (location.status === 'offline') return 'text-[var(--red)]'
  return 'text-[var(--amber)]'
}

function capabilityLabel(name: string): string {
  if (name === 'fileBrowsing') return 'File browsing'
  if (name === 'reachable') return 'Reachable'
  return name
}

function fieldError(message: string | null) {
  if (!message) return null
  return <p className="m-0 mt-1 text-[0.71875rem] text-[var(--red)]">{message}</p>
}

function validate(form: SSHFormState): string[] {
  const errors: string[] = []
  const port = Number(form.port)
  if (!form.host.trim()) errors.push('Host is required.')
  if (!form.username.trim()) errors.push('Username is required.')
  if (!Number.isInteger(port) || port < 1 || port > 65535) errors.push('Port must be between 1 and 65535.')
  if (form.authMode === 'existing' && !form.credentialId.trim()) {
    errors.push('Credential reference is required for existing credential auth.')
  }
  if (form.authMode === 'password' && !form.password) errors.push('Password is required.')
  if (form.authMode === 'key' && !form.privateKey.trim()) errors.push('Private key is required.')
  return errors
}

function locationMatches(location: ExecutionLocation, query: string): boolean {
  if (!query) return true
  const haystack = [
    locationLabel(location),
    locationDescription(location),
    location.kind,
    location.address?.host,
    location.address?.username,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  return haystack.includes(query.toLowerCase())
}

// Order for display: the local (daemon) machine first, then remote locations
// sorted so anything needing attention floats to the top. The search + issues
// filters are applied here too. The HardDrive/Server icon + the muted
// "Daemon machine" vs "user@host" line already tell local from remote, so the
// list reads clearly as one calm surface without section header rows.
function orderedLocations(
  locations: ExecutionLocation[],
  query: string,
  issuesOnly: boolean,
): ExecutionLocation[] {
  const keep = (rows: ExecutionLocation[]) =>
    rows.filter((l) => locationMatches(l, query) && (!issuesOnly || !l.ready))

  const local = keep(locations.filter((l) => l.kind === 'local'))
  const remote = keep(locations.filter((l) => l.kind !== 'local')).sort((a, b) => {
    const aReady = a.ready ? 1 : 0
    const bReady = b.ready ? 1 : 0
    if (aReady !== bReady) return aReady - bReady
    return locationLabel(a).localeCompare(locationLabel(b))
  })

  return [...local, ...remote]
}

function EmptyLocations() {
  return (
    <div className="flex flex-col items-center justify-center gap-2 rounded-[var(--r-lg)] border border-dashed border-[var(--border)] px-6 py-12 text-center">
      <HardDrive size={24} className="text-[var(--text-3)]" />
      <div className="text-[0.875rem] font-semibold text-[var(--text-1)]">No locations yet</div>
      <p className="m-0 max-w-[360px] text-[0.78125rem] leading-relaxed text-[var(--text-3)]">
        The daemon should always expose this machine. Add an SSH location when projects live on another machine.
      </p>
    </div>
  )
}

function DeleteLocationButton({
  label,
  deleting,
  onDelete,
}: {
  label: string
  deleting: boolean
  onDelete: () => void
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button type="button" variant="ghost-danger" size="icon" disabled={deleting} aria-label={`Delete ${label}`}>
          <Trash2 size={14} />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {label}?</AlertDialogTitle>
          <AlertDialogDescription>
            Existing projects keep their recorded location id, but this location will no longer be available for new
            project browsing.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
          <Button type="button" variant="destructive" onClick={onDelete} disabled={deleting}>
            Delete
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function LocationRow({
  location,
  dense,
  probing,
  deleting,
  onProbe,
  onDelete,
}: {
  location: ExecutionLocation
  dense: boolean
  probing: boolean
  deleting: boolean
  onProbe: () => void
  onDelete: () => void
}) {
  const label = locationLabel(location)
  const description = locationDescription(location)
  const canDelete = location.kind !== 'local'
  const failed = (location.capabilities ?? []).filter((c) => c.status === 'failed')
  const Icon = location.kind === 'local' ? HardDrive : Server
  const setGitDiff = useSetLocationGitDiff()
  // Only SSH locations can run a remote git baseline. gitAvailable reflects the
  // probe's detection; the toggle is the separate, human-granted opt-in.
  const isRemote = location.kind === 'ssh'
  const gitAvailable = (location.capabilities ?? []).some((c) => c.name === 'gitDiff' && c.status === 'passed')

  return (
    <div
      className={cn(
        'group flex flex-col gap-2 px-4 hover:bg-[var(--bg-hover)] @min-[640px]:flex-row @min-[640px]:items-center @min-[640px]:gap-4',
        dense ? 'py-2' : 'py-3',
      )}
    >
      {/* identity */}
      <div className="flex min-w-0 flex-1 items-start gap-3 @min-[640px]:items-center">
        <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-2)] @min-[640px]:mt-0">
          <Icon size={14} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ backgroundColor: statusDot(location) }} />
            <h3 className="m-0 truncate text-[0.875rem] font-medium text-[var(--text-1)]">{label}</h3>
          </div>
          <div className="mt-0.5 truncate font-mono text-[0.75rem] text-[var(--text-3)]">{description}</div>
          {!location.ready && location.lastProbe?.message && (
            <div className={cn('mt-1 text-[0.75rem] leading-relaxed', statusTextClass(location))}>
              {location.lastProbe.message}
            </div>
          )}
        </div>
      </div>

      {/* status + failed-capability chips (colour only on exception) */}
      <div className="flex shrink-0 flex-wrap items-center gap-2 pl-10 @min-[640px]:justify-end @min-[640px]:pl-0">
        {failed.map((capability) => (
          <span
            key={capability.name}
            className="rounded-full border border-[rgba(248,113,113,0.32)] bg-[rgba(248,113,113,0.08)] px-2 py-0.5 text-[0.6875rem] font-medium text-[var(--red)]"
          >
            {capabilityLabel(capability.name)}
          </span>
        ))}
        <span className={cn('text-[0.75rem]', statusTextClass(location))}>{locationStatusLabel(location)}</span>
      </div>

      {/* remote-diff opt-in — only for SSH locations, where running git on the
          host is a deliberate, granted capability (default off) */}
      {isRemote && (
        <div className="flex shrink-0 items-center gap-2 pl-10 @min-[640px]:pl-0">
          <FileDiff size={13} className="text-[var(--text-3)]" />
          <Switch
            checked={!!location.gitDiffEnabled}
            disabled={setGitDiff.isPending}
            onCheckedChange={(checked) => {
              setGitDiff.mutate(
                { locationId: location.id, gitDiffEnabled: checked },
                {
                  onSuccess: () => toast.success(checked ? 'Remote diff enabled' : 'Remote diff disabled'),
                  onError: (err) => toast.error(`Could not update: ${err.message}`),
                },
              )
            }}
            aria-label={`Allow remote git diff on ${label}`}
            title={
              gitAvailable
                ? 'Run a read-only git on this host to diff uncommitted changes'
                : 'git was not detected on this host at the last probe'
            }
          />
        </div>
      )}

      {/* actions — hidden until hover where a pointer can hover; always shown on touch */}
      <div className="flex shrink-0 items-center gap-1 pl-10 transition-opacity focus-within:opacity-100 @min-[640px]:pl-0 [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onProbe}
          disabled={probing}
          aria-label={`Probe ${label}`}
        >
          <RefreshCw size={14} className={cn(probing && 'animate-spin')} />
        </Button>
        {canDelete && <DeleteLocationButton label={label} deleting={deleting} onDelete={onDelete} />}
      </div>
    </div>
  )
}

function AddLocationDialog() {
  const createLocation = useCreateLocation()
  const createCredential = useCredentialCreate()
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<SSHFormState>(EMPTY_FORM)
  const [errors, setErrors] = useState<string[]>([])

  const saving = createLocation.isPending || createCredential.isPending

  function reset() {
    setForm(EMPTY_FORM)
    setErrors([])
  }

  function update<K extends keyof SSHFormState>(key: K, value: SSHFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }))
    setErrors([])
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const validation = validate(form)
    if (validation.length > 0) {
      setErrors(validation)
      return
    }

    try {
      const host = form.host.trim()
      const username = form.username.trim()
      const port = Number(form.port)
      const label = form.label.trim() || `${username}@${host}`
      let auth: LocationAuthInput | undefined

      if (form.authMode === 'existing') {
        auth = { mode: 'keyRef', credentialId: form.credentialId.trim() }
      } else if (form.authMode === 'password') {
        const result = await createCredential.mutateAsync({
          kind: 'ssh_password',
          label,
          storageKind: 'local_encrypted',
          secrets: { password: form.password },
        })
        auth = { mode: 'keyRef', credentialId: result.credential.id }
      } else if (form.authMode === 'key') {
        const secrets: Record<string, string> = { privateKey: form.privateKey }
        if (form.passphrase) secrets.passphrase = form.passphrase
        const result = await createCredential.mutateAsync({
          kind: 'ssh_key',
          label,
          storageKind: 'local_encrypted',
          secrets,
        })
        auth = { mode: 'keyRef', credentialId: result.credential.id }
      }

      await createLocation.mutateAsync({
        kind: 'ssh',
        label,
        address: { host, username, port },
        auth,
      })
      reset()
      setOpen(false)
      toast.success('SSH location added')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to add location')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (saving) return
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger asChild>
        <Button type="button" variant="secondary" size="sm" className="shrink-0">
          <Plus size={14} className="mr-1.5" />
          Add location
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] max-w-md overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Add SSH location</DialogTitle>
          <DialogDescription>
            Use this when project files live on another machine than the daemon.
          </DialogDescription>
        </DialogHeader>

        <form id="add-location-form" className="flex flex-col gap-3" onSubmit={(event) => void handleSubmit(event)}>
          {errors.length > 0 && (
            <div className="rounded-[var(--r-md)] border border-[rgba(248,113,113,0.32)] bg-[rgba(248,113,113,0.08)] px-3 py-2">
              {errors.map((error) => (
                <p key={error} className="m-0 text-[0.75rem] text-[var(--red)]">
                  {error}
                </p>
              ))}
            </div>
          )}

          <div>
            <Label htmlFor="location-label">Label</Label>
            <Input
              id="location-label"
              value={form.label}
              onChange={(event) => update('label', event.target.value)}
              placeholder="Work laptop"
              autoComplete="off"
            />
          </div>

          <div>
            <Label htmlFor="location-host">Host</Label>
            <Input
              id="location-host"
              value={form.host}
              onChange={(event) => update('host', event.target.value)}
              placeholder="devbox.local"
              autoComplete="off"
            />
            {fieldError(errors.find((error) => error.startsWith('Host')) ?? null)}
          </div>

          <div className="grid grid-cols-[minmax(0,1fr)_90px] gap-3">
            <div>
              <Label htmlFor="location-username">Username</Label>
              <Input
                id="location-username"
                value={form.username}
                onChange={(event) => update('username', event.target.value)}
                placeholder="santino"
                autoComplete="username"
              />
              {fieldError(errors.find((error) => error.startsWith('Username')) ?? null)}
            </div>
            <div>
              <Label htmlFor="location-port">Port</Label>
              <Input
                id="location-port"
                inputMode="numeric"
                value={form.port}
                onChange={(event) => update('port', event.target.value)}
              />
              {fieldError(errors.find((error) => error.startsWith('Port')) ?? null)}
            </div>
          </div>

          <div>
            <Label htmlFor="location-auth-mode">Authentication</Label>
            <Select value={form.authMode} onValueChange={(value) => update('authMode', value as AuthMode)}>
              <SelectTrigger id="location-auth-mode">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="existing">Existing credential id</SelectItem>
                <SelectItem value="password">Create password credential</SelectItem>
                <SelectItem value="key">Create private key credential</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {form.authMode === 'existing' && (
            <div>
              <Label htmlFor="location-credential">Credential reference</Label>
              <Input
                id="location-credential"
                value={form.credentialId}
                onChange={(event) => update('credentialId', event.target.value)}
                placeholder="cred_..."
                autoComplete="off"
              />
              {fieldError(errors.find((error) => error.startsWith('Credential')) ?? null)}
            </div>
          )}

          {form.authMode === 'password' && (
            <div>
              <Label htmlFor="location-password">Password</Label>
              <Input
                id="location-password"
                type="password"
                value={form.password}
                onChange={(event) => update('password', event.target.value)}
                autoComplete="new-password"
              />
              {fieldError(errors.find((error) => error.startsWith('Password')) ?? null)}
            </div>
          )}

          {form.authMode === 'key' && (
            <>
              <div>
                <Label htmlFor="location-private-key">Private key</Label>
                <Textarea
                  id="location-private-key"
                  value={form.privateKey}
                  onChange={(event) => update('privateKey', event.target.value)}
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                  className="min-h-[120px] font-mono text-[0.75rem]"
                />
                {fieldError(errors.find((error) => error.startsWith('Private')) ?? null)}
              </div>
              <div>
                <Label htmlFor="location-passphrase">Passphrase</Label>
                <Input
                  id="location-passphrase"
                  type="password"
                  value={form.passphrase}
                  onChange={(event) => update('passphrase', event.target.value)}
                  autoComplete="new-password"
                />
              </div>
            </>
          )}
        </form>

        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={saving}>
            Cancel
          </Button>
          <Button type="submit" form="add-location-form" disabled={saving}>
            {saving ? 'Adding…' : 'Add location'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export default function Locations() {
  const locationsQuery = useLocations()
  const locations = useMemo(() => locationsQuery.data ?? [], [locationsQuery.data])
  const probeLocation = useProbeLocation()
  const deleteLocation = useDeleteLocation()

  const [query, setQuery] = useState('')
  const [issuesOnly, setIssuesOnly] = useState(false)

  const attentionCount = useMemo(() => locations.filter((l) => !l.ready).length, [locations])
  const fleet = locations.length >= FLEET_MIN

  const visible = useMemo(
    () => orderedLocations(locations, query, issuesOnly),
    [locations, query, issuesOnly],
  )

  async function handleProbe(locationId: string) {
    try {
      await probeLocation.mutateAsync(locationId)
      toast.success('Location probed')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Probe failed')
    }
  }

  async function handleDelete(locationId: string) {
    try {
      await deleteLocation.mutateAsync(locationId)
      toast.success('Location deleted')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Delete failed')
    }
  }

  const summary =
    attentionCount === 0
      ? `${locations.length} location${locations.length === 1 ? '' : 's'} · all reachable`
      : `${locations.length} locations · ${attentionCount} need${attentionCount === 1 ? 's' : ''} attention`

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-6">
        <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex flex-col gap-1">
            <h1 className="m-0 hidden text-2xl font-bold text-[var(--text-1)] md:block">Locations</h1>
            <p className="m-0 max-w-prose text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
              Where Agen8 looks for project roots. Local is the machine running the daemon; SSH locations let the daemon
              browse another machine without managing that machine&apos;s AI harness.
            </p>
          </div>
          <AddLocationDialog />
        </header>

        {locationsQuery.isLoading ? (
          <div className="overflow-hidden rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-surface)]">
            {[0, 1].map((i) => (
              <div
                key={i}
                className="h-[60px] border-b border-[var(--border)] last:border-b-0 bg-[var(--bg-surface)] skeleton"
              />
            ))}
          </div>
        ) : locations.length === 0 ? (
          <EmptyLocations />
        ) : (
          <>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="m-0 text-[0.8125rem] text-[var(--text-3)]">{summary}</p>
              {fleet && (
                <div className="flex items-center gap-2">
                  <div className="relative">
                    <Search
                      size={14}
                      className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)]"
                    />
                    <Input
                      value={query}
                      onChange={(event) => setQuery(event.target.value)}
                      placeholder="Search locations"
                      aria-label="Search locations"
                      className="h-8 w-[180px] pl-8 text-[0.8125rem]"
                    />
                  </div>
                  <Button
                    type="button"
                    variant={issuesOnly ? 'secondary' : 'ghost'}
                    size="sm"
                    onClick={() => setIssuesOnly((v) => !v)}
                    aria-pressed={issuesOnly}
                    className={cn(issuesOnly && 'text-[var(--amber)]')}
                  >
                    <AlertTriangle size={14} className="mr-1.5" />
                    Issues{attentionCount ? ` (${attentionCount})` : ''}
                  </Button>
                </div>
              )}
            </div>

            <div className="@container">
              <div className="overflow-hidden rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-surface)]">
                {visible.length === 0 ? (
                  <div className="px-4 py-10 text-center text-[0.8125rem] text-[var(--text-3)]">
                    No locations match your filters.
                  </div>
                ) : (
                  <div className="divide-y divide-[var(--border)]">
                    {visible.map((location) => (
                      <LocationRow
                        key={location.id}
                        location={location}
                        dense={fleet}
                        probing={probeLocation.isPending && probeLocation.variables === location.id}
                        deleting={deleteLocation.isPending && deleteLocation.variables === location.id}
                        onProbe={() => void handleProbe(location.id)}
                        onDelete={() => void handleDelete(location.id)}
                      />
                    ))}
                  </div>
                )}
              </div>
            </div>

            <p className="m-0 text-[0.71875rem] leading-relaxed text-[var(--text-3)]">
              Agen8 probes reachability and directory access only — harness setup, model auth, and session routing stay
              outside the location model. SSH credentials are stored locally and encrypted.
            </p>
          </>
        )}
      </div>
    </div>
  )
}
