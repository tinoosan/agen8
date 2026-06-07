/**
 * CredentialRail — left pane of the credentials console. Search + "New
 * credential" + a status-grouped list. Each row shows a status dot, the label,
 * a relative "updated" subtitle, and the injection mini-badge (HDR/BRR/QRY)
 * recovered from the credential's field names.
 */
import { useMemo, useState } from 'react'
import { Search, Plus, KeyRound } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { CredentialView } from '../../lib/types'
import { deriveInjection, INJECTION_META } from './credentialModel'
import { formatRelative } from '@/lib/format'

function statusDotColor(status: CredentialView['status']): string {
  if (status === 'active') return 'var(--green)'
  if (status === 'invalid') return 'var(--red)'
  return 'var(--amber)'
}

function MiniBadge({ credential }: { credential: CredentialView }) {
  const meta = INJECTION_META[deriveInjection(credential.fields)]
  return (
    <span
      className={cn('shrink-0 rounded-[4px] px-[5px] py-[2px] font-mono text-[0.59375rem] font-bold', meta.badgeClass)}
      title={meta.label}
    >
      {meta.badge}
    </span>
  )
}

function RailRow({
  credential,
  active,
  onClick,
}: {
  credential: CredentialView
  active: boolean
  onClick: () => void
}) {
  const updated = formatRelative(credential.updatedAt)
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'mb-0.5 flex w-full items-center gap-[9px] rounded-[var(--r-md)] border px-2.5 py-[9px] text-left transition-colors',
        active
          ? 'border-[var(--accent-border)] bg-[var(--accent-dim)]'
          : 'border-transparent bg-transparent hover:bg-[var(--bg-hover)]',
      )}
    >
      <span
        className="h-[7px] w-[7px] shrink-0 rounded-full"
        style={{ backgroundColor: statusDotColor(credential.status) }}
      />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[0.8125rem] font-semibold text-[var(--text-1)]">
          {credential.label || 'Untitled credential'}
        </span>
        {updated && (
          <span className="mt-[2px] block truncate font-mono text-[0.6875rem] text-[var(--text-3)]">
            updated {updated}
          </span>
        )}
      </span>
      <MiniBadge credential={credential} />
    </button>
  )
}

function RailGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <div className="px-2 pb-[5px] pt-[10px] text-[0.65625rem] font-bold uppercase tracking-[0.07em] text-[var(--text-3)]">
        {label}
      </div>
      {children}
    </>
  )
}

export function CredentialRail({
  credentials,
  loading,
  selectedId,
  isCreating,
  onSelect,
  onNew,
}: {
  credentials: CredentialView[]
  loading: boolean
  selectedId: string | null
  isCreating: boolean
  onSelect: (id: string) => void
  onNew: () => void
}) {
  const [query, setQuery] = useState('')

  const { active, disabled } = useMemo(() => {
    const q = query.trim().toLowerCase()
    const filtered = q
      ? credentials.filter((c) => c.label.toLowerCase().includes(q))
      : credentials
    return {
      active: filtered.filter((c) => c.status === 'active'),
      disabled: filtered.filter((c) => c.status !== 'active'),
    }
  }, [credentials, query])

  const empty = !loading && credentials.length === 0
  const noMatches = !loading && credentials.length > 0 && active.length === 0 && disabled.length === 0

  return (
    <div className="flex flex-col border-r border-[var(--border)] bg-[color-mix(in_srgb,var(--bg-panel)_82%,var(--bg-app))]">
      <div className="flex flex-col gap-2.5 p-3.5 pb-2">
        <div className="flex items-center gap-2 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-app)] px-2.5 py-2">
          <Search size={13} className="shrink-0 text-[var(--text-3)]" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search by label"
            className="min-w-0 flex-1 border-none bg-transparent text-[0.78125rem] text-[var(--text-1)] outline-none placeholder:text-[var(--text-3)]"
          />
        </div>
        <button
          type="button"
          onClick={onNew}
          className="flex w-full items-center justify-center gap-[7px] rounded-[var(--r-md)] bg-[var(--accent)] px-3 py-2 text-[0.8125rem] font-semibold text-white transition-opacity hover:opacity-90"
        >
          <Plus size={14} strokeWidth={2.4} />
          New credential
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {loading && (
          <div className="flex flex-col gap-1.5 px-1 pt-2">
            {[0, 1, 2].map((i) => (
              <div key={i} className="h-[44px] rounded-[var(--r-md)] bg-[var(--bg-elevated)] skeleton" />
            ))}
          </div>
        )}

        {empty && (
          <div className="flex flex-col items-center gap-2 px-3 pt-10 text-center">
            <KeyRound size={20} className="text-[var(--text-3)]" />
            <p className="m-0 text-[0.78125rem] text-[var(--text-3)]">No credentials yet.</p>
            <p className="m-0 text-[0.71875rem] text-[var(--text-3)]">
              Add one to let the <b className="text-[var(--text-2)]">http</b> tool authenticate calls.
            </p>
          </div>
        )}

        {noMatches && (
          <p className="px-2 pt-4 text-center text-[0.75rem] text-[var(--text-3)]">No credentials match “{query}”.</p>
        )}

        {isCreating && (
          <div className="mb-1 mt-1 flex items-center gap-[9px] rounded-[var(--r-md)] border border-dashed border-[var(--accent-border)] bg-[var(--accent-dim)] px-2.5 py-[9px]">
            <Plus size={13} className="shrink-0 text-[var(--accent)]" />
            <span className="text-[0.8125rem] font-semibold text-[var(--text-1)]">New credential</span>
          </div>
        )}

        {active.length > 0 && (
          <RailGroup label="Active">
            {active.map((c) => (
              <RailRow key={c.id} credential={c} active={c.id === selectedId} onClick={() => onSelect(c.id)} />
            ))}
          </RailGroup>
        )}

        {disabled.length > 0 && (
          <RailGroup label="Disabled">
            {disabled.map((c) => (
              <RailRow key={c.id} credential={c} active={c.id === selectedId} onClick={() => onSelect(c.id)} />
            ))}
          </RailGroup>
        )}
      </div>
    </div>
  )
}
