import React, { lazy, Suspense, useState, useMemo, memo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { AgentEvent } from '../../lib/types'
import { ChevronRight } from 'lucide-react'
import { createPatch } from 'diff'
import clsx from 'clsx'
import {
  humanizeKind,
  getKindStyle,
  getStatusClass,
  relativeTime,
  formatDuration,
  basename,
  isDiffSkipped,
  guessLang,
  FILE_WRITE_KINDS,
} from './activityHelpers'
import { DiffBlock } from './DiffBlock'

const ActivityCodeBlock = lazy(() => import('./ActivityCodeBlock'))

/* ── Helpers ─────────────────────────────────────────── */

function renderJSONOrText(data: unknown): string {
  if (typeof data === 'string') return data
  return JSON.stringify(data, null, 2)
}

function parseSpaceEntryLine(line: string): { displayName: string; status: string; roles: string[]; coordinatorRole: string } | null {
  const trimmed = String(line ?? '').trim()
  if (!trimmed) return null
  const content = trimmed.startsWith('- ') ? trimmed.slice(2).trim() : trimmed
  if (!content) return null
  const parts = content.split('|').map((part) => part.trim()).filter(Boolean)
  if (parts.length === 0) return null
  let status = ''
  let coordinatorRole = ''
  let roles: string[] = []
  for (const part of parts.slice(1)) {
    const [rawKey, ...rest] = part.split('=')
    const key = String(rawKey ?? '').trim().toLowerCase()
    const value = rest.join('=').trim()
    if (!key || !value) continue
    if (key === 'status') status = value
    if (key === 'coordinator' || key === 'coordinatorrole') coordinatorRole = value
    if (key === 'roles') roles = value.split(',').map((role) => role.trim()).filter(Boolean)
  }
  return {
    displayName: parts[0] || 'Unresolved Space Label',
    status,
    roles,
    coordinatorRole,
  }
}

function parseSpaceRows(data: Record<string, string> | undefined): Array<{ displayName: string; status: string; roles: string[]; coordinatorRole: string }> {
  if (!data) return []
  const candidates = [data.spacesJson, data.spaceEntries, data.result]
  for (const raw of candidates) {
    const text = (raw ?? '').trim()
    if (!text || (!text.startsWith('[') && !text.startsWith('{'))) continue
    try {
      const parsed = JSON.parse(text) as unknown
      if (Array.isArray(parsed) && parsed.every((row) => typeof row === 'string')) {
        return parsed
          .map((line) => parseSpaceEntryLine(line))
          .filter((row): row is { displayName: string; status: string; roles: string[]; coordinatorRole: string } => Boolean(row))
      }
      const rows = Array.isArray(parsed)
        ? parsed
        : (parsed && typeof parsed === 'object' && Array.isArray((parsed as Record<string, unknown>).spaces)
            ? (parsed as Record<string, unknown>).spaces as unknown[]
            : [])
      return rows
        .filter((row): row is Record<string, unknown> => Boolean(row) && typeof row === 'object')
        .map((row) => {
          const displayName = String(row.displayName ?? row.spaceName ?? row.spaceId ?? '').trim() || 'Unresolved Space Label'
          const status = String(row.status ?? '').trim()
          const coordinatorRole = String(row.coordinatorRole ?? '').trim()
          const roles = Array.isArray(row.roles) ? row.roles.filter((role): role is string => typeof role === 'string') : []
          return { displayName, status, roles, coordinatorRole }
        })
    } catch {
      continue
    }
  }
  return []
}

/* ── EventRow component ──────────────────────────────── */

export const EventRow = memo(function EventRow({ event }: { event: AgentEvent }) {
  const [expanded, setExpanded] = useState(false)
  const eventRole = event.data?.role || ''
  const showRole = (event.kind ?? '').trim().toLowerCase() !== 'user_message'
  const message = event.title || event.outputPreview || event.textPreview || ''

  const summary = message || event.kind || ''
  const role = showRole ? eventRole : ''
  const statusClass = getStatusClass(event)
  const isError = event.status === 'error' || event.kind === 'error'
  const isPending = statusClass === 'pending'
  const kindLower = (event.kind ?? '').toLowerCase()
  const spaceAction = kindLower === 'space' ? (event.data?.action || '').toLowerCase() : ''
  const isSpaceListEvent = kindLower === 'list_spaces' || (kindLower === 'space' && spaceAction === 'list')

  // Inline context: file path, command, spawned role, error
  const inlinePath = event.path ? basename(event.path) : null
  const inlineCommand = kindLower.includes('exec') ? (event.data?.command || event.data?.cmd) : null
  const inlineExitCode = event.data?.exit_code ?? event.data?.exitCode
  const inlineSpawnRole = kindLower.includes('spawn') ? (event.data?.spawned_role || event.data?.role_name || event.data?.spawnedRole) : null
  const inlineError = isError ? (event.error || event.data?.error) : null

  // Duration
  const duration = event.duration != null
    ? event.duration / 1_000_000 // Convert nanoseconds to milliseconds
    : (event.completedAt && event.startedAt
      ? new Date(event.completedAt).getTime() - new Date(event.startedAt).getTime()
      : null)

  // Build a rich detail object of all interesting payload fields to display when expanded.
  const detailsList = useMemo(() => {
    const d: Record<string, React.ReactNode> = {}


    // File write diff rendering
    if (FILE_WRITE_KINDS.has(event.kind ?? '') && !isDiffSkipped(event.path)) {
      // write_file stores a computed before->after diff in data.patchPreview
      const fileLang = guessLang(event.path ?? '')
      if (event.kind === 'write_file' && event.data?.patchPreview) {
        d['Diff'] = (
          <DiffBlock
            unified={event.data.patchPreview}
            truncated={event.data.patchTruncated === 'true'}
            lang={fileLang}
          />
        )
      } else if (event.kind === 'edit_file' && event.data?.editsJSON) {
        let combined = ''
        try {
          const edits = JSON.parse(event.data.editsJSON) as { old: string; new: string }[]
          const label = basename(event.path ?? 'file')
          combined = edits
            .map(e => createPatch(label, e.old, e.new, '', '', { context: 3 }))
            .join('\n')
        } catch {
          combined = ''
        }
        if (combined.trim()) d['Diff'] = <DiffBlock unified={combined} lang={fileLang} />
      } else if (event.kind === 'write_file' && event.textPreview && !event.textRedacted) {
        // Fallback for older events that lack patchPreview: show syntax-highlighted content
        const mode = event.data?.writeMode
        const lang = guessLang(event.path ?? '')
        d[mode === 'created' ? 'New file' : 'Written content'] = (
          <Suspense fallback={<pre className="mono m-0 px-2.5 py-2 text-[11px] rounded-[var(--r-md)] bg-[var(--bg-app)] overflow-x-auto">{event.textPreview}</pre>}>
            <ActivityCodeBlock code={event.textPreview} language={lang} compact showLineNumbers />
          </Suspense>
        )
        if (event.textTruncated) {
          d[''] = <span className="text-[10px] text-[var(--text-3)] italic">preview truncated</span>
        }
      }
    }

    if (isSpaceListEvent) {
      const rows = parseSpaceRows(event.data)
      if (rows.length === 0) {
        d['Capture'] = <span className="text-[var(--red)]">Missing result payload for space operation `list`.</span>
      } else {
        d['Spaces'] = (
          <div className="flex flex-col gap-1">
            {rows.map((row, idx) => (
              <div key={`${row.displayName}-${idx}`} className="text-[11px]">
                <span className="font-medium">{row.displayName}</span>
                {row.status && <span className="text-[var(--text-3)]"> · {row.status}</span>}
                {row.coordinatorRole && <div className="text-[10px] text-[var(--text-3)]">coordinator: {row.coordinatorRole}</div>}
                {row.roles.length > 0 && <div className="text-[10px] text-[var(--text-3)]">roles: {row.roles.join(', ')}</div>}
              </div>
            ))}
          </div>
        )
      }
    }

    if (event.data) {
      const remainingData = { ...event.data }
      delete remainingData.role
      // Suppress raw data fields for file writes -- already rendered above
      if (FILE_WRITE_KINDS.has(event.kind ?? '')) {
        delete remainingData.patchPreview
        delete remainingData.patchTruncated
        delete remainingData.editsJSON
        delete remainingData.textPreview
        delete remainingData.textTruncated
        delete remainingData.textRedacted
        delete remainingData.textIsJSON
        delete remainingData.textBytes
      }
      // Suppress fields already shown in structured views
      if (isSpaceListEvent) {
        for (const k of ['spacesJson', 'spaceEntries', 'result', 'count']) {
          delete remainingData[k]
        }
      }

      if (Object.keys(remainingData).length > 0) {
        d['Data payload'] = <span className="mono">{renderJSONOrText(remainingData)}</span>
      }
    }

    if (event.path) d['Path'] = <span className="mono">{event.path}</span>
    if (event.outputPreview && !isSpaceListEvent) {
      d['Output'] = <div className="md-prose"><ReactMarkdown remarkPlugins={[remarkGfm]}>{event.outputPreview}</ReactMarkdown></div>
    }
    if (event.error) d['Error'] = <span className="mono text-[var(--red)]">{event.error}</span>
    return d
  }, [event, isSpaceListEvent])

  const hasDetail = Object.keys(detailsList).length > 0

  // Hide thinking events entirely from activity feed
  if (event.kind?.startsWith('model.thinking')) {
    return null
  }

  return (
    <div
      role={hasDetail ? 'button' : undefined}
      tabIndex={hasDetail ? 0 : undefined}
      aria-expanded={hasDetail ? expanded : undefined}
      className={`activity-row${hasDetail ? ' has-detail' : ''}${isError ? ' is-error' : ''}`}
      onClick={() => hasDetail && setExpanded(e => !e)}
      onKeyDown={hasDetail ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setExpanded(prev => !prev) } } : undefined}
    >
      {/* Status indicator */}
      {isPending ? (
        <span className="spinner shrink-0 mt-[5px] w-1.5 h-1.5 border-[1.5px]" />
      ) : (
        <div className={`status-dot ${statusClass}`} />
      )}

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5 min-w-0">
          {/* Expand chevron */}
          {hasDetail ? (
            <ChevronRight
              size={10}
              className={clsx(
                'text-[var(--text-3)] shrink-0 transition-transform duration-150',
                expanded ? 'rotate-90' : 'rotate-0',
              )}
            />
          ) : (
            <span className="w-2.5 shrink-0" />
          )}

          {/* Role text label */}
          {role && (
            <span className="text-[10px] font-semibold text-[var(--text-3)] tracking-[0.04em] uppercase shrink-0">
              {role}
            </span>
          )}

          {/* Kind pill */}
          {event.kind && (() => {
            const label = humanizeKind(event.kind)
            if (!label) return null
            const style = getKindStyle(event.kind)
            return (
              <span
                className="kind-pill"
                style={{ background: style.bg, color: style.fg }}
              >
                {label}
              </span>
            )
          })()}

          {/* Inline file path */}
          {inlinePath && (
            <span className="mono text-[10px] text-[var(--text-3)] shrink-0">
              {inlinePath}
            </span>
          )}

          {/* Inline command for exec events */}
          {inlineCommand && (
            <span className="mono truncate text-[10px] text-[var(--text-2)] max-w-[160px]">
              {inlineCommand}
            </span>
          )}

          {/* Inline exit code */}
          {inlineExitCode != null && (
            <span className={clsx(
              'mono text-[9px] shrink-0 font-semibold',
              String(inlineExitCode) === '0' ? 'text-[var(--green)]' : 'text-[var(--red)]',
            )}>
              {String(inlineExitCode) === '0' ? 'ok' : `exit ${inlineExitCode}`}
            </span>
          )}

          {/* Inline spawned role */}
          {inlineSpawnRole && (
            <span className="text-[11px] text-[var(--accent)] font-medium">
              {inlineSpawnRole}
            </span>
          )}

          {/* Summary text */}
          <span className="truncate text-xs text-[var(--text-2)] flex-1">
            {summary}
          </span>

          {/* Inline error (red, visible without expanding) */}
          {inlineError && !expanded && (
            <span className="truncate text-[11px] text-[var(--red)] max-w-[200px] shrink">
              {inlineError}
            </span>
          )}

          {/* Duration */}
          {duration != null && duration > 0 && (
            <span className="text-[10px] text-[var(--text-3)] tabular-nums shrink-0">
              {formatDuration(duration)}
            </span>
          )}

          {/* Relative timestamp */}
          {event.startedAt && (
            <span className="text-[10px] text-[var(--text-3)] tabular-nums shrink-0 ml-1">
              {relativeTime(event.startedAt)}
            </span>
          )}
        </div>

        {/* Expanded detail */}
        {expanded && hasDetail && (
          <div className="animate-fade-in mt-1.5 bg-[var(--bg-app)] rounded-[var(--r-md)] border border-[var(--border)] overflow-hidden">
            {Object.entries(detailsList).map(([key, val], i) => {
              // Rich values (diff blocks, syntax-highlighted code) manage their own
              // layout and must not inherit text-wrapping or height constraints.
              const isRich = key === 'Diff' || key === 'Appended' || key === 'Code Executed' ||
                key === 'New file' || key === 'Written content'
              return (
                <div key={key} className="px-2.5 py-1.5" style={{ borderTop: i > 0 ? '1px solid var(--border)' : 'none' }}>
                  {key && (
                    <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">
                      {key}
                    </div>
                  )}
                  <div
                    className={clsx(
                      'text-[11px]',
                      !isRich && 'text-[var(--text-2)] whitespace-pre-wrap break-words max-h-[250px] overflow-y-auto',
                    )}
                  >
                    {val}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
})
