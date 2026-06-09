import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Ban, Check, Copy, FolderDown, KeyRound, Link, TriangleAlert } from 'lucide-react'
import type { Project } from '../../lib/types'
import { projectDisplayName } from '../../lib/projectHelpers'
import { copyText } from '../../lib/utils'
import { formatDate, formatRelative } from '../../lib/format'
import { qk } from '../../lib/queryKeys'
import { createLinkToken, type LinkTokenResult, type LinkTokenSummary } from '../../lib/projectClient'
import { useLinkTokens, useRevokeLinkToken } from '../../hooks/useLinkTokens'
import {
  buildMarkerFiles, MARKER_DIR, supportsDirectoryPicker, writeMarkerToDirectory, type MarkerFile,
} from '../../lib/linkMarker'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import ConfirmationDialog from '../ConfirmationDialog'

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className="h-7 gap-1.5 text-[0.6875rem]"
      onClick={async () => {
        try {
          await copyText(value)
          setCopied(true)
          setTimeout(() => setCopied(false), 1500)
        } catch (err) {
          toast.error(err instanceof Error ? err.message : 'Failed to copy')
        }
      }}
    >
      {copied ? <Check size={12} /> : <Copy size={12} />}
      {copied ? 'Copied' : label}
    </Button>
  )
}

function FileBlock({ file }: { file: MarkerFile }) {
  return (
    <div className="rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)]">
      <div className="flex items-center justify-between gap-2 border-b border-[var(--border)] px-3 py-1.5">
        <span className="font-mono text-[0.6875rem] text-[var(--text-2)]">
          {MARKER_DIR}/{file.name}
        </span>
        <CopyButton value={file.contents} label="Copy" />
      </div>
      <pre className="overflow-x-auto whitespace-pre px-3 py-2 font-mono text-[0.6875rem] leading-[1.5] text-[var(--text-2)]">
        {file.contents}
      </pre>
    </div>
  )
}

// Maps a token's server-evaluated status to a label and color. The server
// already collapsed (active, revokedAt, expiresAt) into one word, so the UI just
// renders it — no client-side clock logic that could disagree with the server.
const STATUS_STYLE: Record<LinkTokenSummary['status'], { label: string; color: string }> = {
  active: { label: 'Active', color: 'var(--green)' },
  revoked: { label: 'Revoked', color: 'var(--text-3)' },
  expired: { label: 'Expired', color: 'var(--amber)' },
}

function TokenRow({
  token,
  onRevoke,
  revoking,
}: {
  token: LinkTokenSummary
  onRevoke: (token: LinkTokenSummary) => void
  revoking: boolean
}) {
  const style = STATUS_STYLE[token.status] ?? STATUS_STYLE.expired
  return (
    <div className="flex items-center justify-between gap-3 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-2">
      <div className="flex min-w-0 flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <span className="font-mono text-[0.75rem] text-[var(--text-1)]">{token.prefix}…</span>
          {token.label && (
            <span className="truncate text-[0.75rem] text-[var(--text-2)]">{token.label}</span>
          )}
        </div>
        <div className="flex items-center gap-2 text-[0.6875rem] text-[var(--text-3)]">
          <span
            className="inline-flex items-center rounded-full px-1.5 py-0.5 text-[0.625rem] font-medium"
            style={{ color: style.color, backgroundColor: `color-mix(in srgb, ${style.color} 14%, transparent)` }}
          >
            {style.label}
          </span>
          <span>created {formatRelative(token.createdAt, { fallback: 'unknown' })}</span>
          {token.status === 'expired' && token.expiresAt && (
            <span>· expired {formatDate(token.expiresAt)}</span>
          )}
        </div>
      </div>
      {token.status === 'active' && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 shrink-0 gap-1.5 text-[0.6875rem] text-[var(--red)] hover:text-[var(--red)]"
          disabled={revoking}
          onClick={() => onRevoke(token)}
        >
          <Ban size={12} />
          Revoke
        </Button>
      )}
    </div>
  )
}

export default function LinkFolderDialog({
  project,
  onClose,
}: {
  project: Project
  onClose: () => void
}) {
  const [issued, setIssued] = useState<LinkTokenResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<LinkTokenSummary | null>(null)
  const canPick = useMemo(() => supportsDirectoryPicker(), [])

  const queryClient = useQueryClient()
  const tokensQuery = useLinkTokens(project.id)
  const revoke = useRevokeLinkToken()
  const tokens = tokensQuery.data ?? []

  // Compose the marker once a token is minted. A build failure (blank inputs) is
  // surfaced as an error rather than emitting a half-written marker.
  const files = useMemo<MarkerFile[] | null>(() => {
    if (!issued) return null
    return buildMarkerFiles({
      serverUrl: window.location.origin,
      projectId: issued.projectId,
      workspaceId: issued.workspaceId,
      token: issued.token,
    })
  }, [issued])

  async function handleMint() {
    setBusy(true)
    setError(null)
    try {
      const result = await createLinkToken(project.id)
      setIssued(result)
      // The freshly minted token should appear in the list when the user steps
      // back to manage links (or reopens the dialog).
      queryClient.invalidateQueries({ queryKey: qk.linkTokensAll })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to mint link token')
    } finally {
      setBusy(false)
    }
  }

  async function handleSave() {
    if (!files) return
    setSaving(true)
    try {
      await writeMarkerToDirectory(files)
      toast.success(`Wrote ${MARKER_DIR}/ into the selected folder`)
    } catch (err) {
      // The user cancelling the folder picker is not an error worth shouting about.
      if (err instanceof DOMException && err.name === 'AbortError') return
      toast.error(err instanceof Error ? err.message : 'Failed to write marker files')
    } finally {
      setSaving(false)
    }
  }

  function confirmRevoke() {
    if (!revokeTarget) return
    revoke.mutate(
      { projectId: project.id, tokenId: revokeTarget.id },
      {
        onSuccess: () => {
          toast.success('Link token revoked')
          setRevokeTarget(null)
        },
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : 'Failed to revoke link token')
        },
      },
    )
  }

  const locked = busy || saving

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !locked) onClose() }}>
      <DialogContent className="max-h-[calc(100vh-2rem)] max-w-[min(92vw,560px)] overflow-hidden rounded-[var(--r-xl)] border-[var(--border)] bg-[var(--bg-panel)] p-0 shadow-[var(--shadow-lg)] gap-0">
        <DialogHeader className="border-b border-[var(--border)] px-5 pt-5 pb-3">
          <DialogTitle className="flex items-center gap-2 text-[0.9375rem] font-semibold text-[var(--text-1)]">
            <Link size={15} className="text-[var(--accent)]" />
            Link this folder
          </DialogTitle>
          <DialogDescription className="text-[0.75rem] text-[var(--text-3)]">
            Bind a folder on disk to{' '}
            <span className="font-semibold text-[var(--text-2)]">{projectDisplayName(project)}</span>{' '}
            by placing an <span className="font-mono">{MARKER_DIR}/</span> marker inside it.
          </DialogDescription>
        </DialogHeader>

        <div className="min-w-0 overflow-y-auto px-5 py-4">
          {!issued ? (
            /* ── Mint step ─────────────────────────────── */
            <div className="flex flex-col gap-4">
              {/* Current link state: lets the owner see whether this project is
                  already linked and revoke a token they no longer trust. */}
              <div className="flex flex-col gap-2">
                <div className="text-[0.6875rem] font-semibold uppercase tracking-wide text-[var(--text-3)]">
                  Existing link tokens
                </div>
                {tokensQuery.isLoading ? (
                  <div className="flex items-center gap-2 text-[0.75rem] text-[var(--text-3)]">
                    <span className="spinner spinner-sm" />
                    Loading…
                  </div>
                ) : tokensQuery.isError ? (
                  <div className="text-[0.75rem] text-[var(--red)]">
                    {tokensQuery.error instanceof Error ? tokensQuery.error.message : 'Failed to load link tokens'}
                  </div>
                ) : tokens.length === 0 ? (
                  <div className="rounded-[var(--r-md)] border border-dashed border-[var(--border)] px-3 py-2.5 text-[0.75rem] text-[var(--text-3)]">
                    No link tokens yet — this project isn&apos;t linked to a folder.
                  </div>
                ) : (
                  <div className="flex flex-col gap-2">
                    {tokens.map((token) => (
                      <TokenRow
                        key={token.id}
                        token={token}
                        onRevoke={setRevokeTarget}
                        revoking={revoke.isPending}
                      />
                    ))}
                  </div>
                )}
              </div>

              <div className="rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-surface)] px-4 py-3 text-[0.8125rem] leading-[1.6] text-[var(--text-2)]">
                Generating a link token mints a secret that binds this folder to the project.
                The token is shown <span className="font-semibold text-[var(--text-1)]">once</span> —
                it is stored in <span className="font-mono text-[0.75rem]">{MARKER_DIR}/token</span> and
                never displayed again.
              </div>
              {error && <div className="text-[0.75rem] text-[var(--red)]">{error}</div>}
              <Button onClick={handleMint} disabled={busy} className="w-fit gap-1.5">
                {busy ? (
                  <>
                    <span className="spinner spinner-sm" />
                    Generating…
                  </>
                ) : (
                  <>
                    <KeyRound size={13} />
                    {tokens.some((t) => t.status === 'active') ? 'Generate another token' : 'Generate link token'}
                  </>
                )}
              </Button>
            </div>
          ) : (
            /* ── Marker step ───────────────────────────── */
            <div className="flex flex-col gap-4">
              {/* One-time token reveal */}
              <div className="flex flex-col gap-2 rounded-[var(--r-lg)] border border-[var(--amber)] bg-[var(--amber-dim)] px-4 py-3">
                <div className="flex items-center gap-2 text-[0.75rem] font-semibold text-[var(--amber)]">
                  <TriangleAlert size={13} />
                  Copy this token now — it won&apos;t be shown again
                </div>
                <div className="flex items-center gap-2">
                  <code
                    data-testid="link-token-value"
                    className="min-w-0 flex-1 truncate rounded-[var(--r-sm)] bg-[var(--bg-panel)] px-2 py-1.5 font-mono text-[0.75rem] text-[var(--text-1)]"
                  >
                    {issued.token}
                  </code>
                  <CopyButton value={issued.token} label="Copy token" />
                </div>
              </div>

              {/* Placement instructions. The folder's path is whatever the user
                  picks below — the project root is workspace-sourced and not known
                  here, so we never print a stale path. */}
              <div className="text-[0.8125rem] leading-[1.6] text-[var(--text-2)]">
                Place these files in an <span className="font-mono text-[0.75rem] text-[var(--text-1)]">{MARKER_DIR}/</span>{' '}
                folder at the root of the folder you&apos;re linking. Commit{' '}
                <span className="font-mono text-[0.75rem]">workspace.json</span> and{' '}
                <span className="font-mono text-[0.75rem]">.gitignore</span>; the{' '}
                <span className="font-mono text-[0.75rem]">token</span> stays local.
              </div>

              {/* Save to folder (Chromium) or visible copy/paste fallback */}
              {canPick ? (
                <div className="flex flex-wrap items-center gap-2">
                  <Button onClick={handleSave} disabled={saving} className="gap-1.5">
                    {saving ? (
                      <>
                        <span className="spinner spinner-sm" />
                        Saving…
                      </>
                    ) : (
                      <>
                        <FolderDown size={13} />
                        Save to folder…
                      </>
                    )}
                  </Button>
                  <span className="text-[0.6875rem] text-[var(--text-3)]">
                    Pick the folder you&apos;re linking; <span className="font-mono">{MARKER_DIR}/</span> is created inside it.
                  </span>
                </div>
              ) : (
                <div className="flex items-start gap-2 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] px-3 py-2 text-[0.75rem] text-[var(--text-3)]">
                  <TriangleAlert size={13} className="mt-0.5 shrink-0 text-[var(--amber)]" />
                  <span>
                    This browser can&apos;t write folders directly. Create an{' '}
                    <span className="font-mono">{MARKER_DIR}/</span> folder at the root of the folder you&apos;re
                    linking and copy each file below into it.
                  </span>
                </div>
              )}

              {files && (
                <div className="flex flex-col gap-2">
                  {files.map((file) => (
                    <FileBlock key={file.name} file={file} />
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-[var(--border)] px-5 py-3">
          <Button variant="ghost" size="sm" onClick={onClose} disabled={locked} className="text-[var(--text-3)]">
            {issued ? 'Done' : 'Cancel'}
          </Button>
        </div>
      </DialogContent>

      <ConfirmationDialog
        open={revokeTarget !== null}
        title="Revoke link token"
        message={
          `Revoking ${revokeTarget?.prefix ?? ''}… immediately breaks any folder bound with it. ` +
          'Sessions using this token can no longer reach the project; you can mint a new one to re-link.'
        }
        confirmLabel="Revoke token"
        tone="danger"
        busy={revoke.isPending}
        onClose={() => { if (!revoke.isPending) setRevokeTarget(null) }}
        onConfirm={confirmRevoke}
      />
    </Dialog>
  )
}
