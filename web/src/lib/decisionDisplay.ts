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
  if (!text || isRawIdentifier(text)) return ''
  return text
}

function isRawIdentifier(value: string): boolean {
  return /^(member|user|session|thread|channel|space|project|task|kr|mission|dec)-[a-z0-9-]{4,}$/i.test(value.trim())
}
