function normalizeWhitespace(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

// Loose UUID shape (8-4-4-4-12 hex). Deliberately not RFC-strict on the
// version/variant nibbles — machine refs here aren't guaranteed RFC UUIDs.
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function isUuid(value: string): boolean {
  return UUID_RE.test(value.trim())
}

// A raw entity identifier ("member-9f3a...", "dec-...") surfacing where a human
// label belongs. Broad prefix set: every entity kind that can appear as an
// actor / source / member id.
const PREFIXED_ID_RE =
  /^(member|user|session|thread|channel|space|project|task|kr|mission|dec)-[a-z0-9-]{4,}$/i

export function isPrefixedId(value: string): boolean {
  return PREFIXED_ID_RE.test(value.trim())
}

// Title-hiding scope is intentionally NARROWER than isPrefixedId: only the
// entity kinds that appear as auto-generated *titles* (space/run/task/kr/
// mission). Broadening it to the full set would suppress legitimate single-
// token titles like "user-friendly" or "session-summary", so keep it separate.
const TITLE_ID_RE = /^(space|run|task|kr|mission)-[a-z0-9-]{4,}$/i

export function looksLikeOpaqueId(value: string | null | undefined): boolean {
  const text = (value ?? '').trim()
  if (!text) return false
  return isUuid(text) || TITLE_ID_RE.test(text)
}

export function sanitizeDisplayTitle(title: string | null | undefined): string | null {
  const text = normalizeWhitespace(title ?? '')
  if (!text) return null
  if (looksLikeOpaqueId(text)) return null
  return text
}

export function sanitizeDecisionTitle(title: string | null | undefined): string {
  const text = normalizeWhitespace(title ?? '')
  if (!text) return 'Decision recorded'

  const stripped = normalizeWhitespace(
    text
      .replace(/\b(?:Mission|KR|Task):\s+[^\s]+/gi, ' ')
      .replace(/\b(?:mission|kr|task)-[a-z0-9-]+\b/gi, ' ')
      .replace(/\s+[|·>-]\s+/g, ' '),
  )

  if (!stripped) return 'Decision recorded'
  if (looksLikeOpaqueId(stripped)) return 'Decision recorded'
  return stripped
}

export function safeReferenceLabel(value: string | null | undefined): string | null {
  const text = normalizeWhitespace(value ?? '')
  if (!text) return null
  if (looksLikeOpaqueId(text)) return null
  if (/^(unknown|none|null)$/i.test(text)) return null
  return text
}
