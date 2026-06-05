/**
 * credentialModel — pure logic + types for the HTTP-tool credentials console.
 *
 * This is the seam between the UI and the server's secret model, so it is kept
 * deliberately small and side-effect free (everything here is unit-tested).
 *
 * HTTP credentials are always `api_key` kind. We persist the host + injection
 * config *inside the encrypted secrets map* (no schema migration), using key
 * names that line up with the `http` tool's injection reader
 * (internal/mcp/tools/http) so a future host-keyed resolver can consume them
 * unchanged:
 *
 *   value      → the secret the tool injects (required by the api_key validator)
 *   host       → exact host-match key (e.g. "api.openai.com")
 *   injection  → "bearer" | "header" | "query"
 *   headerName → header field name, header mode only (server classifies PUBLIC)
 *   paramName  → query param name,  query  mode only (server classifies PUBLIC)
 *
 * Read-back caveat: `credential.list/get` return field *names* but never values,
 * and we deliberately do NOT call the resolve path from the browser (it would
 * hand the decrypted secret to the client). So host/value cannot be displayed
 * after reload — only the injection *mode* is recoverable, because the presence
 * of `headerName` / `paramName` in the returned field list encodes it exactly.
 */
import type { CredentialFieldView } from '../../hooks/useCredentials'

export type InjectionMode = 'bearer' | 'header' | 'query'

/** Reserved secret keys. Must match the http tool's FieldName/value lookup. */
export const SECRET_KEYS = {
  value: 'value',
  host: 'host',
  injection: 'injection',
  headerName: 'headerName',
  paramName: 'paramName',
} as const

export interface InjectionMeta {
  mode: InjectionMode
  /** Short label for the auth type, e.g. "Header". */
  label: string
  /** 3-letter rail mini-badge. */
  badge: string
  /** Tailwind classes for the mini-badge pill. */
  badgeClass: string
}

export const INJECTION_META: Record<InjectionMode, InjectionMeta> = {
  header: {
    mode: 'header',
    label: 'Header',
    badge: 'HDR',
    badgeClass: 'text-[#c4b5fd] bg-[rgba(167,139,250,0.12)]',
  },
  bearer: {
    mode: 'bearer',
    label: 'Bearer token',
    badge: 'BRR',
    badgeClass: 'text-[var(--blue)] bg-[rgba(96,165,250,0.12)]',
  },
  query: {
    mode: 'query',
    label: 'Query param',
    badge: 'QRY',
    badgeClass: 'text-[#7dd3fc] bg-[rgba(125,211,252,0.12)]',
  },
}

/**
 * Recover the injection mode from the field names the server returns. This is
 * exact (not a guess) because we author `headerName`/`paramName` on write. A
 * credential with neither marker is single-value injection → bearer; that is the
 * correct reading, not a fallback masking missing data (an api_key always has a
 * `value`).
 */
export function deriveInjection(fields: CredentialFieldView[] | undefined): InjectionMode {
  const names = new Set((fields ?? []).map((f) => f.name))
  if (names.has(SECRET_KEYS.headerName)) return 'header'
  if (names.has(SECRET_KEYS.paramName)) return 'query'
  return 'bearer'
}

/** The editable secret/config half of a credential (the write-only fields). */
export interface AuthDraft {
  injection: InjectionMode
  host: string
  /** Header name (header mode) or param name (query mode); unused for bearer. */
  fieldName: string
  value: string
}

export function emptyAuthDraft(): AuthDraft {
  return { injection: 'header', host: '', fieldName: '', value: '' }
}

export interface BuildSecretsResult {
  ok: boolean
  errors: string[]
  secrets?: Record<string, string>
}

/**
 * Validate the auth draft and assemble the secrets map sent to the server.
 * Fails loudly: returns the list of missing required fields rather than
 * substituting defaults. The server replaces the entire secret bag on update,
 * so this map is always the complete set.
 */
export function buildSecrets(draft: AuthDraft): BuildSecretsResult {
  const host = draft.host.trim()
  const value = draft.value.trim()
  const fieldName = draft.fieldName.trim()
  const errors: string[] = []

  if (!host) errors.push('Host is required.')
  if (!value) errors.push('Value is required.')
  if (draft.injection === 'header' && !fieldName) errors.push('Header name is required.')
  if (draft.injection === 'query' && !fieldName) errors.push('Query param name is required.')

  if (errors.length > 0) return { ok: false, errors }

  const secrets: Record<string, string> = {
    [SECRET_KEYS.value]: value,
    [SECRET_KEYS.host]: host,
    [SECRET_KEYS.injection]: draft.injection,
  }
  if (draft.injection === 'header') secrets[SECRET_KEYS.headerName] = fieldName
  if (draft.injection === 'query') secrets[SECRET_KEYS.paramName] = fieldName

  return { ok: true, errors: [], secrets }
}

export interface InjectionPreview {
  /** What the http tool does, e.g. "adds header" / "appends query". */
  action: string
  /** The wire effect with the secret redacted. */
  effect: string
}

/** Build the live injection-preview lines. Empty inputs render placeholders. */
export function previewInjection(draft: AuthDraft): InjectionPreview {
  const host = draft.host.trim() || '‹host›'
  const redacted = '•••••••• (redacted)'
  switch (draft.injection) {
    case 'bearer':
      return { action: `https://${host}/… → adds header`, effect: `Authorization: Bearer ${redacted}` }
    case 'header': {
      const name = draft.fieldName.trim() || '‹header name›'
      return { action: `https://${host}/… → adds header`, effect: `${name}: ${redacted}` }
    }
    case 'query': {
      const name = draft.fieldName.trim() || '‹param name›'
      return { action: `https://${host}/… → appends query`, effect: `?…&${name}=${redacted}` }
    }
  }
}

/** Compact relative time for the rail/editor subtitles. */
export function formatRelative(iso?: string): string {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diffMs = Date.now() - then
  const sec = Math.round(diffMs / 1000)
  if (sec < 45) return 'just now'
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h ago`
  const day = Math.round(hr / 24)
  if (day < 30) return `${day}d ago`
  const mo = Math.round(day / 30)
  if (mo < 12) return `${mo}mo ago`
  return `${Math.round(mo / 12)}y ago`
}
