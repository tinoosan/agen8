import { describe, expect, it } from 'vitest'
import { displayableAttention, WAITING_GRACE_MS, type AttentionEntry } from './useAttention'

function entry(kind: AttentionEntry['kind'], ageMs: number, now: number): AttentionEntry {
  const since = new Date(now - ageMs).toISOString()
  return { sessionRef: `s-${kind}-${ageMs}`, kind, since, updatedAt: since }
}

describe('displayableAttention', () => {
  const now = 1_700_000_000_000

  it('hides waiting entries younger than the grace period', () => {
    const fresh = entry('waiting', WAITING_GRACE_MS - 1000, now)
    expect(displayableAttention([fresh], now)).toEqual([])
  })

  it('shows waiting entries once they cross the grace period', () => {
    const lasted = entry('waiting', WAITING_GRACE_MS + 1000, now)
    expect(displayableAttention([lasted], now)).toEqual([lasted])
  })

  it('shows approval prompts immediately', () => {
    const approval = entry('needs_approval', 0, now)
    expect(displayableAttention([approval], now)).toEqual([approval])
  })

  it('filters mixed lists per entry', () => {
    const freshWait = entry('waiting', 1000, now)
    const oldWait = entry('waiting', WAITING_GRACE_MS * 2, now)
    const approval = entry('needs_approval', 500, now)
    expect(displayableAttention([freshWait, oldWait, approval], now)).toEqual([oldWait, approval])
  })
})
