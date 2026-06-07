/**
 * CredentialEditor — right pane. Edits one HTTP credential (or composes a new
 * one). The page remounts this with a `key` per selection, so local form state
 * resets cleanly on switch.
 *
 * Backend-shaped behavior:
 *  - Label + Active status are readable and independently writable.
 *  - Host / auth-type / field-name / value are write-only: the server returns
 *    field *names* but never values, and `credential.update` REPLACES the whole
 *    secret bag. So in edit mode they sit behind an explicit "Replace secret"
 *    unlock — never sent unless the user opts in and re-enters the full set.
 */
import { useMemo, useState } from 'react'
import { Eye, EyeOff, Trash2, ChevronRight, Lock, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Switch } from '@/components/ui/switch'
import { InlineBanner, type BannerState } from '../fields'
import type { CredentialStatus, CredentialView } from '../../lib/types'
import {
  type AuthDraft,
  type InjectionMode,
  buildSecrets,
  deriveInjection,
  emptyAuthDraft,
  INJECTION_META,
  previewInjection,
} from './credentialModel'
import { formatRelative } from '@/lib/format'

export interface CredentialUpdatePatch {
  label?: string
  status?: CredentialStatus
  secrets?: Record<string, string>
}

interface CredentialEditorProps {
  mode: 'create' | 'edit'
  credential: CredentialView | null
  saving: boolean
  onCreate: (params: { label: string; secrets: Record<string, string> }) => void
  onUpdate: (patch: CredentialUpdatePatch) => void
  onDelete: () => void
  onCancelCreate: () => void
}

/* ── small presentational atoms (match the approved mockup) ── */

function FieldLabel({ children, required }: { children: React.ReactNode; required?: boolean }) {
  return (
    <label className="mb-1.5 block text-[0.75rem] font-semibold text-[var(--text-2)]">
      {children}
      {required && <span className="ml-0.5 text-[var(--accent)]">*</span>}
    </label>
  )
}

function TextInput({
  value,
  onChange,
  placeholder,
  mono,
  type = 'text',
  disabled,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  mono?: boolean
  type?: string
  disabled?: boolean
}) {
  return (
    <input
      type={type}
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className={cn(
        'w-full rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-app)] px-[11px] py-[9px] text-[0.8125rem] text-[var(--text-1)] outline-none transition-shadow',
        'placeholder:text-[var(--text-3)] focus:border-[var(--border-focus)] focus:shadow-[0_0_0_3px_var(--accent-glow)]',
        'disabled:opacity-50',
        mono && 'font-mono',
      )}
    />
  )
}

function Segmented<T extends string>({
  options,
  value,
  onChange,
  full,
}: {
  options: Array<{ value: T; label: string; soon?: boolean }>
  value: T
  onChange: (v: T) => void
  full?: boolean
}) {
  return (
    <div
      className={cn(
        'inline-flex gap-[3px] rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-app)] p-[3px]',
        full && 'flex w-full',
      )}
    >
      {options.map((opt) => {
        const on = opt.value === value
        return (
          <button
            key={opt.value}
            type="button"
            disabled={opt.soon}
            onClick={() => !opt.soon && onChange(opt.value)}
            className={cn(
              'flex items-center justify-center gap-1.5 rounded-[6px] px-3.5 py-[7px] text-[0.78125rem] font-semibold transition-colors',
              full && 'flex-1',
              on
                ? 'bg-[var(--bg-elevated)] text-[var(--text-1)] shadow-[0_1px_2px_rgba(0,0,0,0.3)]'
                : 'bg-transparent text-[var(--text-2)] hover:text-[var(--text-1)]',
              opt.soon && 'cursor-default text-[var(--text-3)] hover:text-[var(--text-3)]',
            )}
          >
            {opt.label}
            {opt.soon && (
              <span className="rounded-full border border-[var(--border)] px-[5px] py-px text-[0.5625rem] uppercase tracking-wide text-[var(--text-3)]">
                soon
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

function InjectionPreview({ draft }: { draft: AuthDraft }) {
  const preview = previewInjection(draft)
  return (
    <div className="rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-app)] px-[15px] py-[13px]">
      <div className="mb-2 flex items-center gap-1.5 text-[0.65625rem] uppercase tracking-[0.06em] text-[var(--text-3)]">
        <ChevronRight size={12} />
        Injection preview
      </div>
      <div className="font-mono text-[0.75rem] leading-[1.9] text-[var(--text-2)]">
        <div>
          <span className="text-[var(--text-1)]">GET</span> {preview.action}
        </div>
        <div>
          <span className="rounded-[4px] bg-[rgba(255,255,255,0.04)] px-1.5 py-px text-[var(--text-3)]">{preview.effect}</span>
        </div>
      </div>
    </div>
  )
}

/* ── auth fields block (shared by create + edit-replace) ───── */

function AuthFields({ draft, setDraft }: { draft: AuthDraft; setDraft: (d: AuthDraft) => void }) {
  const [reveal, setReveal] = useState(false)
  const authType: 'apiKey' | 'bearer' = draft.injection === 'bearer' ? 'bearer' : 'apiKey'
  const isApiKey = authType === 'apiKey'

  return (
    <>
      <div className="mb-4">
        <FieldLabel>Auth type</FieldLabel>
        <Segmented<'apiKey' | 'bearer' | 'basic'>
          value={authType}
          onChange={(v) => {
            if (v === 'basic') return
            setDraft({
              ...draft,
              injection: v === 'bearer' ? 'bearer' : draft.injection === 'bearer' ? 'header' : draft.injection,
            })
          }}
          options={[
            { value: 'apiKey', label: 'API Key' },
            { value: 'bearer', label: 'Bearer' },
            { value: 'basic', label: 'Basic', soon: true },
          ]}
        />
      </div>

      {isApiKey && (
        <div className="mb-4 grid grid-cols-2 gap-3.5">
          <div>
            <FieldLabel required>{draft.injection === 'query' ? 'Key — query param name' : 'Key — header name'}</FieldLabel>
            <TextInput
              mono
              value={draft.fieldName}
              onChange={(v) => setDraft({ ...draft, fieldName: v })}
              placeholder={draft.injection === 'query' ? 'api_key' : 'Authorization'}
            />
          </div>
          <div>
            <FieldLabel>Add to</FieldLabel>
            <Segmented
              full
              value={draft.injection as Exclude<InjectionMode, 'bearer'>}
              onChange={(v) => setDraft({ ...draft, injection: v })}
              options={[
                { value: 'header', label: 'Header' },
                { value: 'query', label: 'Query param' },
              ]}
            />
          </div>
        </div>
      )}

      <div className="mb-4">
        <FieldLabel required>Value</FieldLabel>
        <div className="relative flex items-center">
          <TextInput
            mono
            type={reveal ? 'text' : 'password'}
            value={draft.value}
            onChange={(v) => setDraft({ ...draft, value: v })}
            placeholder={draft.injection === 'bearer' ? 'sk-live-…' : 'paste the secret value'}
          />
          <button
            type="button"
            onClick={() => setReveal((r) => !r)}
            className="absolute right-[11px] flex text-[var(--text-3)] hover:text-[var(--text-1)]"
            aria-label={reveal ? 'Hide value' : 'Reveal value'}
          >
            {reveal ? <EyeOff size={15} /> : <Eye size={15} />}
          </button>
        </div>
        <div className="mt-1.5 text-[0.71875rem] text-[var(--text-3)]">Stored encrypted at rest. Never returned to the model.</div>
      </div>

      <InjectionPreview draft={draft} />
    </>
  )
}

/* ── main editor ───────────────────────────────────────────── */

export function CredentialEditor({
  mode,
  credential,
  saving,
  onCreate,
  onUpdate,
  onDelete,
  onCancelCreate,
}: CredentialEditorProps) {
  const isCreate = mode === 'create'

  const [label, setLabel] = useState(credential?.label ?? '')
  const [active, setActive] = useState(credential ? credential.status === 'active' : true)
  const [draft, setDraft] = useState<AuthDraft>(emptyAuthDraft)
  const [replacing, setReplacing] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [banner, setBanner] = useState<BannerState | null>(null)

  const derivedInjection = useMemo(() => deriveInjection(credential?.fields), [credential?.fields])
  const derivedMeta = INJECTION_META[derivedInjection]

  const labelDirty = isCreate ? label.trim() !== '' : label.trim() !== (credential?.label ?? '')
  const statusDirty = !isCreate && active !== (credential?.status === 'active')
  const secretActive = isCreate || replacing
  const metaDirty = labelDirty || statusDirty
  const dirty = isCreate ? true : metaDirty || (replacing && (draft.host !== '' || draft.value !== '' || draft.fieldName !== ''))

  function fail(message: string) {
    setBanner({ kind: 'error', message })
  }

  function handleSave() {
    setBanner(null)
    const trimmedLabel = label.trim()
    if (!trimmedLabel) {
      fail('Label is required.')
      return
    }

    if (isCreate) {
      const built = buildSecrets(draft)
      if (!built.ok || !built.secrets) {
        fail(built.errors.join(' '))
        return
      }
      onCreate({ label: trimmedLabel, secrets: built.secrets })
      return
    }

    // edit
    const patch: CredentialUpdatePatch = {}
    if (labelDirty) patch.label = trimmedLabel
    if (statusDirty) patch.status = active ? 'active' : 'disabled'
    if (replacing) {
      const built = buildSecrets(draft)
      if (!built.ok || !built.secrets) {
        fail(built.errors.join(' '))
        return
      }
      patch.secrets = built.secrets
    }
    if (Object.keys(patch).length === 0) {
      toast.info('No changes to save.')
      return
    }
    onUpdate(patch)
  }

  function handleRevert() {
    setLabel(credential?.label ?? '')
    setActive(credential ? credential.status === 'active' : true)
    setDraft(emptyAuthDraft())
    setReplacing(false)
    setBanner(null)
  }

  const title = isCreate ? 'New credential' : credential?.label || 'Untitled credential'
  const subtitle = isCreate
    ? 'Not saved yet'
    : `${credential?.id ?? ''}${credential?.updatedAt ? ` · updated ${formatRelative(credential.updatedAt)}` : ''}`

  return (
    <div className="flex min-w-0 flex-col">
      {/* head */}
      <div className="flex items-start justify-between gap-4 border-b border-[var(--border)] px-6 py-4">
        <div className="min-w-0">
          <div className="truncate text-[1.125rem] font-bold text-[var(--text-1)]">{title}</div>
          <div className="mt-1 truncate font-mono text-[0.71875rem] text-[var(--text-3)]">{subtitle}</div>
        </div>
        {!isCreate && (
          <div className="flex shrink-0 items-center gap-3.5">
            <label className="flex cursor-pointer items-center gap-2 text-[0.78125rem] text-[var(--text-2)]">
              <Switch checked={active} onCheckedChange={setActive} />
              {active ? 'Active' : 'Disabled'}
            </label>
            <button
              type="button"
              onClick={() => setConfirmDelete(true)}
              className="flex items-center gap-1.5 rounded-[var(--r-md)] px-2 py-1.5 text-[0.75rem] text-[var(--text-3)] transition-colors hover:bg-[var(--red-dim)] hover:text-[var(--red)]"
            >
              <Trash2 size={13} />
              Delete
            </button>
          </div>
        )}
      </div>

      {/* body */}
      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
        <div className="max-w-[640px]">
          {banner && <InlineBanner banner={banner} onDismiss={() => setBanner(null)} />}

          {confirmDelete && (
            <div className="mb-4 flex items-center justify-between gap-3 rounded-[var(--r-md)] border border-[var(--red)] bg-[var(--red-dim)] px-3.5 py-3">
              <span className="text-[0.78125rem] text-[var(--text-1)]">Delete this credential? This can’t be undone.</span>
              <div className="flex shrink-0 gap-2">
                <button
                  type="button"
                  onClick={() => setConfirmDelete(false)}
                  className="rounded-[var(--r-md)] border border-[var(--border)] px-2.5 py-1.5 text-[0.75rem] text-[var(--text-2)] hover:text-[var(--text-1)]"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  disabled={saving}
                  onClick={onDelete}
                  className="rounded-[var(--r-md)] bg-[var(--red)] px-2.5 py-1.5 text-[0.75rem] font-semibold text-white disabled:opacity-50"
                >
                  {saving ? 'Deleting…' : 'Delete'}
                </button>
              </div>
            </div>
          )}

          <div className="mb-4">
            <FieldLabel required>Label</FieldLabel>
            <TextInput value={label} onChange={setLabel} placeholder="e.g. OpenAI (production)" />
          </div>

          {secretActive ? (
            <>
              <div className="mb-4">
                <FieldLabel required>Host</FieldLabel>
                <TextInput mono value={draft.host} onChange={(v) => setDraft({ ...draft, host: v })} placeholder="api.openai.com" />
                <div className="mt-1.5 text-[0.71875rem] text-[var(--text-3)]">
                  Match key — a request to this host gets this credential injected. Exact host match.
                </div>
              </div>
              <AuthFields draft={draft} setDraft={setDraft} />
              {!isCreate && (
                <button
                  type="button"
                  onClick={() => {
                    setReplacing(false)
                    setDraft(emptyAuthDraft())
                    setBanner(null)
                  }}
                  className="mt-3 text-[0.75rem] text-[var(--text-3)] underline-offset-2 hover:text-[var(--text-2)] hover:underline"
                >
                  Cancel secret replacement
                </button>
              )}
            </>
          ) : (
            /* edit, locked: show what's known, gate the write-only fields */
            <div className="rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] p-4">
              <div className="mb-3 flex items-center gap-2">
                <span className={cn('rounded-[4px] px-[5px] py-[2px] font-mono text-[0.59375rem] font-bold', derivedMeta.badgeClass)}>
                  {derivedMeta.badge}
                </span>
                <span className="text-[0.8125rem] font-semibold text-[var(--text-1)]">{derivedMeta.label} injection</span>
              </div>
              <div className="flex items-start gap-2 text-[0.75rem] leading-relaxed text-[var(--text-3)]">
                <ShieldCheck size={14} className="mt-px shrink-0 text-[var(--green)]" />
                <span>
                  The host and secret value are stored encrypted and aren’t shown here. Replace the secret to change the host,
                  auth type, or value.
                </span>
              </div>
              <button
                type="button"
                onClick={() => {
                  setReplacing(true)
                  setDraft({ ...emptyAuthDraft(), injection: derivedInjection })
                }}
                className="mt-3.5 flex items-center gap-1.5 rounded-[var(--r-md)] border border-[var(--border-strong)] bg-[var(--bg-elevated)] px-3 py-1.5 text-[0.78125rem] font-semibold text-[var(--text-1)] hover:bg-[var(--bg-surface)]"
              >
                <Lock size={13} />
                Replace secret
              </button>
            </div>
          )}
        </div>
      </div>

      {/* savebar */}
      <div className="flex items-center justify-between border-t border-[var(--border)] px-6 py-3">
        <span className="flex items-center gap-1.5 text-[0.75rem] text-[var(--amber)]">
          {dirty && <span className="h-[7px] w-[7px] rounded-full bg-[var(--amber)]" />}
          {dirty ? 'Unsaved changes' : ''}
        </span>
        <div className="flex gap-2.5">
          <button
            type="button"
            onClick={isCreate ? onCancelCreate : handleRevert}
            disabled={saving}
            className="rounded-[var(--r-md)] px-3 py-2 text-[0.8125rem] font-semibold text-[var(--text-2)] hover:text-[var(--text-1)] disabled:opacity-50"
          >
            {isCreate ? 'Cancel' : 'Revert'}
          </button>
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || !dirty}
            className="rounded-[var(--r-md)] bg-[var(--accent)] px-3.5 py-2 text-[0.8125rem] font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-50"
          >
            {saving ? 'Saving…' : isCreate ? 'Create credential' : 'Save changes'}
          </button>
        </div>
      </div>
    </div>
  )
}
