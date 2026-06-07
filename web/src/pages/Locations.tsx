/**
 * Locations — daemon-local and remote project-root addressability.
 *
 * Locations let Agen8 browse roots from the daemon machine or through SSH.
 * They deliberately do not manage Codex/Claude installation, login, channels,
 * or harness sessions.
 */
import { useMemo, useState } from 'react'
import {
  FolderOpen,
  HardDrive,
  KeyRound,
  Plus,
  RefreshCw,
  Server,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
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
  type LocationAuthInput,
} from '../hooks/useLocations'
import type { ExecutionLocation } from '../lib/types'
import { formatRelative } from '../lib/format'
import { locationDescription, locationLabel, locationStatusLabel } from '../lib/projectFormat'
import { cn } from '@/lib/utils'

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

function capabilityLabel(name: string): string {
  if (name === 'fileBrowsing') return 'File browsing'
  if (name === 'reachable') return 'Reachable'
  return name
}

function capabilityClass(status: string): string {
  if (status === 'passed') return 'border-[rgba(52,211,153,0.32)] text-[var(--green)] bg-[rgba(52,211,153,0.08)]'
  if (status === 'failed') return 'border-[rgba(248,113,113,0.32)] text-[var(--red)] bg-[rgba(248,113,113,0.08)]'
  return 'border-[var(--border)] text-[var(--text-3)] bg-[var(--bg-elevated)]'
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

function LocationRow({
  location,
  probing,
  deleting,
  onProbe,
  onDelete,
}: {
  location: ExecutionLocation
  probing: boolean
  deleting: boolean
  onProbe: () => void
  onDelete: () => void
}) {
  const label = locationLabel(location)
  const description = locationDescription(location)
  const updated = formatRelative(location.updatedAt ?? location.createdAt)
  const canDelete = location.kind !== 'local'

  return (
    <div className="rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-panel)] px-4 py-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-start">
        <div className="flex min-w-0 flex-1 items-start gap-3">
          <span className="mt-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-2)]">
            {location.kind === 'ssh' ? <Server size={16} /> : <HardDrive size={16} />}
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="m-0 truncate text-[0.9375rem] font-semibold text-[var(--text-1)]">{label}</h2>
              <span
                className="inline-flex items-center gap-1.5 rounded-full border border-[var(--border)] bg-[var(--bg-elevated)] px-2 py-0.5 text-[0.6875rem] font-medium text-[var(--text-3)]"
              >
                <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: statusDot(location) }} />
                {locationStatusLabel(location)}
              </span>
              <span className="rounded-full bg-[var(--bg-elevated)] px-2 py-0.5 text-[0.6875rem] uppercase text-[var(--text-3)]">
                {location.kind}
              </span>
            </div>
            <p className="m-0 mt-1 font-mono text-[0.75rem] text-[var(--text-3)]">{description}</p>
            {location.kind === 'local' && (
              <p className="m-0 mt-1 text-[0.75rem] text-[var(--text-4)]">
                This is the machine running the Agen8 daemon.
              </p>
            )}
            {location.kind === 'ssh' && (
              <p className="m-0 mt-1 text-[0.75rem] text-[var(--text-4)]">
                Agen8 connects from the daemon to this host for project-root browsing.
                {location.auth?.hasCredential ? ' Credential reference is configured.' : ' No credential reference is configured.'}
              </p>
            )}
            <div className="mt-2 flex flex-wrap gap-1.5">
              {(location.capabilities ?? []).map((capability) => (
                <span
                  key={capability.name}
                  className={cn(
                    'rounded-full border px-2 py-0.5 text-[0.6875rem] font-medium',
                    capabilityClass(capability.status),
                  )}
                >
                  {capabilityLabel(capability.name)}
                </span>
              ))}
            </div>
            {location.lastProbe?.message && (
              <p className="m-0 mt-2 text-[0.75rem] leading-relaxed text-[var(--text-3)]">
                Last probe: {location.lastProbe.message}
              </p>
            )}
            <p className="m-0 mt-2 text-[0.6875rem] text-[var(--text-4)]">
              {updated ? `Updated ${updated}` : `Location ${location.id}`}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2 md:justify-end">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onProbe}
            disabled={probing}
            aria-label={`Probe ${label}`}
          >
            <RefreshCw size={14} className={cn('mr-1.5', probing && 'animate-spin')} />
            Probe
          </Button>
          {canDelete && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={deleting}
                  aria-label={`Delete ${label}`}
                >
                  <Trash2 size={14} className="mr-1.5" />
                  Delete
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete {label}?</AlertDialogTitle>
                  <AlertDialogDescription>
                    Existing projects keep their recorded location id, but this location will no longer be available for new project browsing.
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
          )}
        </div>
      </div>
    </div>
  )
}

export default function Locations() {
  const locationsQuery = useLocations()
  const locations = useMemo(() => locationsQuery.data ?? [], [locationsQuery.data])
  const createLocation = useCreateLocation()
  const probeLocation = useProbeLocation()
  const deleteLocation = useDeleteLocation()
  const createCredential = useCredentialCreate()

  const [form, setForm] = useState<SSHFormState>(EMPTY_FORM)
  const [errors, setErrors] = useState<string[]>([])

  const readyCount = useMemo(() => locations.filter((location) => location.ready).length, [locations])
  const remoteCount = useMemo(() => locations.filter((location) => location.kind === 'ssh').length, [locations])

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
      setForm(EMPTY_FORM)
      setErrors([])
      toast.success('SSH location added')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to add location')
    }
  }

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

  const saving = createLocation.isPending || createCredential.isPending

  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)]">
      <div className="mx-auto flex w-full max-w-[1180px] flex-col gap-5">
        <header className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className="m-0 text-2xl font-bold text-[var(--text-1)] hidden md:block">Locations</h1>
            <p className="m-0 mt-1 max-w-[720px] text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
              Locations tell Agen8 where project roots live. Local means the machine running this daemon; SSH locations let
              the daemon browse another machine without managing that machine&apos;s AI harness.
            </p>
          </div>
          <div className="flex gap-2 text-[0.75rem] text-[var(--text-3)]">
            <span className="rounded-full border border-[var(--border)] px-2.5 py-1">{locations.length} total</span>
            <span className="rounded-full border border-[var(--border)] px-2.5 py-1">{readyCount} ready</span>
            <span className="rounded-full border border-[var(--border)] px-2.5 py-1">{remoteCount} SSH</span>
          </div>
        </header>

        <section className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_380px]">
          <div className="flex min-w-0 flex-col gap-3">
            {locationsQuery.isLoading ? (
              [0, 1].map((i) => (
                <div key={i} className="h-[132px] rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-panel)] skeleton" />
              ))
            ) : locations.length === 0 ? (
              <EmptyLocations />
            ) : (
              locations.map((location) => (
                <LocationRow
                  key={location.id}
                  location={location}
                  probing={probeLocation.isPending && probeLocation.variables === location.id}
                  deleting={deleteLocation.isPending && deleteLocation.variables === location.id}
                  onProbe={() => void handleProbe(location.id)}
                  onDelete={() => void handleDelete(location.id)}
                />
              ))
            )}
          </div>

          <aside className="rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-panel)] p-4">
            <div className="mb-4 flex items-start gap-3">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-2)]">
                <Plus size={16} />
              </span>
              <div>
                <h2 className="m-0 text-[0.9375rem] font-semibold text-[var(--text-1)]">Add SSH location</h2>
                <p className="m-0 mt-1 text-[0.75rem] leading-relaxed text-[var(--text-3)]">
                  Use this when the project files are on another machine than the daemon.
                </p>
              </div>
            </div>

            <form className="flex flex-col gap-3" onSubmit={(event) => void handleSubmit(event)}>
              {errors.length > 0 && (
                <div className="rounded-[var(--r-md)] border border-[rgba(248,113,113,0.32)] bg-[rgba(248,113,113,0.08)] px-3 py-2">
                  {errors.map((error) => (
                    <p key={error} className="m-0 text-[0.75rem] text-[var(--red)]">{error}</p>
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

              <Button type="submit" disabled={saving} className="mt-1 w-full">
                <FolderOpen size={14} className="mr-1.5" />
                {saving ? 'Adding…' : 'Add location'}
              </Button>
            </form>

            <div className="mt-4 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-2">
              <div className="flex items-center gap-2 text-[0.75rem] font-semibold text-[var(--text-2)]">
                <ShieldCheck size={14} />
                Project-root browsing only
              </div>
              <p className="m-0 mt-1 text-[0.71875rem] leading-relaxed text-[var(--text-3)]">
                Agen8 probes reachability and directory access. Harness setup, model auth, and session routing stay outside
                the location model.
              </p>
              <p className="m-0 mt-2 flex items-center gap-1.5 text-[0.71875rem] text-[var(--text-4)]">
                <KeyRound size={12} />
                SSH credentials are stored locally and encrypted.
              </p>
            </div>
          </aside>
        </section>
      </div>
    </div>
  )
}
