const UNQUOTED_REF_CHARS = /^[A-Za-z0-9._~:/\\@?&=+,%#-]+/

function isAtBoundary(input: string, index: number): boolean {
  if (index === 0) return true
  return /[\s([{,;:]/.test(input[index - 1] ?? '')
}

export interface ActiveAtRef {
  query: string
  replaceStart: number
  replaceEnd: number
}

export function activeAtTokenAtEnd(input: string): ActiveAtRef | null {
  const at = input.lastIndexOf('@')
  if (at < 0 || !isAtBoundary(input, at)) return null

  const tail = input.slice(at + 1)
  if (tail.startsWith('"') || tail.startsWith("'")) {
    const quote = tail[0]
    const body = tail.slice(1)
    if (body.includes(quote)) return null
    return { query: body, replaceStart: at, replaceEnd: input.length }
  }

  if (tail.length === 0) {
    return { query: '', replaceStart: at, replaceEnd: input.length }
  }
  if (/\s/.test(tail)) return null

  const match = tail.match(UNQUOTED_REF_CHARS)
  if (!match || match[0] !== tail) return null
  return { query: tail, replaceStart: at, replaceEnd: input.length }
}

export function extractAtRefs(input: string): string[] {
  const refs: string[] = []
  let i = 0
  while (i < input.length) {
    const at = input.indexOf('@', i)
    if (at < 0) break
    if (!isAtBoundary(input, at)) {
      i = at + 1
      continue
    }

    const next = input[at + 1]
    if (next === '"' || next === "'") {
      const end = input.indexOf(next, at + 2)
      if (end > at + 2) refs.push(input.slice(at + 2, end))
      i = end >= 0 ? end + 1 : at + 2
      continue
    }

    const tail = input.slice(at + 1)
    const match = tail.match(UNQUOTED_REF_CHARS)
    if (match?.[0]) {
      refs.push(match[0])
      i = at + 1 + match[0].length
      continue
    }
    i = at + 1
  }
  return refs
}

export function formatAtRef(path: string): string {
  const trimmed = path.trim()
  if (!trimmed) return '@'
  if (!/\s/.test(trimmed) && !trimmed.includes('"') && !trimmed.includes("'")) {
    return `@${trimmed}`
  }
  if (!trimmed.includes("'")) return `@'${trimmed}'`
  return `@"${trimmed.replace(/"/g, '')}"`
}
