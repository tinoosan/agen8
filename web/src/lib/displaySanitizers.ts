function normalizeWhitespace(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

export function looksLikeOpaqueId(value: string | null | undefined): boolean {
  const text = (value ?? '').trim()
  if (!text) return false
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(text)) return true
  if (/^(space|run|task|kr|oa|mission|plan|ch)-[a-z0-9-]{4,}$/i.test(text)) return true
  return false
}

export function sanitizeSpaceTitle(title: string | null | undefined): string | null {
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
      .replace(/\b(?:Mission|Plan|KR|Task|OA):\s+[^\s]+/gi, ' ')
      .replace(/\b(?:mission|plan|kr|task|oa)-[a-z0-9-]+\b/gi, ' ')
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
