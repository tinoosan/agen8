/**
 * Credentials — the HTTP-tool credentials console (two-pane "Option B").
 *
 * Left: a searchable, status-grouped rail. Right: a live auth editor with an
 * injection preview. Secrets the `http` tool injects automatically, matched by
 * host, stored locally and encrypted, redacted from model output.
 *
 * Scope: HTTP credentials are always `api_key` kind, so the list is filtered to
 * api_key and creates are always api_key. (SSH credentials live under Locations.)
 */
import { useState } from 'react'
import { KeyRound } from 'lucide-react'
import { toast } from 'sonner'
import {
  useCredentials,
  useCredentialCreate,
  useCredentialUpdate,
  useCredentialDelete,
} from '../hooks/useCredentials'
import { CredentialRail } from '../components/credentials/CredentialRail'
import { CredentialEditor, type CredentialUpdatePatch } from '../components/credentials/CredentialEditor'

function EmptyPane() {
  return (
    <div className="flex flex-col items-center justify-center gap-3 px-8 text-center">
      <KeyRound size={26} className="text-[var(--text-3)]" />
      <div className="text-[0.875rem] font-semibold text-[var(--text-1)]">Select a credential</div>
      <p className="m-0 max-w-[320px] text-[0.78125rem] leading-relaxed text-[var(--text-3)]">
        Choose a credential from the list to edit it, or create a new one to let the{' '}
        <b className="text-[var(--text-2)]">http</b> tool authenticate calls to a host.
      </p>
    </div>
  )
}

export default function Credentials() {
  const credentialsQuery = useCredentials({ kind: 'api_key' })
  const credentials = credentialsQuery.data ?? []

  const createMut = useCredentialCreate()
  const updateMut = useCredentialUpdate()
  const deleteMut = useCredentialDelete()
  const saving = createMut.isPending || updateMut.isPending || deleteMut.isPending

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  // Derive the open credential during render (no effect → no cascading renders).
  // When nothing is explicitly selected — initial load, or the selection was
  // just deleted — fall back to the first credential so the right pane is never
  // empty while credentials exist.
  const selectedCredential = creating
    ? null
    : credentials.find((c) => c.id === selectedId) ?? credentials[0] ?? null

  function handleNew() {
    setCreating(true)
    setSelectedId(null)
  }

  function handleSelect(id: string) {
    setCreating(false)
    setSelectedId(id)
  }

  function handleCreate(params: { label: string; secrets: Record<string, string> }) {
    createMut.mutate(
      { kind: 'api_key', label: params.label, secrets: params.secrets },
      {
        onSuccess: (res) => {
          setCreating(false)
          setSelectedId(res.credential.id)
          toast.success('Credential created')
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to create credential'),
      },
    )
  }

  function handleUpdate(patch: CredentialUpdatePatch) {
    if (!selectedCredential) return
    updateMut.mutate(
      { credentialId: selectedCredential.id, ...patch },
      {
        onSuccess: () => toast.success('Credential saved'),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to save credential'),
      },
    )
  }

  function handleDelete() {
    if (!selectedCredential) return
    deleteMut.mutate(
      { credentialId: selectedCredential.id },
      {
        onSuccess: () => {
          setSelectedId(null)
          toast.success('Credential deleted')
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete credential'),
      },
    )
  }

  return (
    <div className="h-full overflow-y-auto md:overflow-hidden p-[clamp(16px,4vw,32px)]">
      <div className="mx-auto flex h-auto md:h-full w-full max-w-[1180px] flex-col gap-5">
        <header>
          <h1 className="m-0 text-2xl font-bold text-[var(--text-1)] hidden md:block">Credentials</h1>
          <p className="m-0 mt-1 max-w-[620px] text-[0.8125rem] leading-relaxed text-[var(--text-3)]">
            Secrets the <b className="text-[var(--text-2)]">http</b> tool injects automatically, matched by host. Stored
            locally and encrypted; values are redacted from model output.
          </p>
        </header>

        <div className="flex flex-col md:grid min-h-0 md:flex-1 md:grid-cols-[300px_minmax(0,1fr)] overflow-visible md:overflow-hidden rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-panel)]">
          <CredentialRail
            credentials={credentials}
            loading={credentialsQuery.isLoading}
            selectedId={selectedCredential?.id ?? null}
            isCreating={creating}
            onSelect={handleSelect}
            onNew={handleNew}
          />

          {creating ? (
            <CredentialEditor
              key="new"
              mode="create"
              credential={null}
              saving={saving}
              onCreate={handleCreate}
              onUpdate={handleUpdate}
              onDelete={handleDelete}
              onCancelCreate={() => setCreating(false)}
            />
          ) : selectedCredential ? (
            <CredentialEditor
              key={selectedCredential.id}
              mode="edit"
              credential={selectedCredential}
              saving={saving}
              onCreate={handleCreate}
              onUpdate={handleUpdate}
              onDelete={handleDelete}
              onCancelCreate={() => setCreating(false)}
            />
          ) : (
            <EmptyPane />
          )}
        </div>
      </div>
    </div>
  )
}
