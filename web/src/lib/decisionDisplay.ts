import { isPrefixedId } from './displaySanitizers'

export interface DecisionActorDisplay {
  label: string
  clusterKey: string
}

export function decisionActorDisplay(decision: {
  memberId?: string
  memberName?: string
  source?: string
  sourceIdentity?: string
}): DecisionActorDisplay {
  const memberName = decision.memberName?.trim()
  const sourceIdentity = decision.sourceIdentity?.trim()
  const memberId = decision.memberId?.trim()
  const source = decision.source?.trim()

  const label =
    memberName ||
    readableIdentity(sourceIdentity) ||
    readableIdentity(memberId) ||
    memberId ||
    sourceIdentity ||
    source ||
    'agent'

  return {
    label,
    clusterKey: memberId || sourceIdentity || label,
  }
}

function readableIdentity(value: string | undefined): string {
  const text = value?.trim()
  if (!text || isPrefixedId(text)) return ''
  return text
}

export type ConfidenceTone = 'high' | 'medium' | 'low'

// Confidence colour-codes a decision for scanning: high (>=80%) green,
// medium (>=60%) amber, low red. One threshold source for every surface.
export function confidenceTone(confidence: number): ConfidenceTone {
  if (confidence >= 0.8) return 'high'
  if (confidence >= 0.6) return 'medium'
  return 'low'
}

const TONE_COLOR: Record<ConfidenceTone, string> = {
  high: 'var(--green)',
  medium: 'var(--amber)',
  low: 'var(--red)',
}

export function confidenceColor(confidence: number): string {
  return TONE_COLOR[confidenceTone(confidence)]
}

// Literal class strings keep Tailwind's JIT happy (no dynamic interpolation).
const TONE_BADGE_CLASS: Record<ConfidenceTone, string> = {
  high: 'bg-[var(--green-dim)] text-[var(--green)]',
  medium: 'bg-[var(--amber-dim)] text-[var(--amber)]',
  low: 'bg-[var(--red-dim)] text-[var(--red)]',
}

export function confidenceBadgeClass(confidence: number): string {
  return TONE_BADGE_CLASS[confidenceTone(confidence)]
}
