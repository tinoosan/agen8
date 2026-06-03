function maybeParseJSON(value: string): unknown {
  const trimmed = value.trim()
  if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) return value
  try {
    return JSON.parse(trimmed)
  } catch {
    return value
  }
}

export function materializeNestedJSONStrings(value: unknown, depth = 0): unknown {
  if (depth > 5) return value
  if (typeof value === 'string') {
    const parsed = maybeParseJSON(value)
    if (parsed === value) return value
    return materializeNestedJSONStrings(parsed, depth + 1)
  }
  if (Array.isArray(value)) {
    return value.map((item) => materializeNestedJSONStrings(item, depth + 1))
  }
  if (!value || typeof value !== 'object') return value

  const next: Record<string, unknown> = {}
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    next[key] = materializeNestedJSONStrings(entry, depth + 1)
  }
  return next
}
